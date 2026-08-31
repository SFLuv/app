"use client"

import { useState } from "react"
import { Loader2, ShieldCheck } from "lucide-react"

import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Switch } from "@/components/ui/switch"
import { useSignetEnrolment } from "@/hooks/useSignetEnrolment"
import type { AppWallet } from "@/lib/wallets/wallets"

/**
 * Signet signer enrolment, for the wallet the user treats as primary.
 *
 * Renders nothing at all unless the on-chain gate says this EOA is in the
 * trial. The gate is deployed closed, so this is inert for everyone until an
 * admin allowlists them — no feature flag needed.
 *
 * The first action is a BUTTON, not the toggle. Enrolment writes to a public
 * chain and asks the user for a signature; the toggle is a local preference
 * over which key signs, and only appears once enrolment has completed. The two
 * warrant different controls even though binding is now rebindable.
 */
export function SignetCard({ wallet }: { wallet: AppWallet | null | undefined }) {
  const { state, loading, binding, error, bind, visible } = useSignetEnrolment(wallet)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [preferSignet, setPreferSignet] = useState(false)

  if (loading && !state) return null
  if (!visible || !state) return null

  const onConfirm = async () => {
    const ok = await bind()
    if (ok) setConfirmOpen(false)
  }

  return (
    <Card className="mt-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-black dark:text-white">
          <ShieldCheck className="h-5 w-5 text-[#eb6c6c]" />
          Signet Signer
          <span className="rounded-full border border-[#eb6c6c]/40 bg-[#eb6c6c]/10 px-2 py-0.5 text-[11px] font-medium text-[#eb6c6c]">
            Trial
          </span>
        </CardTitle>
        <CardDescription>
          Sign transactions from your primary wallet with a threshold key held
          across several independent servers, instead of the key in this browser.
          You still sign in exactly as you do now.
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4">
        {state.boundElsewhere ? (
          <>
          <p className="text-sm text-muted-foreground">
            Your account is currently linked to a different wallet
            (<span className="font-mono text-xs">{state.boundSafe}</span>). You
            can move the link here, but anything already secured under the other
            wallet stays there — so only do this if you mean to switch wallets.
          </p>
          <Button
            variant="outline"
            onClick={() => setConfirmOpen(true)}
            className="mt-3"
          >
            Move link to this wallet
          </Button>
          </>
        ) : !state.walletDeployed ? (
          <p className="text-sm text-muted-foreground">
            This wallet has not been used on chain yet. Make a transaction
            first, then come back.
          </p>
        ) : state.status === "eligible" ? (
          <>
            <p className="text-sm text-muted-foreground">
              Enabling links this wallet to your account on chain. You will be
              asked to sign to confirm it is you. The link applies to this
              wallet only, and can be moved later.
            </p>
            <Button
              onClick={() => setConfirmOpen(true)}
              className="bg-[#eb6c6c] hover:bg-[#eb6c6c]/90"
            >
              Enable Signet Signing
            </Button>
          </>
        ) : state.status === "partly_enrolled" ? (
          <div className="space-y-2">
            <StepRow done label="Wallet linked" />
            <StepRow done={state.signetIsOwner} label="Threshold key created" />
            <p className="text-xs text-muted-foreground">
              Setup will continue automatically once the signing service is
              available. Your wallet keeps working normally in the meantime.
            </p>
          </div>
        ) : (
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium text-black dark:text-white">
                Use Signet to sign
              </p>
              <p className="text-xs text-muted-foreground">
                Turn this off at any time to go back to signing in this browser.
              </p>
            </div>
            <Switch checked={preferSignet} onCheckedChange={setPreferSignet} />
          </div>
        )}

        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
      </CardContent>

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Enable Signet signing?</DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-2 text-sm">
                <p>
                  This links{" "}
                  <span className="font-mono text-xs">{state.wallet}</span> to
                  your account on chain. You will be asked to sign first — only
                  you can authorize this, and no one can do it for you.
                </p>
                <p>
                  Your existing key keeps working — this adds a second way to
                  sign, it does not take anything away.
                </p>
              </div>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={binding}>
              Cancel
            </Button>
            <Button
              onClick={onConfirm}
              disabled={binding}
              className="bg-[#eb6c6c] hover:bg-[#eb6c6c]/90"
            >
              {binding ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Linking...
                </>
              ) : (
                "Sign and enable"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

function StepRow({ done, label }: { done?: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <span
        className={
          done
            ? "h-2 w-2 rounded-full bg-emerald-500"
            : "h-2 w-2 rounded-full bg-gray-300 dark:bg-gray-600"
        }
      />
      <span className={done ? "text-black dark:text-white" : "text-muted-foreground"}>
        {label}
      </span>
    </div>
  )
}
