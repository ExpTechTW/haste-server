import { formatCount, formatCountShort, type TimeUnit, type Translate } from "@/lib/i18n"

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

const none = (t: Translate): ExpiryOption => ({
  seconds: NO_EXPIRY,
  short: "∞",
  label: t("expiry.none"),
})

/**
 * The ladder as the picker shows it: no-expiry first, then the server's list.
 *
 * Labels are derived from the durations rather than sent alongside them. The
 * server has no business holding UI copy in three languages, and "6 hours" is
 * not information the number 21600 was missing.
 */
export function expiryOptions(t: Translate, secs: readonly number[]): ExpiryOption[] {
  return [none(t), ...secs.filter((s) => s > 0).map((s) => describeSeconds(t, s))]
}

/**
 * The option for a chosen value, falling back to no-expiry for anything the
 * server does not offer — the same answer the API would give it.
 */
export function expiryOption(
  t: Translate,
  seconds: number,
  secs: readonly number[],
): ExpiryOption {
  if (seconds === NO_EXPIRY) return none(t)
  return secs.includes(seconds) ? describeSeconds(t, seconds) : none(t)
}

/** 21600 -> `{ short: "6h", label: "6 hours" }`. */
function describeSeconds(t: Translate, seconds: number): ExpiryOption {
  if (seconds % 86_400 === 0) return unit(t, seconds, seconds / 86_400, "day")
  if (seconds % 3_600 === 0) return unit(t, seconds, seconds / 3_600, "hour")
  return unit(t, seconds, Math.round(seconds / 60), "minute")
}

function unit(
  t: Translate,
  seconds: number,
  count: number,
  name: TimeUnit,
): ExpiryOption {
  return {
    seconds,
    short: formatCountShort(t, name, count),
    label: formatCount(t, name, count),
  }
}

/** The parts of a countdown, floored — never more time than there really is. */
export function countdownParts(
  iso: string,
  now: number = Date.now(),
): { days: number; hours: number; minutes: number; seconds: number } | null {
  const left = new Date(iso).getTime() - now
  if (!Number.isFinite(left) || left <= 0) return null

  const total = Math.floor(left / 1000)
  return {
    days: Math.floor(total / 86_400),
    hours: Math.floor((total % 86_400) / 3_600),
    minutes: Math.floor((total % 3_600) / 60),
    seconds: total % 60,
  }
}

/**
 * A running countdown, at whatever resolution is still meaningful.
 *
 * Two units, never three: "29d 23h" for a month-long paste, "5h 59m" for a
 * day, and mm:ss once there is under an hour left, where seconds are the whole
 * point. Floored rather than rounded, so it never claims more time than
 * remains — and unlike a coarse label it visibly moves, so "5h 59m" a moment
 * after choosing six hours reads as counting down rather than as being wrong.
 *
 * Null once the deadline has passed; the caller says so in words instead.
 */
export function countdown(t: Translate, iso: string, now?: number): string | null {
  const left = countdownParts(iso, now)
  if (!left) return null

  if (left.days > 0) {
    return `${formatCountShort(t, "day", left.days)} ${formatCountShort(t, "hour", left.hours)}`
  }
  if (left.hours > 0) {
    return `${formatCountShort(t, "hour", left.hours)} ${formatCountShort(t, "minute", left.minutes)}`
  }
  return `${left.minutes}:${String(left.seconds).padStart(2, "0")}`
}

/**
 * The same span written out, for somewhere with room: "5 hours 59 minutes".
 *
 * Used in the detail panel beside the exact instant, where the abbreviations
 * that fit a status bar would be needlessly terse.
 */
export function countdownLong(t: Translate, iso: string, now?: number): string | null {
  const left = countdownParts(iso, now)
  if (!left) return null

  if (left.days > 0) {
    return `${formatCount(t, "day", left.days)} ${formatCount(t, "hour", left.hours)}`
  }
  if (left.hours > 0) {
    return `${formatCount(t, "hour", left.hours)} ${formatCount(t, "minute", left.minutes)}`
  }
  return `${formatCount(t, "minute", left.minutes)} ${formatCount(t, "second", left.seconds)}`
}
