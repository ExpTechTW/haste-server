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
