"use client"

import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { ArrowUpRight, ArrowDownLeft, Clock, CheckCircle, X } from "lucide-react"
import type { WalletTransaction } from "@/types/privy-wallet"

interface TransactionHistoryListProps {
  transactions: WalletTransaction[]
  walletAddress: string
}

export function TransactionHistoryList({ transactions, walletAddress }: TransactionHistoryListProps) {
  if (transactions.length === 0) {
    return (
      <div className="text-center py-6 sm:py-8">
        <div className="h-10 w-10 sm:h-12 sm:w-12 mx-auto rounded-full bg-muted flex items-center justify-center mb-3">
          <Clock className="h-5 w-5 sm:h-6 sm:w-6 text-muted-foreground" />
        </div>
        <p className="text-sm sm:text-base text-muted-foreground">No transactions yet</p>
        <p className="text-xs sm:text-sm text-muted-foreground mt-1">Your transaction history will appear here</p>
      </div>
    )
  }

  return (
    <div className="space-y-2 sm:space-y-3">
      {transactions.map((transaction) => {
        const isSent = transaction.type === "send"

        const getStatusIcon = () => {
          switch (transaction.status) {
            case "confirmed":
              return <CheckCircle className="h-3 w-3 sm:h-4 sm:w-4 text-green-600" />
            case "pending":
              return <Clock className="h-3 w-3 sm:h-4 sm:w-4 text-yellow-600" />
            case "failed":
              return <X className="h-3 w-3 sm:h-4 sm:w-4 text-red-600" />
            default:
              return <Clock className="h-3 w-3 sm:h-4 sm:w-4 text-gray-400" />
          }
        }

        const getStatusColor = () => {
          return "text-gray-400"
        }

        return (
          <div
            key={transaction.id}
            className="flex items-center justify-between p-2.5 sm:p-3 rounded-lg border bg-card/50"
          >
            <div className="flex items-center gap-2 sm:gap-3 min-w-0 flex-1">
              <Avatar className="h-7 w-7 sm:h-8 sm:w-8 flex-shrink-0">
                <AvatarFallback className={isSent ? "bg-red-100 text-red-600" : "bg-green-100 text-green-600"}>
                  {isSent ? (
                    <ArrowUpRight className="h-3 w-3 sm:h-4 sm:w-4" />
                  ) : (
                    <ArrowDownLeft className="h-3 w-3 sm:h-4 sm:w-4" />
                  )}
                </AvatarFallback>
              </Avatar>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5 sm:gap-2">
                  <p className="font-medium text-xs sm:text-sm">{isSent ? "Sent" : "Received"}</p>
                  {getStatusIcon()}
                </div>
                <p className="text-xs text-muted-foreground truncate">
                  {isSent
                    ? `To: ${transaction.toAddress?.slice(0, 6)}...${transaction.toAddress?.slice(-4)}`
                    : `From: ${transaction.fromAddress?.slice(0, 6)}...${transaction.fromAddress?.slice(-4)}`}
                </p>
                <div className="flex items-center gap-2 mt-0.5">
                  <p className="text-xs text-muted-foreground">
                    {new Date(transaction.timestamp).toLocaleDateString()}
                  </p>
                  {transaction.hash && (
                    <p className="text-xs text-muted-foreground font-mono hidden sm:block">
                      {transaction.hash.slice(0, 4)}...{transaction.hash.slice(-4)}
                    </p>
                  )}
                </div>
                {transaction.memo && (
                  <p className="text-xs text-muted-foreground truncate mt-1 hidden sm:block">{transaction.memo}</p>
                )}
              </div>
            </div>
            <div className="text-right flex-shrink-0 ml-2">
              <p className={`font-medium text-xs sm:text-sm ${isSent ? "text-red-600" : "text-green-600"}`}>
                {isSent ? "-" : "+"}
                {transaction.amount.toLocaleString()} {transaction.currency}
              </p>
              <Badge variant="secondary" className={`text-xs mt-1 ${getStatusColor()}`}>
                {transaction.status}
              </Badge>
            </div>
          </div>
        )
      })}
    </div>
  )
}
