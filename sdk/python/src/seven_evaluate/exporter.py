"""A bounded, non-blocking EEP exporter for application-side sidecars."""
from __future__ import annotations

import json
import queue
import threading
import time
import urllib.request


class AsyncEEPExporter:
    """Queue completed evidence without making a production request wait.

    The receiver is durable only after its 202 acknowledgement. Events that do
    not fit this bounded local queue are counted in ``dropped`` for source-side
    coverage reconciliation.
    """

    def __init__(self, endpoint: str, plan_id: str, max_queue: int = 1000, timeout: float = 1.0):
        self.endpoint, self.plan_id, self.timeout = endpoint, plan_id, timeout
        self._queue: queue.Queue[dict | None] = queue.Queue(max_queue)
        self.sent = self.retried = self.dropped = 0
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._run, name="seven-eep-exporter", daemon=True)
        self._thread.start()

    def emit(self, event: dict) -> bool:
        try:
            self._queue.put_nowait(event)
            return True
        except queue.Full:
            self.dropped += 1
            return False

    def close(self, timeout: float = 2.0) -> None:
        self._stop.set()
        try:
            self._queue.put_nowait(None)
        except queue.Full:
            pass
        self._thread.join(timeout)

    def _run(self) -> None:
        while not self._stop.is_set():
            event = self._queue.get()
            if event is None:
                return
            body = json.dumps({"plan_id": self.plan_id, "events": [event]}).encode()
            request = urllib.request.Request(self.endpoint, data=body, headers={"Content-Type": "application/json"})
            try:
                with urllib.request.urlopen(request, timeout=self.timeout) as response:
                    if response.status == 202:
                        self.sent += 1
                        continue
            except Exception:
                self.retried += 1
            # Retry once after a short delay without blocking the business caller.
            time.sleep(0.1)
            if not self._stop.is_set():
                # ``emit`` is also the single place that accounts for a full
                # queue. Retrying must not double-count one source-side drop.
                self.emit(event)
