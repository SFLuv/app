"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ArrowDownToLine, CheckCircle2, ExternalLink, Landmark, Loader2, RefreshCw, ShieldCheck } from "lucide-react"
import { formatUnits, parseUnits, type Address } from "viem"
import { useToast } from "@/hooks/use-toast"
import { useApp } from "@/context/AppProvider"
import { useChainConfig } from "@/context/ChainConfigProvider"
import type { AuthedLocation } from "@/types/location"
import type { AppWallet, TxState } from "@/lib/wallets/wallets"
import { openPlaidLink, PlaidExitError } from "@/lib/bridge/plaid"
import {
  kybStatusLabel,
  unwrapStatusLabel,
  type MerchantBankAccount,
  type MerchantPayoutStatusResponse,
  type ProvisionResponse,
} from "@/types/merchant-payout"

interface LocationPayoutCardProps {
  location: AuthedLocation
  /** Called after an unwrap lands, so a host page can refresh balances it owns. */
  onUnwrapped?: () => void | Promise<void>
}

// LocationPayoutCard is the "Bank & Unwrap" section of a location's settings.
//
// The merchant sees three states in order — verify the business, connect a
// bank, unwrap — and never a crypto address. The destination is provisioned by
// the backend from Bridge the moment a bank is linked, so there is nothing for
// them to type and nothing to get wrong. Unwrapping is signed by the
// location's own wallet; tips, if the location has a tipping wallet, can be
// unwrapped in the same go as a second transaction from that wallet.
export function LocationPayoutCard({ location, onUnwrapped }: LocationPayoutCardProps) {
  const { user, wallets, authFetch } = useApp()
  const chainConfig = useChainConfig()
  const { toast } = useToast()

  const [status, setStatus] = useState<MerchantPayoutStatusResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<"" | "kyb" | "plaid" | "provision" | "bank" | "unwrap">("")

  const [amountInput, setAmountInput] = useState("")
  const [includeTips, setIncludeTips] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [tillBalance, setTillBalance] = useState<bigint | null>(null)
  const [tipBalance, setTipBalance] = useState<bigint | null>(null)

  const payToAddress = (location.pay_to_address || "").trim()
  const tipToAddress = (location.tip_to_address || "").trim()

  const tillWallet = useMemo<AppWallet | undefined>(
    () => wallets.find((w) => w.type === "smartwallet" && w.address?.toLowerCase() === payToAddress.toLowerCase()),
    [wallets, payToAddress],
  )
  const tipWallet = useMemo<AppWallet | undefined>(
    () =>
      tipToAddress && tipToAddress.toLowerCase() !== payToAddress.toLowerCase()
        ? wallets.find((w) => w.type === "smartwallet" && w.address?.toLowerCase() === tipToAddress.toLowerCase())
        : undefined,
    [wallets, tipToAddress, payToAddress],
  )

  const loadStatus = useCallback(async () => {
    try {
      const res = await authFetch("/merchant/payout/status")
      if (!res.ok) throw new Error(`status ${res.status}`)
      setStatus((await res.json()) as MerchantPayoutStatusResponse)
    } catch (error) {
      console.error("merchant payout status", error)
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void loadStatus()
  }, [loadStatus])

  useEffect(() => {
    let cancelled = false
    const read = async () => {
      if (tillWallet) {
        const b = await tillWallet.getBalance(chainConfig.tokenAddress as Address)
        if (!cancelled) setTillBalance(b)
      }
      if (tipWallet) {
        const b = await tipWallet.getBalance(chainConfig.tokenAddress as Address)
        if (!cancelled) setTipBalance(b)
      }
    }
    void read()
    return () => {
      cancelled = true
    }
  }, [tillWallet, tipWallet, chainConfig.tokenAddress, status])

  if (user?.isMerchant !== true) return null

  const profile = status?.profile ?? null
  const kyb = kybStatusLabel(profile?.kyb_status)
  const banks: MerchantBankAccount[] = status?.bank_accounts ?? []
  const liquidation = status?.locations.find((l) => Number(l.location_id) === Number(location.id)) ?? null
  const currentBank = liquidation
    ? banks.find((b) => b.bridge_external_account_id === liquidation.bridge_external_account_id) ?? null
    : null
  const recentUnwraps = (status?.unwraps ?? []).filter((u) => Number(u.location_id) === Number(location.id)).slice(0, 5)

  const fmt = (wei: bigint | null | undefined) =>
    wei === null || wei === undefined ? "—" : `${Number(formatUnits(wei, chainConfig.tokenDecimals)).toLocaleString(undefined, { maximumFractionDigits: 2 })} ${chainConfig.tokenSymbol}`

  // --- actions ---------------------------------------------------------------

  const startKYB = async () => {
    setBusy("kyb")
    try {
      const res = await authFetch("/merchant/payout/kyb-link", { method: "POST" })
      const body = (await res.json().catch(() => ({}))) as { url?: string; error?: string }
      if (!res.ok || !body.url) throw new Error(body.error || "Could not start verification")
      window.open(body.url, "_blank", "noopener")
      toast({ title: "Verification opened in a new tab", description: "Come back here when you're done — this page will update." })
    } catch (error) {
      toast({ title: "Couldn't start verification", description: error instanceof Error ? error.message : undefined, variant: "destructive" })
    } finally {
      setBusy("")
    }
  }

  const connectBank = async () => {
    setBusy("plaid")
    try {
      const tokenRes = await authFetch("/merchant/payout/plaid/link-token", { method: "POST" })
      const tokenBody = (await tokenRes.json().catch(() => ({}))) as { link_token?: string; error?: string }
      if (!tokenRes.ok || !tokenBody.link_token) throw new Error(tokenBody.error || "Could not start the bank connection")

      const { publicToken } = await openPlaidLink(tokenBody.link_token)

      const exchangeRes = await authFetch("/merchant/payout/plaid/exchange", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ link_token: tokenBody.link_token, public_token: publicToken }),
      })
      const exchangeBody = (await exchangeRes.json().catch(() => ({}))) as ProvisionResponse & { error?: string }
      if (!exchangeRes.ok) throw new Error(exchangeBody.error || "The bank connection could not be completed")

      toast({
        title: "Bank connected",
        description: exchangeBody.message || "Your payout address is set up. You can unwrap now.",
      })
      await loadStatus()
    } catch (error) {
      if (error instanceof PlaidExitError) return
      toast({ title: "Bank connection failed", description: error instanceof Error ? error.message : undefined, variant: "destructive" })
    } finally {
      setBusy("")
    }
  }

  const finishSetup = async () => {
    setBusy("provision")
    try {
      // Scoped to THIS location. Setting up one shop no longer routes every
      // other shop's takings to the same bank — a second location asks for its
      // own bank even when it is the same account.
      const res = await authFetch("/merchant/payout/provision", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ location_id: location.id }),
      })
      const body = (await res.json().catch(() => ({}))) as ProvisionResponse & { error?: string }
      if (!res.ok) throw new Error(body.error || "Setup could not be completed")
      if (body.message) toast({ title: body.message })
      await loadStatus()
    } catch (error) {
      toast({ title: "Setup failed", description: error instanceof Error ? error.message : undefined, variant: "destructive" })
    } finally {
      setBusy("")
    }
  }

  const changeBank = async (externalAccountId: string) => {
    if (!externalAccountId || externalAccountId === liquidation?.bridge_external_account_id) return
    setBusy("bank")
    try {
      const res = await authFetch(`/locations/${location.id}/payout-bank`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ bridge_external_account_id: externalAccountId }),
      })
      const body = (await res.json().catch(() => ({}))) as { error?: string }
      if (!res.ok) throw new Error(body.error || "Could not change the payout bank")
      toast({ title: "Payout bank updated" })
      await loadStatus()
    } catch (error) {
      toast({ title: "Couldn't change bank", description: error instanceof Error ? error.message : undefined, variant: "destructive" })
    } finally {
      setBusy("")
    }
  }

  const parsedAmount = (() => {
    try {
      const v = parseUnits(amountInput || "0", chainConfig.tokenDecimals)
      return v > 0n ? v : null
    } catch {
      return null
    }
  })()

  const openConfirm = async () => {
    if (!tillWallet || !liquidation || !parsedAmount || !tillWallet.address) return
    if (tillBalance !== null && parsedAmount > tillBalance) {
      toast({ title: "Not enough SFLUV in this location's wallet", variant: "destructive" })
      return
    }
    setBusy("unwrap")
    try {
      // Sweeping the till and its tips is one redemption against the
      // location's monthly allowance, so the check is on what leaves in
      // total — not on the till alone, which would let a merchant pass a
      // small till amount and then send the tips as a second redemption.
      const sweepingTips = includeTips && tipBalance !== null && tipBalance > 0n
      const redemptionTotal = parsedAmount + (sweepingTips ? (tipBalance as bigint) : 0n)
      const res = await authFetch("/unwrap/eligibility", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          wallet_address: tillWallet.address,
          amount_wei: redemptionTotal.toString(),
          location_id: location.id,
        }),
      })
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { reason?: string }
        toast({ title: "Can't unwrap right now", description: body.reason || "This wallet is not eligible to unwrap.", variant: "destructive" })
        return
      }
      setConfirmOpen(true)
    } finally {
      setBusy("")
    }
  }

  const record = async (wallet: AppWallet, role: "payment" | "tipping", amount: bigint, receipt: TxState) => {
    if (!receipt.hash || !wallet.address || !liquidation) return
    // The bundler's hash is the user operation's, not the transaction's.
    // Prefer the real one so the ledger and explorer links line up with
    // what Bridge sees; the sweep can still reconcile by amount and time
    // if this lookup comes back empty.
    const txHash = (await wallet.findUnwrapTxHash(liquidation.address as Address, amount)) ?? receipt.hash
    await authFetch("/unwrap/record", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        wallet_address: wallet.address,
        tx_hash: txHash,
        amount_wei: amount.toString(),
        destination_address: liquidation.address,
        location_id: location.id,
        wallet_role: role,
      }),
    })
  }

  const executeUnwrap = async () => {
    if (!tillWallet || !liquidation || !parsedAmount) return
    setBusy("unwrap")
    setConfirmOpen(false)
    try {
      const to = liquidation.address as Address
      const tillReceipt = await tillWallet.cashOut(parsedAmount, to)
      if (!tillReceipt || tillReceipt.error || !tillReceipt.hash) {
        throw new Error(tillReceipt?.error || "The unwrap transaction could not be submitted")
      }
      await record(tillWallet, "payment", parsedAmount, tillReceipt)

      let tipsNote = ""
      if (includeTips && tipWallet && tipBalance && tipBalance > 0n) {
        const tipReceipt = await tipWallet.cashOut(tipBalance, to)
        if (!tipReceipt || tipReceipt.error || !tipReceipt.hash) {
          tipsNote = ` Tips were not unwrapped: ${tipReceipt?.error || "transaction failed"}.`
        } else {
          await record(tipWallet, "tipping", tipBalance, tipReceipt)
          tipsNote = ` Tips (${fmt(tipBalance)}) are on their way too.`
        }
      }

      toast({
        title: "Unwrap submitted",
        description: `${fmt(parsedAmount)} is on its way to ${currentBank ? `${currentBank.bank_name} ····${currentBank.last_4}` : "your bank"}. Expect it in 1–2 business days.${tipsNote}`,
      })
      setAmountInput("")
      setIncludeTips(false)
      await loadStatus()
      await onUnwrapped?.()
    } catch (error) {
      toast({ title: "Unwrap failed", description: error instanceof Error ? error.message : undefined, variant: "destructive" })
    } finally {
      setBusy("")
    }
  }

  // --- render ------------------------------------------------------------------

  const header = (
    <div className="flex items-start justify-between gap-3">
      <div className="space-y-1">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-black dark:text-white">
          <Landmark className="h-4 w-4" />
          Bank &amp; Unwrap
        </h3>
        <p className="text-xs text-muted-foreground">
          Unwrap this location&apos;s SFLuv straight to your business bank account.
        </p>
      </div>
      <Button type="button" size="icon" variant="ghost" onClick={() => void loadStatus()} aria-label="Refresh" disabled={loading}>
        <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
      </Button>
    </div>
  )

  if (loading) {
    return (
      <div className="rounded-xl border bg-background/70 p-4">
        {header}
        <div className="mt-4 flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Checking payout status…
        </div>
      </div>
    )
  }

  if (!status?.enabled) {
    return (
      <div className="rounded-xl border bg-background/70 p-4">
        {header}
        <p className="mt-4 rounded-lg border border-dashed px-3 py-4 text-sm text-muted-foreground">
          Bank payouts aren&apos;t available yet. Check back soon.
        </p>
      </div>
    )
  }

  // Step 1 — verify the business.
  if (!kyb.done) {
    return (
      <div className="rounded-xl border bg-background/70 p-4">
        {header}
        <div className="mt-4 space-y-3">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <ShieldCheck className="h-4 w-4 text-muted-foreground" />
            <span className="text-muted-foreground">Business verification:</span>
            <span className={`rounded-full border px-2 py-0.5 text-xs ${kyb.failed ? "border-red-300 text-red-600" : ""}`}>{kyb.label}</span>
          </div>
          <p className="text-xs text-muted-foreground">
            A one-time verification with Bridge, our banking partner. It asks for your business details and an ID for the owner,
            and takes a few minutes. Approval usually comes within a business day.
          </p>
          <Button type="button" onClick={() => void startKYB()} disabled={busy !== ""}>
            {busy === "kyb" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <ExternalLink className="mr-2 h-4 w-4" />}
            {profile?.bridge_customer_id ? "Continue verification" : "Verify your business"}
          </Button>
          {status.production === false && <p className="text-[11px] text-muted-foreground">Sandbox mode — no real bank activity.</p>}
        </div>
      </div>
    )
  }

  // Step 2 — connect a bank.
  if (banks.length === 0) {
    return (
      <div className="rounded-xl border bg-background/70 p-4">
        {header}
        <div className="mt-4 space-y-3">
          <div className="flex items-center gap-2 text-sm">
            <CheckCircle2 className="h-4 w-4 text-green-600" />
            <span>Business verified</span>
          </div>
          <p className="text-xs text-muted-foreground">
            Connect the bank account you want payouts in. You&apos;ll log in to your bank securely through Plaid — we never see your account number.
          </p>
          <Button type="button" onClick={() => void connectBank()} disabled={busy !== ""}>
            {busy === "plaid" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Landmark className="mr-2 h-4 w-4" />}
            Connect bank account
          </Button>
        </div>
      </div>
    )
  }

  // Step 2b — bank linked but this location has no destination yet.
  if (!liquidation) {
    return (
      <div className="rounded-xl border bg-background/70 p-4">
        {header}
        <div className="mt-4 space-y-3">
          {/* Deliberately its own step even when the business already banks
              with us elsewhere. Payouts are attached per location, so a second
              shop names its bank rather than inheriting the first one's. */}
          <p className="text-sm text-muted-foreground">
            Your bank is connected. Payouts are set up per location — connect this one to finish.
          </p>
          <Button type="button" onClick={() => void finishSetup()} disabled={busy !== ""}>
            {busy === "provision" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <CheckCircle2 className="mr-2 h-4 w-4" />}
            Finish payout setup
          </Button>
        </div>
      </div>
    )
  }

  // Step 3 — unwrap.
  const canUnwrap = Boolean(tillWallet && parsedAmount && busy === "")
  return (
    <div className="rounded-xl border bg-background/70 p-4">
      {header}

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">Payout bank</Label>
          {banks.length > 1 ? (
            <Select value={liquidation.bridge_external_account_id} onValueChange={(v) => void changeBank(v)} disabled={busy !== ""}>
              <SelectTrigger>
                <SelectValue placeholder="Choose a bank" />
              </SelectTrigger>
              <SelectContent>
                {banks.map((b) => (
                  <SelectItem key={b.bridge_external_account_id} value={b.bridge_external_account_id}>
                    {b.bank_name || "Bank"} ····{b.last_4}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <p className="text-sm font-medium text-black dark:text-white">
              {currentBank ? `${currentBank.bank_name || "Bank"} ····${currentBank.last_4}` : "Connected"}
            </p>
          )}
          <button type="button" className="text-xs text-[#eb6c6c] hover:underline" onClick={() => void connectBank()} disabled={busy !== ""}>
            + Connect another bank
          </button>
        </div>

        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">Available to unwrap</Label>
          <p className="text-sm font-medium text-black dark:text-white">{fmt(tillBalance)}</p>
          {tipWallet && <p className="text-xs text-muted-foreground">Tips: {fmt(tipBalance)}</p>}
          {!tillWallet && <p className="text-xs text-red-600">This location&apos;s wallet isn&apos;t loaded in this session.</p>}
        </div>
      </div>

      <div className="mt-4 space-y-3">
        <div className="space-y-1.5">
          <Label htmlFor={`unwrap-amount-${location.id}`}>Amount ({chainConfig.tokenSymbol})</Label>
          <div className="flex gap-2">
            <Input
              id={`unwrap-amount-${location.id}`}
              type="number"
              inputMode="decimal"
              min="0"
              placeholder="0.00"
              value={amountInput}
              onChange={(e) => setAmountInput(e.target.value)}
              disabled={busy !== ""}
            />
            <Button
              type="button"
              variant="outline"
              onClick={() => tillBalance !== null && setAmountInput(formatUnits(tillBalance, chainConfig.tokenDecimals))}
              disabled={busy !== "" || tillBalance === null || tillBalance === 0n}
            >
              Max
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            This location's first redemption each month can be any amount; further redemptions in the same month must be at least $500. Sweeping tips with the till counts as one redemption. No fees are taken from your payout.
          </p>
        </div>

        {tipWallet && tipBalance !== null && tipBalance > 0n && (
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={includeTips} onCheckedChange={(v) => setIncludeTips(v === true)} disabled={busy !== ""} />
            Also unwrap all tips ({fmt(tipBalance)})
          </label>
        )}

        <Button type="button" className="w-full sm:w-auto" onClick={() => void openConfirm()} disabled={!canUnwrap}>
          {busy === "unwrap" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <ArrowDownToLine className="mr-2 h-4 w-4" />}
          Unwrap
        </Button>
      </div>

      {recentUnwraps.length > 0 && (
        <div className="mt-5 space-y-2">
          <p className="text-xs font-medium text-muted-foreground">Recent unwraps</p>
          <ul className="divide-y rounded-lg border">
            {recentUnwraps.map((u) => {
              const s = unwrapStatusLabel(u.status)
              return (
                <li key={u.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                  <div className="min-w-0">
                    <p className="font-medium text-black dark:text-white">
                      {fmt(BigInt(u.amount_wei))}
                      {u.wallet_role === "tipping" && <span className="ml-1 text-xs text-muted-foreground">(tips)</span>}
                    </p>
                    <p className="text-xs text-muted-foreground">{new Date(u.created_at).toLocaleDateString()}</p>
                  </div>
                  <span
                    className={`rounded-full border px-2 py-0.5 text-xs ${
                      s.tone === "ok" ? "border-green-300 text-green-700" : s.tone === "bad" ? "border-red-300 text-red-600" : ""
                    }`}
                  >
                    {s.label}
                  </span>
                </li>
              )
            })}
          </ul>
        </div>
      )}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Unwrap {parsedAmount ? fmt(parsedAmount) : ""}?</DialogTitle>
            <DialogDescription>
              This converts SFLuv to US dollars and sends it to{" "}
              <span className="font-medium">{currentBank ? `${currentBank.bank_name || "your bank"} ····${currentBank.last_4}` : "your bank"}</span>.
              It usually arrives in 1–2 business days and can&apos;t be reversed.
              {includeTips && tipBalance ? ` Tips (${fmt(tipBalance)}) will be unwrapped in a second transaction.` : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setConfirmOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={() => void executeUnwrap()}>
              Confirm unwrap
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
