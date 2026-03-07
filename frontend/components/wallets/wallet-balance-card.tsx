"use client"

import { useMemo } from "react"
import { Card, CardContent } from "@/components/ui/card"
import { SYMBOL } from "@/lib/constants"

interface WalletBalanceCardProps {
  balance: number | null
  showBalance: boolean
}

export function WalletBalanceCard({ balance, showBalance }: WalletBalanceCardProps) {
  const formattedBalance = useMemo(() => {
    if (!showBalance || balance === null) return "••••"
    return balance.toLocaleString("en-US", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
  }, [balance, showBalance])

  return (
    <Card className="overflow-hidden border border-primary/15 bg-gradient-to-br from-[#fff7f7] via-background to-muted/20 shadow-sm dark:from-[#3a1f1f]/40 dark:via-background">
      <CardContent className="space-y-3 py-5 sm:py-6">
        <p className="text-[11px] sm:text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
          Available Balance
        </p>
        <div className="flex items-end gap-2">
          <p className="text-3xl sm:text-4xl font-semibold leading-none">
            {formattedBalance}
          </p>
          <span className="pb-0.5 text-sm sm:text-base font-medium text-muted-foreground">
            {SYMBOL}
          </span>
        </div>
        <div className="h-1.5 w-28 rounded-full bg-gradient-to-r from-[#eb6c6c] to-[#f29b9b]" />
      </CardContent>
    </Card>
  )
}
