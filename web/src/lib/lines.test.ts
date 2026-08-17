import { describe, expect, it } from "vitest"

import { clampRange, formatLineHash, parseLineHash, selectLine } from "./lines"

describe("parseLineHash", () => {
  it.each([
    ["#L17", { start: 17, end: 17 }],
    ["#L17-L25", { start: 17, end: 25 }],
    ["L17-L25", { start: 17, end: 25 }],
    // Hand-edited links show up in both forms.
    ["#L17-25", { start: 17, end: 25 }],
    // A backwards range is normalised, so shift-clicking upwards produces the
    // same link as the equivalent click downwards.
    ["#L25-L17", { start: 17, end: 25 }],
    ["#L1", { start: 1, end: 1 }],
  ])("reads %s", (hash, expected) => {
    expect(parseLineHash(hash)).toEqual(expected)
  })

  it.each(["", "#", "#L", "#L0", "#L-5", "#section", "#L1x", "#Lfoo", "#17"])(
    "rejects %j",
    (hash) => {
      expect(parseLineHash(hash)).toBeNull()
    },
  )
})

describe("formatLineHash", () => {
  it("collapses a single-line range", () => {
    expect(formatLineHash({ start: 17, end: 17 })).toBe("#L17")
  })

  it("writes both bounds for a range", () => {
    expect(formatLineHash({ start: 17, end: 25 })).toBe("#L17-L25")
  })

  it("round-trips through parseLineHash", () => {
    for (const range of [
      { start: 1, end: 1 },
      { start: 3, end: 9 },
      { start: 120, end: 4000 },
    ]) {
      expect(parseLineHash(formatLineHash(range))).toEqual(range)
    }
  })
})

describe("selectLine", () => {
  it("starts a fresh selection on a plain click", () => {
    expect(selectLine(null, 17, false)).toEqual({ start: 17, end: 17 })
    expect(selectLine({ start: 3, end: 9 }, 17, false)).toEqual({ start: 17, end: 17 })
  })

  it("extends downwards from the anchor", () => {
    expect(selectLine({ start: 17, end: 17 }, 25, true)).toEqual({ start: 17, end: 25 })
  })

  it("extends upwards from the anchor", () => {
    expect(selectLine({ start: 17, end: 17 }, 5, true)).toEqual({ start: 5, end: 17 })
  })

  it("re-anchors on the start, so shrinking a range works", () => {
    expect(selectLine({ start: 10, end: 40 }, 20, true)).toEqual({ start: 10, end: 20 })
  })

  it("has nothing to extend without a current selection", () => {
    expect(selectLine(null, 17, true)).toEqual({ start: 17, end: 17 })
  })
})

describe("clampRange", () => {
  it("trims a range that runs past the end", () => {
    expect(clampRange({ start: 5, end: 900 }, 20)).toEqual({ start: 5, end: 20 })
  })

  it("drops a range that starts past the end", () => {
    expect(clampRange({ start: 50, end: 60 }, 20)).toBeNull()
  })

  it("passes through a range that fits", () => {
    expect(clampRange({ start: 5, end: 9 }, 20)).toEqual({ start: 5, end: 9 })
  })

  it("handles no selection", () => {
    expect(clampRange(null, 20)).toBeNull()
  })
})
