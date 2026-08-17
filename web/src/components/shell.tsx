import * as React from "react"
import { Link } from "react-router-dom"

import { GitHubIcon } from "@/components/icons"
import { ModeToggle } from "@/components/mode-toggle"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

const ORGANISATION = "ExpTech Studio"
const PRODUCT = "haste"
const REPOSITORY = "https://github.com/ExpTechTW"

/** Full-viewport column: fixed header, flexible body, fixed status bar. */
export function Shell({ children }: { children: React.ReactNode }) {
  return <div className="flex h-full min-h-0 flex-col">{children}</div>
}

export function HeaderBar({ children }: { children?: React.ReactNode }) {
  return (
    <header className="flex h-13 shrink-0 items-center gap-3 border-b px-3 sm:px-4">
      <Brand />
      <div className="ml-auto flex items-center gap-1">
        {children}
        <GitHubLink />
        <ModeToggle />
      </div>
    </header>
  )
}

// The visible wordmark is the accessible name here; an aria-label would
// override it and collide with the New paste action beside it.
function Brand() {
  return (
    <Link
      to="/"
      className="group flex items-center gap-2 rounded-md outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
    >
      {/*
        The logo is an opaque JPEG, so it is framed as a rounded tile the way an
        organisation avatar is: a bare square would read as a broken transparency
        against the light theme. The name beside it is the accessible label, so
        the image itself stays decorative.
      */}
      <img
        src="/ExpTech.jpg"
        alt=""
        aria-hidden="true"
        width={28}
        height={28}
        className="size-7 shrink-0 rounded-md object-cover"
      />
      {/* Stacked rather than inline: two short lines cost far less width than
          one long one, which is what leaves room for the actions on a phone. */}
      <span className="flex flex-col leading-tight">
        <span className="text-[13px] font-semibold tracking-tight sm:text-sm">
          {ORGANISATION}
        </span>
        <span className="font-mono text-[11px] text-muted-foreground">{PRODUCT}</span>
      </span>
    </Link>
  )
}

function GitHubLink() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          asChild
          aria-label={`${ORGANISATION} on GitHub`}
        >
          <a href={REPOSITORY} target="_blank" rel="noreferrer">
            <GitHubIcon className="size-4" />
          </a>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{ORGANISATION} on GitHub</TooltipContent>
    </Tooltip>
  )
}

/** The bottom bar shared by both views. */
export function StatusBar({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  return (
    <footer
      className={cn(
        // No wrapping: a flex container wraps before it shrinks, so allowing it
        // would drop the language picker and Save onto a second row the moment
        // the character counter grew. Nowrap lets the picker absorb the space
        // instead, down to its own minimum width. min-height rather than a
        // fixed one so nothing can be clipped if a row ever does grow.
        "flex min-h-13 shrink-0 items-center gap-3 border-t px-3 py-2 sm:px-4",
        className,
      )}
    >
      {children}
    </footer>
  )
}

/** Renders a shortcut hint as a keycap. */
export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="pointer-events-none inline-flex h-5 items-center gap-0.5 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground">
      {children}
    </kbd>
  )
}
