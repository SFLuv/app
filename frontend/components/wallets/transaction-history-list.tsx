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
      <div className="text-center py-8">
        <div className="h-12 w-12 mx-auto rounded-full bg-muted flex items-center justify-center mb-3">
          <Clock className="h-6 w-6 text-muted-foreground" />
        </div>
        <p className="text-muted-foreground">No transactions yet</p>
        <p className="text-sm text-muted-foreground">Your transaction history will appear here</p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {transactions.map((transaction) => {
        const isSent = transaction.fromAddress === walletAddress
        const isReceived = transaction.toAddress === walletAddress

        const getStatusIcon = () => {
          switch (transaction.status) {
            case "confirmed":
              return <CheckCircle className="h-4 w-4 text-green-600" />
            case "pending":
              return <Clock className="h-4 w-4 text-yellow-600" />
            case "failed":
              return <X className="h-4 w-4 text-red-600" />
            default:
              return <Clock className="h-4 w-4 text-gray-400" />
          }
        }

        const getStatusColor = () => {
          switch (transaction.status) {
            case "confirmed":
              return "text-green-600"
            case "pending":
              return "text-yellow-600"
            case "failed":
              return "text-red-600"
            default:
              return "text-gray-400"
          }
        }

        return (
          <div key={transaction.id} className="flex items-center justify-between p-3 rounded-lg border">
            <div className="flex items-center gap-3">
              <Avatar className="h-8 w-8">
                <AvatarFallback className={isSent ? "bg-red-100 text-red-600" : "bg-green-100 text-green-600"}>
                  {isSent ? <ArrowUpRight className="h-4 w-4" /> : <ArrowDownLeft className="h-4 w-4" />}
                </AvatarFallback>
              </Avatar>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <p className="font-medium text-sm">{isSent ? "Sent" : "Received"}</p>
                  {getStatusIcon()}
                </div>
                <p className="text-xs text-muted-foreground truncate">
                  {isSent
                    ? `To: ${transaction.toAddress?.slice(0, 8)}...${transaction.toAddress?.slice(-4)}`
                    : `From: ${transaction.fromAddress?.slice(0, 8)}...${transaction.fromAddress?.slice(-4)}`}
                </p>
                <p className="text-xs text-muted-foreground">{new Date(transaction.timestamp).toLocaleDateString()}</p>
                {transaction.memo && <p className="text-xs text-muted-foreground truncate mt-1">{transaction.memo}</p>}
              </div>
            </div>
            <div className="text-right">
              <p className={`font-medium text-sm ${isSent ? "text-red-600" : "text-green-600"}`}>
                {isSent ? "-" : "+"}
                {transaction.amount.toLocaleString()} {transaction.currency}
              </p>
              <Badge variant="secondary" className={`text-xs ${getStatusColor()}`}>
                {transaction.status}
              </Badge>
              {transaction.hash && (
                <p className="text-xs text-muted-foreground font-mono mt-1">
                  {transaction.hash.slice(0, 4)}...{transaction.hash.slice(-4)}
                </p>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}
