"use client"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { TrendingUp, TrendingDown } from "lucide-react"
import type { ConnectedWallet } from "@/types/privy-wallet"
import type { WalletBalance } from "@/types/privy-wallet"

interface WalletBalanceCardProps {
  wallet: ConnectedWallet
  balance: WalletBalance
  showBalance: boolean
}

export function WalletBalanceCard({ wallet, balance, showBalance }: WalletBalanceCardProps) {
  const isPositive = balance.priceChange24h > 0

  const getWalletDisplayName = (walletType: string) => {
    switch (walletType) {
      case "metamask":
        return "MetaMask"
      case "coinbase_wallet":
        return "Coinbase Wallet"
      case "wallet_connect":
        return "WalletConnect"
      case "rainbow":
        return "Rainbow"
      case "trust":
        return "Trust Wallet"
      default:
        return walletType.charAt(0).toUpperCase() + walletType.slice(1)
    }
  }

  const getNetworkDisplayName = (chainType: string) => {
    switch (chainType) {
      case "ethereum":
        return "Ethereum"
      case "polygon":
        return "Polygon"
      case "arbitrum":
        return "Arbitrum"
      case "optimism":
        return "Optimism"
      case "base":
        return "Base"
      default:
        return chainType.charAt(0).toUpperCase() + chainType.slice(1)
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Avatar className="h-10 w-10">
              <AvatarImage src={`/placeholder.svg?height=40&width=40&text=${wallet.walletClientType}`} />
              <AvatarFallback>{wallet.walletClientType.slice(0, 2).toUpperCase()}</AvatarFallback>
            </Avatar>
            <div>
              <CardTitle className="text-lg">{getWalletDisplayName(wallet.walletClientType)}</CardTitle>
              <CardDescription className="font-mono text-xs">
                {wallet.address.slice(0, 8)}...{wallet.address.slice(-6)}
              </CardDescription>
            </div>
          </div>
          <Badge variant="secondary">{getNetworkDisplayName(wallet.type)}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div>
            <p className="text-3xl font-bold">{showBalance ? `$${balance.usdValue.toLocaleString()}` : "••••••••"}</p>
            <p className="text-lg text-muted-foreground">
              {showBalance ? `${balance.balance} ${balance.currency}` : "•••• ETH"}
            </p>
            <div className="flex items-center gap-2 mt-1">
              <div className={`flex items-center gap-1 text-sm ${isPositive ? "text-green-600" : "text-red-600"}`}>
                {isPositive ? <TrendingUp className="h-4 w-4" /> : <TrendingDown className="h-4 w-4" />}
                <span>
                  {isPositive ? "+" : ""}
                  {balance.priceChange24h}%
                </span>
              </div>
              <span className="text-sm text-muted-foreground">24h</span>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4 pt-4 border-t">
            <div>
              <p className="text-sm text-muted-foreground">Network</p>
              <p className="font-medium">{getNetworkDisplayName(wallet.type)}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Wallet Type</p>
              <p className="font-medium">{getWalletDisplayName(wallet.walletClientType)}</p>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
