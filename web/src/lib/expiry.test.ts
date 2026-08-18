import { describe, expect, it } from "vitest"

import {
  NO_EXPIRY,
  expiryOption,
  expiryOptions,
  formatRemaining,
  formatRemainingShort,
  remainingParts,
} from "./expiry"

const NOW = Date.parse("2026-08-18T10:00:00Z")
const inSeconds = (n: number) => new Date(NOW + n * 1000).toISOString()

describe("remainingParts", () => {
  it("promotes the unit once rounding fills the one below it", () => {
    // The reason this exists: flooring makes every span read one short the
    // instant it is chosen, and naive rounding produces "60 minutes".
    const cases: Array<[number, string]> = [
      [30, "1 minute"],
      [90, "2 minutes"],
      [45 * 60, "45 minutes"],
      [59 * 60 + 40, "1 hour"],
      [60 * 60, "1 hour"],
      [6 * 3600 - 1, "6 hours"],
      [23 * 3600 + 40 * 60, "1 day"],
      [24 * 3600, "1 day"],
      [30 * 86400 - 1, "30 days"],
    ]
    for (const [seconds, want] of cases) {
      expect(formatRemaining(inSeconds(seconds), NOW), `${seconds}s`).toBe(want)
    }
  })

  it("never counts down past zero", () => {
    expect(remainingParts(inSeconds(0), NOW)).toBeNull()
    expect(remainingParts(inSeconds(-1), NOW)).toBeNull()
    expect(formatRemaining(inSeconds(-86400), NOW)).toBeNull()
  })

  it("returns null rather than NaN for an unparseable instant", () => {
    expect(remainingParts("not a date", NOW)).toBeNull()
  })

  it("abbreviates to the same number it spells out", () => {
    for (const seconds of [90, 45 * 60, 6 * 3600, 3 * 86400]) {
      const long = formatRemaining(inSeconds(seconds), NOW)!
      const short = formatRemainingShort(inSeconds(seconds), NOW)!
      const [count, unit] = long.split(" ")
      expect(short).toBe(count + unit[0])
    }
  })
})

describe("the ladder", () => {
  // What /api/config publishes today. The picker is built from whatever the
  // server sends, so these are inputs to the test rather than constants.
  const SERVER = [3600, 21600, 43200, 86400, 259200, 604800, 1209600, 2592000]

  it("names each rung the way a person would", () => {
    expect(expiryOptions(SERVER).map((o) => o.label)).toEqual([
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
    expect(expiryOptions(SERVER).map((o) => o.short)).toEqual([
      "∞", "1h", "6h", "12h", "1d", "3d", "7d", "14d", "30d",
    ])
  })

  it("offers exactly what the server sent, plus no-expiry", () => {
    const narrow = expiryOptions([21600, 604800])
    expect(narrow.map((o) => o.seconds)).toEqual([NO_EXPIRY, 21600, 604800])
    // Not a duration, so a server offering no rungs must still offer this one.
    expect(expiryOptions([]).map((o) => o.seconds)).toEqual([NO_EXPIRY])
  })

  it("falls back to hours and minutes for a span that is not whole days", () => {
    expect(expiryOptions([90 * 60, 2 * 3600]).map((o) => o.label)).toEqual([
      "No expiry",
      "90 minutes",
      "2 hours",
    ])
  })

  // A value off the ladder is one the API would reject, so the picker must not
  // present it as a live selection.
  it("falls back to no-expiry for a value the server does not offer", () => {
    expect(expiryOption(12345, SERVER).seconds).toBe(NO_EXPIRY)
    expect(expiryOption(21600, SERVER).label).toBe("6 hours")
    expect(expiryOption(NO_EXPIRY, SERVER).label).toBe("No expiry")
  })
})
