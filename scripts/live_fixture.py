"""Inject deterministic completed production events into a local seven-eval server."""
from __future__ import annotations

import argparse
import json
import time
import urllib.request


def event(index: int, revision: int = 1) -> dict:
    answer = "退款需 3 个工作日" if index % 3 else "退款需 5 个工作日"
    return {
        "schema_version": "evaluate.event/v1",
        "source": "controlled-fixture",
        "project": "support-rag",
        "event_id": f"support-{index}-r{revision}",
        "event_time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "trace_id": f"trace-{index}",
        "evidence_revision": revision,
        "case": {"id": f"trace-{index}", "inputs": {"question": "退款多久？"}, "references": {"exact": "退款需 3 个工作日"}},
        "result": {"response": answer, "contexts": ["退款会在 3 个工作日内处理。"], "latency_ms": 120 + index},
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--endpoint", default="http://127.0.0.1:7431/api/ingest/events")
    parser.add_argument("--plan", default="production-support")
    parser.add_argument("--count", type=int, default=12)
    parser.add_argument("--start", type=int, default=0, help="first deterministic event index")
    parser.add_argument("--revision", type=int, default=1, help="evidence revision for generated events")
    args = parser.parse_args()
    payload = json.dumps(
        {
            "plan_id": args.plan,
            "events": [event(i, args.revision) for i in range(args.start, args.start + args.count)],
        }
    ).encode()
    request = urllib.request.Request(args.endpoint, data=payload, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=5) as response:
        print(response.read().decode())


if __name__ == "__main__":
    main()
