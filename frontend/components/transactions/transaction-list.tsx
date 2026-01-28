"use client"

import { useState } from "react"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import { Search, Filter, ArrowUpRight, ArrowDownLeft, CheckCircle, AlertCircle, Clock } from "lucide-react"
import type { Transaction } from "@/types/transaction"
import { transactionTypeLabels } from "@/types/transaction"
import { SYMBOL } from "@/lib/constants"
import { useApp } from "@/context/AppProvider"
import { useTransactions } from "@/context/TransactionProvider"
import { useContacts } from "@/context/ContactsProvider"

interface TransactionListProps {
  transactions: Transaction[]
  wallet: string
  onSelectTransaction: (transaction: Transaction) => void
}

export function TransactionList({ transactions, onSelectTransaction, wallet }: TransactionListProps) {
  const [searchQuery, setSearchQuery] = useState("")
  const [currentPage, setCurrentPage] = useState(1)
  const [typeFilter, setTypeFilter] = useState<string>("all")

  const { user, status } = useApp()
  const { contacts } = useContacts()
  const { transactionsStatus } = useTransactions()

  const ITEMS_PER_PAGE = 10

  // Get available transaction types based on user role
  const getTransactionTypes = () => {
    return [
      { value: "all", label: "All Types" },
      { value: "volunteer_reward", label: "Volunteer Reward" },
      { value: "currency_unwrap", label: "Currency Unwrap" },
      { value: "currency_transfer", label: "Currency Transfer" },
    ]
  }

  // Filter transactions
  const filteredTransactions = transactions.filter((transaction) => {
    // Filter by type
    if (typeFilter !== "all" && transaction.type !== typeFilter) {
      return false
    }

    // Filter by search query
    if (searchQuery) {
      const query = searchQuery.toLowerCase()
      return (
        transaction.fromName?.toLowerCase().includes(query) ||
        transaction.toName?.toLowerCase().includes(query) ||
        transaction.description?.toLowerCase().includes(query) ||
        transaction.transactionId.toLowerCase().includes(query) ||
        (transaction.category && transaction.category.toLowerCase().includes(query))
      )
    }

    return true
  })

  // Calculate pagination
  const paginatedTransactions = filteredTransactions.slice(
    (currentPage - 1) * ITEMS_PER_PAGE,
    currentPage * ITEMS_PER_PAGE,
  )

  // Format date
  const formatDate = (dateString: string) => {
    const date = new Date(Number(dateString) * 1000)
    return date.toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    })
  }

  // Get status icon
  const getStatusIcon = (status: string) => {
    switch (status) {
      case "completed":
        return <CheckCircle className="h-4 w-4 text-green-500" />
      case "pending":
        return <Clock className="h-4 w-4 text-yellow-500" />
      case "failed":
        return <AlertCircle className="h-4 w-4 text-red-500" />
      default:
        return null
    }
  }

  if (status === "loading" || transactionsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
          <Input
            placeholder="Search transactions..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10 text-black dark:text-white bg-secondary"
          />
        </div>

        <Select value={typeFilter} onValueChange={setTypeFilter}>
          <SelectTrigger className="w-[180px] text-black dark:text-white bg-secondary">
            <SelectValue placeholder="Filter by type" />
          </SelectTrigger>
          <SelectContent>
            {getTransactionTypes().map((type) => (
              <SelectItem key={type.value} value={type.value}>
                {type.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button
          variant="outline"
          className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
          onClick={() => {
            setSearchQuery("")
            setTypeFilter("all")
          }}
        >
          <Filter className="h-4 w-4 mr-2" />
          Reset Filters
        </Button>
      </div>

      <div className="space-y-4">
        {paginatedTransactions.length === 0 ? (
          <div className="text-center py-12">
            <h3 className="text-xl font-medium text-black dark:text-white">No transactions found</h3>
            <p className="text-gray-600 dark:text-gray-400 mt-2">
              Try adjusting your search or filters to find transactions
            </p>
          </div>
        ) : (
          paginatedTransactions.map((transaction) => {
            const received = transaction.toAddress.toLowerCase() === wallet.toLowerCase()

            return (
              <Card
                key={transaction.id}
                className="overflow-hidden cursor-pointer hover:shadow-md transition-shadow"
                onClick={() => onSelectTransaction(transaction)}
              >
                <CardContent className="p-4 overflow-hidden">
                  <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                    <div className="flex items-center gap-3 min-w-0">
                      <div
                        className={`flex-shrink-0 flex items-center justify-center w-10 h-10 rounded-full ${
                          received ? "bg-green-100" : "bg-red-100"
                        }`}
                      >
                        {received ? (
                          <ArrowDownLeft className="h-5 w-5 text-green-600" />
                        ) : (
                          <ArrowUpRight className="h-5 w-5 text-red-600" />
                        )}
                      </div>
                      <div className="min-w-0">
                        <h3 className="font-medium text-black dark:text-white truncate">
                          {received
                            ? `Received from ${transaction.fromName || transaction.fromAddress.slice(0, 4) + "..." + transaction.fromAddress.slice(-4)}`
                            : `Sent to ${transaction.toName || transaction.toAddress.slice(0, 4) + "..." + transaction.toAddress.slice(-4)}`}
                        </h3>
                        <p className="text-sm text-gray-600 dark:text-gray-400">{formatDate(transaction.timestamp)}</p>
                      </div>
                    </div>

                    <div className="flex flex-wrap items-center gap-2 md:text-right">
                      <Badge variant="outline" className="bg-secondary text-black dark:text-white">
                        {transactionTypeLabels[transaction.type]}
                      </Badge>
                      <div className="flex items-center gap-1">
                        {getStatusIcon(transaction.status)}
                        <span className="text-sm text-gray-600 dark:text-gray-400">
                          {transaction.status.charAt(0).toUpperCase() + transaction.status.slice(1)}
                        </span>
                      </div>
                      <span
                        className={`font-bold ${received ? "text-green-600" : "text-red-600"} md:ml-4`}
                      >
                        {received ? "+" : "-"}
                        {transaction.amount} {SYMBOL}
                      </span>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })
        )}
      </div>
    </div>
  )
}
