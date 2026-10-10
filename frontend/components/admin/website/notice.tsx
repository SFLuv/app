"use client"

import { createContext, useCallback, useContext, useState, type ReactNode } from "react"
import { AlertCircle, CheckCircle2, X } from "lucide-react"

interface Notice {
  title: string
  description?: string
  variant?: "default" | "destructive"
}

interface Shown extends Notice {
  id: number
}

const NoticeContext = createContext<(notice: Notice) => void>(() => {})

let counter = 0

/**
 * Visible feedback for the Website tools: "Saved", "Published", and — more
 * importantly — plain-language errors.
 *
 * The app's own `useToast` is not usable here: it is only a state store, and no
 * Toaster is mounted anywhere, so its calls display nothing. Editors have to be
 * told when a save fails, so these tools carry their own. The call shape
 * matches `toast()` on purpose.
 */
export function NoticeProvider({ children }: { children: ReactNode }) {
  const [shown, setShown] = useState<Shown[]>([])

  const dismiss = useCallback((id: number) => setShown((list) => list.filter((n) => n.id !== id)), [])

  const push = useCallback(
    (notice: Notice) => {
      counter += 1
      const id = counter
      setShown((list) => [...list.slice(-3), { ...notice, id }])
      // Errors stay longer: they are what someone has to read and act on.
      setTimeout(() => dismiss(id), notice.variant === "destructive" ? 12000 : 5000)
    },
    [dismiss],
  )

  return (
    <NoticeContext.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-[min(24rem,calc(100vw-2rem))] flex-col gap-2">
        {shown.map((n) => {
          const bad = n.variant === "destructive"
          return (
            <div
              key={n.id}
              role={bad ? "alert" : "status"}
              className={`pointer-events-auto flex items-start gap-3 rounded-lg border p-3 shadow-lg ${
                bad ? "border-destructive/40 bg-background text-destructive" : "border-border bg-background text-foreground"
              }`}
            >
              {bad ? <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" /> : <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-green-600" />}
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{n.title}</p>
                {n.description && <p className="mt-0.5 text-sm text-muted-foreground">{n.description}</p>}
              </div>
              <button type="button" aria-label="Dismiss" className="shrink-0 text-muted-foreground hover:text-foreground" onClick={() => dismiss(n.id)}>
                <X className="h-4 w-4" />
              </button>
            </div>
          )
        })}
      </div>
    </NoticeContext.Provider>
  )
}

export function useNotice() {
  return { toast: useContext(NoticeContext) }
}
