import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { execFileSync, spawnSync } from "node:child_process";
const build = mkdtempSync(join(tmpdir(), "seven-web-tests-"));
try {
  execFileSync(
    process.execPath,
    [
      "node_modules/typescript/bin/tsc",
      "src/domain/presentation.ts",
      "src/api/client.ts",
      "--target",
      "ES2022",
      "--module",
      "commonjs",
      "--moduleResolution",
      "node",
      "--strict",
      "--skipLibCheck",
      "--outDir",
      build,
    ],
    { stdio: "inherit" },
  );
  const result = spawnSync(
    process.execPath,
    ["--test", "tests/domain.test.cjs"],
    {
      stdio: "inherit",
      env: { ...process.env, SEVEN_WEB_TEST_BUILD: build },
    },
  );
  process.exitCode = result.status ?? 1;
} finally {
  rmSync(build, { recursive: true, force: true });
}
