"use client"

import { useMemo } from "react"
import { Card, CardContent } from "@/components/ui/card"
import { USDC, USDC_SYMBOL } from "./usdc-theme"

// USDC balance card. Mirrors WalletBalanceCard's layout but in the USDC-blue
// theme so the underlying-token balance reads as distinct from the SFLUV balance.
interface UsdcBalanceCardProps {
  balance: number | null
  showBalance: boolean
  loading?: boolean
}

export function UsdcBalanceCard({ balance, showBalance, loading = false }: UsdcBalanceCardProps) {
  const formattedBalance = useMemo(() => {
    if (!showBalance) return "••••"
    if (loading && balance === null) return "…"
    if (balance === null) return "—"
    return balance.toLocaleString("en-US", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
  }, [balance, showBalance, loading])

  return (
    <Card className={`overflow-hidden border ${USDC.cardBorder} ${USDC.cardBg} shadow-sm`}>
      <CardContent className="space-y-3 py-5 sm:py-6">
        <p className={`text-[11px] sm:text-xs font-medium uppercase tracking-[0.16em] ${USDC.accentText}`}>
          USDC on Celo
        </p>
        <div className="flex items-end gap-2">
          <p className="text-3xl sm:text-4xl font-semibold leading-none">{formattedBalance}</p>
          <span className="pb-0.5 text-sm sm:text-base font-medium text-muted-foreground">{USDC_SYMBOL}</span>
        </div>
        <div className={`h-1.5 w-28 rounded-full ${USDC.accentBar}`} />
      </CardContent>
    </Card>
  )
}
