"""Collect verbatim dependency notices from resolved build inputs (no license inference)."""

import argparse
import hashlib
import json
import re
import subprocess
import zipfile
from email.parser import BytesParser
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def documents(directory):
    # Include nested license directories, without walking nested dependency stores.
    for path in sorted(directory.rglob("*")):
        relative = path.relative_to(directory)
        if "node_modules" in relative.parts or path.is_symlink() or not path.is_file():
            continue
        name = path.name.lower()
        if name.startswith(("license", "licence", "copying", "notice", "copyright")):
            yield relative.as_posix(), path.read_bytes()
        elif name.startswith("readme"):
            data = path.read_bytes()
            if re.search(rb"(?im)^#+\s+licen[cs]e\s*$", data):
                yield relative.as_posix(), data


def collect(output, wheels):
    output.mkdir(parents=True, exist_ok=False)
    entries = []
    seen = set()

    def add(ecosystem, name, version, declared, materials):
        identity = (ecosystem, name, version)
        if identity in seen:
            return
        seen.add(identity)
        slug = re.sub(r"[^a-zA-Z0-9._-]", "_", name + "-" + version)
        folder = output / ecosystem / slug
        folder.mkdir(parents=True)
        files = []
        for index, (source_name, data) in enumerate(materials):
            destination = folder / (str(index) + "-" + Path(source_name).name)
            destination.write_bytes(data)
            files.append(
                {
                    "source": source_name,
                    "path": destination.relative_to(output).as_posix(),
                    "sha256": hashlib.sha256(data).hexdigest(),
                }
            )
        entries.append(
            {
                "ecosystem": ecosystem,
                "name": name,
                "version": version,
                "declared_license": declared,
                "notices": files,
            }
        )

    goroot = Path(
        subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, text=True).strip()
    )
    go_version = subprocess.check_output(
        ["go", "env", "GOVERSION"], cwd=ROOT, text=True
    ).strip()
    add(
        "toolchain",
        "go-runtime",
        go_version,
        "BSD-3-Clause",
        [
            (name, (goroot / name).read_bytes())
            for name in ["LICENSE", "PATENTS"]
            if (goroot / name).is_file()
        ],
    )
    raw = subprocess.check_output(
        ["go", "list", "-m", "-json", "all"], cwd=ROOT, text=True
    )
    decoder = json.JSONDecoder()
    while raw.strip():
        module, end = decoder.raw_decode(raw.lstrip())
        raw = raw.lstrip()[end:]
        if not module.get("Main"):
            if not module.get("Dir"):
                fetched = json.loads(
                    subprocess.check_output(
                        [
                            "go",
                            "mod",
                            "download",
                            "-json",
                            module["Path"] + "@" + module["Version"],
                        ],
                        cwd=ROOT,
                        text=True,
                    )
                )
                module["Dir"] = fetched["Dir"]
            add(
                "go",
                module["Path"],
                module["Version"],
                None,
                documents(Path(module["Dir"])),
            )

    for project in ["web", "sdk/typescript"]:
        store = ROOT / project / "node_modules/.pnpm"
        if not store.exists():
            raise RuntimeError("run make setup before collecting npm notices")
        for metadata in sorted(store.glob("*/node_modules/*/package.json")) + sorted(
            store.glob("*/node_modules/@*/*/package.json")
        ):
            if metadata.parent.is_symlink():
                continue
            package = json.loads(metadata.read_text())
            materials = list(documents(metadata.parent))
            # These platform packages explicitly share their upstream repository/version.
            # npm omits the text in the native package; use the installed matching parent,
            # recording its provenance instead of synthesizing a license from the label.
            parent_name = None
            if package["name"].startswith("@esbuild/"):
                parent_name = "esbuild"
            elif package["name"].startswith("@rollup/rollup-"):
                parent_name = "rollup"
            if not materials and parent_name:
                parent = (
                    store
                    / (parent_name + "@" + package["version"])
                    / "node_modules"
                    / parent_name
                )
                parent_metadata = json.loads((parent / "package.json").read_text())
                if parent_metadata["version"] != package["version"]:
                    raise RuntimeError("native dependency version mismatch")
                materials = [
                    (parent_name + "@" + package["version"] + "/" + p, data)
                    for p, data in documents(parent)
                ]
            add(
                "npm",
                package["name"],
                package["version"],
                package.get("license"),
                materials,
            )

    wheel_files = sorted(wheels.glob("*.whl"))
    if not wheel_files:
        raise RuntimeError("wheelhouse is empty")
    for wheel in wheel_files:
        with zipfile.ZipFile(wheel) as archive:
            metadata_path = next(
                p for p in archive.namelist() if p.endswith(".dist-info/METADATA")
            )
            metadata = BytesParser().parsebytes(archive.read(metadata_path))
            materials = [
                (p, archive.read(p))
                for p in archive.namelist()
                if not p.endswith("/")
                and (
                    Path(p)
                    .name.lower()
                    .startswith(
                        ("license", "licence", "copying", "notice", "copyright")
                    )
                    or ".dist-info/licenses/" in p
                )
            ]
            add(
                "python",
                metadata["Name"],
                metadata["Version"],
                metadata.get("License-Expression") or metadata.get("License"),
                materials,
            )
    entries.sort(key=lambda item: (item["ecosystem"], item["name"], item["version"]))
    missing = [
        f"{e['ecosystem']}:{e['name']}@{e['version']}"
        for e in entries
        if not e["notices"]
    ]
    (output / "inventory.json").write_text(
        json.dumps({"dependencies": entries, "missing_notices": missing}, indent=2)
        + "\n"
    )
    if missing:
        raise RuntimeError("dependency notice material missing: " + ", ".join(missing))
    print(json.dumps({"dependency_count": len(entries), "missing_notices": missing}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--wheels", type=Path, required=True)
    args = parser.parse_args()
    collect(args.output, args.wheels)
