"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Plus, Wallet, Settings, ArrowRight } from "lucide-react"
import { WalletDetailModal } from "@/components/wallets/wallet-detail-modal"
import { useWallets, usePrivy } from "@privy-io/react-auth"
import type { ConnectedWallet } from "@/types/privy-wallet"
import { useApp } from "@/context/AppProvider"
import { AppWallet } from "@/lib/wallets/wallets"

export default function WalletsPage() {
  const router = useRouter()
  const { wallets } = useApp()
  const { connectWallet } = usePrivy()

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

  // Handle wallet selection
  const handleSelectWallet = (wallet: AppWallet, index: number) => {
    // Navigate to the specific wallet page
    router.push(`/dashboard/wallets/${index}`)
  }

  // Handle disconnect wallet
  const handleDisconnectWallet = (wallet: AppWallet) => {
    // In a real implementation, you would call Privy's disconnect method
    console.log("Disconnecting wallet:", wallet.address)
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Connected Wallets</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Manage wallets connected to your SFLuv account</p>
      </div>

      <div className="flex flex-col sm:flex-row gap-4">
        <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={connectWallet}>
          <Plus className="h-4 w-4 mr-2" />
          Connect New Wallet
        </Button>
      </div>

      <div className="space-y-4">
        {wallets.length === 0 ? (
          <div className="text-center py-12">
            <Wallet className="h-12 w-12 text-gray-300 dark:text-gray-600 mx-auto mb-4" />
            <h3 className="text-xl font-medium text-black dark:text-white mb-2">No wallets connected</h3>
            <p className="text-gray-600 dark:text-gray-400 max-w-md mx-auto mb-6">
              Connect your first wallet to start managing your cryptocurrency and making transactions.
            </p>
            <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={connectWallet}>
              <Plus className="h-4 w-4 mr-2" />
              Connect Your First Wallet
            </Button>
          </div>
        ) : (
          wallets.map((wallet, index) => (
            <Card key={wallet.address} className="overflow-hidden cursor-pointer hover:shadow-md transition-shadow">
              <CardContent className="p-4">
                <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                  <div className="flex items-center gap-3">
                    <div>
                      <div className="flex items-center gap-2">
                        <h3 className="font-medium text-black dark:text-white">
                          {getWalletDisplayName(wallet.name)}
                        </h3>
                        <Badge variant="outline" className="bg-secondary text-black dark:text-white">
                          {getNetworkDisplayName(wallet.type)}
                        </Badge>
                      </div>
                      {wallet.address &&
                        <p className="text-sm text-gray-600 dark:text-gray-400 truncate max-w-[200px] md:max-w-[300px] font-mono">
                          {wallet.address.slice(0, 8)}...{wallet.address.slice(-6)}
                        </p>
                      }
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    <Button
                      onClick={() => handleSelectWallet(wallet, index)}
                      className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                    >
                      Open Wallet
                      <ArrowRight className="h-4 w-4 ml-2" />
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          ))
        )}
      </div>

      <div className="text-sm text-gray-500 dark:text-gray-400">
        Showing {wallets.length} connected wallet{wallets.length !== 1 ? "s" : ""}
      </div>
    </div>
  )
}
