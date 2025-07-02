"use client"

import { useState, useEffect, useMemo } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Send, QrCode, Eye, EyeOff, RefreshCw, Settings, ArrowLeft, Wallet } from "lucide-react"
import { SendCryptoModal } from "@/components/wallets/send-crypto-modal"
import { ReceiveCryptoModal } from "@/components/wallets/receive-crypto-modal"
import { TransactionHistoryList } from "@/components/wallets/transaction-history-list"
import { WalletBalanceCard } from "@/components/wallets/wallet-balance-card"
import { mockTransactions, mockWalletBalance } from "@/data/mock-wallet-data"
import { useToast } from "@/hooks/use-toast"
import { useParams, useRouter } from "next/navigation"
import { useWallets } from "@privy-io/react-auth"
import { useApp } from "@/context/AppProvider"

export default function WalletDetailsPage() {
  const params = useParams()
  const router = useRouter()
  const walletIndex = Number.parseInt(params.index as string) || 0
  const { wallets } = useApp()

  // Get the specific wallet by index
  const wallet = wallets[walletIndex] || wallets[0]

  const [showSendModal, setShowSendModal] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showReceiveModal, setShowReceiveModal] = useState(false)
  const [showBalance, setShowBalance] = useState(true)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [balance, setBalance] = useState<number | null>(null)
  const { toast } = useToast()

  useEffect(() => { if(!showReceiveModal && !showSendModal) updateBalance() }, [showReceiveModal, showSendModal])

  const updateBalance = async () => {
    try {
      const b = await wallet.getBalanceFormatted()
      if(b === null) {
        setError("Wallet not initialized.")
        return
      }
      setBalance(b)
    }
    catch(error) {
      console.error(error)
    }
  }

  useEffect(() => {
    if(error) console.error(error)
  }, [error])

  // Redirect if no wallet found
  useEffect(() => {
    if (wallets.length === 0) {
      router.push("/dashboard/wallets")
    }
  }, [wallets, router])

  const handleRefresh = async () => {
    setIsRefreshing(true)
    // Mock refresh delay
    await new Promise((resolve) => setTimeout(resolve, 1000))
    setIsRefreshing(false)
    toast({
      title: "Wallet Refreshed",
      description: "Your wallet balance and transactions have been updated.",
    })
  }

  // Filter transactions for this wallet
  const walletTransactions = mockTransactions.filter(
    (tx) => tx.fromAddress === wallet?.address || tx.toAddress === wallet?.address,
  )

  if (!wallet) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <div className="text-center">
          <Wallet className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
          <h2 className="text-xl font-semibold mb-2">No Wallet Found</h2>
          <p className="text-muted-foreground mb-4">Please connect a wallet to continue</p>
          <Button onClick={() => router.push("/dashboard/wallets")}>Go to Wallets</Button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background">
      {/* Mobile Header */}
      <div className="sticky top-0 z-10 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 border-b">
        <div className="flex items-center justify-between p-4">
          <div className="flex items-center gap-3">
            <Button variant="ghost" size="sm" onClick={() => router.back()}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div className="flex items-center gap-2">
              <Avatar className="h-8 w-8">
                <AvatarImage src={`/placeholder.svg?height=32&width=32&text=${wallet.name}`} />
                <AvatarFallback>{wallet.name.slice(0, 2).toUpperCase()}</AvatarFallback>
              </Avatar>
              <div>
                <h1 className="font-semibold text-lg">
                  {wallet.name.toUpperCase()}
                </h1>
                <Badge variant="secondary" className="text-xs">
                  {wallet.type.toUpperCase()}
                </Badge>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => setShowBalance(!showBalance)}>
              {showBalance ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </Button>
            <Button variant="ghost" size="sm" onClick={handleRefresh} disabled={isRefreshing}>
              <RefreshCw className={`h-4 w-4 ${isRefreshing ? "animate-spin" : ""}`} />
            </Button>
            <Button variant="ghost" size="sm">
              <Settings className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>

      <div className="container mx-auto p-4 space-y-6 max-w-2xl">
        {/* Balance Card */}
        <WalletBalanceCard wallet={wallet} balance={balance} showBalance={showBalance} />

        {/* Quick Actions */}
        <Card>
          <CardContent className="p-4">
            <div className="grid grid-cols-2 gap-3">
              <Button onClick={() => setShowSendModal(true)} className="h-16 flex-col gap-2">
                <Send className="h-5 w-5" />
                <span className="text-sm">Send</span>
              </Button>
              <Button variant="outline" onClick={() => setShowReceiveModal(true)} className="h-16 flex-col gap-2">
                <QrCode className="h-5 w-5" />
                <span className="text-sm">Receive</span>
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* Quick Stats */}
        <div className="grid grid-cols-2 gap-4">
          <Card>
            <CardContent className="p-4">
              <div className="flex items-center gap-2">
                <div className="h-8 w-8 rounded-full bg-green-100 dark:bg-green-900/20 flex items-center justify-center">
                  <ArrowLeft className="h-4 w-4 text-green-600 dark:text-green-400 rotate-45" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground">Received</p>
                  <p className="font-semibold">
                    {walletTransactions.filter((tx) => tx.toAddress === wallet.address).length}
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <div className="flex items-center gap-2">
                <div className="h-8 w-8 rounded-full bg-red-100 dark:bg-red-900/20 flex items-center justify-center">
                  <ArrowLeft className="h-4 w-4 text-red-600 dark:text-red-400 -rotate-45" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground">Sent</p>
                  <p className="font-semibold">
                    {walletTransactions.filter((tx) => tx.fromAddress === wallet.address).length}
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Transaction History */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="text-lg">Recent Activity</CardTitle>
                <CardDescription>Your latest transactions</CardDescription>
              </div>
              <Button variant="ghost" size="sm">
                View All
              </Button>
            </div>
          </CardHeader>
          <CardContent className="px-4 pb-4">
            <TransactionHistoryList transactions={walletTransactions.slice(0, 10)} walletAddress={wallet.address || "0x"} />
          </CardContent>
        </Card>

        {/* Security Notice */}
        <Card className="border-amber-200 dark:border-amber-800">
          <CardContent className="p-4">
            <div className="flex items-start gap-3">
              <div className="h-8 w-8 rounded-full bg-amber-100 dark:bg-amber-900/20 flex items-center justify-center flex-shrink-0">
                <Wallet className="h-4 w-4 text-amber-600 dark:text-amber-400" />
              </div>
              <div className="space-y-1">
                <p className="font-medium text-sm">Keep Your Wallet Secure</p>
                <p className="text-xs text-muted-foreground">
                  Never share your private keys or seed phrase. Always verify recipient addresses before sending.
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Modals */}
      <SendCryptoModal open={showSendModal} onOpenChange={setShowSendModal} wallet={wallet} balance={balance || 0} />
      <ReceiveCryptoModal open={showReceiveModal} onOpenChange={setShowReceiveModal} wallet={wallet} />
    </div>
  )
}
