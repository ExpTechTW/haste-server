/**
 * GitHub-style line selection in the URL fragment: `#L17` for one line,
 * `#L17-L25` for a range.
 */

export interface LineRange {
  start: number
  end: number
}

// The trailing bound accepts both `L25` and a bare `25`, because links get
// hand-edited and both forms read the same way.
const HASH = /^#?L(\d+)(?:-L?(\d+))?$/

/** Reads a fragment into a range, or null when it selects nothing. */
export function parseLineHash(hash: string): LineRange | null {
  const match = HASH.exec(hash.trim())
  if (!match) return null

  const first = Number(match[1])
  const second = match[2] === undefined ? first : Number(match[2])
  if (!Number.isInteger(first) || first < 1) return null
  if (!Number.isInteger(second) || second < 1) return null

  // A backwards range is still a range; normalising means a shift-click
  // upwards produces the same link as the equivalent click downwards.
  return { start: Math.min(first, second), end: Math.max(first, second) }
}

/** Renders a range back into a fragment. */
export function formatLineHash(range: LineRange): string {
  return range.start === range.end ? `#L${range.start}` : `#L${range.start}-L${range.end}`
}

/**
 * Applies a click on line `line` to the current selection. Shift anchors to the
 * start of what is already selected, which is how GitHub extends a range.
 */
export function selectLine(
  current: LineRange | null,
  line: number,
  extend: boolean,
): LineRange {
  if (!extend || !current) return { start: line, end: line }
  return { start: Math.min(current.start, line), end: Math.max(current.start, line) }
}

/** Drops a range that points past the end of the content. */
export function clampRange(range: LineRange | null, lineCount: number): LineRange | null {
  if (!range || range.start > lineCount) return null
  return { start: range.start, end: Math.min(range.end, lineCount) }
}
