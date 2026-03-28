"use client"

import { useState, useEffect, useMemo, useRef, useCallback } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Send, QrCode, Eye, EyeOff, RefreshCw, ArrowLeft, ArrowDownLeft, Wallet, Pencil, Check, X, BellOff, Bell, Banknote, Copy } from "lucide-react"
import { SendCryptoModal } from "@/components/wallets/send-crypto-modal"
import { ReceiveCryptoModal } from "@/components/wallets/receive-crypto-modal"
import { TransactionHistoryList } from "@/components/wallets/transaction-history-list"
import { WalletBalanceCard } from "@/components/wallets/wallet-balance-card"
import { TransactionModal } from "@/components/transactions/transaction-modal"
import { useToast } from "@/hooks/use-toast"
import { useParams, useRouter, useSearchParams } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { CHAIN, HONEY_TOKEN } from "@/lib/constants"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { CashOutCryptoModal } from "@/components/wallets/cashOut_crypto_modal"
import { NotificationModal } from "@/components/notifications/notification-modal"
import { useTransactions } from "@/context/TransactionProvider"
import { WalletTransaction } from "@/types/privy-wallet"
import type { Transaction } from "@/types/transaction"
import { useIsMobile } from "@/hooks/use-mobile"

type BalanceUpdateResult = "changed" | "unchanged" | "unknown"

export default function WalletDetailsPage() {
  const [showSendModal, setShowSendModal] = useState(false)
  const [showCashoutModal, setShowCashoutModal] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showReceiveModal, setShowReceiveModal] = useState(false)
  const [showBalance, setShowBalance] = useState(true)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [balance, setBalance] = useState<number | null>(null)
  const [isEditingName, setIsEditingName] = useState(false)
  const [walletName, setWalletName] = useState("")
  const [isSavingName, setIsSavingName] = useState(false)
  const [notificationModalOpen, setNotificationModalOpen] = useState<boolean>(false)
  const [walletTransactions, setWalletTransactions] = useState<WalletTransaction[]>([])
  const [walletTransactionDetails, setWalletTransactionDetails] = useState<Transaction[]>([])
  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null)
  const [transactionModalOpen, setTransactionModalOpen] = useState(false)
  const [walletCanUnwrap, setWalletCanUnwrap] = useState<boolean>(false)
  const [walletCanMint, setWalletCanMint] = useState<boolean>(false)
  const [gasTokenBalance, setGasTokenBalance] = useState<number | null>(null)
  const [backingBalancesLoading, setBackingBalancesLoading] = useState<boolean>(false)
  const [byusdBalance, setByusdBalance] = useState<number | null>(null)
  const [honeyBalance, setHoneyBalance] = useState<number | null>(null)
  const [showMintModal, setShowMintModal] = useState<boolean>(false)
  const [mintAsset, setMintAsset] = useState<"BYUSD" | "HONEY">("BYUSD")
  const [mintAmount, setMintAmount] = useState<string>("")
  const [isMinting, setIsMinting] = useState<boolean>(false)
  const [mintError, setMintError] = useState<string | null>(null)
  const [addressCopied, setAddressCopied] = useState(false)
  const nameInputRef = useRef<HTMLInputElement>(null)
  const latestTxMarkerRef = useRef<string | null>(null)
  const txPollingInFlightRef = useRef(false)
  const txLastPollAtRef = useRef(0)
  const balanceSnapshotRef = useRef<number | null>(null)
  const balanceRetryTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const addressCopiedTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const { toast } = useToast()
  const params = useParams()
  const router = useRouter()
  const searchParams = useSearchParams()
  const walletAddress = params.address as string
  const isMobile = useIsMobile()
  const {
    user,
    wallets,
    status,
    walletsStatus,
    updateWallet,
    ponderSubscriptions,
  } = useApp()
  const {
    getTransactionsPage,
    refreshTransactions,
    toWalletTransaction
  } = useTransactions()
  const getTransactionsPageRef = useRef(getTransactionsPage)
  const refreshTransactionsRef = useRef(refreshTransactions)
  const toWalletTransactionRef = useRef(toWalletTransaction)
  const isAdminOrMerchant = user?.isAdmin === true || user?.isMerchant === true
  const sendFlowDefault = !isAdminOrMerchant && isMobile ? "scan" : "manual"
  const showScanSendButton = !isAdminOrMerchant
  const fromWalletMenu = searchParams.get("fromWalletMenu") === "1"

  // Get the specific wallet by index
  const wallet = useMemo(() => {
    if(walletsStatus !== "available") return undefined
    if(wallets.length === 0) return undefined

    let w = wallets.find((w) => w.address?.toLowerCase() === walletAddress.toLowerCase())
    if(!w) {
      router.replace("/wallets")
      return undefined
    }
    return w
  }, [walletAddress, wallets, walletsStatus, router])
  const displayWalletAddress = useMemo(() => {
    if (!wallet?.address) return ""
    if (wallet.address.length <= 12) return wallet.address
    return `${wallet.address.slice(0, 6)}...${wallet.address.slice(-4)}`
  }, [wallet?.address])
  const formatGasBalance = useCallback((value: number | null): string => {
    if (value === null) return "..."
    if (value >= 1000) return value.toLocaleString(undefined, { maximumFractionDigits: 2 })
    if (value >= 1) return value.toLocaleString(undefined, { maximumFractionDigits: 4 })
    return value.toLocaleString(undefined, { maximumFractionDigits: 6 })
  }, [])

  const ponder = useMemo(() => {
    if(ponderSubscriptions?.length === 0) return undefined

    let p = ponderSubscriptions?.find((s) => s.address?.toLowerCase() === walletAddress.toLowerCase())
    return p
  }, [ponderSubscriptions])

  useEffect(() => {
    let cancelled = false

    const resolveUnwrapEligibility = async () => {
      if (!wallet || wallet.type !== "smartwallet") {
        if (!cancelled) setWalletCanUnwrap(false)
        return
      }

      const canUnwrap = await wallet.hasRedeemerRole()
      if (!cancelled) {
        setWalletCanUnwrap(canUnwrap)
      }
    }

    resolveUnwrapEligibility()

    return () => {
      cancelled = true
    }
  }, [wallet])

  useEffect(() => {
    if (!wallet) {
      setWalletCanMint(false)
      return
    }

    setWalletCanMint(wallet.isMinter === true)
  }, [wallet])

  useEffect(() => {
    getTransactionsPageRef.current = getTransactionsPage
  }, [getTransactionsPage])

  useEffect(() => {
    refreshTransactionsRef.current = refreshTransactions
  }, [refreshTransactions])

  useEffect(() => {
    toWalletTransactionRef.current = toWalletTransaction
  }, [toWalletTransaction])

  const getLatestTxMarker = useCallback((transactions: WalletTransaction[]) => {
    if (transactions.length === 0) return null
    const latest = transactions[0]
    return `${latest.id}:${latest.hash || ""}`
  }, [])

  const txPageHandler = useCallback(async () => {
    const walletTxs = (await getTransactionsPageRef.current(walletAddress, 0, {
      paginationDetails: {
        count: 5,
        desc: true
      }
    }))

    setWalletTransactionDetails(walletTxs.txs)
    const mapped = walletTxs.txs.map((w) => toWalletTransactionRef.current(walletAddress, w))
    setWalletTransactions(mapped)
    latestTxMarkerRef.current = getLatestTxMarker(mapped)
  }, [getLatestTxMarker, walletAddress])

  const txPageRefresher = useCallback(async () => {
    const walletTxs = (await refreshTransactionsRef.current(walletAddress, 0, {
      paginationDetails: {
        count: 5,
        desc: true
      }
    }))

    setWalletTransactionDetails(walletTxs.txs)
    const mapped = walletTxs.txs.map((w) => toWalletTransactionRef.current(walletAddress, w))
    setWalletTransactions(mapped)
    latestTxMarkerRef.current = getLatestTxMarker(mapped)
  }, [getLatestTxMarker, walletAddress])

  useEffect(() => {
    if (status !== "authenticated") return
    void txPageHandler()
  }, [status, txPageHandler])


  const updateBalance = useCallback(async (): Promise<BalanceUpdateResult> => {
    if(!wallet) return "unknown"
    try {
      const b = await wallet.getSFLUVBalanceFormatted()
      if(b === null) {
        setError("Wallet not initialized.")
        return "unknown"
      }
      const previous = balanceSnapshotRef.current
      balanceSnapshotRef.current = b
      setBalance(b)
      if (previous === null) return "changed"
      return b !== previous ? "changed" : "unchanged"
    }
    catch(error) {
      console.error(error)
      return "unknown"
    }
  }, [wallet])

  useEffect(() => {
    balanceSnapshotRef.current = balance
  }, [balance])

  useEffect(() => {
    return () => {
      if (balanceRetryTimeoutRef.current) {
        clearTimeout(balanceRetryTimeoutRef.current)
      }
    }
  }, [])

  const updateBalanceWithRetry = useCallback(async () => {
    const balanceUpdate = await updateBalance()
    if (balanceUpdate !== "unchanged") {
      return
    }
    if (balanceRetryTimeoutRef.current) {
      clearTimeout(balanceRetryTimeoutRef.current)
    }
    balanceRetryTimeoutRef.current = setTimeout(() => {
      void updateBalance()
      balanceRetryTimeoutRef.current = null
    }, 2000)
  }, [updateBalance])

  const updateBackingBalances = useCallback(async () => {
    if (!wallet || !walletCanMint) {
      setByusdBalance(null)
      setHoneyBalance(null)
      setBackingBalancesLoading(false)
      return
    }

    setBackingBalancesLoading(true)
    try {
      const [byusd, honey] = await Promise.all([
        wallet.getBYUSDBalanceFormatted(),
        wallet.getHoneyBalanceFormatted()
      ])
      setByusdBalance(byusd)
      setHoneyBalance(honey)
    }
    catch (error) {
      console.error(error)
      setByusdBalance(null)
      setHoneyBalance(null)
    } finally {
      setBackingBalancesLoading(false)
    }
  }, [wallet, walletCanMint])

  const updateGasBalance = useCallback(async () => {
    if (!wallet || wallet.type !== "eoa") {
      setGasTokenBalance(null)
      return
    }

    try {
      const nativeBalance = await wallet.getGasTokenBalanceFormatted()
      setGasTokenBalance(nativeBalance)
    }
    catch (error) {
      console.error(error)
      setGasTokenBalance(null)
    }
  }, [wallet])

  const updateBalanceWithRetryRef = useRef(updateBalanceWithRetry)
  const updateBackingBalancesRef = useRef(updateBackingBalances)
  const updateGasBalanceRef = useRef(updateGasBalance)

  useEffect(() => {
    updateBalanceWithRetryRef.current = updateBalanceWithRetry
  }, [updateBalanceWithRetry])

  useEffect(() => {
    updateBackingBalancesRef.current = updateBackingBalances
  }, [updateBackingBalances])

  useEffect(() => {
    updateGasBalanceRef.current = updateGasBalance
  }, [updateGasBalance])

  useEffect(() => {
    if(!showReceiveModal && !showSendModal) {
      void updateBalance()
      void updateBackingBalances()
      void updateGasBalance()
    }
  }, [showReceiveModal, showSendModal, updateBalance, updateBackingBalances, updateGasBalance])

  const toggleNotificationModal = () => {
    setNotificationModalOpen(!notificationModalOpen)
  }


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

  const handleCopyWalletAddress = async () => {
    if (!wallet?.address) return
    try {
      await navigator.clipboard.writeText(wallet.address)
      setAddressCopied(true)
      if (addressCopiedTimeoutRef.current) {
        clearTimeout(addressCopiedTimeoutRef.current)
      }
      addressCopiedTimeoutRef.current = setTimeout(() => {
        setAddressCopied(false)
        addressCopiedTimeoutRef.current = null
      }, 1500)
      toast({
        title: "Address Copied",
        description: "Wallet address copied to clipboard.",
      })
    } catch {
      toast({
        title: "Copy Failed",
        description: "Unable to copy wallet address.",
        variant: "destructive",
      })
    }
  }

  useEffect(() => {
    return () => {
      if (addressCopiedTimeoutRef.current) {
        clearTimeout(addressCopiedTimeoutRef.current)
      }
    }
  }, [])

  const openMintModal = (asset: "BYUSD" | "HONEY") => {
    setMintAsset(asset)
    setMintAmount("")
    setMintError(null)
    setShowMintModal(true)
  }

  const handleMint = async () => {
    if (!wallet) return
    if (!mintAmount || Number(mintAmount) <= 0) {
      setMintError("Enter an amount greater than 0.")
      return
    }

    setIsMinting(true)
    setMintError(null)
    try {
      const receipt = mintAsset === "BYUSD"
        ? await wallet.mintSFLUVFromBYUSD(mintAmount)
        : await wallet.mintSFLUVFromHONEY(mintAmount)

      if (!receipt) {
        setMintError("Mint request did not submit.")
        return
      }

      if (receipt.error) {
        setMintError(receipt.error)
        toast({
          title: "Mint Failed",
          description: receipt.hash
            ? `${receipt.error} (tx: ${receipt.hash})`
            : receipt.error,
          variant: "destructive"
        })
        return
      }

      toast({
        title: "Mint Submitted",
        description: receipt.hash
          ? `Transaction hash: ${receipt.hash}`
          : "Mint transaction submitted"
      })
      setShowMintModal(false)
      setMintAmount("")
      await updateBalance()
      await updateBackingBalances()
      await txPageRefresher()
    }
    catch (error) {
      const message = error instanceof Error ? error.message : "unknown error"
      setMintError(message)
      toast({
        title: "Mint Failed",
        description: message,
        variant: "destructive"
      })
    } finally {
      setIsMinting(false)
    }
  }

  useEffect(() => {
    if (status !== "authenticated" || !wallet?.address) return

    let active = true
    txLastPollAtRef.current = 0

    const pollTransactions = async () => {
      if (!active || txPollingInFlightRef.current) return
      const now = Date.now()
      if (now - txLastPollAtRef.current < 2000) return
      txPollingInFlightRef.current = true
      txLastPollAtRef.current = now

      try {
        const walletTxs = await refreshTransactionsRef.current(walletAddress, 0, {
          paginationDetails: {
            count: 5,
            desc: true
          }
        })

        const mapped = walletTxs.txs.map((w) => toWalletTransactionRef.current(walletAddress, w))
        setWalletTransactionDetails(walletTxs.txs)
        const nextMarker = getLatestTxMarker(mapped)
        const previousMarker = latestTxMarkerRef.current

        setWalletTransactions(mapped)
        latestTxMarkerRef.current = nextMarker

        if (nextMarker && nextMarker !== previousMarker) {
          await updateBalanceWithRetryRef.current()
          await updateBackingBalancesRef.current()
          await updateGasBalanceRef.current()
        }
      } catch (pollError) {
        console.error(pollError)
      } finally {
        txPollingInFlightRef.current = false
      }
    }

    const intervalId = setInterval(() => {
      void pollTransactions()
    }, 2000)

    return () => {
      active = false
      clearInterval(intervalId)
      txLastPollAtRef.current = 0
    }
  }, [getLatestTxMarker, status, wallet?.address, walletAddress])


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
    await updateGasBalance()
    await txPageRefresher()
    setIsRefreshing(false)
    toast({
      title: "Wallet Refreshed",
      description: "Your wallet balance and transactions have been updated.",
    })
  }

  const handleBackNavigation = () => {
    if (fromWalletMenu) {
      router.push("/wallets")
      return
    }
    router.back()
  }

  const handleSelectTransaction = useCallback((walletTx: WalletTransaction) => {
    const match = walletTransactionDetails.find((tx) => tx.id === walletTx.id || tx.transactionId === walletTx.hash)
    if (!match) {
      return
    }

    setSelectedTransaction(match)
    setTransactionModalOpen(true)
  }, [walletTransactionDetails])

  const handleCloseTransactionModal = () => {
    setTransactionModalOpen(false)
    setSelectedTransaction(null)
  }


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
    <div className="min-h-screen bg-background">
      {/* Fixed Mobile Header with proper z-index */}
      <div className="relative top-0 z-50 bg-background/95 backdrop-blur-md supports-[backdrop-filter]:bg-background/80 border-b shadow-sm">
        <div className="flex items-center justify-between p-3 sm:p-4">
          <div className="flex items-center gap-2 sm:gap-3 min-w-0 flex-1">
            <Button variant="ghost" size="sm" onClick={handleBackNavigation} className="flex-shrink-0">
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div className="flex items-center gap-2 min-w-0 flex-1">
              {/* <Avatar className="h-7 w-7 sm:h-8 sm:w-8 flex-shrink-0">
                <AvatarImage src={`/placeholder.svg?height=32&width=32&text=${wallet.name}`} />
                <AvatarFallback className="text-xs">{wallet.name.slice(0, 2).toUpperCase()}</AvatarFallback>
              </Avatar> */}
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
                <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                  <span className="truncate font-mono">{displayWalletAddress}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleCopyWalletAddress}
                    className="h-5 w-5 p-0 text-muted-foreground hover:text-foreground"
                    aria-label="Copy wallet address"
                  >
                    {addressCopied ? <Check className="h-3 w-3 text-green-600" /> : <Copy className="h-3 w-3" />}
                  </Button>
                </div>
                {wallet.type === "eoa" && (
                  <div className="mt-1">
                    <Badge variant="warning" className="text-[10px] sm:text-xs">
                      Gas: {formatGasBalance(gasTokenBalance)} {CHAIN.nativeCurrency.symbol}
                    </Badge>
                  </div>
                )}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-1 sm:gap-2 flex-shrink-0">
            {(user?.isAdmin || user?.isMerchant) &&
              <Button variant="ghost" size="sm" onClick={toggleNotificationModal}>
                  {ponder?.id ? <BellOff className="h-4 w-4" /> : <Bell className="h-4 w-4" />}
              </Button>
            }
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
          <WalletBalanceCard
            balance={balance || 0}
            showBalance={showBalance}
          />

          {/* Quick Actions */}
          <Card>
            <CardContent className="p-3 sm:p-4">
              <div
                className={`grid gap-3 ${
                  walletCanUnwrap ? "grid-cols-3" : "grid-cols-2"
                }`}
              >
                <Button
                  onClick={() => setShowSendModal(true)}
                  className="h-14 sm:h-16 flex-col gap-1.5 sm:gap-2 text-sm"
                >
                  {showScanSendButton ? (
                    <QrCode className="h-4 w-4 sm:h-5 sm:w-5" />
                  ) : (
                    <Send className="h-4 w-4 sm:h-5 sm:w-5" />
                  )}
                  <span>{showScanSendButton ? "Scan/Send" : "Send"}</span>
                </Button>

                <Button
                  variant="outline"
                  onClick={() => setShowReceiveModal(true)}
                  className="h-14 sm:h-16 flex-col gap-1.5 sm:gap-2 text-sm hover:bg-primary/65"
                >
                  <ArrowDownLeft className="h-4 w-4 sm:h-5 sm:w-5" />
                  <span>Receive</span>
                </Button>

                {walletCanUnwrap && (
                  <Button
                    variant="outline"
                    onClick={() => setShowCashoutModal(true)}
                    className="h-14 sm:h-16 flex-col gap-1.5 sm:gap-2 text-sm hover:bg-primary/65"
                  >
                    <Banknote className="h-4 w-4 sm:h-5 sm:w-5" />
                    <span>Unwrap SFLUV</span>
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>

          {walletCanMint && (
            <Card className="border-sky-200 dark:border-sky-800 bg-sky-50/40 dark:bg-sky-900/10">
              <CardHeader className="pb-2 sm:pb-3">
                <CardTitle className="text-base sm:text-lg">Minter Backing Assets</CardTitle>
                <CardDescription className="text-xs sm:text-sm">
                  Use backing assets to mint new SFLUV to this wallet.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3 sm:space-y-4">
                <div className="rounded-lg border border-blue-200 dark:border-blue-800 bg-blue-50/60 dark:bg-blue-900/15 p-3 sm:p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="space-y-1">
                      <p className="text-xs text-muted-foreground uppercase tracking-wide">BYUSD Balance</p>
                      <p className="text-lg sm:text-xl font-semibold">
                        {showBalance ? (backingBalancesLoading ? "Loading..." : byusdBalance !== null ? byusdBalance.toFixed(2) : "Unavailable") : "•••••"} BYUSD
                      </p>
                    </div>
                    <Button
                      onClick={() => openMintModal("BYUSD")}
                      className="bg-[#eb6c6c] hover:bg-[#d55c5c] text-white"
                    >
                      Mint SFLUV
                    </Button>
                  </div>
                </div>

                <div className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50/60 dark:bg-amber-900/15 p-3 sm:p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="space-y-1">
                      <p className="text-xs text-muted-foreground uppercase tracking-wide">Honey Balance</p>
                      <p className="text-lg sm:text-xl font-semibold">
                        {showBalance ? (backingBalancesLoading ? "Loading..." : honeyBalance !== null ? honeyBalance.toFixed(2) : "Unavailable") : "•••••"} HONEY
                      </p>
                    </div>
                    <Button
                      onClick={() => openMintModal("HONEY")}
                      className="bg-[#eb6c6c] hover:bg-[#d55c5c] text-white"
                    >
                      Mint SFLUV
                    </Button>
                  </div>
                </div>
                {!backingBalancesLoading && (!HONEY_TOKEN || HONEY_TOKEN.length !== 42) && (
                  <p className="text-xs text-amber-700 dark:text-amber-300">
                    Honey balance unavailable: set `NEXT_PUBLIC_HONEY_ADDRESS` in frontend env.
                  </p>
                )}
              </CardContent>
            </Card>
          )}

          {/* Quick Stats */}
          {/* <div className="grid grid-cols-2 gap-3 sm:gap-4">
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
          </div> */}

          {/* Transaction History */}
          <Card>
            <CardHeader className="pb-2 sm:pb-3">
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="text-base sm:text-lg">Recent Activity</CardTitle>
                  <CardDescription className="text-xs sm:text-sm">Your latest transactions</CardDescription>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-xs sm:text-sm"
                  onClick={() => router.push("/wallets/" + params.address + "/transactions")}
                >
                  View All
                </Button>
              </div>
            </CardHeader>
            <CardContent className="px-3 sm:px-4 pb-3 sm:pb-4">
              <TransactionHistoryList
                transactions={walletTransactions.slice(0, 10)}
                walletAddress={wallet.address || "0x"}
                onSelectTransaction={handleSelectTransaction}
              />
            </CardContent>
          </Card>

        </div>
      </div>

      {/* Modals */}
      <SendCryptoModal
        open={showSendModal}
        onOpenChange={setShowSendModal}
        wallet={wallet}
        balance={balance || 0}
        defaultFlow={sendFlowDefault}
      />
      <ReceiveCryptoModal open={showReceiveModal} onOpenChange={setShowReceiveModal} wallet={wallet} />
      <CashOutCryptoModal open={showCashoutModal} onOpenChange={setShowCashoutModal} wallet={wallet} />
      <NotificationModal
        open={notificationModalOpen}
        onOpenChange={setNotificationModalOpen}
        id={ponder?.id}
        emailAddress={ponder?.data}
        address={wallet?.address || ""}
      />
      <TransactionModal
        transaction={selectedTransaction}
        wallet={wallet?.address || walletAddress}
        isOpen={transactionModalOpen}
        onClose={handleCloseTransactionModal}
      />

      <Dialog open={showMintModal} onOpenChange={setShowMintModal}>
        <DialogContent className="sm:max-w-[520px]">
          <DialogHeader>
            <DialogTitle>Mint SFLUV from {mintAsset}</DialogTitle>
            <DialogDescription>
              Enter how much {mintAsset} to convert into SFLUV.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="mint-amount">Amount ({mintAsset})</Label>
              <Input
                id="mint-amount"
                type="number"
                min="0"
                step="0.000001"
                placeholder="0.00"
                value={mintAmount}
                onChange={(e) => setMintAmount(e.target.value)}
              />
            </div>
            {mintError && (
              <p className="text-xs text-red-600 break-words">{mintError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowMintModal(false)}
              disabled={isMinting}
            >
              Cancel
            </Button>
            <Button
              onClick={handleMint}
              disabled={isMinting}
              className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
            >
              {isMinting ? "Minting..." : "Mint SFLUV"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
