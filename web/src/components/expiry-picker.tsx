import { CheckIcon, ClockIcon, InfinityIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { NO_EXPIRY, expiryOption, type ExpiryOption } from "@/lib/expiry"
import { cn } from "@/lib/utils"

/**
 * How long the paste should live.
 *
 * A plain menu rather than the searchable popover the language picker uses:
 * nine fixed rungs are faster to scan than to type at, and the trigger has to
 * survive a phone's status bar sharing one nowrap row with the counter, the
 * language picker and Save. Hence the terse trigger — "1h", "∞" — with the
 * spelled-out form kept for the menu, where there is room to read.
 */
export function ExpiryPicker({
  value,
  options,
  cleanupEverySecs,
  onChange,
}: {
  value: number
  options: ExpiryOption[]
  /** How often the server sweeps, for the note about when data is erased. */
  cleanupEverySecs: number
  onChange: (seconds: number) => void
}) {
  const current = expiryOption(value)
  const permanent = value === NO_EXPIRY

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              aria-label={`Expires: ${current.label}`}
              className="shrink-0 gap-1.5 font-normal tabular-nums"
            >
              {/* The infinity mark is the whole label when nothing expires;
                  pairing it with a clock would say the same thing twice and
                  read as a clock set to infinity. */}
              {permanent ? (
                <InfinityIcon className="opacity-60" />
              ) : (
                <>
                  <ClockIcon className="opacity-60" />
                  {current.short}
                </>
              )}
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>
          {permanent ? "No timed deletion" : `Deleted after ${current.label}`}
        </TooltipContent>
      </Tooltip>

      {/* A fixed width, not a minimum: the note at the foot wraps to whatever
          the menu is, and left to size itself it stretches the menu past the
          edge of a phone and clips its own last line. */}
      <DropdownMenuContent align="end" className="w-56">
        {/* Split rather than listed flat: "no expiry" is a different kind of
            answer from a duration, and the rule says so without a heading. */}
        {options
          .filter((o) => o.seconds === NO_EXPIRY)
          .map((option) => (
            <Choice key={option.seconds} option={option} value={value} onChange={onChange} />
          ))}
        <DropdownMenuSeparator />
        {options
          .filter((o) => o.seconds !== NO_EXPIRY)
          .map((option) => (
            <Choice key={option.seconds} option={option} value={value} onChange={onChange} />
          ))}

        {/*
          In the menu rather than the trigger's tooltip, because a tooltip never
          opens on a touch device and this is the caveat most worth reading: the
          link dies on time, but the bytes survive until the next sweep.
        */}
        <DropdownMenuSeparator />
        <p className="px-2 py-1.5 text-xs leading-relaxed text-muted-foreground">
          Links die on time. Erasing the data waits for the next cleanup, up to{" "}
          {formatInterval(cleanupEverySecs)} later.
        </p>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function Choice({
  option,
  value,
  onChange,
}: {
  option: ExpiryOption
  value: number
  onChange: (seconds: number) => void
}) {
  return (
    <DropdownMenuItem onSelect={() => onChange(option.seconds)}>
      {/* Reserved rather than conditional, so the labels do not shift sideways
          as the selection moves down the list. */}
      <CheckIcon className={cn("size-4", option.seconds === value ? "opacity-100" : "opacity-0")} />
      {option.label}
    </DropdownMenuItem>
  )
}

/** "1 hour", "30 minutes" — the sweep interval as the note reads it. */
function formatInterval(seconds: number): string {
  if (seconds < 3600) {
    const minutes = Math.max(Math.round(seconds / 60), 1)
    return `${minutes} minute${minutes === 1 ? "" : "s"}`
  }
  const hours = Math.round(seconds / 3600)
  return `${hours} hour${hours === 1 ? "" : "s"}`
}
