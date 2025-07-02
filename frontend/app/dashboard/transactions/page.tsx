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
  const transactions = user?.role === "merchant" ? mockMerchantTransactions : mockUserTransactions

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
    router.push(`/dashboard/transactions?id=${transaction.id}`, { scroll: false })
  }

  // Handle modal close
  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedTransaction(null)
    // Remove transaction ID from URL
    router.push("/dashboard/transactions", { scroll: false })
  }

  return (
    <div className="space-y-6 max-w-full overflow-x-hidden">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Transactions</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">
          {user?.role === "merchant" ? "View your transaction history and analytics" : "View your transaction history"}
        </p>
      </div>

      {user?.role === "merchant" ? (
        <Tabs defaultValue="list" value={activeTab} onValueChange={setActiveTab} className="w-full">
          <TabsList className="grid grid-cols-2 w-full mb-6 bg-secondary">
            <TabsTrigger value="list" className="text-black dark:text-white">
              Transaction List
            </TabsTrigger>
            <TabsTrigger value="analytics" className="text-black dark:text-white">
              Analytics
            </TabsTrigger>
          </TabsList>
          <TabsContent value="list">
            <TransactionList
              transactions={transactions}
              onSelectTransaction={handleSelectTransaction}
              userRole={user.role}
            />
          </TabsContent>
          <TabsContent value="analytics">
            <TransactionAnalytics analytics={mockMerchantAnalytics} />
          </TabsContent>
        </Tabs>
      ) : (
        <TransactionList
          transactions={transactions}
          onSelectTransaction={handleSelectTransaction}
          userRole={user?.role || "user"}
        />
      )}

      <TransactionModal transaction={selectedTransaction} isOpen={isModalOpen} onClose={handleCloseModal} />
    </div>
  )
}
