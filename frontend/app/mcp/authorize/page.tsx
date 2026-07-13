"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { useSearchParams } from "next/navigation"
import { usePrivy } from "@privy-io/react-auth"
import { BACKEND } from "@/lib/constants"
import { Button } from "@/components/ui/button"

type Phase = "loading" | "need_login" | "authorizing" | "redirecting" | "error"

// This page is the identity step of the MCP OAuth flow. An admin's AI agent
// sends them here (with a one-time login_state); we log them in via Privy, hand
// their Privy access token + the login_state to the backend, and the backend
// returns the URL to redirect back to the agent with an authorization code.
export default function McpAuthorizePage() {
  const searchParams = useSearchParams()
  const loginState = searchParams.get("login_state") || ""
  const { ready, authenticated, login, getAccessToken } = usePrivy()

  const [phase, setPhase] = useState<Phase>("loading")
  const [message, setMessage] = useState<string>("")
  const startedRef = useRef(false)

  const errorText = (code: string): string => {
    switch (code) {
      case "not_admin":
        return "This SFLUV account is not an admin, so it can't connect an AI agent to admin data."
      case "expired_login":
        return "This authorization request expired. Start the connection again from your AI agent."
      case "login_required":
        return "Your login could not be verified. Please try again."
      case "invalid_redirect":
      case "invalid_request":
        return "This authorization request was malformed. Start again from your AI agent."
      default:
        return "Something went wrong completing authorization. Please try again."
    }
  }

  const complete = useCallback(async () => {
    setPhase("authorizing")
    try {
      const token = await getAccessToken()
      if (!token) {
        setPhase("need_login")
        return
      }
      const res = await fetch(`${BACKEND}/oauth/mcp/complete`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ login_state: loginState }),
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok || !data?.redirect_url) {
        setMessage(errorText(data?.error || ""))
        setPhase("error")
        return
      }
      setPhase("redirecting")
      // Hand control back to the AI agent's redirect URI with the auth code.
      window.location.href = data.redirect_url as string
    } catch {
      setMessage(errorText(""))
      setPhase("error")
    }
  }, [getAccessToken, loginState])

  useEffect(() => {
    if (!ready) return
    if (!loginState) {
      setMessage("Missing authorization request. Start the connection from your AI agent.")
      setPhase("error")
      return
    }
    if (!authenticated) {
      setPhase("need_login")
      return
    }
    if (startedRef.current) return
    startedRef.current = true
    void complete()
  }, [ready, authenticated, loginState, complete])

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <div className="mx-auto w-full max-w-md text-center">
        <div className="rounded-lg border bg-card/95 p-6 shadow-sm">
          <img src="/icon.png" alt="SFLUV" className="mx-auto h-16 w-16 object-contain" />
          <h1 className="mt-4 text-2xl font-bold text-black dark:text-white">Connect your AI agent</h1>

          {(phase === "loading" || phase === "authorizing" || phase === "redirecting") && (
            <p className="mt-3 text-sm text-muted-foreground">
              {phase === "redirecting" ? "Returning you to your AI agent…" : "Authorizing admin access…"}
            </p>
          )}

          {phase === "need_login" && (
            <>
              <p className="mt-3 text-sm text-muted-foreground">
                Sign in with your SFLUV admin account to grant your AI agent read-only access to admin data.
              </p>
              <Button className="mt-5 bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={() => login()}>
                Sign in to continue
              </Button>
            </>
          )}

          {phase === "error" && (
            <p className="mt-3 text-sm text-[#b42318] dark:text-[#ffb4a8]">{message}</p>
          )}

          <p className="mt-5 text-xs text-muted-foreground">
            Your agent will only get read-only access, and only while your account remains an admin.
          </p>
        </div>
      </div>
    </main>
  )
}
