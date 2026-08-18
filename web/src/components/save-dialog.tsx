import * as React from "react"
import { LoaderCircleIcon, SaveIcon } from "lucide-react"

import { ExpiryPicker } from "@/components/expiry-picker"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { ExpiryOption } from "@/lib/expiry"
import { useT } from "@/lib/i18n"
import { cn, countCodePoints } from "@/lib/utils"

/**
 * The last step before a paste becomes a link.
 *
 * A paste is write-once, so everything decided here is decided for good — which
 * is the argument for a dialog rather than more controls in the status bar.
 * Naming it, choosing how long it lives and seeing what language it was taken
 * for are all the same act: publishing, done deliberately and once.
 *
 * The editor's own bar keeps only what belongs to editing: the character count
 * and the language, which drives highlighting while you type.
 */
export function SaveDialog({
  open,
  onOpenChange,
  title,
  onTitleChange,
  maxTitleChars,
  expiresIn,
  onExpiryChange,
  expiryOptions,
  cleanupEverySecs,
  languageLabel,
  chars,
  saving,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  onTitleChange: (title: string) => void
  maxTitleChars: number
  expiresIn: number
  onExpiryChange: (seconds: number) => void
  expiryOptions: ExpiryOption[]
  cleanupEverySecs: number
  languageLabel: string
  chars: number
  saving: boolean
  onConfirm: () => void
}) {
  const t = useT()

  // Counted in code points, matching both the server's limit and the character
  // counter in the status bar; a title is a likely place for an emoji.
  const titleChars = countCodePoints(title.trim())
  const tooLong = titleChars > maxTitleChars

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        closeLabel={t("save.close")}
        onKeyDown={(event) => {
          // Enter commits from anywhere in the dialog. The title is a single
          // line, so there is nothing Enter could mean instead.
          if (event.key === "Enter" && !event.shiftKey && !tooLong && !saving) {
            event.preventDefault()
            onConfirm()
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>{t("save.title")}</DialogTitle>
          <DialogDescription>{t("save.description")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between gap-2">
              <label htmlFor="paste-title" className="text-sm font-medium">
                {t("save.titleLabel")}
                <span className="ml-2 text-xs font-normal text-muted-foreground">
                  {t("save.titleOptional")}
                </span>
              </label>
              <span
                className={cn(
                  "font-mono text-xs tabular-nums",
                  tooLong ? "font-semibold text-destructive" : "text-muted-foreground",
                )}
              >
                {titleChars} / {maxTitleChars}
              </span>
            </div>

            <input
              id="paste-title"
              value={title}
              onChange={(event) => onTitleChange(event.target.value)}
              placeholder={t("save.titlePlaceholder")}
              // Not maxLength: a hard stop swallows a paste of a longer name
              // with no explanation. The counter turning red says what happened.
              autoFocus
              autoComplete="off"
              spellCheck={false}
              aria-invalid={tooLong}
              aria-describedby="paste-title-hint"
              className="w-full rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20"
            />
            <p
              id="paste-title-hint"
              className={cn("text-xs", tooLong ? "text-destructive" : "text-muted-foreground")}
            >
              {tooLong ? t("save.titleTooLong", { max: maxTitleChars }) : t("save.titleHint")}
            </p>
          </div>

          <Row label={t("save.expiry")}>
            <ExpiryPicker
              value={expiresIn}
              options={expiryOptions}
              cleanupEverySecs={cleanupEverySecs}
              onChange={onExpiryChange}
            />
          </Row>

          {/* Shown rather than editable: the language belongs to the editor,
              where changing it recolours the text you are looking at. Here it
              is a receipt for what is about to be saved. */}
          <Row label={t("save.language")}>
            <Badge variant="secondary" className="font-normal">
              {languageLabel}
            </Badge>
          </Row>

          <Row label={t("save.size")}>
            <span className="font-mono text-xs tabular-nums text-muted-foreground">
              {t("paste.chars", { count: chars.toLocaleString() })}
            </span>
          </Row>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t("save.cancel")}
          </Button>
          <Button onClick={onConfirm} disabled={saving || tooLong}>
            {saving ? <LoaderCircleIcon className="animate-spin" /> : <SaveIcon />}
            {t("save.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-9 items-center justify-between gap-3">
      <span className="text-sm font-medium">{label}</span>
      {children}
    </div>
  )
}
