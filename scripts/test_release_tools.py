"""Regression tests for preserving dependency attribution in release archives."""

import json
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch

import dependency_notices


class NoticesTest(unittest.TestCase):
    def test_verbatim_go_npm_python_and_native_parent(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            go = root / "gomodule"
            go.mkdir()
            original = b"Copyright Fixture\r\nExact license bytes\r\n"
            (go / "LICENSE").write_bytes(original)
            for project in ["web", "sdk/typescript"]:
                (root / project / "node_modules/.pnpm").mkdir(parents=True)
            store = root / "web/node_modules/.pnpm"
            for name, relative in [
                ("esbuild", "esbuild@0.24.2/node_modules/esbuild"),
                ("@esbuild/darwin-arm64", "native/node_modules/@esbuild/darwin-arm64"),
            ]:
                folder = store / relative
                folder.mkdir(parents=True)
                (folder / "package.json").write_text(
                    json.dumps({"name": name, "version": "0.24.2", "license": "MIT"})
                )
                if name == "esbuild":
                    (folder / "LICENSE.md").write_bytes(original)
            wheels = root / "wheels"
            wheels.mkdir()
            with zipfile.ZipFile(wheels / "fixture.whl", "w") as wheel:
                wheel.writestr(
                    "fixture-1.dist-info/METADATA",
                    "Name: fixture\nVersion: 1\nLicense-Expression: MIT\n",
                )
                wheel.writestr("fixture-1.dist-info/licenses/LICENSE", original)
            module = json.dumps(
                {"Path": "example.invalid/module", "Version": "v1", "Dir": str(go)}
            )
            output = root / "notices"
            with (
                patch.object(dependency_notices, "ROOT", root),
                patch.object(
                    dependency_notices.subprocess,
                    "check_output",
                    side_effect=lambda command, **kwargs: (
                        str(go)
                        if command[-1] == "GOROOT"
                        else "go1.25.1"
                        if command[-1] == "GOVERSION"
                        else module
                    ),
                ),
            ):
                dependency_notices.collect(output, wheels)
            inventory = json.loads((output / "inventory.json").read_text())
            self.assertEqual(len(inventory["dependencies"]), 5)
            self.assertEqual(inventory["missing_notices"], [])
            for entry in inventory["dependencies"]:
                for material in entry["notices"]:
                    self.assertEqual((output / material["path"]).read_bytes(), original)

    def test_license_in_readme_is_preserved(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            content = b"# Example\n\n## License\n\nCopyright holder\nLicense text\n"
            (root / "README.md").write_bytes(content)
            (root / "link").symlink_to(root, target_is_directory=True)
            self.assertEqual(
                list(dependency_notices.documents(root)), [("README.md", content)]
            )

    def test_missing_notice_fails_instead_of_inventing_text(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "LICENSE").write_text("Go runtime notice fixture")
            for project in ["web", "sdk/typescript"]:
                (root / project / "node_modules/.pnpm").mkdir(parents=True)
            wheels = root / "wheels"
            wheels.mkdir()
            with zipfile.ZipFile(wheels / "fixture.whl", "w") as wheel:
                wheel.writestr(
                    "fixture-1.dist-info/METADATA",
                    "Name: fixture\nVersion: 1\nLicense-Expression: MIT\n",
                )
            with (
                patch.object(dependency_notices, "ROOT", root),
                patch.object(
                    dependency_notices.subprocess,
                    "check_output",
                    side_effect=lambda command, **kwargs: (
                        str(root)
                        if command[-1] == "GOROOT"
                        else "go1.25.1"
                        if command[-1] == "GOVERSION"
                        else ""
                    ),
                ),
                self.assertRaisesRegex(RuntimeError, "missing: python:fixture@1"),
            ):
                dependency_notices.collect(root / "notices", wheels)


if __name__ == "__main__":
    unittest.main()
