import { describe, expect, it } from "vitest"

import { ensureHighlighter, logSeverityPalette } from "./highlighter"

/**
 * The log grammar tags severity levels but the GitHub themes have no rules for
 * those scopes, so without the overrides in highlighter.ts each level inherits
 * an unrelated colour and ERROR ends up looking calmer than WARN. These tests
 * pin the ordering back down.
 */

const SAMPLE = `2024-06-01 10:00:00 TRACE entering handler
2024-06-01 10:00:01 DEBUG cache warm
2024-06-01 10:00:02 INFO request completed
2024-06-01 10:00:03 WARN slow query
2024-06-01 10:00:04 ERROR connection refused
2024-06-01 10:00:05 FATAL shutting down`

async function severityColours(theme: "github-light" | "github-dark") {
  const shiki = await ensureHighlighter("log")
  const { tokens } = shiki.codeToTokens(SAMPLE, { lang: "log", theme })

  const found: Record<string, string> = {}
  for (const line of tokens) {
    for (const token of line) {
      const word = token.content.trim()
      if (/^(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)$/.test(word) && token.color) {
        found[word] = token.color.toUpperCase()
      }
    }
  }
  return found
}

/**
 * The severity rules are appended to the theme, and appending to the wrong
 * field replaces the theme's own rules instead of extending them — which leaves
 * log levels coloured and every other language rendered flat. The severity
 * tests above cannot see that, so ordinary syntax is checked here too.
 */
describe.each(["github-light", "github-dark"] as const)("theme integrity in %s", (theme) => {
  const DART = `import 'package:flutter/material.dart';

class Greeting extends StatelessWidget {
  const Greeting({super.key, required this.name});
  final String name;

  @override
  Widget build(BuildContext context) => Text('Hello');
}`

  it("still colours ordinary code", async () => {
    const shiki = await ensureHighlighter("dart")
    const { tokens } = shiki.codeToTokens(DART, { lang: "dart", theme })

    const colours = new Set<string>()
    for (const line of tokens) {
      for (const token of line) {
        if (token.content.trim() && token.color) colours.add(token.color.toUpperCase())
      }
    }

    // Keywords, types, strings and punctuation should not all share one colour.
    expect(colours.size).toBeGreaterThanOrEqual(4)
  })
})

describe.each([
  ["github-light", "light"],
  ["github-dark", "dark"],
] as const)("log severity in %s", (theme, key) => {
  const palette = logSeverityPalette[key]

  it("colours each level from the severity palette", async () => {
    const colours = await severityColours(theme)

    expect(colours.ERROR).toBe(palette.error.toUpperCase())
    expect(colours.FATAL).toBe(palette.error.toUpperCase())
    expect(colours.WARN).toBe(palette.warning.toUpperCase())
    expect(colours.INFO).toBe(palette.info.toUpperCase())
    expect(colours.DEBUG).toBe(palette.debug.toUpperCase())
    expect(colours.TRACE).toBe(palette.verbose.toUpperCase())
  })

  it("keeps every level visually distinct from the ones around it", async () => {
    const colours = await severityColours(theme)

    // The original failure: ERROR and WARN reading as the same kind of event,
    // or ERROR sitting on the calm end of the scale.
    expect(colours.ERROR).not.toBe(colours.WARN)
    expect(colours.WARN).not.toBe(colours.INFO)
    expect(colours.DEBUG).not.toBe(colours.INFO)
  })
})
