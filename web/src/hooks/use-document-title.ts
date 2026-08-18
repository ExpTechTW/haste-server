import * as React from "react"

/** The product name, as it is written. */
export const PRODUCT = "Haste"

/**
 * Keeps the browser tab describing the page you are actually on.
 *
 * The server writes a title into the shell for whatever unfurls the link, but
 * that is the document as first delivered; moving between routes in the client
 * would otherwise leave the tab naming the page you arrived at an hour ago.
 *
 * Pass null while the page has nothing specific to say — during a load, say —
 * and the product name stands alone rather than flickering through a
 * placeholder.
 */
export function useDocumentTitle(title: string | null): void {
  React.useEffect(() => {
    document.title = title ? `${title} · ${PRODUCT}` : PRODUCT
  }, [title])
}
