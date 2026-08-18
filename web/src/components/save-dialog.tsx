import * as React from "react"
import { LoaderCircleIcon, LockIcon, SaveIcon } from "lucide-react"

import { ExpiryPicker } from "@/components/expiry-picker"
import { LanguagePicker } from "@/components/language-picker"
import { Kbd } from "@/components/shell"
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
import { cn, countCodePoints, modKey } from "@/lib/utils"

/**
 * The last step before a paste becomes a link.
 *
 * A paste is write-once, so everything decided here is decided for good — which
 * is the argument for a dialog rather than more controls in the status bar.
 * Naming it, choosing how long it lives and confirming what it was taken for
 * are one act: publishing, done deliberately and once.
 *
 * Three bands, in the order the decisions matter: what it is called, how it is
 * stored, and what that commits you to. The editor's own bar keeps only what
 * belongs to editing — the character count, and the language that recolours the
 * text while you type.
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
  language,
  detectedLanguage,
  onLanguageChange,
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
  language: string
  detectedLanguage: string
  onLanguageChange: (language: string) => void
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
      <DialogContent closeLabel={t("save.close")} className="gap-0 p-0">
        <DialogHeader className="px-5 pt-5 pb-4 sm:px-6 sm:pt-6">
          <DialogTitle>{t("save.title")}</DialogTitle>
          <DialogDescription>{t("save.description")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 border-y bg-surface px-5 py-5 sm:px-6">
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
              onKeyDown={(event) => {
                // A single-line field: Enter has nothing else it could mean.
                if (event.key === "Enter" && !tooLong && !saving) {
                  event.preventDefault()
                  onConfirm()
                }
              }}
              placeholder={t("save.titlePlaceholder")}
              // Not maxLength: a hard stop swallows a paste of a longer name
              // with no explanation. The counter turning red says what happened.
              autoFocus
              autoComplete="off"
              spellCheck={false}
              aria-invalid={tooLong}
              aria-describedby={tooLong ? "paste-title-error" : undefined}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20"
            />
            {tooLong && (
              <p id="paste-title-error" className="text-xs text-destructive">
                {t("save.titleTooLong", { max: maxTitleChars })}
              </p>
            )}
          </div>

          {/* One panel, one rule between: two settings that are read together. */}
          <div className="divide-y rounded-lg border bg-background">
            <Row label={t("save.expiry")}>
              <ExpiryPicker
                value={expiresIn}
                options={expiryOptions}
                cleanupEverySecs={cleanupEverySecs}
                onChange={onExpiryChange}
              />
            </Row>
            <Row label={t("save.language")}>
              <LanguagePicker
                value={language}
                detected={detectedLanguage}
                onChange={onLanguageChange}
              />
            </Row>
          </div>
        </div>

        {/*
          The consequence, next to the setting that answers it: a lifetime is
          the only delete this server has, so the moment to think about it is
          while the picker above is still in reach.
        */}
        <div className="flex gap-2.5 px-5 py-4 sm:px-6">
          <LockIcon className="mt-0.5 size-4 shrink-0 text-warning" />
          <p className="text-xs leading-relaxed text-muted-foreground">{t("save.noDelete")}</p>
        </div>

        <DialogFooter className="border-t px-5 py-4 sm:px-6">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t("save.cancel")}
          </Button>
          <Button onClick={onConfirm} disabled={saving || tooLong}>
            {saving ? <LoaderCircleIcon className="animate-spin" /> : <SaveIcon />}
            {t("save.confirm")}
            {/* Hidden where there is no keyboard to press it with. */}
            <span className="hidden sm:inline-flex">
              <Kbd>{modKey()}S</Kbd>
            </span>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-12 items-center justify-between gap-3 px-3">
      <span className="text-sm font-medium">{label}</span>
      {children}
    </div>
  )
}
