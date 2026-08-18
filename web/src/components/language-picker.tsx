import * as React from "react"
import { CheckIcon, ChevronsUpDownIcon, WandSparklesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useLanguageLabel } from "@/hooks/use-language-label"
import { useT } from "@/lib/i18n"
import { LANGUAGES, PLAIN } from "@/lib/languages"
import { cn } from "@/lib/utils"

/** Sentinel for "let the detector decide". */
export const AUTO = "auto"

/**
 * Searchable language picker. The catalogue runs to eighty-odd entries, which a
 * plain dropdown turns into a scrolling chore — typing "flutter" or "golang"
 * should land on the right grammar immediately.
 */
export function LanguagePicker({
  value,
  detected,
  onChange,
}: {
  value: string
  detected: string
  onChange: (value: string) => void
}) {
  const [open, setOpen] = React.useState(false)
  const t = useT()
  const languageLabel = useLanguageLabel()

  // "Auto(Dart)" rather than "Auto · Dart": short enough to say both things at
  // any width, so the phone no longer has to drop half of it.
  const label =
    value === AUTO
      ? detected === PLAIN
        ? t("lang.auto")
        : `${t("lang.auto")}(${languageLabel(detected)})`
      : languageLabel(value)

  const select = (next: string) => {
    onChange(next)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          role="combobox"
          aria-expanded={open}
          aria-label={t("lang.aria")}
          // Allowed to shrink: the character counter grows as a paste fills up, and
          // this is what keeps the row from wrapping when it does.
          className="w-[7.5rem] min-w-18 shrink justify-between font-normal sm:w-[11rem]"
        >
          <span className="truncate">{label}</span>
          <ChevronsUpDownIcon className="opacity-50" />
        </Button>
      </PopoverTrigger>

      <PopoverContent align="end" className="w-[15rem] p-0">
        <Command
          filter={(itemValue, search, keywords) => {
            const haystack = [itemValue, ...(keywords ?? [])].join(" ").toLowerCase()
            const needle = search.toLowerCase()
            if (!needle) return 1
            // Prefer prefix matches so "go" ranks Go above Django-ish aliases.
            if (haystack.startsWith(needle)) return 1
            return haystack.includes(needle) ? 0.5 : 0
          }}
        >
          <CommandInput placeholder={t("lang.search")} />
          <CommandList>
            <CommandEmpty>{t("lang.empty")}</CommandEmpty>

            <CommandGroup>
              <CommandItem value={AUTO} keywords={["automatic", "detect"]} onSelect={select}>
                <WandSparklesIcon className="opacity-60" />
                <span className="flex-1 truncate">
                  {t("lang.auto")}
                  {detected !== PLAIN && (
                    <span className="text-muted-foreground">({languageLabel(detected)})</span>
                  )}
                </span>
                <CheckIcon className={cn("size-3.5", value !== AUTO && "invisible")} />
              </CommandItem>
            </CommandGroup>

            <CommandGroup>
              {LANGUAGES.map((lang) => (
                <CommandItem
                  key={lang.id}
                  value={lang.id}
                  keywords={[lang.label, lang.aliases ?? ""]}
                  onSelect={select}
                >
                  <span className="flex-1 truncate">{languageLabel(lang.id)}</span>
                  <CheckIcon className={cn("size-3.5", value !== lang.id && "invisible")} />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
