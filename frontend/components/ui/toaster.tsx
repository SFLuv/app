"use client"

import type { CSSProperties } from "react"
import { useTheme } from "next-themes"
import { Toaster as Sonner } from "sonner"

// sonner reads its colours from these variables on the toaster. They point at
// the app's own (app/globals.css, --toast-*), which change with the theme, so a
// toast is a card in the app's type and radius, tinted by what it reports:
// green for success, yellow for a warning, red for an error.
const palette = {
  fontFamily: "inherit",
  "--normal-bg": "hsl(var(--card))",
  "--normal-border": "hsl(var(--border))",
  "--normal-text": "hsl(var(--card-foreground))",
  "--success-bg": "hsl(var(--toast-success-bg))",
  "--success-border": "hsl(var(--toast-success-border))",
  "--success-text": "hsl(var(--toast-success-text))",
  "--warning-bg": "hsl(var(--toast-warning-bg))",
  "--warning-border": "hsl(var(--toast-warning-border))",
  "--warning-text": "hsl(var(--toast-warning-text))",
  "--error-bg": "hsl(var(--toast-error-bg))",
  "--error-border": "hsl(var(--toast-error-border))",
  "--error-text": "hsl(var(--toast-error-text))",
  "--info-bg": "hsl(var(--card))",
  "--info-border": "hsl(var(--border))",
  "--info-text": "hsl(var(--card-foreground))",
  "--border-radius": "var(--radius)",
} as CSSProperties

// Toaster renders every toast() from hooks/use-toast. Mounted once in app/layout.tsx.
export function Toaster() {
  const { resolvedTheme } = useTheme()

  return (
    <Sonner
      theme={resolvedTheme === "dark" ? "dark" : "light"}
      position="bottom-right"
      richColors
      closeButton
      gap={8}
      offset={24}
      style={palette}
      toastOptions={{
        classNames: {
          toast:
            "gap-3 p-4 shadow-[0_2px_6px_hsl(var(--foreground)/0.06),0_10px_30px_hsl(var(--foreground)/0.10)]",
          title: "text-sm font-semibold",
          description: "text-sm opacity-90",
        },
      }}
    />
  )
}
