"use client"

import { useTheme } from "next-themes"
import { Toaster as Sonner } from "sonner"

// Toaster renders every toast() from hooks/use-toast. Mounted once in app/layout.tsx.
export function Toaster() {
  const { resolvedTheme } = useTheme()

  return (
    <Sonner
      theme={resolvedTheme === "dark" ? "dark" : "light"}
      position="top-center"
      richColors
      closeButton
    />
  )
}
