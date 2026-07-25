"use client"

import { useEffect, useState } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ArrowDownToLine, CheckCircle, Landmark, Loader2, Save, ShieldCheck } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { useApp } from "@/context/AppProvider"
import { useChainConfig } from "@/context/ChainConfigProvider"
import { AppWallet } from "@/lib/wallets/wallets"
import { Address, parseUnits } from "viem"

interface AdminCashOutCardProps {
  wallet: AppWallet
  onBalanceChanged?: () => void
}

function isEthereumAddress(value: string): boolean {
  return /^0x[0-9a-fA-F]{40}$/.test(value.trim())
}

// AdminCashOutCard is the admin-account equivalent of the merchant CashOutCard.
// Admins hold no locations, so their liquidation address lives on their user row
// and is read/written through the dedicated /admin/liquidation-address routes
// rather than the per-location merchant endpoints.
export function AdminCashOutCard({ wallet, onBalanceChanged }: AdminCashOutCardProps) {
  const { user, authFetch } = useApp()
  const chainConfig = useChainConfig()
  const { toast } = useToast()

  const [savedAddress, setSavedAddress] = useState("")
  const [addressInput, setAddressInput] = useState("")
  const [loadingAddress, setLoadingAddress] = useState(true)
  const [savingAddress, setSavingAddress] = useState(false)
  const [amountInput, setAmountInput] = useState("")
  const [confirming, setConfirming] = useState(false)
  const [cashingOut, setCashingOut] = useState(false)
  const [lastTxHash, setLastTxHash] = useState<string | null>(null)

  const isAdmin = user?.isAdmin === true

  useEffect(() => {
    if (!isAdmin) return
    let cancelled = false

    const loadAddress = async () => {
      try {
        const res = await authFetch("/admin/liquidation-address")
        if (!res.ok) return
        const body = (await res.json()) as { liquidation_address: string }
        if (cancelled) return
        setSavedAddress(body.liquidation_address ?? "")
        setAddressInput(body.liquidation_address ?? "")
      } catch (error) {
        console.error("error loading admin liquidation address", error)
      } finally {
        if (!cancelled) setLoadingAddress(false)
      }
    }

    void loadAddress()
    return () => {
      cancelled = true
    }
  }, [isAdmin, authFetch])

  if (!isAdmin) return null
  if (wallet.type !== "smartwallet") return null

  const addressDirty = addressInput.trim() !== savedAddress

  const saveAddress = async () => {
    const trimmed = addressInput.trim()
    if (trimmed !== "" && !isEthereumAddress(trimmed)) {
      toast({
        title: "Invalid address",
        description: "The liquidation address must be a valid 0x address.",
        variant: "destructive",
      })
      return
    }

    setSavingAddress(true)
    try {
      const res = await authFetch("/admin/liquidation-address", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ liquidation_address: trimmed }),
      })
      if (!res.ok) {
        const message = await res.text()
        throw new Error(message || `Request failed with status ${res.status}`)
      }
      const body = (await res.json()) as { liquidation_address: string }
      setSavedAddress(body.liquidation_address)
      setAddressInput(body.liquidation_address)
      toast({
        title: body.liquidation_address
          ? "Admin liquidation address saved"
          : "Admin liquidation address cleared",
      })
    } catch (error) {
      console.error("error saving admin liquidation address", error)
      toast({
        title: "Unable to save liquidation address",
        description: error instanceof Error ? error.message : "Please try again.",
        variant: "destructive",
      })
    } finally {
      setSavingAddress(false)
    }
  }

  const startCashOut = () => {
    setLastTxHash(null)

    if (!savedAddress) {
      toast({
        title: "No liquidation address",
        description: "Save your Bridge liquidation address before cashing out.",
        variant: "destructive",
      })
      return
    }
    if (addressDirty) {
      toast({
        title: "Unsaved address",
        description: "Save the liquidation address before cashing out.",
        variant: "destructive",
      })
      return
    }

    let amountWei: bigint
    try {
      amountWei = parseUnits(amountInput, chainConfig.tokenDecimals)
    } catch {
      toast({ title: "Invalid amount", variant: "destructive" })
      return
    }
    if (amountWei <= 0n) {
      toast({ title: "Enter an amount greater than zero", variant: "destructive" })
      return
    }

    setConfirming(true)
  }

  const executeCashOut = async () => {
    if (!wallet.address || !savedAddress) return

    let amountWei: bigint
    try {
      amountWei = parseUnits(amountInput, chainConfig.tokenDecimals)
    } catch {
      setConfirming(false)
      return
    }

    setCashingOut(true)
    try {
      const eligibilityRes = await authFetch("/unwrap/eligibility", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          wallet_address: wallet.address,
          amount_wei: amountWei.toString(),
        }),
      })
      if (!eligibilityRes.ok) {
        let reason = "This wallet is not eligible to cash out right now."
        try {
          const body = (await eligibilityRes.json()) as { reason?: string }
          if (body.reason) reason = body.reason
        } catch {
          // keep the default reason
        }
        toast({ title: "Cash out not allowed", description: reason, variant: "destructive" })
        return
      }

      const receipt = await wallet.cashOut(amountWei, savedAddress as Address)
      if (!receipt || receipt.error || !receipt.hash) {
        toast({
          title: "Cash out failed",
          description: receipt?.error ?? "Unable to submit the transaction.",
          variant: "destructive",
        })
        return
      }

      setLastTxHash(receipt.hash)

      const recordRes = await authFetch("/unwrap/record", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          wallet_address: wallet.address,
          tx_hash: receipt.hash,
          amount_wei: amountWei.toString(),
          destination_address: savedAddress,
        }),
      })
      if (!recordRes.ok) {
        console.error("error recording admin unwrap", recordRes.status)
      }

      toast({
        title: "Cash out submitted",
        description: `${amountInput} ${chainConfig.tokenSymbol} is on its way to your bank as USDC.`,
      })
      setAmountInput("")
      onBalanceChanged?.()
    } catch (error) {
      console.error("error cashing out", error)
      toast({
        title: "Cash out failed",
        description: error instanceof Error ? error.message : "Please try again.",
        variant: "destructive",
      })
    } finally {
      setCashingOut(false)
      setConfirming(false)
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" />
          Admin Cash Out to Bank
        </CardTitle>
        <CardDescription>
          Unwraps SFLUV to USDC and sends it to this admin account&apos;s Bridge liquidation address,
          which settles to the linked bank account.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="admin-liquidation-address">Admin liquidation address</Label>
          <div className="flex gap-2">
            <Input
              id="admin-liquidation-address"
              placeholder={loadingAddress ? "Loading…" : "0x…"}
              value={addressInput}
              onChange={(event) => setAddressInput(event.target.value)}
              disabled={loadingAddress}
              spellCheck={false}
              autoComplete="off"
            />
            <Button
              variant="outline"
              size="icon"
              onClick={saveAddress}
              disabled={savingAddress || loadingAddress || !addressDirty}
              aria-label="Save admin liquidation address"
            >
              {savingAddress ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Save className="h-4 w-4" />
              )}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Applies to this admin account only, separate from any merchant location. Double-check it
            — funds sent to a wrong address cannot be recovered.
          </p>
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="admin-cash-out-amount">Amount ({chainConfig.tokenSymbol})</Label>
          <Input
            id="admin-cash-out-amount"
            type="number"
            inputMode="decimal"
            min="0"
            placeholder="0.00"
            value={amountInput}
            onChange={(event) => setAmountInput(event.target.value)}
          />
        </div>

        {confirming ? (
          <div className="space-y-2 rounded-md border p-3">
            <p className="text-sm">
              Send{" "}
              <span className="font-semibold">
                {amountInput} {chainConfig.tokenSymbol}
              </span>{" "}
              as USDC to
            </p>
            <p className="break-all font-mono text-xs">{savedAddress}</p>
            <div className="flex gap-2 pt-1">
              <Button onClick={executeCashOut} disabled={cashingOut} className="flex-1">
                {cashingOut ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <CheckCircle className="mr-2 h-4 w-4" />
                )}
                Confirm cash out
              </Button>
              <Button variant="outline" onClick={() => setConfirming(false)} disabled={cashingOut}>
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <Button onClick={startCashOut} className="w-full" disabled={cashingOut || loadingAddress}>
            <ArrowDownToLine className="mr-2 h-4 w-4" />
            <Landmark className="mr-2 h-4 w-4" />
            Cash Out
          </Button>
        )}

        {lastTxHash && (
          <p className="break-all text-xs text-muted-foreground">
            Submitted: <span className="font-mono">{lastTxHash}</span>
          </p>
        )}
      </CardContent>
    </Card>
  )
}
