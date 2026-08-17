import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
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

  const days = Math.floor(ms / 86_400_000)
  if (days >= 1) return `expires in ${days} day${days === 1 ? "" : "s"}`
  const hours = Math.floor(ms / 3_600_000)
  if (hours >= 1) return `expires in ${hours} hour${hours === 1 ? "" : "s"}`
  const minutes = Math.max(1, Math.floor(ms / 60_000))
  return `expires in ${minutes} min`
}
