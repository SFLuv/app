"use client"

import { useCallback, useEffect, useState } from "react"
import { ExternalLink, Loader2, RefreshCw } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import {
  kybStatusLabel,
  unwrapStatusLabel,
  type AdminMerchantPayoutBusiness,
  type AdminMerchantPayoutsResponse,
  type MerchantOwnerCandidate,
  type Unwrap,
} from "@/types/merchant-payout"

// Amounts in the ledger are token base units; SFLUV and USDC are both 6 dp.
const TOKEN_DECIMALS = 6

function formatAmount(wei: string): string {
  try {
    const value = BigInt(wei)
    const whole = value / BigInt(10 ** TOKEN_DECIMALS)
    const frac = (value % BigInt(10 ** TOKEN_DECIMALS)).toString().padStart(TOKEN_DECIMALS, "0").slice(0, 2)
    return `$${whole.toString()}.${frac}`
  } catch {
    return wei
  }
}

function shortHex(value: string, keep = 6): string {
  if (!value || value.length <= keep * 2 + 2) return value
  return `${value.slice(0, keep + 2)}…${value.slice(-keep)}`
}

function shortDid(did: string): string {
  return did.replace(/^did:privy:/, "")
}

export function MerchantPayoutsPanel() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [loading, setLoading] = useState(true)
  const [businesses, setBusinesses] = useState<AdminMerchantPayoutBusiness[]>([])
  const [unwraps, setUnwraps] = useState<Unwrap[]>([])
  const [disabledReason, setDisabledReason] = useState<string | null>(null)

  const [attachEmail, setAttachEmail] = useState("")
  const [attachOwner, setAttachOwner] = useState("")
  const [showOwnerId, setShowOwnerId] = useState(false)
  const [attachCustomer, setAttachCustomer] = useState("")
  const [attaching, setAttaching] = useState(false)
  // Set when an email matches several accounts; the admin picks one and we
  // resubmit with the owner id instead of guessing.
  const [candidates, setCandidates] = useState<MerchantOwnerCandidate[]>([])

  const [overrideDrafts, setOverrideDrafts] = useState<Record<number, string>>({})
  const [overriding, setOverriding] = useState<number | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await authFetch("/admin/merchant-payouts")
      if (res.status === 503) {
        setDisabledReason("Bridge payouts are not configured on this server (no BRIDGE_API_KEY).")
        setBusinesses([])
        setUnwraps([])
        return
      }
      if (!res.ok) throw new Error((await res.text()) || "Unable to load merchant payouts.")
      const data = (await res.json()) as AdminMerchantPayoutsResponse
      setDisabledReason(null)
      setBusinesses(data.businesses || [])
      setUnwraps(data.unwraps || [])
    } catch (err) {
      toast({
        title: "Could not load merchant payouts",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setLoading(false)
    }
  }, [authFetch, toast])

  useEffect(() => {
    void load()
  }, [load])

  const attach = async (ownerOverride?: string) => {
    const owner = (ownerOverride ?? attachOwner).trim()
    const email = attachEmail.trim()
    const customer = attachCustomer.trim()
    if ((!owner && !email) || !customer) {
      toast({ title: "Enter the merchant's email and the Bridge customer id", variant: "destructive" })
      return
    }
    setAttaching(true)
    try {
      const res = await authFetch("/admin/merchant-payouts/attach-customer", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(owner ? { owner_id: owner, bridge_customer_id: customer } : { owner_email: email, bridge_customer_id: customer }),
      })
      if (res.status === 409) {
        const body = await res.json()
        setCandidates(body?.candidates || [])
        toast({ title: "Several accounts use that email", description: "Pick the right one below." })
        return
      }
      if (!res.ok) {
        let message = "Unable to attach that customer."
        try {
          const body = await res.json()
          if (body?.error) message = body.error
        } catch {
          /* non-JSON error body */
        }
        throw new Error(message)
      }
      const body = await res.json()
      const provisioned = body?.provisioning?.provisioned?.length ?? 0
      toast({
        title: "Bridge customer attached",
        description:
          provisioned > 0
            ? `${provisioned} location${provisioned === 1 ? "" : "s"} now have a payout address.`
            : body?.provisioning?.message || "Attached. Provision once a bank is linked.",
      })
      setAttachEmail("")
      setAttachOwner("")
      setAttachCustomer("")
      setCandidates([])
      await load()
    } catch (err) {
      toast({
        title: "Attach failed",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setAttaching(false)
    }
  }

  const saveOverride = async (locationId: number) => {
    const address = (overrideDrafts[locationId] || "").trim()
    if (!address) return
    setOverriding(locationId)
    try {
      const res = await authFetch(`/admin/locations/${locationId}/liquidation-address`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ address }),
      })
      if (!res.ok) {
        let message = "Unable to save that address."
        try {
          const body = await res.json()
          if (body?.error) message = body.error
        } catch {
          /* non-JSON error body */
        }
        throw new Error(message)
      }
      toast({ title: "Payout address overridden", description: `Location ${locationId} now unwraps to ${shortHex(address)}.` })
      setOverrideDrafts((current) => ({ ...current, [locationId]: "" }))
      await load()
    } catch (err) {
      toast({
        title: "Override refused",
        description: err instanceof Error ? err.message : "Unexpected error.",
        variant: "destructive",
      })
    } finally {
      setOverriding(null)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold">Merchant payouts</h2>
          <p className="text-sm text-muted-foreground">
            Bridge business verification, linked banks, and where each location&apos;s unwraps are sent.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
          {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
          Refresh
        </Button>
      </div>

      {disabledReason ? (
        <Card>
          <CardContent className="py-6 text-sm text-muted-foreground">{disabledReason}</CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Attach an existing Bridge customer</CardTitle>
          <CardDescription>
            For a business that was verified outside the app. Enter the merchant&apos;s email and the Bridge customer
            id: this links the customer to their account, pulls in their linked banks, and provisions a payout address
            for every approved location.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
            <div className="space-y-1">
              <Label htmlFor="attach-email">Merchant email</Label>
              <Input
                id="attach-email"
                type="email"
                value={attachEmail}
                onChange={(e) => {
                  setAttachEmail(e.target.value)
                  setCandidates([])
                }}
                placeholder="owner@business.com"
                autoComplete="off"
                disabled={showOwnerId}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="attach-customer">Bridge customer id</Label>
              <Input
                id="attach-customer"
                value={attachCustomer}
                onChange={(e) => setAttachCustomer(e.target.value)}
                placeholder="uuid from the Bridge dashboard"
                autoComplete="off"
              />
            </div>
            <Button onClick={() => void attach()} disabled={attaching || !!disabledReason}>
              {attaching ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Attach
            </Button>
          </div>

          {candidates.length > 0 ? (
            <div className="rounded-md border p-3">
              <div className="mb-2 text-sm font-medium">Which account?</div>
              <ul className="space-y-2">
                {candidates.map((c) => (
                  <li key={c.owner_id} className="flex flex-wrap items-center justify-between gap-2 text-sm">
                    <div>
                      <div>{c.contact_name || "(no name)"}</div>
                      <div className="text-xs text-muted-foreground">
                        {c.location_names.length > 0 ? c.location_names.join(", ") : "no approved locations"}
                        {" · "}
                        <span className="font-mono">{shortDid(c.owner_id)}</span>
                      </div>
                    </div>
                    <Button size="sm" variant="outline" disabled={attaching} onClick={() => void attach(c.owner_id)}>
                      Use this one
                    </Button>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          <button
            type="button"
            className="text-xs text-muted-foreground underline-offset-2 hover:underline"
            onClick={() => setShowOwnerId((v) => !v)}
          >
            {showOwnerId ? "Use email instead" : "Attach by owner id instead"}
          </button>
          {showOwnerId ? (
            <div className="space-y-1">
              <Label htmlFor="attach-owner">Owner user id (did:privy:…)</Label>
              <Input
                id="attach-owner"
                value={attachOwner}
                onChange={(e) => setAttachOwner(e.target.value)}
                placeholder="did:privy:…"
                autoComplete="off"
                className="font-mono text-xs"
              />
            </div>
          ) : null}
        </CardContent>
      </Card>

      {businesses.length === 0 && !loading ? (
        <Card>
          <CardContent className="py-6 text-sm text-muted-foreground">
            No business has started Bridge verification yet. Approving a location starts it automatically.
          </CardContent>
        </Card>
      ) : null}

      {businesses.map((business) => {
        const kyb = kybStatusLabel(business.profile.kyb_status)
        return (
          <Card key={business.profile.owner_id}>
            <CardHeader className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <CardTitle className="text-base">
                  {business.locations[0]?.name || "Business"}
                  {business.locations.length > 1 ? ` (+${business.locations.length - 1} more)` : ""}
                </CardTitle>
                <Badge variant={kyb.done ? "default" : kyb.failed ? "destructive" : "secondary"}>KYB: {kyb.label}</Badge>
                {business.profile.tos_status && business.profile.tos_status !== "approved" ? (
                  <Badge variant="outline">ToS: {business.profile.tos_status}</Badge>
                ) : null}
              </div>
              <CardDescription className="font-mono text-xs">
                owner {shortDid(business.profile.owner_id)}
                {business.profile.bridge_customer_id ? ` · bridge customer ${business.profile.bridge_customer_id}` : ""}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <div className="mb-1 text-sm font-medium">Linked banks</div>
                {business.bank_accounts.length === 0 ? (
                  <p className="text-sm text-muted-foreground">None yet. The merchant connects one from Settings.</p>
                ) : (
                  <ul className="space-y-1 text-sm">
                    {business.bank_accounts.map((bank) => (
                      <li key={bank.bridge_external_account_id} className="flex flex-wrap items-center gap-2">
                        <span>
                          {bank.bank_name || "Bank"} ••••{bank.last_4}
                        </span>
                        <span className="text-muted-foreground">{bank.account_owner_name}</span>
                        <span className="font-mono text-xs text-muted-foreground">{bank.bridge_external_account_id}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              <div>
                <div className="mb-1 text-sm font-medium">Locations</div>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="text-left text-xs text-muted-foreground">
                      <tr>
                        <th className="py-1 pr-3">Location</th>
                        <th className="py-1 pr-3">Payout address</th>
                        <th className="py-1 pr-3">Bank</th>
                        <th className="py-1 pr-3">Source</th>
                        <th className="py-1">Override</th>
                      </tr>
                    </thead>
                    <tbody>
                      {business.locations.map((loc) => {
                        const la = loc.liquidation_address
                        const bank = la
                          ? business.bank_accounts.find((b) => b.bridge_external_account_id === la.bridge_external_account_id)
                          : undefined
                        return (
                          <tr key={loc.location_id} className="border-t align-top">
                            <td className="py-2 pr-3">
                              <div>{loc.name || `Location ${loc.location_id}`}</div>
                              <div className="text-xs text-muted-foreground">#{loc.location_id}</div>
                            </td>
                            <td className="py-2 pr-3 font-mono text-xs">
                              {la ? (
                                <span title={la.address}>{shortHex(la.address)}</span>
                              ) : (
                                <span className="text-muted-foreground">not provisioned</span>
                              )}
                            </td>
                            <td className="py-2 pr-3 text-xs">{bank ? `••••${bank.last_4}` : la ? shortHex(la.bridge_external_account_id, 4) : "—"}</td>
                            <td className="py-2 pr-3">
                              {la ? <Badge variant={la.source === "admin" ? "outline" : "secondary"}>{la.source}</Badge> : null}
                            </td>
                            <td className="py-2">
                              <div className="flex items-center gap-2">
                                <Input
                                  className="h-8 w-56 font-mono text-xs"
                                  placeholder="0x… issued by Bridge"
                                  value={overrideDrafts[loc.location_id] || ""}
                                  onChange={(e) =>
                                    setOverrideDrafts((current) => ({ ...current, [loc.location_id]: e.target.value }))
                                  }
                                  autoComplete="off"
                                />
                                <Button
                                  size="sm"
                                  variant="outline"
                                  disabled={overriding === loc.location_id || !(overrideDrafts[loc.location_id] || "").trim()}
                                  onClick={() => void saveOverride(loc.location_id)}
                                >
                                  {overriding === loc.location_id ? <Loader2 className="h-4 w-4 animate-spin" /> : "Save"}
                                </Button>
                              </div>
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
                <p className="mt-2 text-xs text-muted-foreground">
                  Overrides are only accepted for addresses Bridge issued to this business. Locations paying into the same
                  bank share one Bridge address; unwraps are matched to locations by transaction hash.
                </p>
              </div>
            </CardContent>
          </Card>
        )
      })}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Recent unwraps</CardTitle>
          <CardDescription>Every unwrap recorded by the web app, with its status from Bridge.</CardDescription>
        </CardHeader>
        <CardContent>
          {unwraps.length === 0 ? (
            <p className="text-sm text-muted-foreground">No unwraps yet.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs text-muted-foreground">
                  <tr>
                    <th className="py-1 pr-3">When</th>
                    <th className="py-1 pr-3">Location</th>
                    <th className="py-1 pr-3">From</th>
                    <th className="py-1 pr-3">Amount</th>
                    <th className="py-1 pr-3">Status</th>
                    <th className="py-1 pr-3">Bridge</th>
                    <th className="py-1">Tx</th>
                  </tr>
                </thead>
                <tbody>
                  {unwraps.map((u) => {
                    const status = unwrapStatusLabel(u.status)
                    return (
                      <tr key={u.id} className="border-t">
                        <td className="py-2 pr-3 whitespace-nowrap">{new Date(u.created_at).toLocaleString()}</td>
                        <td className="py-2 pr-3">{u.location_id ? `#${u.location_id}` : "—"}</td>
                        <td className="py-2 pr-3">{u.wallet_role === "tipping" ? "Tips" : "Till"}</td>
                        <td className="py-2 pr-3 whitespace-nowrap">{formatAmount(u.amount_wei)}</td>
                        <td className="py-2 pr-3">
                          <Badge variant={status.tone === "ok" ? "default" : status.tone === "bad" ? "destructive" : "secondary"}>
                            {status.label}
                          </Badge>
                        </td>
                        <td className="py-2 pr-3 text-xs text-muted-foreground">
                          {u.bridge_state || "—"}
                          {u.bank_reference ? ` · trace ${u.bank_reference}` : ""}
                        </td>
                        <td className="py-2 font-mono text-xs">
                          <a
                            href={`https://celoscan.io/tx/${u.tx_hash}`}
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex items-center gap-1 underline-offset-2 hover:underline"
                          >
                            {shortHex(u.tx_hash, 4)}
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
