/**
 * Lifetimes a paste can be given, and how to describe one.
 *
 * The ladder is not defined here — it comes from /api/config, because the API
 * accepts exactly these values and nothing between them. Deriving the picker
 * from the server's own list is what keeps the two from disagreeing about what
 * is on offer, and adding a rung is then a one-line server change.
 */

/** Asking for nothing. Matches the server, where 0 means no lifetime. */
export const NO_EXPIRY = 0

export interface ExpiryOption {
  /** Seconds, as sent to the API. */
  seconds: number
  /** Terse form for the trigger, where width is scarce. */
  short: string
  /** Spelled out, for the menu. */
  label: string
}

const NONE: ExpiryOption = { seconds: NO_EXPIRY, short: "∞", label: "No expiry" }

/**
 * The ladder as the picker shows it: no-expiry first, then the server's list.
 *
 * Labels are derived from the durations rather than sent alongside them. The
 * server has no business holding UI copy, and "6 hours" is not information the
 * number 21600 was missing.
 */
export function expiryOptions(secs: readonly number[]): ExpiryOption[] {
  return [NONE, ...secs.filter((s) => s > 0).map(describeSeconds)]
}

/** The option for a chosen value, falling back to no-expiry for anything the
 * server does not offer — the same answer the API would give it. */
export function expiryOption(seconds: number, secs: readonly number[]): ExpiryOption {
  if (seconds === NO_EXPIRY) return NONE
  return secs.includes(seconds) ? describeSeconds(seconds) : NONE
}

/** 21600 -> `{ short: "6h", label: "6 hours" }`. */
function describeSeconds(seconds: number): ExpiryOption {
  if (seconds % 86_400 === 0) return unit(seconds, seconds / 86_400, "day", "d")
  if (seconds % 3_600 === 0) return unit(seconds, seconds / 3_600, "hour", "h")
  return unit(seconds, Math.round(seconds / 60), "minute", "m")
}

function unit(seconds: number, count: number, name: string, letter: string): ExpiryOption {
  return {
    seconds,
    short: `${count}${letter}`,
    label: `${count} ${name}${count === 1 ? "" : "s"}`,
  }
}

/**
 * How long is left, as a count and the unit it is counted in.
 *
 * Rounded to nearest, and the unit is chosen after rounding. Flooring reads as
 * a bug at the top of every span — choose "6 hours" and the very next screen
 * says "5h left" — and the exact instant is one hover away for anyone who
 * needs to plan around it.
 *
 * Null once the paste is gone, because by then the page is showing something
 * that no longer exists and a duration would only obscure that.
 */
export function remainingParts(
  iso: string,
  now: number = Date.now(),
): { count: number; unit: "minute" | "hour" | "day" } | null {
  const left = new Date(iso).getTime() - now
  if (!Number.isFinite(left) || left <= 0) return null

  // Each step promotes the unit when rounding has pushed the count to the top
  // of its range, so 59m40s reads as "1 hour" rather than "60 minutes".
  const minutes = Math.round(left / 60_000)
  if (minutes < 60) return { count: Math.max(minutes, 1), unit: "minute" }

  const hours = Math.round(left / 3_600_000)
  if (hours < 24) return { count: hours, unit: "hour" }

  return { count: Math.round(left / 86_400_000), unit: "day" }
}

/** "6 hours", "1 day". */
export function formatRemaining(iso: string, now?: number): string | null {
  const left = remainingParts(iso, now)
  if (!left) return null
  return `${left.count} ${left.unit}${left.count === 1 ? "" : "s"}`
}

/** "6h", "1d" — the same thing where a status bar has no room for words. */
export function formatRemainingShort(iso: string, now?: number): string | null {
  const left = remainingParts(iso, now)
  if (!left) return null
  return `${left.count}${left.unit[0]}`
}
