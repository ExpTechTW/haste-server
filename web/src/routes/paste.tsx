import * as React from "react"
import { useLocation, useNavigate, useParams } from "react-router-dom"
import {
  CopyIcon,
  DownloadIcon,
  EllipsisIcon,
  FileTextIcon,
  LinkIcon,
  PlusIcon,
} from "lucide-react"
import { toast } from "sonner"

import { CodeView } from "@/components/code-view"
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
import { ApiError, fetchPaste, type Paste } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { PLAIN, languageLabel } from "@/lib/languages"
import { clampRange, formatLineHash, parseLineHash, selectLine } from "@/lib/lines"

export function PastePage() {
  const { code = "" } = useParams()
  const location = useLocation()
  const navigate = useNavigate()

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
    if (ok) toast.success(selection ? "Link to lines copied" : "Link copied")
    else toast.error("Could not copy link")
  }, [paste, selection])

  const copyContent = React.useCallback(async () => {
    if (!paste?.content) return
    const ok = await copyText(paste.content)
    if (ok) toast.success("Content copied")
    else toast.error("Could not copy content")
  }, [paste])

  // Bare keys rather than chorded shortcuts: nothing on this page takes text
  // input, so there is nothing to collide with.
  React.useEffect(() => {
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
  }, [copyLink, navigate, paste])

  if (error) {
    return <NotFound code={code} message={error.message} />
  }

  return (
    <Shell>
      <HeaderBar>
        <IconAction label="Copy link" shortcut="C" onClick={() => void copyLink()}>
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
            label="Copy content"
            onClick={() => void copyContent()}
            disabled={!paste?.content}
          >
            <CopyIcon />
          </IconAction>
          <IconAction label="View raw" shortcut="R" asLink href={paste?.rawUrl}>
            <FileTextIcon />
          </IconAction>
          <IconAction
            label={paste ? `Download ${paste.filename}` : "Download"}
            shortcut="S"
            asLink
            href={paste?.downloadUrl}
          >
            <DownloadIcon />
          </IconAction>
          <IconAction label="New paste" shortcut="N" onClick={() => navigate("/")}>
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
            <span className="text-xs text-muted-foreground">
              {languageLabel(paste.language || PLAIN)}
            </span>
            <span className="text-xs text-muted-foreground tabular-nums">
              {paste.chars.toLocaleString()} chars
            </span>
          </>
        ) : (
          <span className="text-xs text-muted-foreground">Loading…</span>
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
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="More actions" className="sm:hidden">
          <EllipsisIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        <DropdownMenuItem onSelect={onCopyContent} disabled={!paste?.content}>
          <CopyIcon />
          Copy content
        </DropdownMenuItem>
        {paste && (
          <>
            {/* Full navigations, not client routes: both are served by the backend. */}
            <DropdownMenuItem asChild>
              <a href={paste.rawUrl}>
                <FileTextIcon />
                View raw
              </a>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <a href={paste.downloadUrl}>
                <DownloadIcon />
                Download
              </a>
            </DropdownMenuItem>
          </>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onNew}>
          <PlusIcon />
          New paste
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

function NotFound({ code, message }: { code: string; message: string }) {
  const navigate = useNavigate()

  return (
    <Shell>
      <HeaderBar />
      <main className="flex min-h-0 flex-1 items-center justify-center px-6">
        <div className="flex max-w-sm flex-col items-center gap-4 text-center">
          <Badge variant="secondary" className="font-mono">
            /{code}
          </Badge>
          <div className="space-y-1.5">
            <h1 className="text-lg font-semibold">Nothing here</h1>
            <p className="text-sm text-muted-foreground">{message}</p>
          </div>
          <Button onClick={() => navigate("/")}>
            <PlusIcon />
            New paste
            <Kbd>N</Kbd>
          </Button>
        </div>
      </main>
    </Shell>
  )
}
