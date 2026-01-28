"use client"

import { useState, useEffect } from "react"
import { useParams, useRouter, useSearchParams } from "next/navigation"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { TransactionList } from "@/components/transactions/transaction-list"
import { TransactionAnalytics } from "@/components/transactions/transaction-analytics"
import { TransactionModal } from "@/components/transactions/transaction-modal"
import { useApp } from "@/context/AppProvider"
import type { Transaction } from "@/types/transaction"
import { Pagination } from "@/components/opportunities/pagination"
import { Button } from "@/components/ui/button"
import { ArrowLeft } from "lucide-react"
import { useTransactions } from "@/context/TransactionProvider"

export default function TransactionsPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { user, status, walletsStatus } = useApp()
  const [activeTab, setActiveTab] = useState("list")
  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null)
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [currentPage, setCurrentPage] = useState(0)
  const [totalPages, setTotalPages] = useState(0)

  const ITEMS_PER_PAGE = 10


  const params = useParams()
  const walletAddress = params.address as string

  const {
    getTransactionsPage,
    toWalletTransaction
  } = useTransactions()

  useEffect(() => {
    txPageHandler()
  }, [currentPage])


  const txPageHandler = async () => {
    console.log("fetching")

    const walletTxs = (await getTransactionsPage(walletAddress, currentPage, {
      paginationDetails: {
        count: ITEMS_PER_PAGE,
        desc: true
      }
    }))

    console.log(walletTxs)

    setTransactions(walletTxs.txs)
    setTotalPages(Math.ceil(walletTxs.total / ITEMS_PER_PAGE))
  }

  // Get transaction ID from URL if present
  const transactionId = searchParams.get("id")

  // Determine which transactions to show based on user role

  // Find transaction by ID if provided in URL
  useEffect(() => {
    if (transactionId) {
      const transaction = transactions.find((tx) => tx.id === transactionId)
      if (transaction) {
        setSelectedTransaction(transaction)
        setIsModalOpen(true)
      }
    }
  }, [transactionId, transactions])

  // Handle transaction selection
  const handleSelectTransaction = (transaction: Transaction) => {
    setSelectedTransaction(transaction)
    setIsModalOpen(true)
    // Update URL with transaction ID
  }

  // Handle modal close
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
    <div className="min-h-screen bg-background mt--6">
      {/* Fixed Mobile Header with proper z-index */}
      <div className="relative top-0 z-50 bg-background/95 backdrop-blur-md supports-[backdrop-filter]:bg-background/80 border-b shadow-sm">
        <div className="space-y-6 max-w-full">
          <div>
            <h1 className="text-3xl font-bold text-black dark:text-white">
              <p className="flex-1">
              <Button variant="ghost" size="sm" onClick={() => router.back()} className="flex-shrink-0 mr-2">
                <ArrowLeft className="h-4 w-4" />
              </Button>
              Transactions
              </p>
            </h1>
            <p className="text-gray-600 dark:text-gray-400 mt-1">
              {user?.isMerchant ? "View your transaction history and analytics" : "View your transaction history"}
            </p>
          </div>

          {/* {user?.isMerchant ? (
            <Tabs defaultValue="list" value={activeTab} onValueChange={setActiveTab} className="w-full">
              <TabsList className="grid grid-cols-1 w-full mb-6 bg-secondary">
                <TabsTrigger value="list" className="text-black dark:text-white">
                  Transaction List
                </TabsTrigger>
                <TabsTrigger value="analytics" className="text-black dark:text-white">
                  Analytics
                </TabsTrigger>
              </TabsList>
              <TabsContent value="list">
                <TransactionList
                  wallet={walletAddress}
                  transactions={transactions}
                  onSelectTransaction={handleSelectTransaction}
                />
              </TabsContent>
              <TabsContent value="analytics">
                <TransactionAnalytics analytics={mockMerchantAnalytics} />
              </TabsContent>
            </Tabs>
          ) : ( */}
            <>
              <TransactionList
                wallet={walletAddress}
                transactions={transactions}
                onSelectTransaction={handleSelectTransaction}
              />

            {totalPages > 1 && (
              <div className="mt-8">
                <Pagination currentPage={currentPage + 1} totalPages={totalPages} onPageChange={(p) => setCurrentPage(p-1)} />
              </div>
            )}
            <div className="pb-1 text-sm text-gray-500 dark:text-gray-400">
              Showing {transactions.length} transaction{transactions.length === 1 ? "" : "s"}.
            </div>

            </>
          {/* )} */}

          <TransactionModal transaction={selectedTransaction} wallet={walletAddress} isOpen={isModalOpen} onClose={handleCloseModal} />

        </div>
      </div>
    </div>
  )
}
