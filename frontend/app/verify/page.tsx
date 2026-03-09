"use client"

import { useEffect, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Loader2, CheckCircle2, AlertTriangle } from "lucide-react"
import { BACKEND } from "@/lib/constants"

type VerifyState = "loading" | "success" | "expired" | "error"

export default function VerifyEmailPage() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const [state, setState] = useState<VerifyState>("loading")
  const [message, setMessage] = useState("Verifying your email...")

  useEffect(() => {
    const token = searchParams.get("token")?.trim() || ""
    if (!token) {
      setState("error")
      setMessage("Missing verification token.")
      return
    }

    let ignore = false
    const verify = async () => {
      setState("loading")
      setMessage("Verifying your email...")
      try {
        const res = await fetch(`${BACKEND}/users/verified-emails/verify`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ token }),
        })
        if (res.ok) {
          if (ignore) return
          setState("success")
          setMessage("Your email has been verified successfully.")
          return
        }

        const text = (await res.text()).trim()
        if (ignore) return
        if (res.status === 410) {
          setState("expired")
          setMessage(text || "This verification link has expired.")
          return
        }
        setState("error")
        setMessage(text || "Unable to verify this email link.")
      } catch {
        if (ignore) return
        setState("error")
        setMessage("Unable to verify this email link right now.")
      }
    }

    void verify()
    return () => {
      ignore = true
    }
  }, [searchParams])

  return (
    <div className="container mx-auto p-4 min-h-[60vh] flex items-center justify-center">
      <Card className="w-full max-w-xl">
        <CardHeader>
          <CardTitle>Email Verification</CardTitle>
          <CardDescription>Confirm your email address to use it for notifications and role requests.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {state === "loading" && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              <span>{message}</span>
            </div>
          )}

          {state === "success" && (
            <div className="flex items-center gap-2 text-sm text-green-700 bg-green-50 dark:bg-green-900/20 rounded-md p-3">
              <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
              <span>{message}</span>
            </div>
          )}

          {(state === "expired" || state === "error") && (
            <div className="flex items-center gap-2 text-sm text-red-700 bg-red-50 dark:bg-red-900/20 rounded-md p-3">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{message}</span>
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            <Button onClick={() => router.push("/settings")}>Go to Settings</Button>
            <Button variant="outline" onClick={() => router.push("/map")}>
              Back Home
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
