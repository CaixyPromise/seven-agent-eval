import { createInterface } from "node:readline";
import { pathToFileURL } from "node:url";
import { Console } from "node:console";
import { decode, encode } from "./codec";
globalThis.console = new Console(process.stderr, process.stderr);
const methods = [
  "describe",
  "subject/create",
  "subject/cleanup",
  "subject/invoke",
  "evaluator/evaluate",
  "processor/process",
  "conclusion/conclude",
];
let bytes = 0;
// An unresolved Promise is still an in-flight operation, even without I/O.
// Keep it alive until the Go deadline/cancellation terminates this worker.
const operationLifetime = setInterval(() => {}, 60_000);
process.stdin.on("data", (chunk) => {
  bytes += chunk.length;
  if (bytes > 64 * 1024 * 1024) process.exit(1);
});
async function dispatch(method: string, params: any) {
  if (method === "initialize")
    return { protocol_version: "1.0", capabilities: methods };
  if (!methods.includes(method)) throw new Error("unsupported operation");
  const module = await import(pathToFileURL(params.path).href);
  const entries = [...new Set(Object.values(module))].filter(
    (v: any) => typeof v === "function" && v.__seven_descriptor__,
  ) as any[];
  if (entries.length !== 1) throw new Error("one registered export required");
  const fn = entries[0];
  if (method === "describe") return fn.__seven_descriptor__;
  if (fn.__seven_descriptor__.kind !== method.split("/")[0])
    throw new Error("kind mismatch");
  const handler =
    method === "subject/create"
      ? fn.__seven_session_create__
      : method === "subject/cleanup"
        ? fn.__seven_session_cleanup__
        : fn;
  if (typeof handler !== "function") throw new Error("missing handler");
  return await handler(params.context ?? {});
}
for await (const line of createInterface({
  input: process.stdin,
  crlfDelay: Infinity,
})) {
  let request: any = null;
  let response: string;
  try {
    request = decode(line);
    if (request.jsonrpc !== "2.0" || !Number.isSafeInteger(request.id))
      throw new Error("invalid request");
    response = encode({
      jsonrpc: "2.0",
      id: request.id,
      result: await dispatch(request.method, request.params ?? {}),
    });
    if (Buffer.byteLength(response) > 8 * 1024 * 1024)
      throw new Error("output limit");
  } catch {
    response = encode({
      jsonrpc: "2.0",
      id: Number.isSafeInteger(request?.id) ? request.id : null,
      error: { code: -32000, message: "worker operation failed" },
    });
  }
  process.stdout.write(response + "\n");
}
// User timers must not keep a completed one-operation worker alive.
clearInterval(operationLifetime);
process.stdout.write("", () => process.exit(0));
