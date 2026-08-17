import * as React from "react"

import { highlight } from "@/lib/highlighter"

/**
 * Renders highlighted code.
 *
 * The unhighlighted text is painted immediately using the same markup Shiki
 * emits, so the grammar arriving later recolours the view in place instead of
 * replacing a spinner and shifting the layout.
 */
export function CodeView({ code, language }: { code: string; language: string }) {
  const [html, setHtml] = React.useState<string | null>(null)

  React.useEffect(() => {
    let cancelled = false
    setHtml(null)

    highlight(code, language)
      .then((result) => {
        if (!cancelled) setHtml(result)
      })
      .catch(() => {
        // Leave the plain rendering in place: readable beats coloured.
      })

    return () => {
      cancelled = true
    }
  }, [code, language])

  if (html === null) {
    return (
      <pre className="shiki">
        <code>
          {code.split("\n").map((line, i) => (
            // Lines have no identity beyond their position, and the list is
            // replaced wholesale the moment highlighting lands.
            // eslint-disable-next-line react/no-array-index-key
            <span className="line" key={i}>
              {line}
            </span>
          ))}
        </code>
      </pre>
    )
  }

  // Shiki escapes the source text when it builds this markup, so the paste
  // cannot inject nodes here.
  return <div dangerouslySetInnerHTML={{ __html: html }} />
}
