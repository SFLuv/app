"use client"

import { useEffect, useRef, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { usePrivy } from "@privy-io/react-auth"
import { useApp } from "@/context/AppProvider"
import { Button } from "@/components/ui/button"

type Phase = "loading" | "need_login" | "joining" | "done" | "error"

// Landing page for organization invite emails. Requires (or starts) a Privy
// login, then accepts the invite token and sends the user to their new
// organization tab in settings.
export default function OrganizationJoinPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = searchParams.get("token") || ""
  const { ready, authenticated, login } = usePrivy()
  const { authFetch, status } = useApp()

  const [phase, setPhase] = useState<Phase>("loading")
  const [message, setMessage] = useState("")
  const startedRef = useRef(false)

  useEffect(() => {
    if (!ready) return
    if (!token) {
      setMessage("This invite link is missing its token. Ask for a new invitation.")
      setPhase("error")
      return
    }
    if (!authenticated) {
      setPhase("need_login")
      return
    }
    if (status !== "authenticated") return // wait for app auth to settle
    if (startedRef.current) return
    startedRef.current = true

    const join = async () => {
      setPhase("joining")
      try {
        const res = await authFetch("/organizations/join", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ token }),
        })
        if (!res.ok) {
          setMessage((await res.text()) || "Unable to join the organization.")
          setPhase("error")
          return
        }
        setPhase("done")
        setTimeout(() => router.replace("/settings?tab=organization"), 1200)
      } catch {
        setMessage("Unable to join the organization. Please try again.")
        setPhase("error")
      }
    }
    void join()
  }, [ready, authenticated, status, token, authFetch, router])

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <div className="mx-auto w-full max-w-md text-center">
        <div className="rounded-lg border bg-card/95 p-6 shadow-sm">
          <img src="/icon.png" alt="SFLUV" className="mx-auto h-16 w-16 object-contain" />
          <h1 className="mt-4 text-2xl font-bold text-black dark:text-white">Join your organization</h1>

          {(phase === "loading" || phase === "joining") && (
            <p className="mt-3 text-sm text-muted-foreground">Accepting your invitation…</p>
          )}

          {phase === "need_login" && (
            <>
              <p className="mt-3 text-sm text-muted-foreground">
                Sign in (or create your SFLuv account) with the email address this invitation was sent to.
              </p>
              <Button className="mt-5 bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={() => login()}>
                Sign in to continue
              </Button>
            </>
          )}

          {phase === "done" && (
            <p className="mt-3 text-sm text-green-700 dark:text-green-300">You're in! Taking you to your organization…</p>
          )}

          {phase === "error" && <p className="mt-3 text-sm text-[#b42318] dark:text-[#ffb4a8]">{message}</p>}
        </div>
      </div>
    </main>
  )
}
