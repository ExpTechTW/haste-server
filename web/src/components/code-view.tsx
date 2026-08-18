import * as React from "react"

import { highlight } from "@/lib/highlighter"
import { useT } from "@/lib/i18n"
import type { LineRange } from "@/lib/lines"

/**
 * Renders highlighted, line-addressable code.
 *
 * The unhighlighted text is painted immediately using the same markup Shiki
 * emits, so the grammar arriving later recolours the view in place instead of
 * replacing a spinner and shifting the layout. Both renderings carry the line
 * ids, so `#L17-L25` works before the grammar has loaded.
 */
export function CodeView({
  code,
  language,
  selection,
  onSelectLine,
}: {
  code: string
  language: string
  selection: LineRange | null
  /** extend is true when the click should grow the current range. */
  onSelectLine: (line: number, extend: boolean) => void
}) {
  const t = useT()
  const [html, setHtml] = React.useState<string | null>(null)
  const containerRef = React.useRef<HTMLDivElement>(null)
  const hasScrolled = React.useRef(false)

  React.useEffect(() => {
    let cancelled = false
    setHtml(null)
    hasScrolled.current = false

    highlight(code, language, { addressable: (n) => t("paste.line", { n }) })
      .then((result) => {
        if (!cancelled) setHtml(result)
      })
      .catch(() => {
        // Leave the plain rendering in place: readable beats coloured.
      })

    return () => {
      cancelled = true
    }
    // t is stable per locale, so this re-highlights on a language switch and
    // never per render — which it has to, because the gutter's accessible
    // names are baked into the markup.
  }, [code, language, t])

  // Toggled as an attribute rather than baked into the markup, so changing the
  // selection never costs a re-highlight of the whole document.
  React.useEffect(() => {
    const root = containerRef.current
    if (!root) return

    for (const el of root.querySelectorAll<HTMLElement>(".line[data-highlighted]")) {
      delete el.dataset.highlighted
    }
    if (!selection) return

    for (let line = selection.start; line <= selection.end; line++) {
      const el = root.querySelector<HTMLElement>(`.line[data-line="${line}"]`)
      if (el) el.dataset.highlighted = ""
    }
  }, [selection, html])

  // Only on arrival: re-scrolling on every selection change would fight the
  // user as they shift-click their way down a range.
  React.useEffect(() => {
    if (!selection || hasScrolled.current) return
    const el = containerRef.current?.querySelector(`.line[data-line="${selection.start}"]`)
    if (!el) return
    hasScrolled.current = true
    el.scrollIntoView({ block: "center" })
  }, [selection, html])

  const onClick = (event: React.MouseEvent<HTMLDivElement>) => {
    const link = (event.target as HTMLElement).closest<HTMLElement>(".line-link")
    if (!link) return

    const line = Number(link.closest<HTMLElement>(".line")?.dataset.line)
    if (!Number.isInteger(line) || line < 1) return

    // The href stays a real link for middle-click and copy-link-address; only
    // the plain left click is taken over so shift can extend the range.
    event.preventDefault()
    onSelectLine(line, event.shiftKey)
  }

  return (
    <div ref={containerRef} onClick={onClick}>
      {html === null ? (
        <pre className="shiki">
          <code>
            {code.split("\n").map((line, index) => (
              // Lines have no identity beyond their position, and the list is
              // replaced wholesale the moment highlighting lands.
              // eslint-disable-next-line react/no-array-index-key
              <span className="line" data-line={index + 1} key={index}>
                <a
                  className="line-link"
                  href={`#L${index + 1}`}
                  aria-label={t("paste.line", { n: index + 1 })}
                />
                {line}
              </span>
            ))}
          </code>
        </pre>
      ) : (
        // Shiki escapes the source text when it builds this markup, so the
        // paste cannot inject nodes here.
        <div dangerouslySetInnerHTML={{ __html: html }} />
      )}
    </div>
  )
}
