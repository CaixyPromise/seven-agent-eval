import { build } from "esbuild";
await build({
  entryPoints: ["src/worker.ts"],
  bundle: true,
  platform: "node",
  format: "esm",
  target: "node20",
  outfile: "dist/worker.mjs",
});
await build({
  entryPoints: ["src/codec.ts", "src/index.ts"],
  bundle: true,
  platform: "node",
  format: "esm",
  target: "node20",
  outdir: "dist",
  outExtension: { ".js": ".mjs" },
});
