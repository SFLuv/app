"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { AlertTriangle, Check, Copy, KeyRound, Loader2, RefreshCw, ShieldCheck } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Separator } from "@/components/ui/separator"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import type {
  ProtocolAccount,
  ProtocolAddressWindow,
  ProtocolPanelResponse,
  ProtocolRotateAllResponse,
  ProtocolRotateResponse,
} from "@/types/protocol"

/** A copiable address, shown in full rather than truncated. */
function CopyableAddress({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  const { toast } = useToast()

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard access can be refused outright, and an address nobody can
      // copy is still an address they can read and select.
      toast({ title: "Could not copy", description: "Select the address and copy it manually.", variant: "destructive" })
    }
  }

  if (!value) {
    return <span className="text-sm text-muted-foreground">—</span>
  }

  return (
    <div className="flex min-w-0 items-center gap-2">
      <code className="min-w-0 break-all rounded bg-muted px-2 py-1 font-mono text-xs">{value}</code>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        className="h-7 w-7 shrink-0"
        onClick={copy}
        aria-label={`Copy ${label}`}
      >
        {copied ? <Check className="h-3.5 w-3.5 text-emerald-600" /> : <Copy className="h-3.5 w-3.5" />}
      </Button>
    </div>
  )
}

function formatMoment(seconds: number | null): string {
  if (!seconds) return "—"
  return new Date(seconds * 1000).toLocaleString()
}

/** What state an account is in, in one phrase. */
function accountState(account: ProtocolAccount): { label: string; tone: "ok" | "warn" | "bad" } {
  if (!account.safe_address) return { label: "No wallet derived", tone: "bad" }
  if (!account.deployed_at) return { label: "Derived, not deployed", tone: "warn" }
  if (!account.signer) return { label: "Deployed, no signer", tone: "bad" }
  return { label: "Active", tone: "ok" }
}

export function ProtocolPanel() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [data, setData] = useState<ProtocolPanelResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  /** The account key awaiting confirmation, or "*" for all of them. */
  const [confirming, setConfirming] = useState<string | null>(null)
  const [rotating, setRotating] = useState(false)

  const load = useCallback(async () => {
    try {
      const res = await authFetch("/admin/protocol")
      if (!res.ok) {
        throw new Error(`The protocol panel could not be loaded (${res.status}).`)
      }
      setData((await res.json()) as ProtocolPanelResponse)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : "The protocol panel could not be loaded.")
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  useEffect(() => {
    void load()
  }, [load])

  const accounts = data?.accounts ?? []
  const rotatable = useMemo(
    () => accounts.filter((account) => account.deployed_at !== null && account.signer !== null),
    [accounts],
  )

  const confirmingAccount = useMemo(
    () => (confirming && confirming !== "*" ? accounts.find((account) => account.key === confirming) ?? null : null),
    [accounts, confirming],
  )

  const rotateOne = async (key: string) => {
    setRotating(true)
    try {
      const res = await authFetch(`/admin/protocol/accounts/${encodeURIComponent(key)}/rotate`, { method: "POST" })
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error((body as { error?: string })?.error ?? `Rotation failed (${res.status}).`)
      }
      const rotated = body as ProtocolRotateResponse
      toast({
        title: `${key} is on a new key`,
        description: `Now signed by ${rotated.new_eoa}. The wallet address is unchanged.`,
      })
      await load()
    } catch (err) {
      toast({
        title: "Rotation failed",
        description: err instanceof Error ? err.message : "The key was not rotated.",
        variant: "destructive",
      })
    } finally {
      setRotating(false)
      setConfirming(null)
    }
  }

  const rotateAll = async () => {
    setRotating(true)
    try {
      const res = await authFetch("/admin/protocol/rotate-all", { method: "POST" })
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error((body as { error?: string })?.error ?? `Rotation failed (${res.status}).`)
      }
      const result = body as ProtocolRotateAllResponse
      // Reported per account rather than as one verdict: a partial rotation is
      // the case that matters, and "3 of 4" is the only honest summary of it.
      if (result.failed.length === 0) {
        toast({ title: `Rotated ${result.rotated.length} key${result.rotated.length === 1 ? "" : "s"}` })
      } else {
        toast({
          title: `Rotated ${result.rotated.length}, failed ${result.failed.length}`,
          description: result.failed.map((failure) => `${failure.account_key}: ${failure.error}`).join(" · "),
          variant: "destructive",
        })
      }
      await load()
    } catch (err) {
      toast({
        title: "Rotation failed",
        description: err instanceof Error ? err.message : "No keys were rotated.",
        variant: "destructive",
      })
    } finally {
      setRotating(false)
      setConfirming(null)
    }
  }

  if (loading) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 py-10 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading protocol wallets…
        </CardContent>
      </Card>
    )
  }

  if (error) {
    return (
      <Card>
        <CardContent className="space-y-3 py-8">
          <p className="text-sm text-destructive">{error}</p>
          <Button variant="outline" size="sm" onClick={() => void load()}>
            Try again
          </Button>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      {data && !data.master_key_configured && (
        <Card className="border-destructive">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="h-5 w-5" />
              No encryption key is configured
            </CardTitle>
            <CardDescription>
              Signer keys are stored encrypted, and the key that decrypts them —{" "}
              <code className="font-mono text-xs">PROTOCOL_KEY_ENCRYPTION_KEY</code> — is not set on this deployment.
              Until it is, these wallets cannot be deployed, rotated or spent from.
            </CardDescription>
          </CardHeader>
        </Card>
      )}

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="h-5 w-5" />
              Protocol wallets
            </CardTitle>
            <CardDescription>
              The wallets the backend signs with. Each one is a smart wallet whose address never changes; rotating a key
              replaces the signer behind it, so nothing that refers to the address has to be updated.
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => void load()} disabled={rotating}>
              <RefreshCw className="mr-2 h-4 w-4" />
              Refresh
            </Button>
            {data?.can_rotate && (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setConfirming("*")}
                disabled={rotating || rotatable.length === 0}
              >
                <KeyRound className="mr-2 h-4 w-4" />
                Rotate all keys
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {accounts.length === 0 && (
            <p className="text-sm text-muted-foreground">
              No protocol wallets are registered on this chain yet. They are created at startup.
            </p>
          )}

          {accounts.map((account) => {
            const state = accountState(account)
            return (
              <div key={account.key} className="rounded-lg border p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0 space-y-1">
                    <div className="flex items-center gap-2">
                      <h3 className="font-semibold">{account.label}</h3>
                      <Badge
                        variant={state.tone === "ok" ? "secondary" : "destructive"}
                        className={state.tone === "warn" ? "bg-amber-100 text-amber-900 hover:bg-amber-100" : ""}
                      >
                        {state.label}
                      </Badge>
                    </div>
                    <p className="max-w-prose text-sm text-muted-foreground">{account.description}</p>
                  </div>
                  {data?.can_rotate && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setConfirming(account.key)}
                      disabled={rotating || account.deployed_at === null || account.signer === null}
                    >
                      <KeyRound className="mr-2 h-4 w-4" />
                      Rotate
                    </Button>
                  )}
                </div>

                <Separator className="my-3" />

                <dl className="grid gap-3 sm:grid-cols-2">
                  <div className="min-w-0 space-y-1">
                    <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      Wallet address
                    </dt>
                    <dd>
                      <CopyableAddress value={account.safe_address} label={`${account.label} wallet address`} />
                    </dd>
                  </div>
                  <div className="min-w-0 space-y-1">
                    <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      Current signer
                    </dt>
                    <dd>
                      <CopyableAddress
                        value={account.signer?.eoa_address ?? ""}
                        label={`${account.label} signing address`}
                      />
                    </dd>
                  </div>
                </dl>

                <p className="mt-3 text-xs text-muted-foreground">
                  {account.deployed_at
                    ? `Deployed ${formatMoment(account.deployed_at)}`
                    : "Not deployed yet — the previous wallet is still in use."}
                  {account.signer?.activated_at ? ` · signer since ${formatMoment(account.signer.activated_at)}` : ""}
                </p>
              </div>
            )
          })}
        </CardContent>
      </Card>

      {data && !data.cutover_complete && (
        <MigrationCard
          data={data}
          busy={rotating}
          onRun={async (action) => {
            setRotating(true)
            try {
              const res = await authFetch(`/admin/protocol/${action}`, { method: "POST" })
              const body = await res.json().catch(() => null)
              if (!res.ok) {
                throw new Error((body as { error?: string })?.error ?? `${action} failed (${res.status}).`)
              }
              toast({
                title: action === "deploy" ? "Wallets deployed" : "Balances moved",
                description:
                  action === "deploy"
                    ? "The wallets are on chain. Move the balances next."
                    : "The old accounts have been emptied into the new wallets.",
              })
              await load()
            } catch (err) {
              toast({
                title: action === "deploy" ? "Deployment failed" : "Move failed",
                description: err instanceof Error ? err.message : "Nothing changed.",
                variant: "destructive",
              })
            } finally {
              setRotating(false)
            }
          }}
        />
      )}

      {data?.cutover_complete && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Payouts go through the sponsored path</CardTitle>
            <CardDescription>
              Now that the balances are in these wallets, every payout is a sponsored transaction: the Citizen Wallet
              engine submits it and the paymaster pays the gas. That means the paymaster is the one account still
              needing CELO — and while the engine is unavailable, payouts stop rather than falling back.
            </CardDescription>
          </CardHeader>
        </Card>
      )}

      <AddressHistoryCard history={data?.history ?? []} />

      {(data?.steps?.length ?? 0) > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Migration steps</CardTitle>
            <CardDescription>
              Each step records that it ran, so a restart or a second deploy cannot repeat one.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {data?.steps.map((step) => (
              <div key={step.step} className="flex flex-wrap items-center justify-between gap-2 border-b pb-2 text-sm last:border-0">
                <code className="font-mono text-xs">{step.step}</code>
                <div className="flex items-center gap-2">
                  {step.detail && <span className="text-xs text-muted-foreground">{step.detail}</span>}
                  <Badge
                    variant={
                      step.status === "done" ? "secondary" : step.status === "failed" ? "destructive" : "outline"
                    }
                  >
                    {step.status}
                  </Badge>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <Dialog open={confirming !== null} onOpenChange={(open) => !open && setConfirming(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {confirming === "*"
                ? `Rotate every protocol key?`
                : `Rotate the ${confirmingAccount?.label ?? "wallet"} key?`}
            </DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-3 text-sm">
                <p>
                  {confirming === "*"
                    ? `A new key is generated for each of the ${rotatable.length} wallets below, and the old key stops being able to sign.`
                    : "A new key is generated, and the old key stops being able to sign."}
                </p>
                <p>
                  The wallet address does not change, so nothing else needs updating. This cannot be undone — the
                  previous key is removed as a signer, not kept as a spare.
                </p>
                {confirming === "*" && (
                  <ul className="list-inside list-disc text-muted-foreground">
                    {rotatable.map((account) => (
                      <li key={account.key}>{account.label}</li>
                    ))}
                  </ul>
                )}
              </div>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(null)} disabled={rotating}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => (confirming === "*" ? void rotateAll() : void rotateOne(confirming as string))}
              disabled={rotating}
            >
              {rotating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <KeyRound className="mr-2 h-4 w-4" />}
              {confirming === "*" ? "Rotate all keys" : "Rotate key"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

/**
 * The two steps of the migration, offered in order.
 *
 * Both are off by default in the environment, because main auto-deploys and a
 * merge must not deploy contracts or move a live balance as a side effect. This
 * card is where someone does it deliberately, and it disappears once there is
 * nothing left to do.
 */
function MigrationCard({
  data,
  busy,
  onRun,
}: {
  data: ProtocolPanelResponse
  busy: boolean
  onRun: (action: "deploy" | "sweep") => Promise<void>
}) {
  const [confirming, setConfirming] = useState<"deploy" | "sweep" | null>(null)
  const anyDeployed = data.accounts.some((account) => account.deployed_at !== null)
  const allDeployed = data.accounts.length > 0 && data.accounts.every((account) => account.deployed_at !== null)

  return (
    <Card>
      <CardHeader>
        <CardTitle>Finish the migration</CardTitle>
        <CardDescription>
          Two steps, in order. Deploying puts the wallets on chain and costs only gas. Moving the balances empties the
          old accounts into them and cannot be undone — it spends with the keys that are being replaced.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => setConfirming("deploy")} disabled={busy || allDeployed}>
          {allDeployed ? "Wallets deployed" : "Deploy wallets"}
        </Button>
        <Button variant="destructive" onClick={() => setConfirming("sweep")} disabled={busy || !anyDeployed}>
          Move balances across
        </Button>
        {!data.deploy_enabled && !allDeployed && (
          <p className="w-full pt-1 text-xs text-muted-foreground">
            Deploying is switched off in the environment (<code className="font-mono">PROTOCOL_DEPLOY_ENABLED</code>);
            this button does it anyway, once, on request.
          </p>
        )}
      </CardContent>

      <Dialog open={confirming !== null} onOpenChange={(open) => !open && setConfirming(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {confirming === "deploy" ? "Deploy the protocol wallets?" : "Move the balances across?"}
            </DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-3 text-sm">
                {confirming === "deploy" ? (
                  <p>
                    Each wallet is deployed at the address already shown above — the address does not change by being
                    deployed. No funds move. Safe to repeat: a wallet already on chain is skipped.
                  </p>
                ) : (
                  <>
                    <p>
                      Every SFLUV and CELO balance on the old accounts is transferred to the new wallets, signed with
                      the keys that are being replaced.
                    </p>
                    <p>
                      This cannot be undone, and it is the point of no return: from here payouts come from the new
                      wallets and go through the sponsored path.
                    </p>
                  </>
                )}
              </div>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(null)} disabled={busy}>
              Cancel
            </Button>
            <Button
              variant={confirming === "deploy" ? "default" : "destructive"}
              disabled={busy}
              onClick={() => {
                const action = confirming
                setConfirming(null)
                if (action) void onRun(action)
              }}
            >
              {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {confirming === "deploy" ? "Deploy" : "Move balances"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

/**
 * The address history, which is what lets a check on an old item answer
 * correctly: a payout made in March is compared against March's wallet, not
 * today's.
 */
function AddressHistoryCard({ history }: { history: ProtocolAddressWindow[] }) {
  const byRole = useMemo(() => {
    const groups = new Map<string, ProtocolAddressWindow[]>()
    for (const window of history) {
      const existing = groups.get(window.role) ?? []
      existing.push(window)
      groups.set(window.role, existing)
    }
    return [...groups.entries()].sort(([left], [right]) => left.localeCompare(right))
  }, [history])

  if (history.length === 0) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Address history</CardTitle>
        <CardDescription>
          Which address held each role, and when. Checks against past payments use the address that was in use at the
          time, so replacing a wallet does not make older payments look missing.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {byRole.map(([role, windows]) => (
          <div key={role} className="space-y-2">
            <h4 className="text-sm font-semibold capitalize">{role.replace(/_/g, " ")}</h4>
            {windows.map((window) => (
              <div key={window.id} className="flex flex-wrap items-center justify-between gap-2 text-sm">
                <CopyableAddress value={window.address} label={`${role} address`} />
                <span className="text-xs text-muted-foreground">
                  {window.ended_at === null ? (
                    <Badge variant="secondary">current</Badge>
                  ) : (
                    `until ${formatMoment(window.ended_at)}`
                  )}
                </span>
              </div>
            ))}
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
