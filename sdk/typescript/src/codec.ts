import { parse, stringify } from "lossless-json";
export function decode(text: string): any {
  return parse(text, undefined, (raw: string) => {
    const match = /^(-?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(raw)!;
    const exponent = Number(match[4] ?? 0) - (match[3]?.length ?? 0);
    if (Math.abs(exponent) > 10_000)
      throw new Error("numeric exponent exceeds limits");
    const digits = (match[2] + (match[3] ?? "")).replace(/^0+(?=\d)/, "");
    const integral =
      exponent >= 0 ||
      /^0*$/.test(digits.slice(Math.max(0, digits.length + exponent)));
    if (integral) {
      const integer =
        exponent >= 0
          ? digits + "0".repeat(exponent)
          : digits.slice(0, Math.max(0, digits.length + exponent)) || "0";
      const value = BigInt(match[1] + integer);
      return value > BigInt(Number.MAX_SAFE_INTEGER) ||
        value < BigInt(Number.MIN_SAFE_INTEGER)
        ? value
        : Number(raw);
    }
    const value = Number(raw);
    if (!Number.isFinite(value) || Number.isInteger(value))
      throw new Error("unsafe numeric value");
    return value;
  });
}
export function encode(value: unknown): string {
  const seen = new Set<object>();
  function check(v: unknown) {
    if (
      v === null ||
      typeof v === "string" ||
      typeof v === "boolean" ||
      typeof v === "bigint"
    )
      return;
    if (typeof v === "number") {
      if (
        !Number.isFinite(v) ||
        (Number.isInteger(v) && !Number.isSafeInteger(v))
      )
        throw new Error("unsafe numeric value");
      return;
    }
    if (typeof v !== "object" || seen.has(v)) throw new Error("non JSON value");
    if (
      !Array.isArray(v) &&
      Object.getPrototypeOf(v) !== Object.prototype &&
      Object.getPrototypeOf(v) !== null
    )
      throw new Error("non JSON object");
    seen.add(v);
    for (const item of Array.isArray(v) ? v : Object.values(v)) check(item);
    seen.delete(v);
  }
  check(value);
  return stringify(value)!;
}
