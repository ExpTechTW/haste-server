import { fileURLToPath, URL } from "node:url"

import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    // Straight into the Go embed directory, so `go build` always picks up the
    // frontend that was built last.
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
    // Grammars are code-split per language; a chunk per language is the point.
    chunkSizeWarningLimit: 900,
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/raw": "http://localhost:8080",
      "/documents": "http://localhost:8080",
    },
  },
})
