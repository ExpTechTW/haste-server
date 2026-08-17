import * as React from "react"

import { ensureHighlighter, highlightSync } from "@/lib/highlighter"
import { cn } from "@/lib/utils"

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
  }, [language])

  const html = React.useMemo(() => {
    if (ready?.language !== language) return null
    // The trailing newline gives the layer the same final empty line the
    // textarea shows, so the two never differ in height by one row.
    return highlightSync(ready.shiki, value + "\n", language)
  }, [ready, value, language])

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
