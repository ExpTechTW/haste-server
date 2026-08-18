import { CheckIcon, MonitorIcon, MoonIcon, SunIcon } from "lucide-react"

import { useTheme, type Theme } from "@/components/theme-provider"
import { useT, type MessageKey } from "@/lib/i18n"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

const OPTIONS: Array<{ value: Theme; label: MessageKey; icon: typeof SunIcon }> = [
  { value: "light", label: "theme.light", icon: SunIcon },
  { value: "dark", label: "theme.dark", icon: MoonIcon },
  { value: "system", label: "theme.system", icon: MonitorIcon },
]

export function ModeToggle() {
  const { theme, resolvedTheme, setTheme } = useTheme()
  const t = useT()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={t("nav.theme")}>
          {resolvedTheme === "dark" ? <MoonIcon /> : <SunIcon />}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {OPTIONS.map(({ value, label, icon: Icon }) => (
          <DropdownMenuItem key={value} onSelect={() => setTheme(value)}>
            <Icon />
            <span className="flex-1">{t(label)}</span>
            {theme === value && <CheckIcon className="size-3.5 opacity-60" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
