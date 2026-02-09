"use client"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { TrendingUp, TrendingDown } from "lucide-react"
import type { ConnectedWallet } from "@/types/privy-wallet"
import type { WalletBalance } from "@/types/privy-wallet"
import { AppWallet } from "@/lib/wallets/wallets"
import { CHAIN, SYMBOL } from "@/lib/constants"

interface WalletBalanceCardProps {
  wallet: AppWallet
  balance: number | null
  showBalance: boolean
  onShowHistoricalBalance?: () => void
}

export function WalletBalanceCard({ wallet, balance, showBalance, onShowHistoricalBalance }: WalletBalanceCardProps) {

  return (
    <Card className="bg-gradient-to-br from-background to-muted/20">
      <CardHeader className="pb-2 sm:pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 sm:gap-3 min-w-0 flex-1">
            {/* <Avatar className="h-8 w-8 sm:h-10 sm:w-10 flex-shrink-0">
              <AvatarImage src={`/placeholder.svg?height=40&width=40&text=${wallet.name}`} />
              <AvatarFallback className="text-xs sm:text-sm">
                {wallet.name.slice(0, 2).toUpperCase()}
              </AvatarFallback>
            </Avatar> */}
            <div className="min-w-0 flex-1">
              <CardTitle className="text-sm sm:text-lg truncate">
                {wallet.name.toUpperCase()}
              </CardTitle>
              <CardDescription className="font-mono text-xs truncate">
                {wallet?.address?.slice(0, 6)}...{wallet?.address?.slice(-4)}
              </CardDescription>
            </div>
          </div>
          <Badge variant="secondary" className="text-xs flex-shrink-0">
            {CHAIN.name}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="space-y-3 sm:space-y-4">
          <div>
            <p className="text-2xl sm:text-3xl font-bold leading-tight">
              {showBalance && balance !== null ? `${balance} ${SYMBOL}` : `•••• ${SYMBOL}`}
            </p>
          </div>

          <div className="pt-3 sm:pt-4 border-t">
            <div className="flex items-center justify-between gap-2">
              <div>
                <p className="text-xs sm:text-sm text-muted-foreground">Wallet Type</p>
                <p className="font-medium text-sm sm:text-base truncate">
                  {wallet.type.toUpperCase()}
                </p>
              </div>
              {onShowHistoricalBalance && (
                <Button
                  size="sm"
                  onClick={onShowHistoricalBalance}
                  className="h-7 px-2 text-[11px] sm:text-xs bg-[#eb6c6c] hover:bg-[#d55c5c] text-white"
                >
                  Get Historical Balance
                </Button>
              )}
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
