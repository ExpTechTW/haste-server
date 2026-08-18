import * as React from "react"
import { useNavigate } from "react-router-dom"
import { ArrowLeftIcon, CheckIcon, ChevronRightIcon, CopyIcon, SendIcon } from "lucide-react"
import { toast } from "sonner"

import { HeaderBar, Shell } from "@/components/shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { useServerConfig } from "@/hooks/use-server-config"
import { copyText } from "@/lib/clipboard"
import { useT, type MessageKey } from "@/lib/i18n"
import { cn } from "@/lib/utils"

type Method = "GET" | "POST"

interface Field {
  name: string
  type: string
  required?: boolean
  desc: MessageKey
}

interface Endpoint {
  method: Method
  path: string
  summary: MessageKey
  /** Prose that does not fit in the summary line. */
  note?: MessageKey
  /** Path and query parameters. */
  params?: Field[]
  /** JSON body fields, for the endpoints that take an envelope. */
  body?: Field[]
  /** Prefilled body for "try it", and the body shown in the curl example. */
  example?: string
  responses: Array<[number, MessageKey]>
}

/**
 * The API, as data.
 *
 * Kept as a list rather than an OpenAPI document because that is all this page
 * needs: nine endpoints, rendered once. A spec would buy code generation this
 * project has no use for, at the cost of a second description of the same
 * server that could drift from the handlers just as easily.
 */
const ENDPOINTS: Endpoint[] = [
  {
    method: "POST",
    path: "/api/pastes",
    summary: "docs.create.summary",
    note: "docs.create.body",
    params: [
      { name: "expiresIn", type: "string", desc: "docs.p.expiresInQuery" },
      { name: "language", type: "string", desc: "docs.p.languageQuery" },
    ],
    body: [
      { name: "content", type: "string", required: true, desc: "docs.p.content" },
      { name: "language", type: "string", desc: "docs.p.language" },
      { name: "expiresIn", type: "integer", desc: "docs.p.expiresIn" },
    ],
    example: '{\n  "content": "print(1)",\n  "language": "python",\n  "expiresIn": 21600\n}',
    responses: [
      [201, "docs.r.201"],
      [400, "docs.r.400"],
      [413, "docs.r.413"],
      [429, "docs.r.429"],
      [503, "docs.r.503"],
    ],
  },
  {
    method: "GET",
    path: "/api/pastes/{code}",
    summary: "docs.read.summary",
    params: [{ name: "code", type: "string", required: true, desc: "docs.p.code" }],
    responses: [
      [200, "docs.r.200"],
      [404, "docs.r.404"],
    ],
  },
  {
    method: "GET",
    path: "/raw/{code}",
    summary: "docs.raw.summary",
    params: [{ name: "code", type: "string", required: true, desc: "docs.p.code" }],
    responses: [
      [200, "docs.r.200raw"],
      [404, "docs.r.404"],
    ],
  },
  {
    method: "GET",
    path: "/download/{code}",
    summary: "docs.download.summary",
    note: "docs.download.body",
    params: [{ name: "code", type: "string", required: true, desc: "docs.p.code" }],
    responses: [
      [200, "docs.r.200download"],
      [404, "docs.r.404"],
    ],
  },
  {
    method: "GET",
    path: "/api/config",
    summary: "docs.config.summary",
    note: "docs.config.body",
    responses: [[200, "docs.r.200config"]],
  },
  {
    method: "GET",
    path: "/api/stats",
    summary: "docs.stats.summary",
    responses: [[200, "docs.r.200stats"]],
  },
  {
    method: "GET",
    path: "/healthz",
    summary: "docs.health.summary",
    responses: [[200, "docs.r.200ok"]],
  },
  {
    method: "POST",
    path: "/documents",
    summary: "docs.legacyCreate.summary",
    note: "docs.legacy.body",
    example: "print(1)",
    responses: [
      [200, "docs.r.201"],
      [400, "docs.r.400"],
    ],
  },
  {
    method: "GET",
    path: "/documents/{code}",
    summary: "docs.legacyRead.summary",
    note: "docs.legacy.body",
    params: [{ name: "code", type: "string", required: true, desc: "docs.p.code" }],
    responses: [
      [200, "docs.r.200"],
      [404, "docs.r.404"],
    ],
  },
]

export function DocsPage() {
  const t = useT()
  const navigate = useNavigate()
  const config = useServerConfig()

  // What people should call, which behind a proxy is not the host the browser
  // is on. Falls back to that host only when the operator has declared nothing.
  const baseUrl = config.baseUrl || window.location.origin

  return (
    <Shell>
      <HeaderBar />

      <main className="scrollbar-slim min-h-0 flex-1 overflow-auto bg-surface">
        <div className="mx-auto w-full max-w-3xl space-y-8 px-5 py-8 sm:px-8 sm:py-12">
          <header className="space-y-2">
            <h1 className="font-mono text-2xl font-semibold tracking-tight">{t("docs.title")}</h1>
            <p className="text-sm text-muted-foreground">{t("docs.subtitle")}</p>
          </header>

          <BaseUrl url={baseUrl} />

          <div className="overflow-hidden rounded-xl border bg-background">
            {ENDPOINTS.map((endpoint, i) => (
              <Row
                key={endpoint.method + endpoint.path}
                endpoint={endpoint}
                baseUrl={baseUrl}
                first={i === 0}
              />
            ))}
          </div>

          <p className="text-sm leading-relaxed text-muted-foreground">
            <Ticks text={t("docs.errorShape")} />
          </p>

          <Button variant="outline" onClick={() => navigate("/")}>
            <ArrowLeftIcon />
            {t("docs.back")}
          </Button>
        </div>
      </main>
    </Shell>
  )
}

function BaseUrl({ url }: { url: string }) {
  const t = useT()
  return (
    <div className="space-y-2 rounded-xl border bg-background p-4">
      <p className="text-xs font-medium text-muted-foreground">{t("docs.baseUrl")}</p>
      <div className="flex items-center gap-2">
        <code className="scrollbar-slim min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-sm">
          {url}
        </code>
        <CopyButton value={url} />
      </div>
      <p className="text-xs leading-relaxed text-muted-foreground">{t("docs.baseUrlNote")}</p>
    </div>
  )
}

function Row({
  endpoint,
  baseUrl,
  first,
}: {
  endpoint: Endpoint
  baseUrl: string
  first: boolean
}) {
  const t = useT()
  const [open, setOpen] = React.useState(false)

  return (
    <div className={cn(!first && "border-t")}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-3 px-3 py-2.5 text-left outline-none hover:bg-accent/50 focus-visible:bg-accent/50 sm:px-4"
      >
        <ChevronRightIcon
          className={cn(
            "size-3.5 shrink-0 text-muted-foreground transition-transform",
            open && "rotate-90",
          )}
        />
        <MethodBadge method={endpoint.method} />
        <code className="shrink-0 font-mono text-[13px]">{endpoint.path}</code>
        <span className="hidden truncate text-xs text-muted-foreground sm:inline">
          {t(endpoint.summary)}
        </span>
      </button>

      {open && <Detail endpoint={endpoint} baseUrl={baseUrl} />}
    </div>
  )
}

function Detail({ endpoint, baseUrl }: { endpoint: Endpoint; baseUrl: string }) {
  const t = useT()

  return (
    <div className="space-y-5 border-t bg-surface px-3 py-4 sm:px-4">
      <p className="text-sm text-muted-foreground sm:hidden">{t(endpoint.summary)}</p>
      {endpoint.note && (
        <p className="text-sm leading-relaxed text-muted-foreground">
          <Ticks text={t(endpoint.note)} />
        </p>
      )}

      {endpoint.params && (
        <Section title={t("docs.parameters")}>
          <Fields fields={endpoint.params} />
        </Section>
      )}

      {endpoint.body && (
        <Section title={t("docs.requestBody")}>
          <Fields fields={endpoint.body} />
        </Section>
      )}

      <Section title={t("docs.example")}>
        <Code text={curlFor(endpoint, baseUrl)} />
      </Section>

      <Section title={t("docs.responses")}>
        <ul className="space-y-1.5">
          {endpoint.responses.map(([status, desc]) => (
            <li key={status} className="flex gap-3 text-sm">
              <StatusBadge status={status} />
              <span className="text-muted-foreground">{t(desc)}</span>
            </li>
          ))}
        </ul>
      </Section>

      <TryIt endpoint={endpoint} />
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="text-xs font-medium text-muted-foreground">{title}</h3>
      {children}
    </section>
  )
}

function Fields({ fields }: { fields: Field[] }) {
  const t = useT()
  return (
    <ul className="space-y-2">
      {fields.map((f) => (
        <li key={f.name} className="text-sm">
          <span className="font-mono text-[13px]">{f.name}</span>
          <span className="ml-2 font-mono text-xs text-muted-foreground">{f.type}</span>
          <span
            className={cn(
              "ml-2 text-xs",
              f.required ? "text-warning" : "text-muted-foreground/70",
            )}
          >
            {t(f.required ? "docs.required" : "docs.optional")}
          </span>
          <p className="mt-0.5 leading-relaxed text-muted-foreground">{t(f.desc)}</p>
        </li>
      ))}
    </ul>
  )
}

/**
 * Runs the request against the server serving this page.
 *
 * Deliberately not against the published base URL: that may be a different
 * deployment, and a docs page on a laptop should not quietly write to
 * production. The curl example above shows the address to share; this button
 * exercises the one you are looking at.
 */
function TryIt({ endpoint }: { endpoint: Endpoint }) {
  const t = useT()
  const [code, setCode] = React.useState("")
  const [body, setBody] = React.useState(endpoint.example ?? "")
  const [sending, setSending] = React.useState(false)
  const [result, setResult] = React.useState<string | null>(null)

  const needsCode = endpoint.path.includes("{code}")

  const send = async () => {
    setSending(true)
    setResult(null)
    const path = endpoint.path.replace("{code}", encodeURIComponent(code.trim()))
    try {
      const response = await fetch(path, {
        method: endpoint.method,
        headers: endpoint.example?.startsWith("{")
          ? { "Content-Type": "application/json" }
          : undefined,
        body: endpoint.method === "POST" ? body : undefined,
      })
      const text = await response.text()
      setResult(`${response.status} ${response.statusText}\n\n${pretty(text)}`)
    } catch (error) {
      setResult(String(error))
    } finally {
      setSending(false)
    }
  }

  return (
    <Section title={t("docs.tryIt")}>
      <div className="space-y-2">
        {needsCode && (
          <input
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder="k7Qm2Xp9"
            spellCheck={false}
            aria-label="code"
            className="w-full rounded-md border bg-background px-2.5 py-1.5 font-mono text-[13px] outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          />
        )}
        {endpoint.method === "POST" && (
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={endpoint.example?.split("\n").length ?? 4}
            spellCheck={false}
            aria-label={t("docs.requestBody")}
            className="scrollbar-slim w-full resize-y rounded-md border bg-background px-2.5 py-1.5 font-mono text-[13px] outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          />
        )}

        <Button size="sm" onClick={() => void send()} disabled={sending || (needsCode && !code.trim())}>
          <SendIcon />
          {sending ? t("docs.sending") : t("docs.send")}
        </Button>

        {result !== null && (
          <div className="space-y-1.5 pt-1">
            <h4 className="text-xs font-medium text-muted-foreground">{t("docs.response")}</h4>
            <Code text={result} />
          </div>
        )}
      </div>
    </Section>
  )
}

function Code({ text }: { text: string }) {
  return (
    <div className="relative">
      <pre className="scrollbar-slim max-h-80 overflow-auto rounded-md border bg-background px-3 py-2.5 pr-11 font-mono text-[13px] leading-relaxed">
        {text}
      </pre>
      <div className="absolute right-1.5 top-1.5">
        <CopyButton value={text} />
      </div>
    </div>
  )
}

function CopyButton({ value }: { value: string }) {
  const t = useT()
  const [done, setDone] = React.useState(false)

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={t("paste.copyContent")}
      onClick={async () => {
        if (!(await copyText(value))) {
          toast.error(t("paste.contentCopyFailed"))
          return
        }
        setDone(true)
        setTimeout(() => setDone(false), 1200)
      }}
    >
      {done ? <CheckIcon className="text-success" /> : <CopyIcon />}
    </Button>
  )
}

/**
 * Swagger colours GET blue and POST green, and people read these badges by
 * colour before they read the word. The palette tokens have no opinion about
 * HTTP verbs, so these two are named outright.
 */
function MethodBadge({ method }: { method: Method }) {
  return (
    <Badge
      variant="secondary"
      className={cn(
        "shrink-0 font-mono text-[10px] font-semibold",
        method === "GET"
          ? "bg-sky-500/10 text-sky-700 dark:text-sky-400"
          : "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
      )}
    >
      {method}
    </Badge>
  )
}

function StatusBadge({ status }: { status: number }) {
  return (
    <code
      className={cn(
        "shrink-0 font-mono text-[13px] tabular-nums",
        status < 300 ? "text-success" : status < 500 ? "text-warning" : "text-destructive",
      )}
    >
      {status}
    </code>
  )
}

/** Renders `backticked` spans in translated prose as code. */
function Ticks({ text }: { text: string }) {
  return (
    <>
      {text.split(/`([^`]+)`/g).map((part, i) =>
        i % 2 === 1 ? (
          // eslint-disable-next-line react/no-array-index-key
          <code key={i} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em]">
            {part}
          </code>
        ) : (
          part
        ),
      )}
    </>
  )
}

/** The example someone would actually paste into a terminal. */
function curlFor(endpoint: Endpoint, baseUrl: string): string {
  const url = baseUrl + endpoint.path.replace("{code}", "k7Qm2Xp9")

  if (endpoint.method === "GET") {
    return `curl ${url}`
  }
  if (endpoint.example?.startsWith("{")) {
    return [
      `curl -X POST ${url} \\`,
      `  -H 'Content-Type: application/json' \\`,
      `  -d '${endpoint.example.replace(/\n\s*/g, " ").replace(/\s+}/, " }")}'`,
    ].join("\n")
  }
  return `curl --data-binary @file.txt ${url}`
}

/** Pretty-prints a JSON response; leaves anything else alone. */
function pretty(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

export default DocsPage
