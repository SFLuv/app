"use client"

import { useCallback, useEffect, useState } from "react"
import { AlertTriangle, CheckCircle2, Loader2, RefreshCw, Wallet } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"

/**
 * Protocol account balances.
 *
 * The reason this screen exists: when the account that sponsors user
 * transactions ran out of gas, every merchant payment stopped at once, and the
 * first anyone knew of it was a merchant saying payments "weren't working".
 * Nothing showed the balance, so nothing connected the symptom to the cause.
 *
 * So the ordering here is by urgency rather than by tidiness — anything under
 * its floor is pulled to the top — and each balance is shown against the floor
 * it is judged by, because a number on its own does not tell you whether to act.
 */

type AccountBalance = {
  name: string
  address: string
  celo: number
  min_celo: number
  top_up_to_celo: number
  auto_top_up: boolean
  low: boolean
  error?: string
}

type Snapshot = {
  faucet: {
    name: string
    address: string
    celo: number
    sfluv: number
    min_celo: number
    min_sfluv: number
    low_celo: boolean
    low_sfluv: boolean
    error?: string
  }
  accounts: AccountBalance[]
  auto_top_up_enabled: boolean
  /**
   * True once the protocol's own transactions are paid for by the paymaster.
   * It changes what the numbers mean: our wallets stop needing gas, and the
   * paymaster becomes the only CELO balance a person has to top up.
   */
  sponsored: boolean
  taken_at: string
}

const celo = (n: number) => `${n.toFixed(4)} CELO`
const sfluv = (n: number) => `${n.toLocaleString(undefined, { maximumFractionDigits: 2 })} SFLUV`
const short = (a: string) => (a && a.length > 14 ? `${a.slice(0, 8)}…${a.slice(-6)}` : a || "—")

export function BalancesPanel() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [sweeping, setSweeping] = useState(false)
  const [unavailable, setUnavailable] = useState("")

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await authFetch("/admin/balances")
      if (!res.ok) {
        setUnavailable((await res.text()).trim() || "Balances are unavailable.")
        return
      }
      setUnavailable("")
      setSnapshot((await res.json()) as Snapshot)
    } catch {
      setUnavailable("Could not reach the server.")
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const sweep = async () => {
    setSweeping(true)
    try {
      const res = await authFetch("/admin/balances/sweep", { method: "POST" })
      const body = (await res.json().catch(() => null)) as
        | { funded?: string[]; skipped?: string[]; errors?: string[] }
        | null
      if (!res.ok) {
        toast({ title: "Top-up failed", variant: "destructive" })
        return
      }
      const funded = body?.funded?.length ?? 0
      toast({
        title: funded > 0 ? `Topped up ${funded} account${funded === 1 ? "" : "s"}` : "Nothing needed topping up",
        description: body?.skipped?.[0] ?? body?.errors?.[0],
      })
      await load()
    } finally {
      setSweeping(false)
    }
  }

  if (loading && !snapshot) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Reading balances…
      </div>
    )
  }

  if (unavailable) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base text-black dark:text-white">Balances</CardTitle>
          <CardDescription>{unavailable}</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  if (!snapshot) return null

  const { faucet } = snapshot
  // Anything under its floor first: this page is read when something is wrong.
  const accounts = [...snapshot.accounts].sort((a, b) => Number(b.low) - Number(a.low))
  const problems = accounts.filter((a) => a.low).length + (faucet.low_celo || faucet.low_sfluv ? 1 : 0)

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-black dark:text-white">Balances</h2>
          <p className="text-sm text-muted-foreground">
            {problems === 0
              ? "Every account is above its floor."
              : `${problems} account${problems === 1 ? "" : "s"} below the floor.`}{" "}
            Checked {new Date(snapshot.taken_at).toLocaleTimeString()}.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            <RefreshCw className={`mr-2 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
          <Button size="sm" onClick={() => void sweep()} disabled={sweeping}>
            {sweeping ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <Wallet className="mr-2 h-3.5 w-3.5" />}
            Top up now
          </Button>
        </div>
      </div>

      {/* The faucet is the only account a person has to fund, so it is stated
          separately rather than as the first row of a list. */}
      <Card className={faucet.low_celo || faucet.low_sfluv ? "border-destructive" : undefined}>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base text-black dark:text-white">
            {faucet.name}
            {faucet.low_celo || faucet.low_sfluv ? (
              <Badge variant="destructive">Needs funding</Badge>
            ) : (
              <Badge variant="outline" className="border-emerald-500 text-emerald-600">
                Healthy
              </Badge>
            )}
          </CardTitle>
          <CardDescription>
            {snapshot.sponsored
              ? "Rewards and redemptions are paid from this balance. Its transactions are sponsored, so it needs no gas of its own — the paymaster below is the balance that needs a person."
              : "Everything below is funded from here. This is the only balance that needs a person."}
            <span className="ml-1 font-mono text-xs">{short(faucet.address)}</span>
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          {/* No floor shown once sponsored: a gas floor on an account that
              never pays gas reads as a balance sitting under its limit, which
              is the opposite of what zero means there. */}
          <Figure
            label={snapshot.sponsored ? "CELO (not needed)" : "CELO (gas)"}
            value={celo(faucet.celo)}
            floor={snapshot.sponsored ? undefined : celo(faucet.min_celo)}
            low={faucet.low_celo}
          />
          <Figure label="SFLUV (rewards)" value={sfluv(faucet.sfluv)} floor={sfluv(faucet.min_sfluv)} low={faucet.low_sfluv} />
          {faucet.error ? (
            <p className="sm:col-span-2 text-sm text-destructive">Could not read: {faucet.error}</p>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base text-black dark:text-white">Protocol accounts</CardTitle>
          <CardDescription>
            {snapshot.sponsored
              ? "Only the paymaster still needs CELO: it pays for every sponsored transaction, and nothing can refill it automatically, because moving CELO out of a smart wallet is itself a transaction it would have to pay for."
              : snapshot.auto_top_up_enabled
                ? "Topped up automatically from the faucet when they fall below their floor."
                : "Automatic top-ups are off — these need funding by hand, or switch them on in funding-accounts.json."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {accounts.length === 0 ? (
            <p className="text-sm text-muted-foreground">No accounts configured.</p>
          ) : (
            accounts.map((account) => (
              <div
                key={account.address}
                className={`flex flex-wrap items-center gap-3 rounded-lg border p-3 ${
                  account.low ? "border-destructive/50 bg-destructive/5" : ""
                }`}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-black dark:text-white">{account.name}</span>
                    {account.low ? (
                      <Badge variant="destructive" className="gap-1">
                        <AlertTriangle className="h-3 w-3" /> Below floor
                      </Badge>
                    ) : account.error ? null : (
                      <CheckCircle2 className="h-4 w-4 text-emerald-600" />
                    )}
                    {!account.auto_top_up && (
                      <Badge variant="outline" className="text-muted-foreground">
                        Watched only
                      </Badge>
                    )}
                  </div>
                  <span className="font-mono text-xs text-muted-foreground">{short(account.address)}</span>
                </div>
                <div className="text-right">
                  {account.error ? (
                    // Unknown and empty must not look the same: an unreadable
                    // balance showing as 0.0000 would read as an emergency.
                    <span className="text-sm text-destructive">Unknown — {account.error}</span>
                  ) : (
                    <>
                      <div className={`font-medium tabular-nums ${account.low ? "text-destructive" : "text-black dark:text-white"}`}>
                        {celo(account.celo)}
                      </div>
                      <div className="text-xs text-muted-foreground tabular-nums">
                        floor {celo(account.min_celo)} · tops up to {celo(account.top_up_to_celo)}
                      </div>
                    </>
                  )}
                </div>
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}

/** A balance against the floor it is judged by, where it is judged by one. */
function Figure({
  label,
  value,
  floor,
  low,
}: {
  label: string
  value: string
  /** Omitted for a balance nothing is judging, which is how a sponsored
      account's gas reads: zero is correct rather than short. */
  floor?: string
  low: boolean
}) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className={`text-2xl font-semibold tabular-nums ${low ? "text-destructive" : "text-black dark:text-white"}`}>
        {value}
      </div>
      {floor ? (
        <div className="text-xs text-muted-foreground tabular-nums">floor {floor}</div>
      ) : (
        <div className="text-xs text-muted-foreground">not required</div>
      )}
    </div>
  )
}
