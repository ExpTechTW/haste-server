import * as React from "react"

import { fetchConfig, type ServerConfig } from "@/lib/api"

/**
 * Defaults matching the server's own, so the editor is usable and correctly
 * bounded on the very first frame instead of waiting on a round trip.
 */
const FALLBACK: ServerConfig = {
  maxChars: 40_000,
  expiryOptionsSecs: [3600, 21600, 43200, 86400, 259200, 604800, 1209600, 2592000],
  cleanupEverySecs: 3600,
}

// Shared across mounts: the limits cannot change while the tab is open.
let pending: Promise<ServerConfig> | null = null

export function useServerConfig(): ServerConfig {
  const [config, setConfig] = React.useState<ServerConfig>(FALLBACK)

  React.useEffect(() => {
    pending ??= fetchConfig()
    let cancelled = false
    pending
      .then((cfg) => {
        if (!cancelled) setConfig(cfg)
      })
      .catch(() => {
        // Keep the fallback; the server still enforces the real limit.
        pending = null
      })
    return () => {
      cancelled = true
    }
  }, [])

  return config
}
