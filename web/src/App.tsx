import { BrowserRouter, Route, Routes } from "react-router-dom"

import { ThemeProvider } from "@/components/theme-provider"
import { useMediaQuery } from "@/hooks/use-media-query"
import { I18nProvider, useT } from "@/lib/i18n"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { NotFound } from "@/components/not-found"
import { DocsPage } from "@/routes/docs"
import { EditorPage } from "@/routes/editor"
import { PastePage } from "@/routes/paste"

/**
 * Where transient notices appear.
 *
 * Both pages put their controls in the status bar — Save above all — and a
 * toast at the default offset covers exactly the button whose refusal produced
 * it, so the second attempt lands on the notice instead. Clearing the bar is
 * enough on a wide screen; on a narrow one a wrapped toast is tall enough to
 * reach it anyway, so notices come from the top instead, where the only thing
 * they briefly cover is a row of icons.
 *
 * A z-index would not do: the toast has to sit above the save dialog, and the
 * status bar must not.
 */
function Notices() {
  const narrow = useMediaQuery("(max-width: 640px)")
  return narrow ? (
    <Toaster position="top-center" />
  ) : (
    <Toaster position="bottom-right" offset={{ bottom: "4.25rem", right: "1rem" }} />
  )
}

/** The catch-all. Its own component so it can reach the translator. */
function MissingPage() {
  const t = useT()
  return <NotFound code="" message={t("nf.noPage")} missing />
}

export default function App() {
  return (
    <ThemeProvider>
      <I18nProvider>
        <TooltipProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/" element={<EditorPage />} />
            {/* "docs" is a reserved code, so no paste can ever shadow it. */}
            <Route path="/docs" element={<DocsPage />} />
            {/* The server serves index.html for any unclaimed path, which is
                how a share link resolves here after a hard refresh. */}
            <Route path="/:code" element={<PastePage />} />
            {/* Deeper paths are no one's share link, and without this they
                would match no route and render a blank page. */}
            <Route path="*" element={<MissingPage />} />
          </Routes>
        </BrowserRouter>
          <Notices />
        </TooltipProvider>
      </I18nProvider>
    </ThemeProvider>
  )
}
