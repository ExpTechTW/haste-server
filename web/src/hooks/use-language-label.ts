import * as React from "react"

import { useT } from "@/lib/i18n"
import { PLAIN, languageLabel } from "@/lib/languages"

/**
 * Names a language in the reader's language.
 *
 * Only one of the eighty-odd entries needs translating: "Dart" and "Objective-C"
 * are proper nouns and read the same everywhere, but "Plain text" is an ordinary
 * noun and looks like an oversight sitting in a Chinese or Japanese interface.
 */
export function useLanguageLabel(): (id: string) => string {
  const t = useT()
  return React.useCallback(
    (id: string) => (!id || id === PLAIN ? t("lang.plain") : languageLabel(id)),
    [t],
  )
}
