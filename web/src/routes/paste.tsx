import * as React from "react"
import { useLocation, useNavigate, useParams } from "react-router-dom"
import { CopyIcon, FileTextIcon, LinkIcon, PencilIcon, PlusIcon } from "lucide-react"
import { toast } from "sonner"

import { CodeView } from "@/components/code-view"
import { HeaderBar, Kbd, Shell, StatusBar } from "@/components/shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { ApiError, fetchPaste, type Paste } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { PLAIN, languageLabel } from "@/lib/languages"
import { formatBytes, formatExpiry } from "@/lib/utils"

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

  const copyLink = React.useCallback(async () => {
    const ok = await copyText(paste?.url ?? window.location.href)
    if (ok) toast.success("Link copied")
    else toast.error("Could not copy link")
  }, [paste])

  const copyContent = React.useCallback(async () => {
    if (!paste?.content) return
    const ok = await copyText(paste.content)
    if (ok) toast.success("Content copied")
    else toast.error("Could not copy content")
  }, [paste])

  const duplicate = React.useCallback(() => {
    if (!paste?.content) return
    navigate("/", { state: { content: paste.content, language: paste.language || PLAIN } })
  }, [navigate, paste])

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
        case "d":
          duplicate()
          break
        case "c":
          void copyLink()
          break
        case "r":
          if (paste) window.location.href = paste.rawUrl
          break
        default:
          return
      }
      event.preventDefault()
    }

    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [copyLink, duplicate, navigate, paste])

  if (error) {
    return <NotFound code={code} message={error.message} />
  }

  const expiry = paste ? formatExpiry(paste.expiresAt) : null

  return (
    <Shell>
      <HeaderBar>
        <IconAction label="Copy link" shortcut="C" onClick={() => void copyLink()}>
          <LinkIcon />
        </IconAction>
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
        <IconAction label="Duplicate & edit" shortcut="D" onClick={duplicate} disabled={!paste}>
          <PencilIcon />
        </IconAction>
        <IconAction label="New paste" shortcut="N" onClick={() => navigate("/")}>
          <PlusIcon />
        </IconAction>
      </HeaderBar>

      <main className="scrollbar-slim min-h-0 flex-1 overflow-auto bg-surface py-4">
        {paste ? (
          <CodeView code={paste.content ?? ""} language={paste.language || PLAIN} />
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
            <span className="hidden text-xs text-muted-foreground tabular-nums sm:inline">
              {paste.chars.toLocaleString()} chars
            </span>
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="success" className="font-mono tabular-nums">
                  {paste.ratio.toFixed(1)}×
                </Badge>
              </TooltipTrigger>
              <TooltipContent>
                {formatBytes(paste.bytes)} stored as {formatBytes(paste.stored)}
              </TooltipContent>
            </Tooltip>
            {expiry && (
              <span className="ml-auto text-xs text-muted-foreground">{expiry}</span>
            )}
          </>
        ) : (
          <span className="text-xs text-muted-foreground">Loading…</span>
        )}
      </StatusBar>
    </Shell>
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
