/** How numbers and dates read across the workspace. Formatting only: nothing is computed. */

/**
 * A pass's lap as the driver counts it. Core stores the laps completed when the pass ended
 * (`lap_in_session`), so a pass in the first lap has 0 and reads "L1".
 */
export function lapLabel(lap: number | undefined): string {
  return lap === undefined ? "–" : `L${String(lap + 1)}`;
}

export function seconds(value: number): string {
  return value.toFixed(3);
}

/** A difference with its sign, in seconds: "+0.124", "−0.031". */
export function signedSeconds(value: number): string {
  return `${value < 0 ? "−" : "+"}${Math.abs(value).toFixed(3)}`;
}

export function dayOf(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    weekday: "short",
    day: "numeric",
    month: "short",
  });
}

export function timeOf(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export function plural(count: number, one: string, many = `${one}s`): string {
  return `${String(count)} ${count === 1 ? one : many}`;
}
