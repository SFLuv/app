"use client"

import { useState, useEffect } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { TransactionList } from "@/components/transactions/transaction-list"
import { TransactionAnalytics } from "@/components/transactions/transaction-analytics"
import { TransactionModal } from "@/components/transactions/transaction-modal"
import { useApp } from "@/context/AppProvider"
import { mockMerchantTransactions, mockUserTransactions, mockMerchantAnalytics } from "@/data/mock-transactions"
import type { Transaction } from "@/types/transaction"
import { Button } from "@/components/ui/button"
import { ArrowLeft } from "lucide-react"

export default function TransactionsPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { user } = useApp()
  const [activeTab, setActiveTab] = useState("list")
  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)

  // Get transaction ID from URL if present
  const transactionId = searchParams.get("id")

  // Determine which transactions to show based on user role
  const transactions = user?.isMerchant ? mockMerchantTransactions : mockUserTransactions

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
    router.push(`/transactions?id=${transaction.id}`, { scroll: false })
  }

  // Handle modal close
  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedTransaction(null)
    // Remove transaction ID from URL
    router.push("/transactions", { scroll: false })
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

          {user?.isMerchant ? (
            <Tabs defaultValue="list" value={activeTab} onValueChange={setActiveTab} className="w-full">
              <TabsList className="grid grid-cols-1 w-full mb-6 bg-secondary">
                <TabsTrigger value="list" className="text-black dark:text-white">
                  Transaction List
                </TabsTrigger>
                {/* <TabsTrigger value="analytics" className="text-black dark:text-white">
                  Analytics
                </TabsTrigger> */}
              </TabsList>
              <TabsContent value="list">
                <TransactionList
                  transactions={transactions}
                  onSelectTransaction={handleSelectTransaction}
                />
              </TabsContent>
              {/* <TabsContent value="analytics">
                <TransactionAnalytics analytics={mockMerchantAnalytics} />
              </TabsContent> */}
            </Tabs>
          ) : (
            <TransactionList
              transactions={transactions}
              onSelectTransaction={handleSelectTransaction}
            />
          )}

          <TransactionModal transaction={selectedTransaction} isOpen={isModalOpen} onClose={handleCloseModal} />
        </div>
      </div>
    </div>
  )
}
