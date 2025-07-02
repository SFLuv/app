"use client"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { TrendingUp, TrendingDown } from "lucide-react"
import type { ConnectedWallet } from "@/types/privy-wallet"
import type { WalletBalance } from "@/types/privy-wallet"
import { AppWallet } from "@/lib/wallets/wallets"
import { SYMBOL } from "@/lib/constants"

interface WalletBalanceCardProps {
  wallet: AppWallet
  balance: number | null
  showBalance: boolean
}

export function WalletBalanceCard({ wallet, balance, showBalance }: WalletBalanceCardProps) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Avatar className="h-10 w-10">
              <AvatarImage src={`/placeholder.svg?height=40&width=40&text=${wallet.name}`} />
              <AvatarFallback>{wallet.name.slice(0, 2).toUpperCase()}</AvatarFallback>
            </Avatar>
            <div>
              <CardTitle className="text-lg">{wallet.name.toUpperCase()}</CardTitle>
              <CardDescription className="font-mono text-xs">
                {wallet.address?.slice(0, 8)}...{wallet.address?.slice(-6)}
              </CardDescription>
            </div>
          </div>
          <Badge variant="secondary">{wallet.type.toUpperCase()}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div>
            <p className="text-lg text-muted-foreground">
              {showBalance && balance !== null ? `${balance} ${SYMBOL}` : `•••• ${SYMBOL}`}
            </p>
          </div>

          <div className="grid grid-cols-2 gap-4 pt-4 border-t">
            <div>
              <p className="text-sm text-muted-foreground">Wallet Type</p>
              <p className="font-medium">{wallet.type.toUpperCase()}</p>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
