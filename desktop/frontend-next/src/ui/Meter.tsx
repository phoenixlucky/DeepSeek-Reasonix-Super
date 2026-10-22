import { clampedShare } from "./meterShare";

// One horizontal bar for a quantity Reasonix owns that has a natural maximum.
// The fill is the share of max, shown in the full-state colour once the value
// reaches it; the accessible name and numbers say what the bar measures.
export function Meter({ value, max, label, full = false }: { value: number; max: number; label: string; full?: boolean }) {
  return (
    <span className="mtr" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={max} aria-valuenow={Math.min(value, max)} data-full={full || undefined}>
      <i style={{ width: `${clampedShare(value, max)}%` }} />
    </span>
  );
}
