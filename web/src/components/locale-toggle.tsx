import { CheckIcon, LanguagesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { LOCALES, LOCALE_NAMES, useI18n } from "@/lib/i18n"

/**
 * The language picker.
 *
 * Marked by the 文/A glyph rather than a flag or a language code: a flag names
 * a country and not a language, and a code only helps someone who already knows
 * which code they want. Each language is then listed in itself, because
 * "Japanese" is unreadable to exactly the person looking for it.
 */
export function LocaleToggle() {
  const { locale, preference, setPreference, t } = useI18n()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={t("nav.language")}>
          <LanguagesIcon />
        </Button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="end" className="min-w-44">
        <DropdownMenuItem onSelect={() => setPreference("auto")}>
          <span className="flex-1">
            {t("locale.auto")}
            {/* Which language "automatic" currently lands on, so the choice is
                not a leap of faith. */}
            <span className="text-muted-foreground"> · {LOCALE_NAMES[locale]}</span>
          </span>
          {preference === "auto" && <CheckIcon className="size-3.5 opacity-60" />}
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        {LOCALES.map((value) => (
          <DropdownMenuItem key={value} onSelect={() => setPreference(value)} lang={value}>
            <span className="flex-1">{LOCALE_NAMES[value]}</span>
            {preference === value && <CheckIcon className="size-3.5 opacity-60" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
