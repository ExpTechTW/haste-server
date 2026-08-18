import * as React from "react"
import { useNavigate } from "react-router-dom"
import {
  CornerDownLeftIcon,
  FileUpIcon,
  KeyboardIcon,
  LoaderCircleIcon,
  SaveIcon,
  ScrollTextIcon,
} from "lucide-react"
import { toast } from "sonner"

import { CodeEditor } from "@/components/code-editor"
import { ExpiryPicker } from "@/components/expiry-picker"
import { AUTO, LanguagePicker } from "@/components/language-picker"
import { HeaderBar, Kbd, Shell, StatusBar } from "@/components/shell"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useServerConfig } from "@/hooks/use-server-config"
import { createPaste } from "@/lib/api"
import { NO_EXPIRY, expiryOptions } from "@/lib/expiry"
import { useT, type Translate } from "@/lib/i18n"
import { describeError } from "@/lib/i18n/errors"
import { PLAIN, detectLanguage } from "@/lib/languages"
import { cn, countCodePoints, modKey } from "@/lib/utils"

export function EditorPage() {
  const navigate = useNavigate()
  const config = useServerConfig()
  const t = useT()

  const [content, setContent] = React.useState("")
  const [choice, setChoice] = React.useState(AUTO)
  const [expiresIn, setExpiresIn] = React.useState(NO_EXPIRY)
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

  const expiries = React.useMemo(
    () => expiryOptions(t, config.expiryOptionsSecs),
    [t, config.expiryOptionsSecs],
  )

  // The ladder arrives from the server, so a value chosen against the fallback
  // list has to be dropped if the real one does not offer it — otherwise Save
  // sends a lifetime the API refuses.
  React.useEffect(() => {
    if (expiresIn !== NO_EXPIRY && !config.expiryOptionsSecs.includes(expiresIn)) {
      setExpiresIn(NO_EXPIRY)
    }
  }, [config.expiryOptionsSecs, expiresIn])

  // Code points, matching the runes the server counts.
  const chars = React.useMemo(() => countCodePoints(content), [content])
  const overLimit = chars > config.maxChars
  const empty = content.trim().length === 0

  const save = React.useCallback(async () => {
    if (saving || empty || overLimit) return

    setSaving(true)
    try {
      const paste = await createPaste(content, language === PLAIN ? "" : language, expiresIn)
      // Saying nothing here would let "no expiry" be read as "kept forever",
      // which is the one thing the store cannot promise. Said once, at the
      // moment the choice takes effect, rather than as standing small print.
      if (expiresIn === NO_EXPIRY) {
        toast.info(t("editor.noExpiry"), { description: t("editor.noExpiryBody") })
      }
      // Hand the created paste forward so the next view renders without a
      // second round trip.
      navigate(`/${paste.key}`, { state: { paste } })
    } catch (error) {
      toast.error(t("editor.saveFailed"), {
        description: describeError(t, error, "editor.saveFailedBody"),
      })
      setSaving(false)
    }
  }, [content, empty, expiresIn, language, navigate, overLimit, saving, t])

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
      toast.warning(t("editor.fileTooBig"), {
        description: t("editor.fileTooBigBody", { max: config.maxChars.toLocaleString() }),
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
          invalid={overLimit}
        />

        {empty && !dragging && <Welcome t={t} maxChars={config.maxChars} />}

        {dragging && (
          <div className="pointer-events-none absolute inset-3 flex items-center justify-center rounded-lg border-2 border-dashed border-ring bg-background/85 backdrop-blur-[1px]">
            <div className="flex items-center gap-2 text-sm font-medium">
              <FileUpIcon className="size-4" />
              {t("editor.dropzone")}
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
          <ExpiryPicker
            value={expiresIn}
            options={expiries}
            cleanupEverySecs={config.cleanupEverySecs}
            onChange={setExpiresIn}
          />
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
                  {t("editor.save")}
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {overLimit ? (
                t("editor.overLimit", { max: config.maxChars.toLocaleString() })
              ) : (
                <span className="flex items-center gap-1.5">
                  {t("editor.save")} <Kbd>{modKey()}S</Kbd>
                </span>
              )}
            </TooltipContent>
          </Tooltip>
        </div>
      </StatusBar>
    </Shell>
  )
}

/**
 * What an empty editor says for itself.
 *
 * It replaces the textarea's placeholder rather than sitting alongside it: one
 * piece of guidance, centred where the eye lands, instead of a grey line in the
 * corner. Nothing here takes pointer events, so a click anywhere still puts the
 * caret in the editor underneath — the point of the page is that you can start
 * typing without deciding anything first.
 */
function Welcome({ t, maxChars }: { t: Translate; maxChars: number }) {
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 flex select-none items-center justify-center overflow-hidden px-6"
    >
      {/* A single soft light behind the wordmark. Sized in viewport units so it
          stays a wash rather than becoming a visible disc on a wide screen. */}
      <div className="absolute size-[60vmin] rounded-full bg-foreground/[0.045] blur-3xl" />

      <div className="relative flex flex-col items-center gap-6 text-center">
        <div className="space-y-2.5">
          <h1 className="font-mono text-4xl font-semibold tracking-tight sm:text-5xl">haste</h1>
          <p className="text-sm text-muted-foreground sm:text-base">{t("editor.tagline")}</p>
        </div>

        {/* Dropped on a short viewport before the wordmark is: the hints are
            the part you can do without once you have seen them. */}
        <div className="hidden flex-wrap items-center justify-center gap-2 min-[400px]:flex">
          <Hint icon={<KeyboardIcon />}>{t("editor.hint.type")}</Hint>
          <Hint icon={<FileUpIcon />}>{t("editor.hint.drop")}</Hint>
          <Hint icon={<CornerDownLeftIcon />}>
            <span className="flex items-center gap-1.5">
              <Kbd>{modKey()}S</Kbd> {t("editor.hint.save")}
            </span>
          </Hint>
        </div>

        <p className="flex items-center gap-1.5 text-xs text-muted-foreground/80">
          <ScrollTextIcon className="size-3.5" />
          {t("editor.hint.limit", { max: maxChars.toLocaleString() })}
        </p>
      </div>
    </div>
  )
}

function Hint({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <span className="flex items-center gap-2 rounded-full border bg-background/60 px-3 py-1.5 text-xs text-muted-foreground [&>svg]:size-3.5 [&>svg]:opacity-70">
      {icon}
      {children}
    </span>
  )
}
