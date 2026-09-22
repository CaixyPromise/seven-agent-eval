"""Prepare and seed deterministic packages for the phase-2 browser gate."""

from __future__ import annotations

import argparse
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import shutil
import urllib.error
import urllib.request


ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "testdata" / "browser-acceptance"
DEFAULT_OUTPUT = ROOT / "artifacts" / "phase2-browser" / "package"


def prepare(variant: str, output: Path) -> None:
    source = FIXTURES / variant
    if not source.is_dir():
        raise SystemExit(f"unknown fixture variant: {variant}")
    if output.exists():
        shutil.rmtree(output)
    shutil.copytree(source, output)
    print(json.dumps({"package": str(output), "variant": variant}, ensure_ascii=False))


def event(item: dict, index: int, event_time: str, revision: int) -> dict:
    identity = item["key"]
    if identity == "long-identity":
        identity = "case-and-trace-with-a-deliberately-long-identifier-" + "0123456789abcdef" * 5
    response = item["response"]
    if revision > 1:
        response = response or "迟到证据补全了原先缺失的回答。"
    return {
        "schema_version": "evaluate.event/v1",
        "source": "browser-acceptance",
        "project": "phase2-closeout",
        "event_id": f"browser:{item['key']}:r{revision}",
        "event_time": event_time,
        "trace_id": identity,
        "evidence_revision": revision,
        "case": {
            "id": identity,
            "inputs": {"question": "退款多久可以到账？", "scenario": item["key"]},
            "references": {"exact": item["reference"]},
        },
        "result": {
            "response": response,
            "contexts": item["contexts"],
            "latency_ms": 120 + index,
            "usage": {"total_tokens": 80 + index},
        },
    }


def post(base: str, plan_id: str, events: list[dict]) -> dict:
    body = json.dumps({"plan_id": plan_id, "events": events}, ensure_ascii=False).encode()
    request = urllib.request.Request(
        base.rstrip("/") + "/api/ingest/events",
        data=body,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        detail = error.read().decode(errors="replace")
        raise RuntimeError(f"fixture ingest failed for {plan_id}: HTTP {error.code}: {detail}") from error


def seed(base: str, revision_plan: str | None, revision: int) -> None:
    templates = json.loads((FIXTURES / "events.json").read_text())
    stamp = (datetime.now(timezone.utc) - timedelta(minutes=3)).replace(second=10, microsecond=0)
    event_time = stamp.isoformat().replace("+00:00", "Z")
    results = {}
    for plan_id, items in templates.items():
        if revision_plan and plan_id != revision_plan:
            continue
        selected = items[:1] if revision > 1 else items
        events = [event(item, index, event_time, revision) for index, item in enumerate(selected)]
        results[plan_id] = post(base, plan_id, events)
    print(json.dumps({"event_time": event_time, "results": results}, indent=2, ensure_ascii=False))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    prepare_parser = subparsers.add_parser("prepare", help="copy a clean browser Evaluation Package")
    prepare_parser.add_argument("--variant", choices=("no-plans", "scenarios"), default="scenarios")
    prepare_parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    seed_parser = subparsers.add_parser("seed", help="inject current-time events into a running fixture")
    seed_parser.add_argument("--base", required=True)
    seed_parser.add_argument("--revision-plan")
    seed_parser.add_argument("--revision", type=int, default=1)
    args = parser.parse_args()
    if args.command == "prepare":
        prepare(args.variant, args.output.resolve())
    else:
        if args.revision < 1:
            parser.error("--revision must be positive")
        seed(args.base, args.revision_plan, args.revision)


if __name__ == "__main__":
    main()
