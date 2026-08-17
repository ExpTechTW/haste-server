import { describe, expect, it } from "vitest"

import { countCodePoints } from "./utils"

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
