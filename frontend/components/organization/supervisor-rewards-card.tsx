"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { getAddress, isAddress } from "viem"
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { AlertTriangle, Check, Loader2 } from "lucide-react"
import { useApp } from "@/context/AppProvider"
import type { WalletResponse } from "@/types/server"

const CUSTOM_REWARDS_ACCOUNT_VALUE = "__custom__"

// SupervisorRewardsCard is the per-user "primary rewards account" control that
// used to live on the settings supervisor tab. Supervisor workflow rewards pay
// to this account; it's a per-user setting (not org-wide), surfaced inline in
// the organization panel now that the settings sub-tab is retired.
export function SupervisorRewardsCard() {
  const { authFetch, user, supervisor, setSupervisor } = useApp()

  const [wallets, setWallets] = useState<WalletResponse[]>([])
  const [walletsLoading, setWalletsLoading] = useState(true)
  const [current, setCurrent] = useState<string>("")
  const [selection, setSelection] = useState<string>("")
  const [customAccount, setCustomAccount] = useState<string>("")
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState("")
  const [success, setSuccess] = useState("")

  const options = useMemo(() => {
    const seen = new Set<string>()
    const opts: Array<{ value: string; label: string }> = []
    for (const wallet of wallets) {
      if (wallet.is_eoa) continue
      const raw = (wallet.smart_address || "").trim()
      if (!raw || !isAddress(raw)) continue
      const normalized = getAddress(raw)
      const key = normalized.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      const fallback = wallet.smart_index !== undefined ? `Smart Wallet ${wallet.smart_index + 1}` : "Smart Wallet"
      const name = (wallet.name || "").trim() || fallback
      const short = `${normalized.slice(0, 6)}...${normalized.slice(-4)}`
      opts.push({ value: normalized, label: `${name} (${short})` })
    }
    return opts
  }, [wallets])

  const load = useCallback(async () => {
    setWalletsLoading(true)
    try {
      const res = await authFetch("/wallets")
      if (res.ok) setWallets(((await res.json()) as WalletResponse[]) || [])
    } catch {
      // Non-fatal: the selector just starts empty.
    } finally {
      setWalletsLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  // Seed the saved account from context (populated at bootstrap).
  useEffect(() => {
    const account = (supervisor?.primary_rewards_account || "").trim()
    if (account && isAddress(account)) setCurrent(getAddress(account))
  }, [supervisor?.primary_rewards_account])

  // Default (no saved account): prefer the user's primary wallet, else the
  // first smart wallet — mirroring the retired settings tab rather than just
  // grabbing options[0].
  const defaultAccount = useMemo(() => {
    const primary = (user?.primaryWalletAddress || "").trim()
    if (primary && isAddress(primary)) {
      const normalized = getAddress(primary)
      if (options.some((o) => o.value === normalized)) return normalized
    }
    return options[0]?.value || ""
  }, [options, user?.primaryWalletAddress])

  // Seed the selection from the saved account once wallets/options resolve.
  useEffect(() => {
    if (walletsLoading) return
    if (current && options.some((o) => o.value === current)) setSelection(current)
    else if (current) {
      setSelection(CUSTOM_REWARDS_ACCOUNT_VALUE)
      setCustomAccount(current)
    } else setSelection(defaultAccount || CUSTOM_REWARDS_ACCOUNT_VALUE)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [walletsLoading, current, defaultAccount])

  const save = async () => {
    setError("")
    setSuccess("")
    const raw = selection === CUSTOM_REWARDS_ACCOUNT_VALUE ? customAccount.trim() : selection.trim()
    if (!raw) return setError("Primary rewards account is required.")
    if (!isAddress(raw)) return setError("Enter a valid Ethereum address.")
    const normalized = getAddress(raw)
    setSaving(true)
    try {
      const res = await authFetch("/supervisors/primary-rewards-account", {
        method: "PUT",
        body: JSON.stringify({ primary_rewards_account: normalized }),
      })
      if (!res.ok) throw new Error((await res.text()) || "Unable to update supervisor rewards account.")
      const updated = await res.json()
      setSupervisor(updated)
      setCurrent(normalized)
      setSuccess("Primary rewards account updated.")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to update supervisor rewards account.")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="supervisor-rewards-account" className="text-sm font-medium">
          Supervisor rewards account
        </Label>
        <p className="text-xs text-muted-foreground">
          Supervisor workflow rewards are paid to this account. Your primary wallet is used by default until you choose another here.
        </p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Select value={selection || undefined} onValueChange={(v) => { setSelection(v); setError(""); setSuccess("") }}>
          <SelectTrigger id="supervisor-rewards-account" className="flex-1">
            <SelectValue placeholder={walletsLoading ? "Loading accounts…" : "Select an account"} />
          </SelectTrigger>
          <SelectContent>
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
            <SelectItem value={CUSTOM_REWARDS_ACCOUNT_VALUE}>Custom address…</SelectItem>
          </SelectContent>
        </Select>
        <Button className="shrink-0" onClick={() => void save()} disabled={saving || walletsLoading}>
          {saving ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" /> Saving…</> : "Save"}
        </Button>
      </div>
      {selection === CUSTOM_REWARDS_ACCOUNT_VALUE && (
        <Input
          placeholder="0x…"
          value={customAccount}
          onChange={(e) => { setCustomAccount(e.target.value); setError(""); setSuccess("") }}
        />
      )}
      {error && (
        <div className="flex items-center gap-2 rounded-lg bg-red-50 p-2 text-sm text-red-600 dark:bg-red-900/20">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}
      {success && (
        <div className="flex items-center gap-2 text-sm text-green-600 dark:text-green-400">
          <Check className="h-4 w-4 shrink-0" />
          <span>{success}</span>
        </div>
      )}
    </div>
  )
}
