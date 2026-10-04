"use client"

import { formatUnits } from "viem"
import { ExternalLink, Landmark } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useChainConfig } from "@/context/ChainConfigProvider"
import { unwrapStatusLabel, type Unwrap } from "@/types/merchant-payout"

type UnwrapDetailModalProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  unwrap: Unwrap | null
}

// What a merchant asks when a payout has not arrived: where is it, where did it
// go, and can I see it. The pending state answers the first — and it is the
// reason in_review is now kept distinct from funds_received, because "in review"
// and "on its way" are different answers to "when do I get paid".
const PENDING_EXPLANATION: Record<string, string> = {
  submitted: "Sent on chain. Waiting for our banking partner to see it.",
  in_review: "Our banking partner is reviewing this payout before releasing it. No action needed from you.",
  funds_received: "Received by our banking partner and being converted.",
  payment_submitted: "Sent to your bank. Bank transfers usually land in 1–2 business days.",
  payment_processed: "Deposited in your bank account.",
  failed: "This payout didn't complete. Contact support with the reference below.",
}

export function UnwrapDetailModal({ open, onOpenChange, unwrap }: UnwrapDetailModalProps) {
  const chainConfig = useChainConfig()

  if (!unwrap) return null

  const status = unwrapStatusLabel(unwrap.status)
  const explorer = (chainConfig.chain?.blockExplorers?.default?.url || "https://celoscan.io").replace(/\/$/, "")

  const amount = (() => {
    try {
      return `${Number(formatUnits(BigInt(unwrap.amount_wei), chainConfig.tokenDecimals)).toLocaleString(undefined, {
        maximumFractionDigits: 2,
      })} ${chainConfig.tokenSymbol}`
    } catch {
      return unwrap.amount_wei
    }
  })()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex flex-wrap items-center gap-2">
            {amount}
            <span
              className={`rounded-full border px-2 py-0.5 text-xs font-normal ${
                status.tone === "ok"
                  ? "border-green-300 text-green-700"
                  : status.tone === "bad"
                    ? "border-red-300 text-red-600"
                    : ""
              }`}
            >
              {status.label}
            </span>
          </DialogTitle>
          <DialogDescription>
            {PENDING_EXPLANATION[unwrap.status] ?? "Waiting on our banking partner."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 text-sm">
          {unwrap.bank_name || unwrap.bank_last_4 ? (
            <div className="flex items-start justify-between gap-4">
              <span className="text-muted-foreground">Paid to</span>
              <span className="flex items-center gap-2 text-right font-medium">
                <Landmark className="h-4 w-4 text-muted-foreground" />
                {unwrap.bank_name || "Your bank"}
                {unwrap.bank_last_4 ? ` ····${unwrap.bank_last_4}` : ""}
              </span>
            </div>
          ) : (
            <div className="flex items-start justify-between gap-4">
              <span className="text-muted-foreground">Paid to</span>
              {/* The bank is matched through the address this payout was sent
                  to. An older payout, or one whose destination has since been
                  cleared, has nothing to match — said plainly rather than
                  guessing at the location's current bank. */}
              <span className="text-right text-muted-foreground">Bank no longer on file</span>
            </div>
          )}

          <Row label="Unwrapped" value={new Date(unwrap.created_at).toLocaleString()} />
          {unwrap.wallet_role === "tipping" && <Row label="From" value="Tipping wallet" />}

          <LinkRow
            label="Transaction"
            value={unwrap.tx_hash}
            href={unwrap.tx_hash ? `${explorer}/tx/${unwrap.tx_hash}` : undefined}
          />
          <LinkRow
            label="Payout address"
            value={unwrap.destination_address}
            href={unwrap.destination_address ? `${explorer}/address/${unwrap.destination_address}` : undefined}
          />

          {unwrap.bank_reference && <Row label="Bank reference" value={unwrap.bank_reference} mono />}

          {/* The partner's own word for the state, kept next to our label so a
              support conversation can quote it without anyone reading a log. */}
          {unwrap.bridge_state && unwrap.bridge_state !== unwrap.status && (
            <Row label="Partner state" value={unwrap.bridge_state} mono />
          )}
          {unwrap.chain && <Row label="Network" value={unwrap.chain} />}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className={`text-right ${mono ? "break-all font-mono text-xs" : ""}`}>{value}</span>
    </div>
  )
}

function LinkRow({ label, value, href }: { label: string; value: string; href?: string }) {
  if (!value) return null
  const short = value.length > 18 ? `${value.slice(0, 10)}…${value.slice(-8)}` : value
  return (
    <div className="flex items-start justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      {href ? (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1.5 break-all font-mono text-xs underline underline-offset-4"
        >
          {short}
          <ExternalLink className="h-3 w-3 shrink-0" />
        </a>
      ) : (
        <span className="break-all font-mono text-xs">{short}</span>
      )}
    </div>
  )
}
