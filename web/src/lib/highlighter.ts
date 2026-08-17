import type { HighlighterCore } from "shiki/core"

import { PLAIN, loaderFor } from "./languages"

/**
 * Both palettes are baked into every token as CSS variables, so light/dark is a
 * class toggle on <html> rather than a re-highlight. `defaultColor: "light"`
 * leaves the light value inline and exposes the dark one as --shiki-dark, which
 * is what index.css overrides.
 */
export const THEMES = { light: "github-light", dark: "github-dark" } as const

let highlighterPromise: Promise<HighlighterCore> | null = null
const loadedLanguages = new Set<string>()

/**
 * Severity colours for log pastes.
 *
 * The log grammar tags levels as `log.error`, `log.warning` and so on, but the
 * GitHub themes carry no rules for those scopes, so each level falls through to
 * whatever generic scope it piggybacks on. The result is backwards: WARN
 * inherits `markup.deleted` and comes out red, while ERROR inherits
 * `string.regexp` and comes out blue — calmer than the warning above it, and in
 * the light theme almost indistinguishable from ordinary text.
 *
 * These rules put the levels back in the order a reader expects. The values are
 * GitHub's own Primer palette, so they sit correctly against both themes.
 */
const LOG_SEVERITY = {
  light: {
    error: "#CF222E",
    warning: "#9A6700",
    info: "#1A7F37",
    debug: "#6E7781",
    verbose: "#8C959F",
  },
  dark: {
    error: "#FF7B72",
    warning: "#D29922",
    info: "#3FB950",
    debug: "#8B949E",
    verbose: "#6E7681",
  },
} as const

type ThemeLike = { settings?: unknown[]; tokenColors?: unknown[] }

function withLogSeverity<T extends ThemeLike>(theme: T, palette: (typeof LOG_SEVERITY)["light"]): T {
  // Appended last and keyed on the innermost scope, so they win over the
  // generic rule the level would otherwise resolve through.
  const rules = [
    { scope: ["log.error"], settings: { foreground: palette.error, fontStyle: "bold" } },
    { scope: ["log.warning"], settings: { foreground: palette.warning, fontStyle: "bold" } },
    { scope: ["log.info"], settings: { foreground: palette.info } },
    { scope: ["log.debug"], settings: { foreground: palette.debug } },
    { scope: ["log.verbose"], settings: { foreground: palette.verbose } },
  ]
  return { ...theme, settings: [...(theme.settings ?? []), ...rules] }
}

async function create(): Promise<HighlighterCore> {
  // Imported dynamically so the engine and themes land in their own chunk,
  // keeping them out of the initial page load.
  const [{ createHighlighterCore }, { createJavaScriptRegexEngine }, light, dark] =
    await Promise.all([
      import("shiki/core"),
      import("shiki/engine/javascript"),
      import("@shikijs/themes/github-light"),
      import("@shikijs/themes/github-dark"),
    ])

  return createHighlighterCore({
    themes: [
      withLogSeverity(light.default as ThemeLike, LOG_SEVERITY.light),
      withLogSeverity(dark.default as ThemeLike, LOG_SEVERITY.dark),
    ] as never,
    langs: [],
    // The JavaScript engine avoids the ~500 KB oniguruma WASM payload.
    engine: createJavaScriptRegexEngine({ forgiving: true }),
  })
}

/** Exposed for the test that guards the severity ordering. */
export const logSeverityPalette = LOG_SEVERITY

/**
 * Loads the highlighter and the grammar for one language. Once this resolves,
 * `highlightSync` can colour that language without another await — which is
 * what lets the editor repaint in the same frame as the keystroke.
 */
export async function ensureHighlighter(language: string): Promise<HighlighterCore> {
  highlighterPromise ??= create()
  const shiki = await highlighterPromise

  const load = loaderFor(language)
  if (load && !loadedLanguages.has(language)) {
    try {
      await shiki.loadLanguage(load() as never)
      loadedLanguages.add(language)
    } catch {
      // A grammar that fails to load costs styling, not the whole view.
    }
  }
  return shiki
}

/**
 * Gives every line an id and a link target so `#L17` can address it.
 *
 * The anchor is deliberately empty: the visible number comes from a CSS
 * counter, so there is no text inside the code block for a selection to pick
 * up. Copying a highlighted range yields the code alone.
 */
const lineAnchors = {
  name: "haste:line-anchors",
  line(node: { properties: Record<string, unknown>; children: unknown[] }, line: number) {
    node.properties.id = `L${line}`
    node.properties["data-line"] = String(line)
    node.children.unshift({
      type: "element",
      tagName: "a",
      properties: {
        class: "line-link",
        href: `#L${line}`,
        "aria-label": `Line ${line}`,
      },
      children: [],
    })
  },
}

export interface HighlightOptions {
  /** Adds per-line ids and gutter links, for views that support #L17-L25. */
  addressable?: boolean
}

/**
 * Renders code to themed HTML with no awaiting. Safe only for a language that
 * `ensureHighlighter` has already resolved for; anything else falls back to
 * plain text rather than throwing.
 */
export function highlightSync(
  shiki: HighlighterCore,
  code: string,
  language: string,
  options: HighlightOptions = {},
): string {
  const lang = loadedLanguages.has(language) ? language : PLAIN
  return shiki.codeToHtml(code, {
    lang,
    themes: THEMES,
    defaultColor: "light",
    transformers: options.addressable ? [lineAnchors as never] : [],
  })
}

/** Loads whatever is needed, then renders. For one-shot views. */
export async function highlight(
  code: string,
  language: string,
  options: HighlightOptions = {},
): Promise<string> {
  const lang = language || PLAIN
  const shiki = await ensureHighlighter(lang)
  return highlightSync(shiki, code, lang, options)
}
