"""Check tracked public payload and local documentation links; never print matched secrets.

Complement with gitleaks git --redact --log-opts=--all. This is not a secret scanner.
"""

import json
import re
import subprocess
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]


def main():
    paths = (
        subprocess.check_output(["git", "ls-files", "-z"], cwd=ROOT)
        .decode()
        .split("\0")
    )
    failures = []
    personal_paths = []
    binaries = []
    for name in filter(None, paths):
        path = ROOT / name
        if not path.exists():
            continue
        if path.is_symlink():
            failures.append({"path": name, "reason": "tracked symlink"})
            continue
        parts = path.relative_to(ROOT).parts
        if (
            any(
                p in {".venv", "node_modules", ".seven-evaluate", "artifacts"}
                for p in parts
            )
            or (path.name.startswith(".env") and path.name != ".env.example")
            or path.suffix in {".pem", ".key", ".eval"}
        ):
            failures.append({"path": name, "reason": "private/generated payload"})
        data = path.read_bytes()
        try:
            content = data.decode("utf-8")
        except UnicodeDecodeError:
            binaries.append(name)
            continue
        if re.search(r"/(?:Users|home)/[A-Za-z0-9_.-]+/", content):
            personal_paths.append(name)
        if path.suffix == ".md":
            # Inline links only; code samples are not documentation links.
            content = re.sub(r"```.*?```", "", content, flags=re.DOTALL)
            for link in re.findall(r"\]\(([^\s)]+)(?:\s+[^)]*)?\)", content):
                if re.match(r"[a-zA-Z]+:", link) or link.startswith("#"):
                    continue
                target = unquote(link.split("#")[0])
                if target and not (path.parent / target).exists():
                    failures.append(
                        {"path": name, "reason": "broken local link", "target": target}
                    )
    result = {
        "tracked_files": len([p for p in paths if p]),
        "failures": failures,
        "personal_path_files": personal_paths,
        "binary_review_required": binaries,
    }
    print(json.dumps(result, indent=2))
    return bool(failures or personal_paths)


if __name__ == "__main__":
    raise SystemExit(main())
