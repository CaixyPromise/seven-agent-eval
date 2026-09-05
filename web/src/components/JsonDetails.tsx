import { pretty } from "../domain/presentation";
export function JsonDetails({
  value,
  label,
}: {
  value: unknown;
  label?: string;
}) {
  return (
    <section className="json-section">
      {label ? <h3>{label}</h3> : null}
      <pre tabIndex={0}>{pretty(value)}</pre>
    </section>
  );
}
