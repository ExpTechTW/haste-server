import { BrowserRouter, Route, Routes } from "react-router-dom"

import { ThemeProvider } from "@/components/theme-provider"
import { I18nProvider, useT } from "@/lib/i18n"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { NotFound } from "@/components/not-found"
import { DocsPage } from "@/routes/docs"
import { EditorPage } from "@/routes/editor"
import { PastePage } from "@/routes/paste"

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
          <Toaster position="bottom-right" />
        </TooltipProvider>
      </I18nProvider>
    </ThemeProvider>
  )
}
