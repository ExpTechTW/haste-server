import { describe, expect, it } from "vitest"

import { countCodePoints, describeTimestamp, formatTimeOfDay, formatTimestamp } from "./utils"

// The server counts runes, so the editor's counter has to agree with it exactly
// — including for the characters that occupy two UTF-16 units.
describe("countCodePoints", () => {
  it.each([
    ["", 0],
    ["hello", 5],
    ["héllo", 5],
    ["世界", 2],
    ["a\nb", 3],
    ["👋", 1],
    ["👨‍👩‍👧", 5], // three emoji joined by two zero-width joiners
    ["a👋b", 3],
    ["👋👋", 2],
  ])("counts %j as %i", (input, expected) => {
    expect(countCodePoints(input)).toBe(expected)
  })

  it("agrees with Array.from across mixed content", () => {
    const samples = [
      "plain ascii",
      "中文字元測試",
      "emoji 👋 mixed 世界 with ascii",
      "🇹🇼 flags are surrogate pairs too",
      "trailing high surrogate is left alone: \ud800",
    ]
    for (const sample of samples) {
      expect(countCodePoints(sample)).toBe(Array.from(sample).length)
    }
  })

  it("counts a full-size paste correctly", () => {
    expect(countCodePoints("界".repeat(4000))).toBe(4000)
    expect(countCodePoints("👋".repeat(4000))).toBe(4000)
  })
})

// The server stores UTC; the page shows UTC+8. Every case here is an instant
// where getting the offset wrong would be visible.
describe("formatTimestamp", () => {
  it.each([
    // 21:36 UTC is already the next morning in Taipei.
    ["2026-08-17T21:36:10Z", "2026-08-18 05:36:10"],
    // Exactly midnight UTC+8, which must read 00 rather than 24.
    ["2026-08-17T16:00:00Z", "2026-08-18 00:00:00"],
    // One second before, so the date has not rolled over yet.
    ["2026-08-17T15:59:59Z", "2026-08-17 23:59:59"],
    // Across a year boundary.
    ["2025-12-31T16:00:00Z", "2026-01-01 00:00:00"],
    // Midsummer: Taipei keeps UTC+8, with no daylight saving to shift it.
    ["2026-07-01T04:00:00Z", "2026-07-01 12:00:00"],
    // Midwinter, same offset.
    ["2026-01-01T04:00:00Z", "2026-01-01 12:00:00"],
  ])("renders %s as %s", (iso, expected) => {
    expect(formatTimestamp(iso)).toBe(expected)
  })

  it("returns null for something that is not a timestamp", () => {
    expect(formatTimestamp("not a date")).toBeNull()
    expect(formatTimestamp("")).toBeNull()
  })

  it("is exactly eight hours ahead of UTC", () => {
    const iso = "2026-03-14T00:00:00Z"
    expect(formatTimestamp(iso)).toBe("2026-03-14 08:00:00")
  })
})

describe("formatTimeOfDay", () => {
  it("drops the year for narrow layouts", () => {
    expect(formatTimeOfDay("2026-08-17T21:36:10Z")).toBe("08-18 05:36")
  })
})

describe("describeTimestamp", () => {
  it("names the zone and gives the UTC instant alongside it", () => {
    expect(describeTimestamp("2026-08-17T21:36:10Z")).toBe(
      "2026-08-18 05:36:10 (UTC+8) · 2026-08-17 21:36:10 UTC",
    )
  })
})
