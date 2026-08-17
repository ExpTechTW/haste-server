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
}

export interface ServerConfig {
  maxChars: number
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

export function createPaste(content: string, language: string): Promise<Paste> {
  return request<Paste>("/api/pastes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ content, language }),
  })
}

export function fetchPaste(key: string): Promise<Paste> {
  return request<Paste>(`/api/pastes/${encodeURIComponent(key)}`)
}
