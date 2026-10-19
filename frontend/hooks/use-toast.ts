"use client"

import { toast as sonnerToast } from "sonner"

type ToastProps = {
  title?: string
  description?: string
  variant?: "default" | "destructive"
}

// toast shows a pop-up through sonner's <Toaster />, which app/layout.tsx mounts
// once for the whole app. This hook used to return a console.log mock, so every
// toast() call in the app displayed nothing.
function toast({ title, description, variant = "default" }: ToastProps) {
  const message = title || description || ""
  const options = title && description ? { description } : undefined
  const id = variant === "destructive" ? sonnerToast.error(message, options) : sonnerToast(message, options)

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
