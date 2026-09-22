"""A bounded, non-blocking EEP exporter for application-side sidecars."""
from __future__ import annotations

import json
import copy
import queue
import threading
import time
import urllib.request
import urllib.error


class AsyncEEPExporter:
    """Queue completed evidence without making a production request wait.

    The receiver is durable only after its 202 acknowledgement. Events that do
    not fit this bounded local queue are counted in ``dropped`` for source-side
    coverage reconciliation.
    """

    def __init__(self, endpoint: str, plan_id: str, max_queue: int = 1000, timeout: float = 1.0):
        if max_queue < 1 or timeout <= 0:
            raise ValueError("positive queue size and timeout required")
        self.endpoint, self.plan_id, self.timeout = endpoint, plan_id, timeout
        self._queue: queue.Queue[dict | None] = queue.Queue(max_queue)
        self.sent = self.retried = self.dropped = 0
        self.rejected = 0
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._run, name="seven-eep-exporter", daemon=True)
        self._thread.start()

    def emit(self, event: dict) -> bool:
        if self._stop.is_set():
            self.dropped += 1
            return False
        try:
            self._queue.put_nowait(copy.deepcopy(event))
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
            try:
                body = json.dumps({"plan_id": self.plan_id, "events": [event]}, allow_nan=False).encode()
            except (ValueError, TypeError):
                self.rejected += 1
                continue
            delay = .1
            # Retain the same event in-flight until acknowledged, preserving
            # FIFO and preventing queue contention from dropping retry traffic.
            while not self._stop.is_set():
                request = urllib.request.Request(self.endpoint, data=body, headers={"Content-Type": "application/json"})
                try:
                    with urllib.request.urlopen(request, timeout=self.timeout) as response:
                        ack = json.load(response)
                        item = ack.get("items", [{}])[0]
                        if response.status == 202 and item.get("status") in ("accepted", "duplicate"):
                            self.sent += 1
                            break
                        if item.get("code") != "QUEUE_FULL":
                            self.rejected += 1
                            break
                except urllib.error.HTTPError as error:
                    if 400 <= error.code < 500 and error.code not in (408, 429):
                        self.rejected += 1
                        break
                except (OSError, ValueError, IndexError):
                    pass
                self.retried += 1
                self._stop.wait(delay)
                delay = min(2.0, delay*2)
            else:
                self.dropped += 1
        while True:
            try:
                if self._queue.get_nowait() is not None:
                    self.dropped += 1
            except queue.Empty:
                break
