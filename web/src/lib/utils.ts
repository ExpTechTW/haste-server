import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** The platform's chord prefix, for rendering shortcut hints. */
export function modKey(): string {
  return /Mac|iPhone|iPad/.test(navigator.userAgent) ? "⌘" : "Ctrl+"
}


/**
 * Counts Unicode code points, matching the runes the server counts.
 *
 * `Array.from(s).length` gives the same answer but materialises one string per
 * character, which is thousands of short-lived allocations on every keystroke.
 * A UTF-16 length is already the code-point count unless the text contains a
 * surrogate pair, so this only has to subtract those.
 */
export function countCodePoints(s: string): number {
  let count = s.length
  for (let i = 0; i < s.length - 1; i++) {
    // A high surrogate followed by a low one is a single code point.
    if ((s.charCodeAt(i) & 0xfc00) === 0xd800 && (s.charCodeAt(i + 1) & 0xfc00) === 0xdc00) {
      count--
      i++
    }
  }
  return count
}

/**
 * The zone timestamps are shown in.
 *
 * Fixed rather than the reader's own: a paste is usually shared alongside a
 * conversation about when something happened, and a timestamp that reads
 * differently for each person defeats that. Taipei observes no daylight saving,
 * so this is UTC+8 all year round.
 */
const DISPLAY_TIME_ZONE = "Asia/Taipei"

function partsIn(iso: string, timeZone: string): Record<string, string> | null {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return null

  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    // Midnight has to read as 00, not 24.
    hourCycle: "h23",
  }).formatToParts(date)

  return Object.fromEntries(parts.map((p) => [p.type, p.value]))
}

/** Formats an instant as `2026-08-18 05:36:10` in UTC+8. */
export function formatTimestamp(iso: string): string | null {
  const p = partsIn(iso, DISPLAY_TIME_ZONE)
  if (!p) return null
  return `${p.year}-${p.month}-${p.day} ${p.hour}:${p.minute}:${p.second}`
}

/** The same instant without the date, for where a full stamp will not fit. */
export function formatTimeOfDay(iso: string): string | null {
  const p = partsIn(iso, DISPLAY_TIME_ZONE)
  if (!p) return null
  return `${p.month}-${p.day} ${p.hour}:${p.minute}`
}

/** Spells out what the displayed time means, for a title attribute. */
export function describeTimestamp(iso: string): string | null {
  const local = formatTimestamp(iso)
  const utc = partsIn(iso, "UTC")
  if (!local || !utc) return null
  return `${local} (UTC+8) · ${utc.year}-${utc.month}-${utc.day} ${utc.hour}:${utc.minute}:${utc.second} UTC`
}
