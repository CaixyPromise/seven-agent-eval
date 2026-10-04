import { test } from "node:test";
import assert from "node:assert/strict";
import { decode, encode } from "../dist/codec.mjs";
import { subject, evaluator, processor, conclusion } from "../dist/index.mjs";
import { readFileSync } from "node:fs";

test("shared protocol fixture preserves numeric and missing states", () => {
  const source = readFileSync(
    new URL(
      "../../../protocol/conformance/cross-language-values.json",
      import.meta.url,
    ),
    "utf8",
  ).trim();
  assert.equal(encode(decode(source)), source);
});

test("lossless JSON distinguishes null, false, zero and adjacent large integers", () => {
  const text =
    "[null,false,0,9007199254740992,9007199254740993,-9007199254740993]";
  const value = decode(text);
  assert.deepEqual(value, [
    null,
    false,
    0,
    9007199254740992n,
    9007199254740993n,
    -9007199254740993n,
  ]);
  assert.equal(encode(value), text);
  assert.equal(decode("9007199254740993.0"), 9007199254740993n);
  assert.equal(decode("9.007199254740993e15"), 9007199254740993n);
  assert.throws(() => decode("9007199254740991.1"));
});
test("non-JSON and lossy results fail explicitly", () => {
  for (const value of [
    undefined,
    NaN,
    Infinity,
    9007199254740992,
    { score: undefined },
    new Date(),
  ])
    assert.throws(() => encode(value));
  const cycle = {};
  cycle.self = cycle;
  assert.throws(() => encode(cycle));
});
test("all extension kinds register plain sync and async functions", async () => {
  for (const [kind, register] of Object.entries({
    subject,
    evaluator,
    processor,
    conclusion,
  })) {
    const fn = register({ id: "example", version: "1" }, async (ctx) => ctx);
    assert.deepEqual(fn.__seven_descriptor__, {
      id: "example",
      version: "1",
      kind,
    });
    assert.deepEqual(await fn({ zero: 0 }), { zero: 0 });
  }
});
test("session hooks use serializable context rather than process memory", async () => {
  const fn = subject({ id: "session" }, (ctx) => ({ output: ctx.session.id }), {
    create: () => ({ id: "isolated" }),
    cleanup: (ctx) => ctx.session,
  });
  const session = decode(encode(await fn.__seven_session_create__({})));
  assert.deepEqual(await fn({ session }), { output: "isolated" });
  assert.deepEqual(await fn.__seven_session_cleanup__({ session }), session);
});
