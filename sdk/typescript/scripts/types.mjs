// Exchange shapes come from the shared schema; conditional invariants are checked by Go.
import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
const schema = JSON.parse(
  readFileSync(
    new URL("../../../protocol/schemas/exchange.json", import.meta.url),
    "utf8",
  ),
);
const name = (s) =>
  s
    .split("_")
    .map((p) => p[0].toUpperCase() + p.slice(1))
    .join("");
function type(s) {
  if (s.$ref) return name(s.$ref.split("/").at(-1));
  if (s.const !== undefined) return JSON.stringify(s.const);
  if (s.enum) return s.enum.map(JSON.stringify).join(" | ");
  if (s.oneOf || s.anyOf) return (s.oneOf || s.anyOf).map(type).join(" | ");
  if (Array.isArray(s.type))
    return s.type.map((t) => type({ ...s, type: t })).join(" | ");
  if (s.type === "array") return `Array<${type(s.items || {})}>`;
  if (s.type === "object") {
    const fields = Object.entries(s.properties || {}).map(
      ([k, v]) =>
        `${JSON.stringify(k)}${(s.required || []).includes(k) ? "" : "?"}: ${type(v)};`,
    );
    if (s.additionalProperties !== false)
      fields.push(
        `[key: string]: ${typeof s.additionalProperties === "object" ? type(s.additionalProperties) : "unknown"};`,
      );
    return `{ ${fields.join(" ")} }`;
  }
  return (
    {
      integer: "number | bigint",
      number: "number",
      boolean: "boolean",
      string: "string",
      null: "null",
    }[s.type] || "unknown"
  );
}
mkdirSync(new URL("../dist/", import.meta.url), { recursive: true });
writeFileSync(
  new URL("../dist/exchange.d.ts", import.meta.url),
  "// Generated from protocol/schemas/exchange.json. Do not edit.\n" +
    Object.entries(schema.$defs)
      .map(([k, v]) => `export type ${name(k)} = ${type(v)};`)
      .join("\n"),
);
