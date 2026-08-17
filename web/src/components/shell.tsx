import * as React from "react"
import { Link } from "react-router-dom"

import { ModeToggle } from "@/components/mode-toggle"
import { cn } from "@/lib/utils"

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
      <svg viewBox="0 0 32 32" className="size-6 shrink-0" aria-hidden="true">
        <path
          d="M9 12 L5.5 16 L9 20 M23 12 L26.5 16 L23 20"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M18.5 9 L13.5 23"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.4"
          strokeLinecap="round"
          className="text-muted-foreground"
        />
      </svg>
      <span className="font-mono text-sm font-semibold tracking-tight">haste</span>
    </Link>
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
        "flex h-13 shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-t px-3 sm:px-4",
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
