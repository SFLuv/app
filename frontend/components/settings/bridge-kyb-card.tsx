"use client"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { ExternalLink, Landmark } from "lucide-react"

// Self-serve Bridge business verification (KYB) for merchant bank cash-outs.
// Once verified, Bridge issues the merchant a liquidation address: USDC sent to
// it settles automatically to their linked bank account. The merchant pastes
// that address into the Cash Out card on their wallet page.
//
// Until a Bridge API key is configured, this is a hosted-link handoff: set
// NEXT_PUBLIC_BRIDGE_KYB_URL to the Bridge-hosted onboarding link. When the
// backend Bridge integration lands, this card can instead create a per-merchant
// KYC Link via the API and show live verification status.
const BRIDGE_KYB_URL = process.env.NEXT_PUBLIC_BRIDGE_KYB_URL ?? ""

export function BridgeKybCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-black dark:text-white">
          <Landmark className="h-5 w-5" />
          Bank Cash-Out Verification
        </CardTitle>
        <CardDescription>
          To cash out SFLUV to your bank account, your business completes a one-time verification
          (KYB) with Bridge, our banking partner.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <ol className="list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
          <li>Verify your business with Bridge and link the bank account you want payouts in.</li>
          <li>
            Bridge gives you a <span className="font-medium">liquidation address</span> — a crypto
            address that automatically settles anything it receives to your bank account.
          </li>
          <li>
            Paste that address into the <span className="font-medium">Cash Out to Bank</span> card
            on your wallet page. Cashing out sends USDC there; it lands in your bank account.
          </li>
        </ol>

        {BRIDGE_KYB_URL ? (
          <Button asChild>
            <a href={BRIDGE_KYB_URL} target="_blank" rel="noopener noreferrer">
              Start business verification
              <ExternalLink className="ml-2 h-4 w-4" />
            </a>
          </Button>
        ) : (
          <Button disabled>Verification opens soon</Button>
        )}
      </CardContent>
    </Card>
  )
}
