import { describe, expect, it } from "vitest"

import {
  EXPIRY_OPTIONS,
  NO_EXPIRY,
  availableExpiries,
  expiryOption,
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
  it("offers only rungs the server will accept", () => {
    const narrow = availableExpiries(6 * 3600, 7 * 86400)
    expect(narrow.map((o) => o.label)).toEqual([
      "No expiry",
      "6 hours",
      "12 hours",
      "1 day",
      "3 days",
      "7 days",
    ])
  })

  it("keeps no-expiry available whatever the bounds are", () => {
    // It is not a duration, so a range that excludes every rung must not also
    // remove the one choice that always applies.
    expect(availableExpiries(0, 0).map((o) => o.seconds)).toEqual([NO_EXPIRY])
  })

  it("climbs, and starts at the server's own minimum", () => {
    const durations = EXPIRY_OPTIONS.filter((o) => o.seconds !== NO_EXPIRY)
    expect(durations[0].seconds).toBe(3600)
    expect(durations.at(-1)!.seconds).toBe(30 * 86400)
    for (let i = 1; i < durations.length; i++) {
      expect(durations[i].seconds).toBeGreaterThan(durations[i - 1].seconds)
    }
  })

  it("falls back to no-expiry for a value that is not on the ladder", () => {
    expect(expiryOption(12345).seconds).toBe(NO_EXPIRY)
    expect(expiryOption(6 * 3600).label).toBe("6 hours")
  })
})
