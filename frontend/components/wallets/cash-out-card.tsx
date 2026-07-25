"use client"

import { useEffect, useMemo, useState } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ArrowDownToLine, CheckCircle, Landmark, Loader2, Save } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { useApp } from "@/context/AppProvider"
import { useChainConfig } from "@/context/ChainConfigProvider"
import { AppWallet } from "@/lib/wallets/wallets"
import { Address, parseUnits } from "viem"

interface CashOutCardProps {
  wallet: AppWallet
  onBalanceChanged?: () => void
}

function isEthereumAddress(value: string): boolean {
  return /^0x[0-9a-fA-F]{40}$/.test(value.trim())
}

// CashOutCard lets a merchant (or admin) store a Bridge liquidation address per
// location and cash out SFLUV: withdrawTo burns the SFLUV and sends the
// underlying USDC to the liquidation address, which settles to their bank.
// Replaces the old PayPal cash-out route.
export function CashOutCard({ wallet, onBalanceChanged }: CashOutCardProps) {
  const { user, userLocations, setUserLocations, authFetch } = useApp()
  const chainConfig = useChainConfig()
  const { toast } = useToast()

  const [selectedLocationId, setSelectedLocationId] = useState<string>("")
  const [addressInput, setAddressInput] = useState("")
  const [savingAddress, setSavingAddress] = useState(false)
  const [amountInput, setAmountInput] = useState("")
  const [confirming, setConfirming] = useState(false)
  const [cashingOut, setCashingOut] = useState(false)
  const [lastTxHash, setLastTxHash] = useState<string | null>(null)

  const locations = useMemo(
    () => userLocations.filter((location) => location.approval === true),
    [userLocations],
  )

  const selectedLocation = useMemo(
    () => locations.find((location) => String(location.id) === selectedLocationId) ?? null,
    [locations, selectedLocationId],
  )

  useEffect(() => {
    if (!selectedLocationId && locations.length > 0) {
      setSelectedLocationId(String(locations[0].id))
    }
  }, [locations, selectedLocationId])

  useEffect(() => {
    setAddressInput(selectedLocation?.liquidation_address ?? "")
    setConfirming(false)
  }, [selectedLocation])

  if (user?.isMerchant !== true && user?.isAdmin !== true) return null
  if (wallet.type !== "smartwallet") return null
  if (locations.length === 0) return null

  const savedAddress = selectedLocation?.liquidation_address ?? ""
  const addressDirty = addressInput.trim() !== savedAddress

  const saveAddress = async () => {
    if (!selectedLocation) return
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
      const res = await authFetch(`/locations/${selectedLocation.id}/liquidation-address`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ liquidation_address: trimmed }),
      })
      if (!res.ok) {
        const message = await res.text()
        throw new Error(message || `Request failed with status ${res.status}`)
      }
      const body = (await res.json()) as { liquidation_address: string }
      setUserLocations((current) =>
        current.map((location) =>
          location.id === selectedLocation.id
            ? { ...location, liquidation_address: body.liquidation_address }
            : location,
        ),
      )
      setAddressInput(body.liquidation_address)
      toast({
        title: body.liquidation_address ? "Liquidation address saved" : "Liquidation address cleared",
        description: body.liquidation_address
          ? "Cash outs from this location will send USDC to this address."
          : undefined,
      })
    } catch (error) {
      console.error("error saving liquidation address", error)
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
    if (!wallet.address || !selectedLocation || !savedAddress) return

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
          location_id: selectedLocation.id,
        }),
      })
      if (!recordRes.ok) {
        console.error("error recording unwrap", recordRes.status)
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
          <Landmark className="h-4 w-4" />
          Cash Out to Bank
        </CardTitle>
        <CardDescription>
          Unwraps SFLUV to USDC and sends it to your Bridge liquidation address, which settles to
          your bank account.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {locations.length > 1 && (
          <div className="space-y-1.5">
            <Label htmlFor="cash-out-location">Location</Label>
            <Select value={selectedLocationId} onValueChange={setSelectedLocationId}>
              <SelectTrigger id="cash-out-location">
                <SelectValue placeholder="Select a location" />
              </SelectTrigger>
              <SelectContent>
                {locations.map((location) => (
                  <SelectItem key={location.id} value={String(location.id)}>
                    {location.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        <div className="space-y-1.5">
          <Label htmlFor="liquidation-address">Liquidation address</Label>
          <div className="flex gap-2">
            <Input
              id="liquidation-address"
              placeholder="0x…"
              value={addressInput}
              onChange={(event) => setAddressInput(event.target.value)}
              spellCheck={false}
              autoComplete="off"
            />
            <Button
              variant="outline"
              size="icon"
              onClick={saveAddress}
              disabled={savingAddress || !addressDirty}
              aria-label="Save liquidation address"
            >
              {savingAddress ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Get this address from your Bridge account after completing business verification in
            Settings. Double-check it — funds sent to a wrong address cannot be recovered.
          </p>
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="cash-out-amount">Amount ({chainConfig.tokenSymbol})</Label>
          <Input
            id="cash-out-amount"
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
              Send <span className="font-semibold">{amountInput} {chainConfig.tokenSymbol}</span> as
              USDC to
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
          <Button onClick={startCashOut} className="w-full" disabled={cashingOut}>
            <ArrowDownToLine className="mr-2 h-4 w-4" />
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
