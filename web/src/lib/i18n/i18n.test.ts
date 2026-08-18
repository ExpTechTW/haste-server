import { describe, expect, it } from "vitest"

import { detectLocale, LOCALES, LOCALE_NAMES } from "./index"
import { en, messages, type MessageKey } from "./messages"

const KEYS = Object.keys(en) as MessageKey[]

describe("the dictionaries", () => {
  // Completeness is a compile error already — `Record<MessageKey, string>`
  // over the English keys. What the type cannot see is the content.
  it("carries every placeholder the English does", () => {
    const placeholders = (s: string) => (s.match(/\{(\w+)\}/g) ?? []).sort()

    for (const locale of LOCALES) {
      for (const key of KEYS) {
        expect(placeholders(messages[locale][key]), `${locale} ${key}`).toEqual(
          placeholders(en[key]),
        )
      }
    }
  })

  it("leaves nothing untranslated", () => {
    // An English string copied into another dictionary is almost always an
    // oversight. The exceptions are the few strings that genuinely are the same
    // in every language: names, and the shell-style error lines on the 404.
    const shared = new Set<MessageKey>([
      "docs.title",
      "docs.baseUrl",
      "nf.noPaste",
      "nf.noPage",
    ])

    for (const locale of LOCALES.filter((l) => l !== "en-US")) {
      for (const key of KEYS) {
        if (shared.has(key)) continue
        expect(messages[locale][key], `${locale} ${key}`).not.toBe(en[key])
      }
    }
  })

  it("has no empty strings", () => {
    for (const locale of LOCALES) {
      for (const key of KEYS) {
        expect(messages[locale][key].trim(), `${locale} ${key}`).not.toBe("")
      }
    }
  })

  it("names each language in itself", () => {
    // A reader who only reads Japanese cannot find "Japanese" in a list.
    expect(LOCALE_NAMES["ja-JP"]).toBe("日本語")
    expect(LOCALE_NAMES["zh-TW"]).toBe("繁體中文")
    expect(Object.keys(LOCALE_NAMES).sort()).toEqual([...LOCALES].sort())
  })
})

describe("detectLocale", () => {
  it("takes the browser's first language this app speaks", () => {
    expect(detectLocale(["ja-JP", "en-US"])).toBe("ja-JP")
    expect(detectLocale(["ko-KR", "ja"])).toBe("ja-JP")
    // Not the first tag: the first one that is on offer.
    expect(detectLocale(["de-DE", "fr-FR", "en-GB"])).toBe("en-US")
  })

  it("sends every Chinese to Traditional", () => {
    // It is the only Chinese here, and it serves a Simplified reader far better
    // than falling through to English would.
    for (const tag of ["zh", "zh-TW", "zh-Hant", "zh-CN", "zh-Hans-CN", "ZH-hk"]) {
      expect(detectLocale([tag]), tag).toBe("zh-TW")
    }
  })

  it("matches on the primary subtag, not the whole tag", () => {
    expect(detectLocale(["en-AU"])).toBe("en-US")
    expect(detectLocale(["ja-JP-u-ca-japanese"])).toBe("ja-JP")
  })

  it("falls back to English for a language it does not speak", () => {
    expect(detectLocale(["ko-KR", "de"])).toBe("en-US")
    expect(detectLocale([])).toBe("en-US")
  })
})
