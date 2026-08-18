import { ApiError } from "@/lib/api"

import type { MessageKey, Translate } from "./index"

/**
 * The stable error codes the server sends, for the ones worth saying in the
 * reader's own language.
 *
 * Keyed on the code rather than the message, because the code is the part of
 * the contract that does not change. Anything absent here — an unforeseen code,
 * or a proxy answering for the server — falls back to whatever text arrived,
 * which is in English but is at least accurate.
 */
const TRANSLATED: Record<string, MessageKey> = {
  network: "err.network",
  not_found: "err.not_found",
  empty: "err.empty",
  too_large: "err.too_large",
  bad_expiry: "err.bad_expiry",
  rate_limited: "err.rate_limited",
  busy: "err.busy",
  no_room: "err.no_room",
}

/** What to show a person when a request failed. */
export function describeError(t: Translate, error: unknown, fallback: MessageKey): string {
  if (!(error instanceof ApiError)) return t(fallback)
  const key = TRANSLATED[error.code]
  return key ? t(key) : error.message
}
