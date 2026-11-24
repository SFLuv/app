"use client"

import { toast as sonnerToast } from "sonner"

/**
 * success (green) is the default, because nearly every toast in the app
 * confirms something that just worked. warning (yellow) is for "done, but look
 * at this" and things still pending; destructive (red) for failures; info for
 * the rare neutral note.
 */
type ToastVariant = "default" | "success" | "info" | "warning" | "destructive"

type ToastProps = {
  title?: string
  description?: string
  variant?: ToastVariant
}

// toast shows a pop-up through sonner's <Toaster />, which app/layout.tsx mounts
// once for the whole app. This hook used to return a console.log mock, so every
// toast() call in the app displayed nothing.
function toast({ title, description, variant = "default" }: ToastProps) {
  const message = title || description || ""
  const options = title && description ? { description } : undefined
  const id =
    variant === "destructive"
      ? sonnerToast.error(message, options)
      : variant === "warning"
        ? sonnerToast.warning(message, options)
        : variant === "info"
          ? sonnerToast(message, options)
          : sonnerToast.success(message, options)

  return {
    id: String(id),
    dismiss: () => sonnerToast.dismiss(id),
  }
}

function dismiss(toastId?: string | number) {
  sonnerToast.dismiss(toastId)
}

// toast and dismiss are module-level, so they are stable across renders and safe
// in effect dependency arrays.
function useToast() {
  return { toast, dismiss }
}

export { useToast, toast }
