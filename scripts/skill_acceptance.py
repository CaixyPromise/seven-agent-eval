"""Exercise shipped SDK Skill templates through real Python/Node workers."""

import json
import shutil
import subprocess
import tempfile
import zipfile
from pathlib import Path

from bundle_fixture import read_fixture

ROOT = Path(__file__).resolve().parents[1]


def main():
    output = ROOT / "artifacts/alpha1-docs/skill"
    output.mkdir(parents=True, exist_ok=True)
    skill = ROOT / "skills/seven-eval-sdk"
    runs = {}
    with tempfile.TemporaryDirectory(prefix="seven-skill-") as temporary:
        scope = Path(temporary)
        for language, suffix in [("python", "py"), ("typescript", "ts")]:
            package = scope / language
            cli = [str(ROOT / "bin/seven-eval"), "--home", str(ROOT), "--package", str(package)]
            subprocess.run([str(ROOT / "bin/seven-eval"), "init", str(package)], check=True, capture_output=True)
            (package / "processors").mkdir(exist_ok=True)
            shutil.copy2(skill / f"assets/team_summary.{suffix}", package / f"processors/team_summary.{suffix}")
            dataset = package / "datasets/sample/cases.jsonl"
            dataset.parent.mkdir(parents=True, exist_ok=True)
            data = (ROOT / "examples/typescript/datasets/sample/cases.jsonl").read_text()
            data += json.dumps({"id": "schema-error", "inputs": {"question": "format"}, "response": "answer", "references": {"schema": {"type": "invalid-type"}}}) + "\n"
            dataset.write_text(data)
            (package / "plans/smoke.yaml").write_text(
                "schema_version: evaluate.plan/v1\nid: smoke\nmode: replay\ndataset: dataset:sample\n"
                "processors: [processor:team-summary]\nconclusions: []\n"
            )
            discovered = json.loads(subprocess.check_output(cli + ["discover", "--json"]))
            assert any(a["id"] == "team-summary" and a["status"] == "valid" for a in discovered["assets"])
            subprocess.run(cli + ["plan", "inspect", "smoke", "--json"], check=True, capture_output=True)
            result = json.loads(subprocess.check_output(cli + ["run", "smoke"]))
            with zipfile.ZipFile(result["bundle"]) as archive:
                run = read_fixture(archive)
            summary = run["outputs"][0]
            assert summary["processor"] == "team-summary@2.0.0"
            assert summary["data"] == {"scored_count": 6, "coverage_ratio": 1, "anomaly_count": 3}, summary
            cases = {c["case"]["id"]: c for c in run["cases"]}
            assert cases["zero"]["result"]["response"] == 0
            assert cases["false"]["result"]["response"] is False
            assert cases["missing"]["result"]["response"] is None
            assert any(m["status"] == "error" and m.get("value") is None for m in cases["schema-error"]["metrics"])
            subprocess.run(cli + ["verify", result["bundle"]], check=True, capture_output=True)
            shutil.copy2(result["bundle"], output / f"{language}.seven.eval")
            runs[language] = run
        assert runs["python"]["outputs"] == runs["typescript"]["outputs"]
        assert runs["python"]["cases"] == runs["typescript"]["cases"]
    record = {
        "status": "passed", "python_ts_equivalent": True,
        "cases_per_language": 6, "kpis": runs["python"]["outputs"][0]["data"],
        "false_zero_null_and_error_preserved": True,
        "temporary_data_removed": not scope.exists(),
        "scope": "actual Skill templates; Go CLI and Python/Node workers; no AI auto-discovery claim",
    }
    (output / "acceptance.json").write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(record, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
