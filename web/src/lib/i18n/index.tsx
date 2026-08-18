import * as React from "react"

import { messages, type MessageKey, type Messages } from "./messages"

export type { MessageKey } from "./messages"

export const LOCALES = ["zh-TW", "en-US", "ja-JP"] as const
export type Locale = (typeof LOCALES)[number]

/** What the picker offers: a language, or letting the browser decide. */
export type LocalePreference = Locale | "auto"

/** Endonyms. A language is named in itself or it is no help to the person
 *  looking for it — someone who reads only Japanese cannot find "Japanese". */
export const LOCALE_NAMES: Record<Locale, string> = {
  "zh-TW": "繁體中文",
  "en-US": "English",
  "ja-JP": "日本語",
}

const STORAGE_KEY = "haste.locale"
const FALLBACK: Locale = "en-US"

/**
 * The best of the browser's languages that this app speaks.
 *
 * Matched by primary subtag, in the browser's own order of preference, so a
 * reader whose first choice is unavailable still gets their second. Any Chinese
 * maps to Traditional: it is the only Chinese on offer, and it serves a
 * Simplified reader far better than English would.
 */
export function detectLocale(
  languages: readonly string[] = navigator.languages?.length
    ? navigator.languages
    : [navigator.language],
): Locale {
  for (const tag of languages) {
    const primary = tag.toLowerCase().split("-")[0]
    if (primary === "zh") return "zh-TW"
    if (primary === "ja") return "ja-JP"
    if (primary === "en") return "en-US"
  }
  return FALLBACK
}

function storedPreference(): LocalePreference {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === "auto" || (LOCALES as readonly string[]).includes(saved ?? "")) {
      return saved as LocalePreference
    }
  } catch {
    // Private browsing and similar. Detection alone is a fine default.
  }
  return "auto"
}

/** Substitutes `{name}` placeholders. */
export type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string

interface I18n {
  locale: Locale
  preference: LocalePreference
  setPreference: (next: LocalePreference) => void
  t: Translate
}

const Context = React.createContext<I18n | null>(null)

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [preference, setPreferenceState] = React.useState<LocalePreference>(storedPreference)
  const [detected, setDetected] = React.useState<Locale>(() => detectLocale())

  // The browser's preference can change while the tab is open.
  React.useEffect(() => {
    const onChange = () => setDetected(detectLocale())
    window.addEventListener("languagechange", onChange)
    return () => window.removeEventListener("languagechange", onChange)
  }, [])

  const locale = preference === "auto" ? detected : preference

  const setPreference = React.useCallback((next: LocalePreference) => {
    setPreferenceState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // The choice still holds for this tab.
    }
  }, [])

  // Keeps the document honest about what it is written in, which is what stops
  // a browser offering to translate a page already in the reader's language,
  // and what tells a screen reader which voice to use.
  React.useEffect(() => {
    document.documentElement.lang = locale
  }, [locale])

  const value = React.useMemo<I18n>(() => {
    const dict: Messages = messages[locale] ?? messages[FALLBACK]
    const t: Translate = (key, vars) => {
      const template = dict[key] ?? messages[FALLBACK][key] ?? key
      if (!vars) return template
      return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
        name in vars ? String(vars[name]) : whole,
      )
    }
    return { locale, preference, setPreference, t }
  }, [locale, preference, setPreference])

  return <Context.Provider value={value}>{children}</Context.Provider>
}

export function useI18n(): I18n {
  const ctx = React.useContext(Context)
  if (!ctx) throw new Error("useI18n must be used inside <I18nProvider>")
  return ctx
}

/** The common case: only the translate function. */
export function useT(): Translate {
  return useI18n().t
}

/**
 * A count and its unit, agreeing in number where the language cares.
 *
 * English needs "1 hour" against "6 hours"; Chinese and Japanese use one form
 * for both, which the dictionaries express by giving `.one` and `.other` the
 * same string rather than by this code knowing which languages inflect.
 */
export type TimeUnit = "second" | "minute" | "hour" | "day"

export function formatCount(t: Translate, unit: TimeUnit, count: number): string {
  return `${count} ${t(`unit.${unit}.${count === 1 ? "one" : "other"}` as MessageKey)}`
}

/** The same, abbreviated: "6h", "6小時", "6時間". */
export function formatCountShort(t: Translate, unit: TimeUnit, count: number): string {
  return `${count}${t(`unit.short.${unit}` as MessageKey)}`
}
