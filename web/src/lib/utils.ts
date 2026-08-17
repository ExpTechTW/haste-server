import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** The platform's chord prefix, for rendering shortcut hints. */
export function modKey(): string {
  return /Mac|iPhone|iPad/.test(navigator.userAgent) ? "⌘" : "Ctrl+"
}

/** Formats a byte count for the compression readout. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

/** Renders a future timestamp as a coarse "expires in" phrase. */
export function formatExpiry(iso: string | null): string | null {
  if (!iso) return null
  const ms = new Date(iso).getTime() - Date.now()
  if (Number.isNaN(ms)) return null
  if (ms <= 0) return "expired"

  // Rounded, not truncated: a paste created seconds ago under a 30-day policy
  // should read "30 days", not "29".
  const days = Math.round(ms / 86_400_000)
  if (ms >= 86_400_000) return `expires in ${days} day${days === 1 ? "" : "s"}`
  const hours = Math.round(ms / 3_600_000)
  if (hours >= 1) return `expires in ${hours} hour${hours === 1 ? "" : "s"}`
  const minutes = Math.max(1, Math.floor(ms / 60_000))
  return `expires in ${minutes} min`
}
