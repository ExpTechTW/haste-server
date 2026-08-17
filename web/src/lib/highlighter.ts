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

async function create(): Promise<HighlighterCore> {
  // Imported dynamically so the engine and themes land in their own chunk: the
  // editor never highlights anything, and should not download a highlighter.
  const [{ createHighlighterCore }, { createJavaScriptRegexEngine }] = await Promise.all([
    import("shiki/core"),
    import("shiki/engine/javascript"),
  ])

  return createHighlighterCore({
    themes: [
      import("@shikijs/themes/github-light"),
      import("@shikijs/themes/github-dark"),
    ],
    langs: [],
    // The JavaScript engine avoids the ~500 KB oniguruma WASM payload.
    engine: createJavaScriptRegexEngine({ forgiving: true }),
  })
}

function highlighter(): Promise<HighlighterCore> {
  highlighterPromise ??= create()
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
 * Warms the highlighter and grammar while the user is still typing, so opening
 * the saved paste does not also pay for the first download.
 */
export function prewarm(language: string): void {
  void highlighter()
  const load = loaderFor(language)
  if (load && !loadedLanguages.has(language)) void load()
}
