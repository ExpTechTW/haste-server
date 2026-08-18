/**
 * Lifetimes a paste can be given, and how to describe one.
 *
 * The ladder is fixed rather than a free duration input: every rung is a span
 * someone would actually pick, and a picker with nine entries needs no
 * validation, no parsing and no way to ask for something the server refuses.
 */

const HOUR = 3600
const DAY = 24 * HOUR

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

export const EXPIRY_OPTIONS: ExpiryOption[] = [
  { seconds: NO_EXPIRY, short: "∞", label: "No expiry" },
  { seconds: HOUR, short: "1h", label: "1 hour" },
  { seconds: 6 * HOUR, short: "6h", label: "6 hours" },
  { seconds: 12 * HOUR, short: "12h", label: "12 hours" },
  { seconds: DAY, short: "1d", label: "1 day" },
  { seconds: 3 * DAY, short: "3d", label: "3 days" },
  { seconds: 7 * DAY, short: "7d", label: "7 days" },
  { seconds: 14 * DAY, short: "14d", label: "14 days" },
  { seconds: 30 * DAY, short: "30d", label: "30 days" },
]

/**
 * The rungs this server will accept.
 *
 * The bounds come from /api/config rather than being assumed, so an instance
 * that narrows them cannot end up offering a choice its own API rejects.
 */
export function availableExpiries(min: number, max: number): ExpiryOption[] {
  return EXPIRY_OPTIONS.filter(
    (o) => o.seconds === NO_EXPIRY || (o.seconds >= min && o.seconds <= max),
  )
}

export function expiryOption(seconds: number): ExpiryOption {
  return EXPIRY_OPTIONS.find((o) => o.seconds === seconds) ?? EXPIRY_OPTIONS[0]
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
