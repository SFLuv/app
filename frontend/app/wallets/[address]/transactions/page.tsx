"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation"
import { TransactionList } from "@/components/transactions/transaction-list"
import { TransactionModal } from "@/components/transactions/transaction-modal"
import { useApp } from "@/context/AppProvider"
import type { Transaction } from "@/types/transaction"
import { Pagination } from "@/components/opportunities/pagination"
import { Button } from "@/components/ui/button"
import { ArrowLeft } from "lucide-react"
import { useTransactions } from "@/context/TransactionProvider"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { SFLUV_DECIMALS, SYMBOL } from "@/lib/constants"

const ITEMS_PER_PAGE = 10
type BalanceUpdateResult = "changed" | "unchanged" | "unknown"

export default function TransactionsPage() {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const params = useParams()
  const walletAddress = params.address as string
  const { status, walletsStatus, wallets, authFetch } = useApp()
  const { getTransactionsPage, refreshTransactions } = useTransactions()

  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null)
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [isModalOpen, setIsModalOpen] = useState(false)
  const pageFromQuery = Number(searchParams.get("page") || "1")
  const [currentPage, setCurrentPage] = useState(
    Number.isFinite(pageFromQuery) && pageFromQuery >= 1 ? pageFromQuery - 1 : 0
  )
  const [totalPages, setTotalPages] = useState(0)

  const [historicalModalOpen, setHistoricalModalOpen] = useState<boolean>(false)
  const [balanceAtDate, setBalanceAtDate] = useState<string>("")
  const [balanceAtTime, setBalanceAtTime] = useState<string>("23:59")
  const [historicalBalanceWei, setHistoricalBalanceWei] = useState<string | null>(null)
  const [historicalBalanceTimestamp, setHistoricalBalanceTimestamp] = useState<number | null>(null)
  const [historicalBalanceLoading, setHistoricalBalanceLoading] = useState<boolean>(false)
  const [historicalBalanceError, setHistoricalBalanceError] = useState<string | null>(null)

  const latestTxMarkerRef = useRef<string | null>(null)
  const txPollingInFlightRef = useRef(false)
  const txLastPollAtRef = useRef(0)
  const balanceSnapshotRef = useRef<number | null>(null)
  const balanceRetryTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const getTransactionsPageRef = useRef(getTransactionsPage)
  const refreshTransactionsRef = useRef(refreshTransactions)
  const updateWalletBalanceWithRetryRef = useRef<() => Promise<void>>(async () => {})

  useEffect(() => {
    getTransactionsPageRef.current = getTransactionsPage
  }, [getTransactionsPage])

  useEffect(() => {
    refreshTransactionsRef.current = refreshTransactions
  }, [refreshTransactions])

  const wallet = useMemo(() => {
    return wallets.find((w) => w.address?.toLowerCase() === walletAddress.toLowerCase())
  }, [walletAddress, wallets])
  const displayWalletAddress = useMemo(() => {
    if (!walletAddress) return ""
    if (walletAddress.length <= 12) return walletAddress
    return `${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}`
  }, [walletAddress])

  const getLatestTxMarker = useCallback((txs: Transaction[]) => {
    if (txs.length === 0) return null
    const latest = txs[0]
    return `${latest.id}:${latest.transactionId || ""}`
  }, [])

  const updateWalletBalance = useCallback(async (): Promise<BalanceUpdateResult> => {
    if (!wallet) return "unknown"
    try {
      const balance = await wallet.getSFLUVBalanceFormatted()
      if (balance === null) {
        return "unknown"
      }
      const previous = balanceSnapshotRef.current
      balanceSnapshotRef.current = balance
      if (previous === null) return "changed"
      return balance !== previous ? "changed" : "unchanged"
    } catch (balanceError) {
      console.error(balanceError)
      return "unknown"
    }
  }, [wallet])

  useEffect(() => {
    let active = true

    const seedBalanceSnapshot = async () => {
      if (!wallet) return
      try {
        const initialBalance = await wallet.getSFLUVBalanceFormatted()
        if (active && initialBalance !== null) {
          balanceSnapshotRef.current = initialBalance
        }
      } catch (balanceError) {
        console.error(balanceError)
      }
    }

    void seedBalanceSnapshot()

    return () => {
      active = false
    }
  }, [wallet])

  useEffect(() => {
    return () => {
      if (balanceRetryTimeoutRef.current) {
        clearTimeout(balanceRetryTimeoutRef.current)
      }
    }
  }, [])

  const updateWalletBalanceWithRetry = useCallback(async () => {
    const balanceUpdate = await updateWalletBalance()
    if (balanceUpdate !== "unchanged") {
      return
    }
    if (balanceRetryTimeoutRef.current) {
      clearTimeout(balanceRetryTimeoutRef.current)
    }
    balanceRetryTimeoutRef.current = setTimeout(() => {
      void updateWalletBalance()
      balanceRetryTimeoutRef.current = null
    }, 2000)
  }, [updateWalletBalance])

  useEffect(() => {
    updateWalletBalanceWithRetryRef.current = updateWalletBalanceWithRetry
  }, [updateWalletBalanceWithRetry])

  const loadTransactionsPage = useCallback(async (page: number) => {
    const walletTxs = await getTransactionsPageRef.current(walletAddress, page, {
      paginationDetails: {
        count: ITEMS_PER_PAGE,
        desc: true
      }
    })

    setTransactions(walletTxs.txs)
    setTotalPages(Math.ceil(walletTxs.total / ITEMS_PER_PAGE))
    if (page === 0) {
      latestTxMarkerRef.current = getLatestTxMarker(walletTxs.txs)
    }
  }, [getLatestTxMarker, walletAddress])

  useEffect(() => {
    void loadTransactionsPage(currentPage)
  }, [currentPage, loadTransactionsPage])

  useEffect(() => {
    const rawPage = Number(searchParams.get("page") || "1")
    const normalizedPage = Number.isFinite(rawPage) && rawPage >= 1 ? rawPage - 1 : 0
    setCurrentPage((prev) => (normalizedPage === prev ? prev : normalizedPage))
  }, [searchParams])

  useEffect(() => {
    const params = new URLSearchParams(searchParams.toString())
    if (currentPage > 0) {
      params.set("page", String(currentPage + 1))
    } else {
      params.delete("page")
    }
    const nextQuery = params.toString()
    if (nextQuery !== searchParams.toString()) {
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false })
    }
  }, [currentPage, pathname, router, searchParams])

  useEffect(() => {
    if (status !== "authenticated" || walletsStatus !== "available") return

    let active = true
    txLastPollAtRef.current = 0

    const pollTransactions = async () => {
      if (!active || txPollingInFlightRef.current) return
      const now = Date.now()
      if (now - txLastPollAtRef.current < 2000) return
      txPollingInFlightRef.current = true
      txLastPollAtRef.current = now

      try {
        const pageResult = await refreshTransactionsRef.current(walletAddress, currentPage, {
          paginationDetails: {
            count: ITEMS_PER_PAGE,
            desc: true
          }
        })

        if (!active) return

        setTransactions(pageResult.txs)
        setTotalPages(Math.ceil(pageResult.total / ITEMS_PER_PAGE))

        let newestMarker: string | null
        if (currentPage === 0) {
          newestMarker = getLatestTxMarker(pageResult.txs)
        } else {
          const newestPage = await getTransactionsPageRef.current(walletAddress, 0, {
            paginationDetails: {
              count: ITEMS_PER_PAGE,
              desc: true
            }
          })
          newestMarker = getLatestTxMarker(newestPage.txs)
        }

        const previousMarker = latestTxMarkerRef.current
        latestTxMarkerRef.current = newestMarker

        if (newestMarker && newestMarker !== previousMarker) {
          await updateWalletBalanceWithRetryRef.current()
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
  }, [
    currentPage,
    getLatestTxMarker,
    status,
    walletAddress,
    walletsStatus
  ])

  const formatWeiToTwoDecimals = (wei: string) => {
    try {
      const value = BigInt(wei)
      const negative = value < 0n
      const abs = negative ? -value : value
      const scale = BigInt(10) ** BigInt(SFLUV_DECIMALS)
      const cents = (abs * 100n) / scale
      const whole = cents / 100n
      const fraction = cents % 100n
      const formatted = `${whole.toString()}.${fraction.toString().padStart(2, "0")}`
      return negative ? `-${formatted}` : formatted
    } catch {
      return "0.00"
    }
  }

  const formatTimestamp = (timestamp?: number | null) => {
    if (!timestamp) return ""
    const date = new Date(timestamp * 1000)
    return date.toLocaleString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit"
    })
  }

  useEffect(() => {
    if (!historicalModalOpen) return
    if (!balanceAtDate) {
      const today = new Date()
      const isoDate = new Date(today.getTime() - today.getTimezoneOffset() * 60000).toISOString().slice(0, 10)
      setBalanceAtDate(isoDate)
    }
    setBalanceAtTime("23:59")
  }, [balanceAtDate, historicalModalOpen])

  const handleBalanceAtTime = async () => {
    const targetAddress = wallet?.address || walletAddress
    if (!targetAddress) return
    if (!balanceAtDate) {
      setHistoricalBalanceError("Please select a date.")
      return
    }

    const timeValue = balanceAtTime || "23:59"
    const timeWithSeconds = timeValue.length === 5 ? `${timeValue}:00` : timeValue
    const parsed = new Date(`${balanceAtDate}T${timeWithSeconds}`).getTime()
    if (Number.isNaN(parsed)) {
      setHistoricalBalanceError("Invalid date/time.")
      return
    }

    const timestamp = Math.floor(parsed / 1000)
    setHistoricalBalanceLoading(true)
    setHistoricalBalanceError(null)
    try {
      const res = await authFetch(`/transactions/balance?address=${encodeURIComponent(targetAddress)}&timestamp=${timestamp}`)
      if (!res.ok) {
        throw new Error("Failed to fetch balance at time.")
      }
      const data = await res.json()
      setHistoricalBalanceWei(data?.balance ?? "0")
      setHistoricalBalanceTimestamp(data?.timestamp ?? timestamp)
    } catch (error) {
      console.error(error)
      setHistoricalBalanceError("Unable to load balance at that time.")
      setHistoricalBalanceWei(null)
      setHistoricalBalanceTimestamp(null)
    } finally {
      setHistoricalBalanceLoading(false)
    }
  }

  const transactionId = searchParams.get("id")

  useEffect(() => {
    if (!transactionId) return
    const transaction = transactions.find((tx) => tx.id === transactionId)
    if (transaction) {
      setSelectedTransaction(transaction)
      setIsModalOpen(true)
    }
  }, [transactionId, transactions])

  const handleSelectTransaction = (transaction: Transaction) => {
    setSelectedTransaction(transaction)
    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedTransaction(null)
  }

  if (status === "loading" || walletsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background">
      <div className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/85">
        <div className="mx-auto w-full max-w-5xl px-3 py-3 sm:px-6 sm:py-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex min-w-0 items-center gap-2">
              <Button variant="ghost" size="sm" onClick={() => router.back()} className="flex-shrink-0">
                <ArrowLeft className="h-4 w-4" />
              </Button>
              <h1 className="truncate text-lg font-bold text-black dark:text-white sm:text-3xl">
                Transactions
              </h1>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="ml-11 h-8 w-fit px-2 text-[11px] sm:ml-0 sm:h-9 sm:px-3 sm:text-xs"
              onClick={() => setHistoricalModalOpen(true)}
            >
              Get Historical Balance
            </Button>
          </div>
          <div className="mt-2 pl-11 sm:pl-12">
            <span className="inline-flex items-center gap-1.5 rounded-md border bg-muted/40 px-2 py-1 text-[11px] sm:text-xs">
              <span className="font-medium text-foreground">{wallet?.name || "Wallet"}</span>
              <span className="font-mono text-muted-foreground">{displayWalletAddress}</span>
            </span>
          </div>
        </div>
      </div>

      <div className="mx-auto w-full max-w-5xl space-y-4 px-3 py-4 sm:space-y-6 sm:px-6 sm:py-6">
        <TransactionList
          wallet={walletAddress}
          transactions={transactions}
          onSelectTransaction={handleSelectTransaction}
        />

        {totalPages > 1 && (
          <div className="rounded-lg border bg-card/50 p-2 sm:p-3">
            <Pagination currentPage={currentPage + 1} totalPages={totalPages} onPageChange={(p) => setCurrentPage(p - 1)} />
          </div>
        )}
        <div className="pb-2 text-xs text-gray-500 dark:text-gray-400 sm:text-sm">
          Showing {transactions.length} transaction{transactions.length === 1 ? "" : "s"}.
        </div>
      </div>

      <TransactionModal transaction={selectedTransaction} wallet={walletAddress} isOpen={isModalOpen} onClose={handleCloseModal} />

      <Dialog open={historicalModalOpen} onOpenChange={setHistoricalModalOpen}>
        <DialogContent className="sm:max-w-[520px]">
          <DialogHeader>
            <DialogTitle>Historical Balance</DialogTitle>
            <DialogDescription>
              Select a day to see the balance at the end of that day (23:59).
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="balance-date" className="text-xs sm:text-sm">Date</Label>
              <Input
                id="balance-date"
                type="date"
                value={balanceAtDate}
                onChange={(e) => setBalanceAtDate(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="balance-time" className="text-xs sm:text-sm">Time</Label>
              <Input
                id="balance-time"
                type="time"
                step="60"
                value={balanceAtTime}
                onChange={(e) => setBalanceAtTime(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Time defaults to 23:59 whenever you change the date.
              </p>
            </div>
            {historicalBalanceError && (
              <p className="text-xs text-red-600">{historicalBalanceError}</p>
            )}
            {historicalBalanceWei !== null && historicalBalanceTimestamp && (
              <div className="rounded-md bg-secondary/50 p-3 text-sm space-y-1">
                <p className="text-xs text-muted-foreground">
                  As of {formatTimestamp(historicalBalanceTimestamp)}
                </p>
                <p className="text-lg font-semibold">
                  {formatWeiToTwoDecimals(historicalBalanceWei)} {SYMBOL}
                </p>
                {BigInt(historicalBalanceWei) < 0n && (
                  <p className="text-xs text-amber-600">
                    History may be incomplete before the indexer start block.
                  </p>
                )}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setHistoricalModalOpen(false)}>
              Close
            </Button>
            <Button
              onClick={handleBalanceAtTime}
              disabled={historicalBalanceLoading}
              className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
            >
              {historicalBalanceLoading ? "Checking..." : "Get Balance"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
