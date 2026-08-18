export interface Paste {
  key: string
  url: string
  rawUrl: string
  downloadUrl: string
  /** Share code plus the extension its language implies, e.g. "k7Qm2Xp9.dart". */
  filename: string
  language?: string
  content?: string
  chars: number
  bytes: number
  stored: number
  ratio: number
  createdAt: string
  /**
   * Present only when a lifetime was chosen at save time. Its absence means no
   * timed deletion was asked for — not that the paste is permanent.
   */
  expiresAt?: string
}

export interface ServerConfig {
  maxChars: number
  /**
   * Every lifetime the API accepts, in seconds, ascending. Anything else is a
   * 400, so the picker is built from this rather than from a range.
   */
  expiryOptionsSecs: number[]
  /**
   * How often the server sweeps. A paste stops being served the instant its
   * lifetime ends, but its bytes are only reclaimed on the next sweep, so this
   * is what the UI quotes as the removal lag.
   */
  cleanupEverySecs: number
  /**
   * The origin this server tells people to call, from HASTE_BASE_URL. Absent
   * when unset, in which case whichever host you reached is the answer.
   */
  baseUrl?: string
}

/** An error the server described in its JSON envelope. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message)
    this.name = "ApiError"
  }
}

async function request<T>(input: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(input, init)
  } catch {
    throw new ApiError(0, "network", "Could not reach the server.")
  }

  if (!response.ok) {
    // Error bodies are JSON by contract, but a proxy or crash can still put
    // something else on the wire — fall back to the status line.
    const body = await response.json().catch(() => null)
    throw new ApiError(
      response.status,
      body?.error ?? "http_error",
      body?.message ?? `Request failed with status ${response.status}.`,
    )
  }
  return (await response.json()) as T
}

export function fetchConfig(): Promise<ServerConfig> {
  return request<ServerConfig>("/api/config")
}

export function createPaste(
  content: string,
  language: string,
  expiresIn: number,
): Promise<Paste> {
  return request<Paste>("/api/pastes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ content, language, expiresIn }),
  })
}

export function fetchPaste(key: string): Promise<Paste> {
  return request<Paste>(`/api/pastes/${encodeURIComponent(key)}`)
}
