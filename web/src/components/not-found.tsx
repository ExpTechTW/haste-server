import * as React from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { ClockIcon, HardDriveIcon, PlusIcon, SearchXIcon } from "lucide-react"

import { HeaderBar, Kbd, Shell } from "@/components/shell"
import { Button } from "@/components/ui/button"
import { useT, type MessageKey } from "@/lib/i18n"

/**
 * What a link that leads nowhere looks like.
 *
 * A paste can be missing for three unrelated reasons and the page says which,
 * because they call for different responses: a mistyped code is worth checking,
 * an expiry is worth asking the sender to repost, and a server that is merely
 * unreachable is worth a refresh. The old page said "Nothing here" to all three.
 *
 * The panel borrows the code view's own vocabulary — mono type, a gutter, muted
 * chrome — so a dead link still looks like part of the tool rather than a
 * browser error page.
 */
export function NotFound({
  code,
  message,
  missing,
}: {
  /** The share code that was asked for; empty when the path was not one. */
  code: string
  /** What went wrong, when it was not a plain miss. */
  message: string
  /** True for a genuine 404, false when the request itself failed. */
  missing: boolean
}) {
  const navigate = useNavigate()
  const t = useT()
  // A path too deep to be a share code has no code to show, so the address bar
  // is the only thing left that says what was asked for.
  const path = useLocation().pathname
  const asked = code || path
  // Shaped like something a shell printed: lowercase, no full stop. The server's
  // own messages are already written that way; a caller's sentence is not.
  const reason = missing && code ? t("nf.noPaste") : lowerFirst(message).replace(/\.$/, "")

  // Owned here rather than by the page that rendered this, because the
  // catch-all route reaches it without mounting one.
  React.useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return
      if (event.key.toLowerCase() !== "n") return
      event.preventDefault()
      navigate("/")
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [navigate])

  return (
    <Shell>
      <HeaderBar />

      <main className="relative flex min-h-0 flex-1 items-center justify-center overflow-auto bg-surface px-5 py-10">
        <div className="absolute size-[55vmin] rounded-full bg-foreground/[0.045] blur-3xl" />

        <div className="relative w-full max-w-lg space-y-7">
          <div className="overflow-hidden rounded-xl border bg-background shadow-sm">
            {/* A title bar, not decoration: it is where the code being looked
                up is shown, which is the first thing to check for a typo. */}
            <div className="flex items-center gap-2 border-b bg-surface px-3.5 py-2">
              <SearchXIcon className="size-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate font-mono text-xs text-muted-foreground">
                {code ? "/" + code : path}
              </span>
              <span className="ml-auto shrink-0 font-mono text-[11px] font-medium text-muted-foreground/70">
                {missing ? "404" : "error"}
              </span>
            </div>

            <div className="scrollbar-slim overflow-x-auto px-4 py-3.5 font-mono text-[13px] leading-relaxed">
              <div className="w-fit min-w-full">
                <p className="text-muted-foreground">
                  <span className="select-none text-muted-foreground/50">$ </span>
                  haste get {asked}
                </p>
                <p className="whitespace-pre text-destructive">error: {reason}</p>
              </div>
            </div>
          </div>

          {missing && code ? (
            <div className="space-y-3.5 px-1">
              <p className="text-sm font-medium">{t("nf.reasons")}</p>
              <Reason icon={<ClockIcon />} title={t("nf.temporary")} body="nf.temporaryBody" />
              <Reason icon={<HardDriveIcon />} title={t("nf.reclaimed")} body="nf.reclaimedBody" />
            </div>
          ) : (
            <p className="px-1 text-sm text-muted-foreground">
              {t(missing ? "nf.notAPaste" : "nf.loadFailed")}
            </p>
          )}

          <div className="flex flex-wrap items-center gap-2 px-1">
            <Button onClick={() => navigate("/")}>
              <PlusIcon />
              {t("paste.new")}
              <Kbd>N</Kbd>
            </Button>
            {!missing && (
              <Button variant="outline" onClick={() => window.location.reload()}>
                {t("nf.retry")}
              </Button>
            )}
          </div>
        </div>
      </main>
    </Shell>
  )
}

function lowerFirst(s: string): string {
  return s.charAt(0).toLowerCase() + s.slice(1)
}

function Reason({
  icon,
  title,
  body,
}: {
  icon: React.ReactNode
  title: string
  body: MessageKey
}) {
  const t = useT()
  return (
    <div className="flex gap-3">
      <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground [&>svg]:size-3.5">
        {icon}
      </span>
      <p className="text-sm text-muted-foreground">
        <span className="font-medium text-foreground">{title}. </span>
        {t(body)}
      </p>
    </div>
  )
}
