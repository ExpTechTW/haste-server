import { describe, expect, it } from "vitest"

import {
  NO_EXPIRY,
  countdown,
  countdownLong,
  countdownParts,
  expiryOption,
  expiryOptions,
} from "./expiry"
import { messages } from "./i18n/messages"
import type { MessageKey, Translate } from "./i18n"

/** The real dictionaries, so a translation that breaks a format is caught. */
function translator(locale: string): Translate {
  const dict = messages[locale]
  return (key: MessageKey, vars) => {
    const template = dict[key]
    if (!vars) return template
    return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
      name in vars ? String(vars[name]) : whole,
    )
  }
}

const t = translator("en-US")
const NOW = Date.parse("2026-08-18T10:00:00Z")
const inSeconds = (n: number) => new Date(NOW + n * 1000).toISOString()

describe("countdown", () => {
  it("shows two units, coarsest first, and seconds only in the last hour", () => {
    const cases: Array<[number, string]> = [
      [30 * 86400, "30d 0h"],
      [29 * 86400 + 23 * 3600 + 59 * 60, "29d 23h"],
      [6 * 3600, "6h 0m"],
      // A moment after choosing "6 hours": still counting down from it, which
      // is what a countdown does, rather than misreporting the span.
      [6 * 3600 - 1, "5h 59m"],
      [3600, "1h 0m"],
      [3599, "59:59"],
      [90, "1:30"],
      [5, "0:05"],
    ]
    for (const [seconds, want] of cases) {
      expect(countdown(t, inSeconds(seconds), NOW), `${seconds}s`).toBe(want)
    }
  })

  it("never rounds up", () => {
    // Rounding up would put a moment on screen that has already gone.
    for (const seconds of [3599, 86399, 59, 1]) {
      const parts = countdownParts(inSeconds(seconds), NOW)!
      const shown = parts.days * 86400 + parts.hours * 3600 + parts.minutes * 60 + parts.seconds
      expect(shown).toBeLessThanOrEqual(seconds)
    }
  })

  it("stops rather than going negative", () => {
    expect(countdown(t, inSeconds(0), NOW)).toBeNull()
    expect(countdown(t, inSeconds(-1), NOW)).toBeNull()
    expect(countdownParts("not a date", NOW)).toBeNull()
  })

  it("spells the same span out for the detail panel", () => {
    expect(countdownLong(t, inSeconds(6 * 3600 - 1), NOW)).toBe("5 hours 59 minutes")
    expect(countdownLong(t, inSeconds(90), NOW)).toBe("1 minute 30 seconds")
    expect(countdownLong(t, inSeconds(2 * 86400), NOW)).toBe("2 days 0 hours")
  })

  it("counts in the reader's language", () => {
    const zh = translator("zh-TW")
    const ja = translator("ja-JP")
    expect(countdown(zh, inSeconds(6 * 3600 - 1), NOW)).toBe("5小時 59分")
    expect(countdown(ja, inSeconds(6 * 3600 - 1), NOW)).toBe("5時間 59分")
    expect(countdownLong(zh, inSeconds(2 * 86400), NOW)).toBe("2 天 0 小時")
  })
})

describe("the ladder", () => {
  // What /api/config publishes today. The picker is built from whatever the
  // server sends, so these are inputs to the test rather than constants.
  const SERVER = [3600, 21600, 43200, 86400, 259200, 604800, 1209600, 2592000]

  it("names each rung the way a person would", () => {
    expect(expiryOptions(t, SERVER).map((o) => o.label)).toEqual([
      "No expiry",
      "1 hour",
      "6 hours",
      "12 hours",
      "1 day",
      "3 days",
      "7 days",
      "14 days",
      "30 days",
    ])
  })

  it("names them in the reader's language too", () => {
    expect(expiryOptions(translator("zh-TW"), [3600, 2592000]).map((o) => o.label)).toEqual([
      "不限制",
      "1 小時",
      "30 天",
    ])
    expect(expiryOptions(translator("ja-JP"), [3600, 2592000]).map((o) => o.label)).toEqual([
      "期限なし",
      "1 時間",
      "30 日",
    ])
  })

  it("offers exactly what the server sent, plus no-expiry", () => {
    expect(expiryOptions(t, [21600, 604800]).map((o) => o.seconds)).toEqual([
      NO_EXPIRY, 21600, 604800,
    ])
    // Not a duration, so a server offering no rungs must still offer this one.
    expect(expiryOptions(t, []).map((o) => o.seconds)).toEqual([NO_EXPIRY])
  })

  it("falls back to hours and minutes for a span that is not whole days", () => {
    expect(expiryOptions(t, [90 * 60, 2 * 3600]).map((o) => o.label)).toEqual([
      "No expiry",
      "90 minutes",
      "2 hours",
    ])
  })

  // A value off the ladder is one the API would reject, so the picker must not
  // present it as a live selection.
  it("falls back to no-expiry for a value the server does not offer", () => {
    expect(expiryOption(t, 12345, SERVER).seconds).toBe(NO_EXPIRY)
    expect(expiryOption(t, 21600, SERVER).label).toBe("6 hours")
    expect(expiryOption(t, NO_EXPIRY, SERVER).label).toBe("No expiry")
  })
})
