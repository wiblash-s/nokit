import { useSyncExternalStore } from "react"

// A tiny dependency-free toast store. Components call `toast(...)` from anywhere
// and render the toasts via the <Toaster /> component, which subscribes to this
// store. Kept intentionally small — enough for success/error feedback such as
// the unban confirmation in the Player Panel.

export type ToastVariant = "default" | "success" | "error"

export interface Toast {
  id: number
  title: string
  description?: string
  variant: ToastVariant
}

type Listener = () => void

let toasts: Toast[] = []
const listeners = new Set<Listener>()
let nextId = 1

function emit() {
  for (const l of listeners) l()
}

function subscribe(listener: Listener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function getSnapshot(): Toast[] {
  return toasts
}

export function dismissToast(id: number) {
  toasts = toasts.filter((t) => t.id !== id)
  emit()
}

export interface ToastOptions {
  description?: string
  variant?: ToastVariant
  /** Auto-dismiss delay in ms. Defaults to 4000. */
  duration?: number
}

export function toast(title: string, opts: ToastOptions = {}) {
  const id = nextId++
  const t: Toast = {
    id,
    title,
    description: opts.description,
    variant: opts.variant ?? "default",
  }
  toasts = [...toasts, t]
  emit()

  const duration = opts.duration ?? 4000
  if (duration > 0) {
    setTimeout(() => dismissToast(id), duration)
  }
  return id
}

/** Subscribe to the current toast list (used by <Toaster />). */
export function useToasts(): Toast[] {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}
