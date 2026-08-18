import * as React from "react"

/**
 * Whether a media query currently matches, kept in step as the viewport changes.
 *
 * For the handful of decisions a CSS class cannot make — where a portalled
 * overlay is anchored, say — rather than as a substitute for responsive
 * styling, which belongs in the markup.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = React.useCallback(
    (onChange: () => void) => {
      const list = window.matchMedia(query)
      list.addEventListener("change", onChange)
      return () => list.removeEventListener("change", onChange)
    },
    [query],
  )

  return React.useSyncExternalStore(
    subscribe,
    () => window.matchMedia(query).matches,
    // No viewport on a server; the wider layout is the safer assumption.
    () => false,
  )
}
