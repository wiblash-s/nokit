import { CheckCircle2, AlertCircle, Info, X } from "lucide-react"

import { cn } from "@/lib/utils"
import { useToasts, dismissToast, type ToastVariant } from "@/hooks/use-toast"

const VARIANT_STYLES: Record<ToastVariant, string> = {
  default: "border-border bg-card text-card-foreground",
  success:
    "border-green-500/30 bg-green-500/10 text-foreground dark:bg-green-500/15",
  error: "border-destructive/30 bg-destructive/10 text-foreground",
}

const VARIANT_ICON = {
  default: Info,
  success: CheckCircle2,
  error: AlertCircle,
}

const VARIANT_ICON_COLOR: Record<ToastVariant, string> = {
  default: "text-muted-foreground",
  success: "text-green-600 dark:text-green-400",
  error: "text-destructive",
}

/**
 * Toaster renders the active toasts in a fixed bottom-right stack. Mount it once
 * near the root of a view that needs feedback (e.g. the Player Panel).
 */
export function Toaster() {
  const toasts = useToasts()

  return (
    <div className="pointer-events-none fixed right-4 bottom-4 z-[100] flex w-full max-w-sm flex-col gap-2">
      {toasts.map((t) => {
        const Icon = VARIANT_ICON[t.variant]
        return (
          <div
            key={t.id}
            role="status"
            className={cn(
              "pointer-events-auto flex items-start gap-2.5 rounded-lg border p-3 shadow-lg",
              "data-[state=open]:animate-in data-[state=open]:slide-in-from-bottom-2",
              VARIANT_STYLES[t.variant]
            )}
          >
            <Icon
              className={cn("mt-0.5 size-4 shrink-0", VARIANT_ICON_COLOR[t.variant])}
            />
            <div className="flex-1 text-sm">
              <p className="font-medium">{t.title}</p>
              {t.description && (
                <p className="mt-0.5 text-muted-foreground">{t.description}</p>
              )}
            </div>
            <button
              type="button"
              onClick={() => dismissToast(t.id)}
              className="text-muted-foreground transition-colors hover:text-foreground"
              aria-label="Dismiss"
            >
              <X className="size-3.5" />
            </button>
          </div>
        )
      })}
    </div>
  )
}
