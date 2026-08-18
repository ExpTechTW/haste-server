import * as React from "react"
import { useLocation, useNavigate, useParams } from "react-router-dom"
import {
  ClockIcon,
  CopyIcon,
  DownloadIcon,
  EllipsisIcon,
  FileTextIcon,
  LinkIcon,
  PlusIcon,
} from "lucide-react"
import { toast } from "sonner"

import { CodeView } from "@/components/code-view"
import { NotFound } from "@/components/not-found"
import { HeaderBar, Kbd, Shell, StatusBar } from "@/components/shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { useServerConfig } from "@/hooks/use-server-config"
import { ApiError, fetchPaste, type Paste } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { describeError } from "@/lib/i18n/errors"
import { countdown, countdownLong } from "@/lib/expiry"
import { formatInterval } from "@/components/expiry-picker"
import { useT } from "@/lib/i18n"
import { PLAIN, languageLabel } from "@/lib/languages"
import { clampRange, formatLineHash, parseLineHash, selectLine } from "@/lib/lines"
import { describeTimestamp, formatTimeOfDay, formatTimestamp } from "@/lib/utils"

export function PastePage() {
  const { code = "" } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const t = useT()

  // A paste that was just created arrives through router state; anything else
  // has to be fetched.
  const handoff = (location.state as { paste?: Paste } | null)?.paste
  const [paste, setPaste] = React.useState<Paste | null>(
    handoff?.key === code ? handoff : null,
  )
  const [error, setError] = React.useState<ApiError | null>(null)

  React.useEffect(() => {
    if (paste?.key === code) return

    let cancelled = false
    setPaste(null)
    setError(null)

    fetchPaste(code)
      .then((result) => {
        if (!cancelled) setPaste(result)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(
          err instanceof ApiError ? err : new ApiError(0, "unknown", "Could not load paste."),
        )
      })

    return () => {
      cancelled = true
    }
  }, [code, paste])

  // The fragment is the whole point of a line selection, so a copied link keeps
  // it: sharing "look at line 42" should not need a second instruction.
  const lineCount = React.useMemo(
    () => (paste?.content ? paste.content.split("\n").length : 0),
    [paste],
  )
  const selection = React.useMemo(
    () => clampRange(parseLineHash(location.hash), lineCount),
    [location.hash, lineCount],
  )

  const onSelectLine = React.useCallback(
    (line: number, extend: boolean) => {
      // Replace rather than push: a walk down a file should not bury the page
      // the reader arrived from under fifty history entries.
      navigate({ hash: formatLineHash(selectLine(selection, line, extend)) }, { replace: true })
    },
    [navigate, selection],
  )

  const copyLink = React.useCallback(async () => {
    const base = paste?.url ?? window.location.href.split("#")[0]
    const ok = await copyText(selection ? base + formatLineHash(selection) : base)
    if (ok) toast.success(t(selection ? "paste.linesCopied" : "paste.linkCopied"))
    else toast.error(t("paste.linkCopyFailed"))
  }, [paste, selection, t])

  const copyContent = React.useCallback(async () => {
    if (!paste?.content) return
    const ok = await copyText(paste.content)
    if (ok) toast.success(t("paste.contentCopied"))
    else toast.error(t("paste.contentCopyFailed"))
  }, [paste, t])

  // Bare keys rather than chorded shortcuts: nothing on this page takes text
  // input, so there is nothing to collide with.
  React.useEffect(() => {
    // NotFound brings its own shortcuts; leaving these bound as well would run
    // both handlers for the same keypress.
    if (error) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return
      const target = event.target as HTMLElement | null
      if (target?.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target?.tagName ?? "")) {
        return
      }

      switch (event.key.toLowerCase()) {
        case "n":
          navigate("/")
          break
        case "c":
          void copyLink()
          break
        case "r":
          if (paste) window.location.href = paste.rawUrl
          break
        case "s":
          // Content-Disposition makes this download rather than navigate.
          if (paste) window.location.href = paste.downloadUrl
          break
        default:
          return
      }
      event.preventDefault()
    }

    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [copyLink, error, navigate, paste])

  if (error) {
    return (
      <NotFound
        code={code}
        message={describeError(t, error, "nf.loadFailed")}
        missing={error.status === 404}
      />
    )
  }

  return (
    <Shell>
      <HeaderBar>
        <IconAction label={t("paste.copyLink")} shortcut="C" onClick={() => void copyLink()}>
          <LinkIcon />
        </IconAction>

        {/*
          Sharing the link is the one action worth a permanent slot. The rest
          stay inline where there is room and collapse into a menu on a phone,
          whose header also has to hold the brand, the GitHub link and the
          theme toggle. `contents` lets them rejoin the row without a wrapper
          box of their own.
        */}
        <span className="hidden sm:contents">
          <IconAction
            label={t("paste.copyContent")}
            onClick={() => void copyContent()}
            disabled={!paste?.content}
          >
            <CopyIcon />
          </IconAction>
          <IconAction label={t("paste.viewRaw")} shortcut="R" asLink href={paste?.rawUrl}>
            <FileTextIcon />
          </IconAction>
          <IconAction
            label={
              paste ? t("paste.downloadNamed", { name: paste.filename }) : t("paste.download")
            }
            shortcut="S"
            asLink
            href={paste?.downloadUrl}
          >
            <DownloadIcon />
          </IconAction>
          <IconAction label={t("paste.new")} shortcut="N" onClick={() => navigate("/")}>
            <PlusIcon />
          </IconAction>
        </span>

        <MoreActions
          paste={paste}
          onCopyContent={() => void copyContent()}
          onNew={() => navigate("/")}
        />
      </HeaderBar>

      <main className="scrollbar-slim min-h-0 flex-1 overflow-auto bg-surface py-4">
        {paste ? (
          <CodeView
            code={paste.content ?? ""}
            language={paste.language || PLAIN}
            selection={selection}
            onSelectLine={onSelectLine}
          />
        ) : (
          <LoadingLines />
        )}
      </main>

      <StatusBar>
        {paste ? (
          <>
            <Badge variant="secondary" className="font-mono">
              {paste.key}
            </Badge>
            {/* Ellipsised, not wrapped: a two-word label like "Objective-C"
                would otherwise break across lines and grow the whole bar. */}
            <span className="min-w-0 truncate text-xs text-muted-foreground">
              {languageLabel(paste.language || PLAIN)}
            </span>
            <span className="hidden text-xs text-muted-foreground tabular-nums sm:inline">
              {t("paste.chars", { count: paste.chars.toLocaleString() })}
            </span>
            {/*
              When it was saved is a fact about the paste, unlike a lifetime,
              which the store cannot promise. Shown in a fixed zone so everyone
              reading the same link reads the same instant; the title carries
              the UTC equivalent for anyone who is not in it.
            */}
            <div className="ml-auto flex shrink-0 items-center gap-2 sm:gap-3">
              {/* Only shown when a lifetime was actually chosen. Its absence is
                  not a promise of permanence, so there is nothing to say. */}
              {paste.expiresAt && <Expiry at={paste.expiresAt} />}

              <time
                className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums"
                dateTime={paste.createdAt}
                title={`${t("paste.savedAt")} · ${describeTimestamp(paste.createdAt) ?? ""}`}
              >
                <span className="sm:hidden">{formatTimeOfDay(paste.createdAt)}</span>
                <span className="hidden sm:inline">{formatTimestamp(paste.createdAt)}</span>
              </time>
            </div>
          </>
        ) : (
          <span className="text-xs text-muted-foreground">{t("paste.loading")}</span>
        )}
      </StatusBar>
    </Shell>
  )
}

/** The secondary actions, folded into one button where width is scarce. */
function MoreActions({
  paste,
  onCopyContent,
  onNew,
}: {
  paste: Paste | null
  onCopyContent: () => void
  onNew: () => void
}) {
  const t = useT()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={t("paste.more")} className="sm:hidden">
          <EllipsisIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        <DropdownMenuItem onSelect={onCopyContent} disabled={!paste?.content}>
          <CopyIcon />
          {t("paste.copyContent")}
        </DropdownMenuItem>
        {paste && (
          <>
            {/* Full navigations, not client routes: both are served by the backend. */}
            <DropdownMenuItem asChild>
              <a href={paste.rawUrl}>
                <FileTextIcon />
                {t("paste.viewRaw")}
              </a>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <a href={paste.downloadUrl}>
                <DownloadIcon />
                {t("paste.download")}
              </a>
            </DropdownMenuItem>
          </>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onNew}>
          <PlusIcon />
          {t("paste.new")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function IconAction({
  label,
  shortcut,
  children,
  onClick,
  disabled,
  asLink,
  href,
}: {
  label: string
  shortcut?: string
  children: React.ReactNode
  onClick?: () => void
  disabled?: boolean
  asLink?: boolean
  href?: string
}) {
  const button =
    asLink && href ? (
      // A full navigation, not a client route: /raw is served by the backend.
      <Button variant="ghost" size="icon-sm" asChild aria-label={label}>
        <a href={href}>{children}</a>
      </Button>
    ) : (
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onClick}
        disabled={disabled || (asLink && !href)}
        aria-label={label}
      >
        {children}
      </Button>
    )

  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>
        <span className="flex items-center gap-1.5">
          {label}
          {shortcut && <Kbd>{shortcut}</Kbd>}
        </span>
      </TooltipContent>
    </Tooltip>
  )
}

/** Placeholder rows sized like real code, so nothing jumps once content lands. */
function LoadingLines() {
  return (
    <div className="space-y-2 px-6" aria-hidden="true">
      {[68, 42, 84, 30, 56, 74, 38].map((width, i) => (
        // eslint-disable-next-line react/no-array-index-key
        <div key={i} className="h-3 animate-pulse rounded bg-muted" style={{ width: `${width}%` }} />
      ))}
    </div>
  )
}
/**
 * How long this paste has left, and exactly when it goes.
 *
 * The countdown is what a reader wants at a glance, so it is the part that sits
 * in the bar; the exact instant is a click away rather than a hover, because a
 * hover is not available on a phone and the instant is the half you need when
 * you are planning around it.
 *
 * It ticks every second. A month-long paste only redraws when the hour changes
 * — the string is the same until then, and React skips an identical state.
 */
function Expiry({ at }: { at: string }) {
  const t = useT()
  const config = useServerConfig()
  const now = useCountdownTick(at)

  const left = countdown(t, at, now)
  const long = countdownLong(t, at, now)

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 shrink-0 gap-1.5 px-2 font-mono text-xs font-normal tabular-nums text-muted-foreground"
        >
          <ClockIcon className="size-3.5 shrink-0 opacity-70" />
          {left ?? t("paste.expired")}
        </Button>
      </PopoverTrigger>

      <PopoverContent align="end" className="w-64 space-y-3 text-sm">
        <div className="space-y-1">
          <p className="text-xs font-medium text-muted-foreground">{t("paste.expiresAt")}</p>
          {/* The absolute instant, in the one zone everyone reading this link
              sees the same. */}
          <time className="block font-mono tabular-nums" dateTime={at}>
            {formatTimestamp(at)}
          </time>
          <p className="text-xs text-muted-foreground">
            {long ? t("paste.left", { value: long }) : t("paste.expired")}
          </p>
        </div>

        <div className="space-y-1 text-xs leading-relaxed text-muted-foreground">
          <p>{t("paste.expiryDetail", { interval: formatInterval(t, config.cleanupEverySecs) })}</p>
          <p>{t("paste.timeZone")}</p>
        </div>
      </PopoverContent>
    </Popover>
  )
}

/**
 * The current time, re-read as often as the countdown actually changes.
 *
 * Seconds only appear in the last hour of a paste's life; above that the finest
 * thing on screen is minutes. Ticking every second for a month-long paste would
 * be 86,400 renders a day that each produce identical markup, so the rate
 * follows the resolution — and the effect only re-runs on the one crossing.
 */
function useCountdownTick(at: string): number {
  const [now, setNow] = React.useState(() => Date.now())
  const showingSeconds = new Date(at).getTime() - now < 3_600_000

  React.useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), showingSeconds ? 1_000 : 30_000)
    return () => clearInterval(id)
  }, [showingSeconds])

  return now
}
