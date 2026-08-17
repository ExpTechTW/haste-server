import * as React from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { FileUpIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { toast } from "sonner"

import { HeaderBar, Kbd, Shell, StatusBar } from "@/components/shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useServerConfig } from "@/hooks/use-server-config"
import { ApiError, createPaste } from "@/lib/api"
import { prewarm } from "@/lib/highlighter"
import { LANGUAGES, PLAIN, detectLanguage, languageLabel } from "@/lib/languages"
import { cn } from "@/lib/utils"

const AUTO = "auto"

/** Text handed over from a paste being duplicated. */
interface EditorState {
  content?: string
  language?: string
}

export function EditorPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const config = useServerConfig()

  const handoff = location.state as EditorState | null
  const [content, setContent] = React.useState(handoff?.content ?? "")
  const [choice, setChoice] = React.useState(handoff?.language ?? AUTO)
  const [saving, setSaving] = React.useState(false)
  const [dragging, setDragging] = React.useState(false)

  const textareaRef = React.useRef<HTMLTextAreaElement>(null)
  // Escape arms a single tab-out, so the editor swallows Tab for indentation
  // without becoming a keyboard trap.
  const tabExits = React.useRef(false)

  // Detection runs against a lagging copy so a fast typist is never blocked by
  // regex work on every keystroke.
  const deferred = React.useDeferredValue(content)
  const detected = React.useMemo(() => detectLanguage(deferred), [deferred])
  const language = choice === AUTO ? detected : choice

  React.useEffect(() => prewarm(language), [language])

  // Code points, matching the runes the server counts.
  const chars = React.useMemo(() => Array.from(content).length, [content])
  const overLimit = chars > config.maxChars
  const empty = content.trim().length === 0

  const save = React.useCallback(async () => {
    if (saving || empty || overLimit) return

    setSaving(true)
    try {
      const paste = await createPaste(content, language === PLAIN ? "" : language)
      // Hand the created paste forward so the next view renders without a
      // second round trip.
      navigate(`/${paste.key}`, { state: { paste } })
    } catch (error) {
      const message =
        error instanceof ApiError ? error.message : "Something went wrong while saving."
      toast.error("Could not save paste", { description: message })
      setSaving(false)
    }
  }, [content, empty, language, navigate, overLimit, saving])

  React.useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const save2 = (event.metaKey || event.ctrlKey) && (event.key === "s" || event.key === "Enter")
      if (save2) {
        event.preventDefault()
        void save()
      }
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [save])

  const onTextareaKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Escape") {
      tabExits.current = true
      return
    }
    if (event.key !== "Tab" || tabExits.current) {
      tabExits.current = false
      return
    }

    event.preventDefault()
    const el = event.currentTarget
    const { selectionStart, selectionEnd } = el
    const next = content.slice(0, selectionStart) + "  " + content.slice(selectionEnd)
    setContent(next)
    // Restore the caret after React commits the new value.
    requestAnimationFrame(() => {
      el.selectionStart = el.selectionEnd = selectionStart + 2
    })
  }

  const readFile = async (file: File) => {
    const text = await file.text()
    setContent(text)
    setChoice(AUTO)
    if (Array.from(text).length > config.maxChars) {
      toast.warning("File is over the limit", {
        description: `Trim it to ${config.maxChars.toLocaleString()} characters before saving.`,
      })
    }
  }

  return (
    <Shell>
      <HeaderBar>
        <Badge variant="secondary" className="mr-1 hidden font-mono sm:inline-flex">
          zstd-{config.zstdLevel}
        </Badge>
      </HeaderBar>

      <main
        className="relative min-h-0 flex-1"
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault()
          setDragging(false)
          const file = e.dataTransfer.files?.[0]
          if (file) void readFile(file)
        }}
      >
        <textarea
          ref={textareaRef}
          value={content}
          onChange={(e) => setContent(e.target.value)}
          onKeyDown={onTextareaKeyDown}
          autoFocus
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          placeholder="Paste your code or logs here…"
          aria-label="Paste content"
          aria-invalid={overLimit}
          className="scrollbar-slim h-full w-full resize-none bg-surface px-4 py-4 font-mono text-[13px] leading-[1.65] outline-none placeholder:text-muted-foreground/60 sm:px-6"
        />

        {dragging && (
          <div className="pointer-events-none absolute inset-3 flex items-center justify-center rounded-lg border-2 border-dashed border-ring bg-background/85 backdrop-blur-[1px]">
            <div className="flex items-center gap-2 text-sm font-medium">
              <FileUpIcon className="size-4" />
              Drop a file to load it
            </div>
          </div>
        )}
      </main>

      <StatusBar>
        <span
          className={cn(
            "font-mono text-xs tabular-nums",
            overLimit ? "font-semibold text-destructive" : "text-muted-foreground",
          )}
          aria-live="polite"
        >
          {chars.toLocaleString()} / {config.maxChars.toLocaleString()}
        </span>

        {config.expires && (
          <span className="hidden text-xs text-muted-foreground sm:inline">
            kept {formatRetention(config.retentionDays)}
          </span>
        )}

        <div className="ml-auto flex items-center gap-2">
          <Select value={choice} onValueChange={setChoice}>
            <SelectTrigger size="sm" className="w-[9.5rem]" aria-label="Syntax highlighting">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={AUTO}>
                Auto{detected !== PLAIN && ` · ${languageLabel(detected)}`}
              </SelectItem>
              {LANGUAGES.map((lang) => (
                <SelectItem key={lang.id} value={lang.id}>
                  {lang.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Tooltip>
            <TooltipTrigger asChild>
              {/* span keeps the tooltip reachable while the button is disabled */}
              <span>
                <Button size="sm" onClick={() => void save()} disabled={saving || empty || overLimit}>
                  {saving ? (
                    <LoaderCircleIcon className="animate-spin" />
                  ) : (
                    <SaveIcon />
                  )}
                  Save
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {overLimit ? (
                `Over the ${config.maxChars.toLocaleString()} character limit`
              ) : (
                <span className="flex items-center gap-1.5">
                  Save <Kbd>⌘S</Kbd>
                </span>
              )}
            </TooltipContent>
          </Tooltip>
        </div>
      </StatusBar>
    </Shell>
  )
}

function formatRetention(days: number): string {
  if (days >= 1) {
    const whole = Math.round(days)
    return `${whole} day${whole === 1 ? "" : "s"}`
  }
  const hours = Math.max(1, Math.round(days * 24))
  return `${hours} hour${hours === 1 ? "" : "s"}`
}
