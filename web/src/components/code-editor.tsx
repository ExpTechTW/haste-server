import * as React from "react"

import { ensureHighlighter, highlightSync } from "@/lib/highlighter"
import { cn } from "@/lib/utils"

/**
 * Above this many characters the editor stops colouring as you type.
 *
 * Highlighting runs synchronously on every render, which is what keeps the
 * visible text from lagging a frame behind the caret. Measured end to end —
 * highlighting plus replacing the DOM — a keystroke costs about 14ms at 40k
 * characters, 21ms at 60k and 37ms at 100k, against a 16ms frame. Past that the
 * guarantee turns into the very stutter it exists to prevent.
 *
 * It sits just above the server's default limit, so in a default deployment it
 * never fires; it is here for operators who raise HASTE_MAX_CHARS. The text
 * stays perfectly editable either way, only the colour goes.
 */
const LIVE_HIGHLIGHT_LIMIT = 50_000

/**
 * A textarea that shows its content highlighted while you type.
 *
 * A transparent textarea sits exactly on top of a highlighted copy of the same
 * text. Two things keep that from falling apart:
 *
 *  - Both layers share one set of metrics (`.code-metrics` in index.css). Two
 *    definitions of the same font and padding would eventually drift, and the
 *    caret would stop landing on the character under it.
 *  - Only the wrapper scrolls. The layers are both full height inside it, so
 *    there is no scroll position to synchronise and no scrollbar on one side
 *    narrowing its text and changing where lines wrap.
 *
 * Highlighting is computed synchronously during render: an async repaint would
 * leave the visible text a frame behind the caret, which reads as a glitch. The
 * grammar is loaded up front so only the render itself is on the hot path.
 */
export function CodeEditor({
  value,
  language,
  onChange,
  onKeyDown,
  placeholder,
  invalid,
  className,
}: {
  value: string
  language: string
  onChange: (value: string) => void
  onKeyDown?: React.KeyboardEventHandler<HTMLTextAreaElement>
  placeholder?: string
  invalid?: boolean
  className?: string
}) {
  const [ready, setReady] = React.useState<{
    shiki: Awaited<ReturnType<typeof ensureHighlighter>>
    language: string
  } | null>(null)

  React.useEffect(() => {
    if (value.length > LIVE_HIGHLIGHT_LIMIT) return
    let cancelled = false
    ensureHighlighter(language)
      .then((shiki) => {
        if (!cancelled) setReady({ shiki, language })
      })
      .catch(() => {
        // Leave the plain layer in place; the text stays readable.
      })
    return () => {
      cancelled = true
    }
    // value is read only through the size gate, so this does not re-run per
    // keystroke — only when the paste crosses the threshold.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language, value.length > LIVE_HIGHLIGHT_LIMIT])

  const tooLargeToColour = value.length > LIVE_HIGHLIGHT_LIMIT

  const html = React.useMemo(() => {
    if (tooLargeToColour) return null
    if (ready?.language !== language) return null
    // The trailing newline gives the layer the same final empty line the
    // textarea shows, so the two never differ in height by one row.
    return highlightSync(ready.shiki, value + "\n", language)
  }, [ready, value, language, tooLargeToColour])

  return (
    <div className={cn("scrollbar-slim h-full overflow-auto", className)}>
      <div className="relative min-h-full">
        <div className="editor-layer" aria-hidden="true">
          {html ? (
            // Shiki escapes the source text when it builds this markup.
            <div dangerouslySetInnerHTML={{ __html: html }} />
          ) : (
            // Same metrics, no colour: the text is visible from the first frame
            // rather than appearing once the grammar arrives.
            <pre className="shiki">{value + "\n"}</pre>
          )}
        </div>

        <textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={onKeyDown}
          autoFocus
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          autoComplete="off"
          placeholder={placeholder}
          aria-label="Paste content"
          aria-invalid={invalid}
          className="code-metrics caret-foreground absolute inset-0 h-full w-full resize-none overflow-hidden bg-transparent text-transparent outline-none placeholder:text-muted-foreground/60"
        />
      </div>
    </div>
  )
}
