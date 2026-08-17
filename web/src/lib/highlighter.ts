import { createHighlighterCore } from "shiki/core"
import { createJavaScriptRegexEngine } from "shiki/engine/javascript"

import { PLAIN, loaderFor } from "./languages"

/**
 * Both palettes are baked into every token as CSS variables, so light/dark is a
 * class toggle on <html> rather than a re-highlight. `defaultColor: "light"`
 * leaves the light value inline and exposes the dark one as --shiki-dark, which
 * is what index.css overrides.
 */
export const THEMES = { light: "github-light", dark: "github-dark" } as const

type Highlighter = Awaited<ReturnType<typeof createHighlighterCore>>

let highlighterPromise: Promise<Highlighter> | null = null
const loadedLanguages = new Set<string>()

function highlighter(): Promise<Highlighter> {
  // The core bundle ships no grammars and the JavaScript regex engine avoids
  // the ~500 KB oniguruma WASM payload entirely.
  highlighterPromise ??= createHighlighterCore({
    themes: [
      import("@shikijs/themes/github-light"),
      import("@shikijs/themes/github-dark"),
    ],
    langs: [],
    engine: createJavaScriptRegexEngine({ forgiving: true }),
  })
  return highlighterPromise
}

/** Renders code to themed HTML, loading the grammar on first use. */
export async function highlight(code: string, language: string): Promise<string> {
  const shiki = await highlighter()

  let lang = language || PLAIN
  const load = loaderFor(lang)
  if (!load) {
    // Unknown ids reach us from the API, where language is a free-form hint.
    lang = PLAIN
  } else if (!loadedLanguages.has(lang)) {
    try {
      await shiki.loadLanguage(load() as never)
      loadedLanguages.add(lang)
    } catch {
      // A grammar that fails to load should cost styling, not the whole view.
      lang = PLAIN
    }
  }

  return shiki.codeToHtml(code, { lang, themes: THEMES, defaultColor: "light" })
}

/**
 * Warms the highlighter while the user is still typing, so saving a paste does
 * not also pay for the first grammar fetch.
 */
export function prewarm(language: string): void {
  void highlighter()
  const load = loaderFor(language)
  if (load && !loadedLanguages.has(language)) void load()
}
