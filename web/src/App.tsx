import { BrowserRouter, Route, Routes } from "react-router-dom"

import { ThemeProvider } from "@/components/theme-provider"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { EditorPage } from "@/routes/editor"
import { PastePage } from "@/routes/paste"

export default function App() {
  return (
    <ThemeProvider>
      <TooltipProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/" element={<EditorPage />} />
            {/* The server serves index.html for any unclaimed path, which is
                how a share link resolves here after a hard refresh. */}
            <Route path="/:code" element={<PastePage />} />
          </Routes>
        </BrowserRouter>
        <Toaster position="bottom-right" />
      </TooltipProvider>
    </ThemeProvider>
  )
}
