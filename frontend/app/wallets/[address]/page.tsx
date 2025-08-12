"use client"

import { useState, useEffect, useMemo, useRef } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Send, QrCode, Eye, EyeOff, RefreshCw, Settings, ArrowLeft, Wallet, Pencil, Check, X } from "lucide-react"
import { SendCryptoModal } from "@/components/wallets/send-crypto-modal"
import { ReceiveCryptoModal } from "@/components/wallets/receive-crypto-modal"
import { TransactionHistoryList } from "@/components/wallets/transaction-history-list"
import { WalletBalanceCard } from "@/components/wallets/wallet-balance-card"
import { mockTransactions, mockWalletBalance } from "@/data/mock-wallet-data"
import { useToast } from "@/hooks/use-toast"
import { useParams, useRouter } from "next/navigation"
import { useWallets } from "@privy-io/react-auth"
import { useApp } from "@/context/AppProvider"
import { CHAIN } from "@/lib/constants"
import { Input } from "@/components/ui/input"

export default function WalletDetailsPage() {
  const params = useParams()
  const router = useRouter()
  const walletAddress = params.address as string
  const { wallets, status, walletsStatus, updateWallet } = useApp()

  useEffect(() => {
      if(status === "unauthenticated") {
        router.replace("/")
      }
    }, [status])

  // Get the specific wallet by index
  const wallet = useMemo(() => {
    if(walletsStatus !== "available") return undefined
    let w = wallets.find((w) => w.address?.toLowerCase() === walletAddress.toLowerCase())
    if(!w) {
      router.replace("/wallets")
      return undefined
    }
    return w
  }, [wallets])



  const [showSendModal, setShowSendModal] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showReceiveModal, setShowReceiveModal] = useState(false)
  const [showBalance, setShowBalance] = useState(true)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [balance, setBalance] = useState<number | null>(null)
  const [isEditingName, setIsEditingName] = useState(false)
  const [walletName, setWalletName] = useState("")
  const [isSavingName, setIsSavingName] = useState(false)
  const nameInputRef = useRef<HTMLInputElement>(null)
  const { toast } = useToast()

  useEffect(() => { if(!showReceiveModal && !showSendModal) updateBalance() }, [showReceiveModal, showSendModal])


  const handleEditName = () => {
    setIsEditingName(true)
  }

  const handleCancelEdit = () => {
    setIsEditingName(false)
    if (wallet) {
      setWalletName(wallet.name)
    }
  }

  const handleSaveName = async () => {
    if (!wallet || !wallet.id || !walletName.trim()) return

    setIsSavingName(true)

    try {
      // Simulate API call delay
      await updateWallet(wallet.id, walletName.trim())


      setIsEditingName(false)
      toast({
        title: "Wallet Renamed",
        description: `Wallet name updated to "${walletName.trim()}"`,
      })
    } catch (error) {
      toast({
        title: "Error",
        description: "Failed to update wallet name. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsSavingName(false)
    }
  }

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      handleSaveName()
    } else if (e.key === "Escape") {
      handleCancelEdit()
    }
  }

  const updateBalance = async () => {
    if(!wallet) return
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
      router.push("/wallets")
    }
  }, [wallets, router])

  const handleRefresh = async () => {
    setIsRefreshing(true)
    // Mock refresh delay
    await updateBalance()
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

  if (status === "loading" || walletsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (!wallet) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <div className="text-center">
          <Wallet className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
          <h2 className="text-xl font-semibold mb-2">No Wallet Found</h2>
          <p className="text-muted-foreground mb-4">Please connect a wallet to continue</p>
          <Button onClick={() => router.push("/wallets")}>Go to Wallets</Button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background mt--6">
      {/* Fixed Mobile Header with proper z-index */}
      <div className="relative top-0 z-50 bg-background/95 backdrop-blur-md supports-[backdrop-filter]:bg-background/80 border-b shadow-sm">
        <div className="flex items-center justify-between p-3 sm:p-4">
          <div className="flex items-center gap-2 sm:gap-3 min-w-0 flex-1">
            <Button variant="ghost" size="sm" onClick={() => router.back()} className="flex-shrink-0">
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div className="flex items-center gap-2 min-w-0 flex-1">
              <Avatar className="h-7 w-7 sm:h-8 sm:w-8 flex-shrink-0">
                <AvatarImage src={`/placeholder.svg?height=32&width=32&text=${wallet.name}`} />
                <AvatarFallback className="text-xs">{wallet.name.slice(0, 2).toUpperCase()}</AvatarFallback>
              </Avatar>
              <div className="min-w-0 flex-1">
                {isEditingName ? (
                  <div className="flex items-center gap-1 sm:gap-2">
                    <Input
                      ref={nameInputRef}
                      value={walletName}
                      onChange={(e) => setWalletName(e.target.value)}
                      onKeyDown={handleKeyPress}
                      onBlur={handleSaveName}
                      className="h-7 sm:h-8 text-sm sm:text-base font-semibold px-2 py-1 min-w-0"
                      placeholder="Wallet name"
                      maxLength={30}
                      disabled={isSavingName}
                    />
                    <div className="flex gap-0.5 sm:gap-1 flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={handleSaveName}
                        disabled={isSavingName || !walletName.trim()}
                        className="h-6 w-6 sm:h-7 sm:w-7 p-0 text-green-600 hover:text-green-700 hover:bg-green-100 dark:hover:bg-green-900/20"
                      >
                        {isSavingName ? (
                          <div className="animate-spin h-3 w-3 sm:h-3.5 sm:w-3.5 border-2 border-green-600 border-t-transparent rounded-full" />
                        ) : (
                          <Check className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={handleCancelEdit}
                        disabled={isSavingName}
                        className="h-6 w-6 sm:h-7 sm:w-7 p-0 text-red-600 hover:text-red-700 hover:bg-red-100 dark:hover:bg-red-900/20"
                      >
                        <X className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center gap-1 sm:gap-2">
                    <h1 className="font-semibold text-base sm:text-lg truncate">{wallet.name}</h1>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={handleEditName}
                      className="h-6 w-6 sm:h-7 sm:w-7 p-0 text-muted-foreground hover:text-foreground flex-shrink-0"
                    >
                      <Pencil className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
                    </Button>
                  </div>
                )}
                <Badge variant="secondary" className="text-xs mt-0.5">
                  {CHAIN.name}
                </Badge>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-1 sm:gap-2 flex-shrink-0">
            <Button variant="ghost" size="sm" onClick={() => setShowBalance(!showBalance)}>
              {showBalance ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </Button>
            <Button variant="ghost" size="sm" onClick={handleRefresh} disabled={isRefreshing}>
              <RefreshCw className={`h-4 w-4 ${isRefreshing ? "animate-spin" : ""}`} />
            </Button>
          </div>
        </div>
      </div>

      {/* Main Content with proper spacing */}
      <div className="pb-6 sm:pb-8">
        <div className="container mx-auto px-3 sm:px-4 pt-4 sm:pt-6 space-y-4 sm:space-y-6 max-w-2xl">
          {/* Balance Card */}
          <WalletBalanceCard wallet={wallet} balance={balance || 0} showBalance={showBalance} />

          {/* Quick Actions */}
          <Card>
            <CardContent className="p-3 sm:p-4">
              <div className="grid grid-cols-2 gap-3">
                <Button
                  onClick={() => setShowSendModal(true)}
                  className="h-14 sm:h-16 flex-col gap-1.5 sm:gap-2 text-sm"
                >
                  <Send className="h-4 w-4 sm:h-5 sm:w-5" />
                  <span>Send</span>
                </Button>
                <Button
                  variant="outline"
                  onClick={() => setShowReceiveModal(true)}
                  className="h-14 sm:h-16 flex-col gap-1.5 sm:gap-2 text-sm"
                >
                  <QrCode className="h-4 w-4 sm:h-5 sm:w-5" />
                  <span>Receive</span>
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* Quick Stats */}
          <div className="grid grid-cols-2 gap-3 sm:gap-4">
            <Card>
              <CardContent className="p-3 sm:p-4">
                <div className="flex items-center gap-2 sm:gap-3">
                  <div className="h-8 w-8 sm:h-9 sm:w-9 rounded-full bg-green-100 dark:bg-green-900/20 flex items-center justify-center flex-shrink-0">
                    <ArrowLeft className="h-3.5 w-3.5 sm:h-4 sm:w-4 text-green-600 dark:text-green-400 rotate-45" />
                  </div>
                  <div className="min-w-0">
                    <p className="text-xs sm:text-sm text-muted-foreground">Received</p>
                    <p className="font-semibold text-sm sm:text-base">
                      {walletTransactions.filter((tx) => tx.toAddress === wallet.address).length}
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-3 sm:p-4">
                <div className="flex items-center gap-2 sm:gap-3">
                  <div className="h-8 w-8 sm:h-9 sm:w-9 rounded-full bg-red-100 dark:bg-red-900/20 flex items-center justify-center flex-shrink-0">
                    <ArrowLeft className="h-3.5 w-3.5 sm:h-4 sm:w-4 text-red-600 dark:text-red-400 -rotate-45" />
                  </div>
                  <div className="min-w-0">
                    <p className="text-xs sm:text-sm text-muted-foreground">Sent</p>
                    <p className="font-semibold text-sm sm:text-base">
                      {walletTransactions.filter((tx) => tx.fromAddress === wallet.address).length}
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Transaction History */}
          <Card>
            <CardHeader className="pb-2 sm:pb-3">
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="text-base sm:text-lg">Recent Activity</CardTitle>
                  <CardDescription className="text-xs sm:text-sm">Your latest transactions</CardDescription>
                </div>
                <Button variant="ghost" size="sm" className="text-xs sm:text-sm">
                  View All
                </Button>
              </div>
            </CardHeader>
            <CardContent className="px-3 sm:px-4 pb-3 sm:pb-4">
              <TransactionHistoryList transactions={walletTransactions.slice(0, 10)} walletAddress={wallet.address || "0x"} />
            </CardContent>
          </Card>

          {/* Security Notice */}
          <Card className="border-amber-200 dark:border-amber-800 bg-amber-50/50 dark:bg-amber-900/10">
            <CardContent className="p-3 sm:p-4">
              <div className="flex items-start gap-2 sm:gap-3">
                <div className="h-7 w-7 sm:h-8 sm:w-8 rounded-full bg-amber-100 dark:bg-amber-900/20 flex items-center justify-center flex-shrink-0 mt-0.5">
                  <Wallet className="h-3.5 w-3.5 sm:h-4 sm:w-4 text-amber-600 dark:text-amber-400" />
                </div>
                <div className="space-y-1 min-w-0">
                  <p className="font-medium text-xs sm:text-sm">Keep Your Wallet Secure</p>
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    Never share your private keys or seed phrase. Always verify recipient addresses before sending.
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Modals */}
      <SendCryptoModal open={showSendModal} onOpenChange={setShowSendModal} wallet={wallet} balance={balance || 0} />
      <ReceiveCryptoModal open={showReceiveModal} onOpenChange={setShowReceiveModal} wallet={wallet} />
    </div>
  )
}
