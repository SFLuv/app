"use client"

import { useMemo, useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Copy, ExternalLink, CheckCircle, AlertCircle, Clock } from "lucide-react"
import type { Transaction } from "@/types/transaction"
import { transactionTypeLabels } from "@/types/transaction"
import { SYMBOL } from "@/lib/constants"

interface TransactionModalProps {
  transaction: Transaction | null
  wallet: string
  isOpen: boolean
  onClose: () => void
}

export function TransactionModal({ transaction, wallet, isOpen, onClose }: TransactionModalProps) {
  const [copied, setCopied] = useState<string | null>(null)

  const received = useMemo(() => {
    return transaction?.toAddress.toLowerCase() === wallet.toLowerCase()
  }, [transaction])

  if (!transaction) return null

  const handleCopy = (text: string, field: string) => {
    navigator.clipboard.writeText(text)
    setCopied(field)
    setTimeout(() => setCopied(null), 2000)
  }

  const formatDate = (dateString: string) => {
    const date = new Date(Number(dateString) * 1000)
    return date.toLocaleString("en-US", {
      year: "numeric",
      month: "long",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case "completed":
        return "bg-green-500"
      case "pending":
        return "bg-yellow-500"
      case "failed":
        return "bg-red-500"
      default:
        return "bg-gray-500"
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case "completed":
        return <CheckCircle className="h-4 w-4" />
      case "pending":
        return <Clock className="h-4 w-4" />
      case "failed":
        return <AlertCircle className="h-4 w-4" />
      default:
        return null
    }
  }

  const getAmountColor = (amount: number) => {
    return received ? "text-green-500" : "text-red-500"
  }

  const formatAmount = (amount: number) => {
    return `${received ? "+" : "-"}${amount} ${SYMBOL}`
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="text-2xl text-black dark:text-white">Transaction Details</DialogTitle>
          <DialogDescription className="flex items-center gap-2">
            <Badge variant="outline" className="bg-secondary text-black dark:text-white">
              {transactionTypeLabels[transaction.type]}
            </Badge>
            <Badge className={`${getStatusColor(transaction.status)} text-white flex items-center gap-1`}>
              {getStatusIcon(transaction.status)}
              <span>{transaction.status.charAt(0).toUpperCase() + transaction.status.slice(1)}</span>
            </Badge>
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6">
          <div className="flex flex-col items-center justify-center py-4 border rounded-lg bg-secondary/50">
            <h3 className={`text-3xl font-bold ${getAmountColor(transaction.amount)}`}>
              {formatAmount(transaction.amount)}
            </h3>
            <p className="text-gray-600 dark:text-gray-400 mt-1">{formatDate(transaction.timestamp)}</p>
          </div>

          <div className="space-y-4">
            <div>
              <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">From</h4>
              <p className="text-black dark:text-white font-medium">{transaction.fromName}</p>
              <div className="flex items-center mt-1">
                <code className="text-xs bg-secondary/50 p-1 rounded text-gray-600 dark:text-gray-300 flex-1 overflow-hidden text-ellipsis">
                  {transaction.fromAddress}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 ml-1"
                  onClick={() => handleCopy(transaction.fromAddress, "from")}
                >
                  {copied === "from" ? (
                    <CheckCircle className="h-3 w-3 text-green-500" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>

            <div>
              <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">To</h4>
              <p className="text-black dark:text-white font-medium">{transaction.toName}</p>
              <div className="flex items-center mt-1">
                <code className="text-xs bg-secondary/50 p-1 rounded text-gray-600 dark:text-gray-300 flex-1 overflow-hidden text-ellipsis">
                  {transaction.toAddress}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 ml-1"
                  onClick={() => handleCopy(transaction.toAddress, "to")}
                >
                  {copied === "to" ? <CheckCircle className="h-3 w-3 text-green-500" /> : <Copy className="h-3 w-3" />}
                </Button>
              </div>
            </div>

            {transaction.description && (
              <div>
                <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">Description</h4>
                <p className="text-black dark:text-white">{transaction.description}</p>
              </div>
            )}

            {transaction.category && (
              <div>
                <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">Category</h4>
                <Badge variant="outline" className="bg-secondary text-black dark:text-white mt-1">
                  {transaction.category}
                </Badge>
              </div>
            )}

            <div>
              <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">Transaction ID</h4>
              <div className="flex items-center mt-1">
                <code className="text-xs bg-secondary/50 p-1 rounded text-gray-600 dark:text-gray-300 flex-1 overflow-hidden text-ellipsis">
                  {transaction.transactionId}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 ml-1"
                  onClick={() => handleCopy(transaction.transactionId, "txid")}
                >
                  {copied === "txid" ? (
                    <CheckCircle className="h-3 w-3 text-green-500" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </Button>
              </div>
            </div>
          </div>
        </div>

        <DialogFooter className="flex justify-between items-center mt-6">
          <Button
            variant="outline"
            className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
            onClick={onClose}
          >
            Close
          </Button>
          <Button
            className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
            onClick={() => window.open(`https://berascan.com/tx/${transaction.transactionId}`, "_blank")}
          >
            <ExternalLink className="h-4 w-4 mr-2" />
            View on Explorer
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
