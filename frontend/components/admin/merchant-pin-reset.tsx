"use client"

import { useEffect, useState } from "react"
import { Loader2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useApp } from "@/context/AppProvider"

/**
 * Sets a merchant's merchant-mode PIN for them, for when they have forgotten it.
 * An empty box asks the server for a random one. Results show inline, since this
 * app renders no toasts.
 */
export function MerchantPinReset({ userId }: { userId: string }) {
  const { authFetch } = useApp()
  const [pinInput, setPinInput] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<{ pin: string; generated: boolean } | null>(null)

  // A different merchant must never see the last one's PIN.
  useEffect(() => {
    setPinInput("")
    setError("")
    setResult(null)
  }, [userId])

  const reset = async () => {
    const pin = pinInput.trim()
    if (pin && !/^\d{6}$/.test(pin)) {
      setError("The PIN must be exactly 6 digits.")
      return
    }
    setBusy(true)
    setError("")
    setResult(null)
    try {
      const res = await authFetch(`/admin/users/${encodeURIComponent(userId)}/merchant-mode-pin`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(pin ? { pin } : {}),
      })
      if (!res.ok) {
        const text = (await res.text().catch(() => "")).trim()
        throw new Error(text || `Couldn't reset the PIN (error ${res.status}).`)
      }
      setResult((await res.json()) as { pin: string; generated: boolean })
      setPinInput("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Couldn't reset the PIN.")
    } finally {
      setBusy(false)
    }
  }

  const inputId = `merchant-pin-${userId}`

  return (
    <div className="space-y-3 rounded-md border p-4">
      <h4 className="text-sm font-semibold">Merchant mode PIN</h4>
      <p className="text-xs text-muted-foreground">
        For a merchant who forgot their PIN. This sets a new PIN without the old one and clears any lockout.
        Leave the box empty to generate a random PIN.
      </p>
      <div className="flex flex-wrap items-end gap-2">
        <div className="space-y-1">
          <Label htmlFor={inputId}>New PIN (optional)</Label>
          <Input
            id={inputId}
            inputMode="numeric"
            autoComplete="off"
            placeholder="6 digits"
            className="w-36 font-mono"
            value={pinInput}
            onChange={(e) => setPinInput(e.target.value.replace(/\D/g, "").slice(0, 6))}
            disabled={busy}
          />
        </div>
        <Button type="button" onClick={() => void reset()} disabled={busy}>
          {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {pinInput ? "Set PIN" : "Generate new PIN"}
        </Button>
      </div>
      {error && <p className="text-sm text-red-600">{error}</p>}
      {result && (
        <p className="text-sm">
          New PIN: <span className="font-mono font-semibold">{result.pin}</span>. It works right away;
          give it to the merchant now, because it isn&apos;t shown again.
        </p>
      )}
    </div>
  )
}
