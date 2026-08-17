import * as React from "react"
import { useNavigate } from "react-router-dom"
import { FileUpIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { toast } from "sonner"

import { CodeEditor } from "@/components/code-editor"
import { AUTO, LanguagePicker } from "@/components/language-picker"
import { HeaderBar, Kbd, Shell, StatusBar } from "@/components/shell"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useServerConfig } from "@/hooks/use-server-config"
import { ApiError, createPaste } from "@/lib/api"
import { PLAIN, detectLanguage } from "@/lib/languages"
import { cn, countCodePoints, modKey } from "@/lib/utils"

export function EditorPage() {
  const navigate = useNavigate()
  const config = useServerConfig()

  const [content, setContent] = React.useState("")
  const [choice, setChoice] = React.useState(AUTO)
  const [saving, setSaving] = React.useState(false)
  const [dragging, setDragging] = React.useState(false)

  // Escape arms a single tab-out, so the editor swallows Tab for indentation
  // without becoming a keyboard trap.
  const tabExits = React.useRef(false)

  // Detection runs against a lagging copy so a fast typist is never blocked by
  // regex work on every keystroke. Highlighting still uses the live value, so
  // only the language label trails, never the text itself.
  const deferred = React.useDeferredValue(content)
  const detected = React.useMemo(() => detectLanguage(deferred), [deferred])
  const language = choice === AUTO ? detected : choice

  // Code points, matching the runes the server counts.
  const chars = React.useMemo(() => countCodePoints(content), [content])
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
      const isSaveChord =
        (event.metaKey || event.ctrlKey) && (event.key === "s" || event.key === "Enter")
      if (isSaveChord) {
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
    if (countCodePoints(text) > config.maxChars) {
      toast.warning("File is over the limit", {
        description: `Trim it to ${config.maxChars.toLocaleString()} characters before saving.`,
      })
    }
  }

  return (
    <Shell>
      <HeaderBar />

      <main
        className="relative min-h-0 flex-1 bg-surface"
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
        <CodeEditor
          value={content}
          language={language}
          onChange={setContent}
          onKeyDown={onTextareaKeyDown}
          placeholder="Paste your code or logs here…"
          invalid={overLimit}
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
            "shrink-0 font-mono text-xs tabular-nums",
            overLimit ? "font-semibold text-destructive" : "text-muted-foreground",
          )}
          aria-live="polite"
        >
          {chars.toLocaleString()} / {config.maxChars.toLocaleString()}
        </span>


        <div className="ml-auto flex min-w-0 items-center gap-2">
          <LanguagePicker value={choice} detected={detected} onChange={setChoice} />

          <Tooltip>
            <TooltipTrigger asChild>
              {/* span keeps the tooltip reachable while the button is disabled */}
              <span>
                <Button size="sm" className="shrink-0" onClick={() => void save()} disabled={saving || empty || overLimit}>
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
                  Save <Kbd>{modKey()}S</Kbd>
                </span>
              )}
            </TooltipContent>
          </Tooltip>
        </div>
      </StatusBar>
    </Shell>
  )
}

