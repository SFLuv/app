import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { useApp } from "./AppProvider";
import { Transaction, TransactionStatus } from "@/types/transaction";
import { Options } from "react-to-pdf";

interface TransactionContext {
  getTransactionsPage: (address: string, page: number, options: TransactionOptions) => Promise<Transaction[]>
}

interface WalletTransactions {
  pages: Transaction[][]
  total: number
}

export interface TransactionOptions {
  paginationDetails: PaginationDetails
}

export interface PaginationDetails {
  count: number
  desc: boolean
}

export type TransactionsStatus = "loading" | "ready"


const TransactionContext = createContext<TransactionContext | null>(null);

export default function TransactionProvider({ children }: { children: ReactNode }) {
  const { status, wallets, authFetch } = useApp()
  const [transactions, setTransactions] = useState<Record<string, WalletTransactions>>({})
  const [transactionsError, setTransactionsError] = useState<string | null>(null)
  const [transactionsStatus, setTransactionsStatus] = useState<TransactionsStatus>("ready")


  useEffect(() => {
    if(status == "unauthenticated") {
      setTransactions({})
      setTransactionsError(null)
    }
  }, [status])

  const getTransactionsPage = async (address: string, page: number, options: TransactionOptions): Promise<Transaction[]> => {
    const paginationString = "&count=" + (options.paginationDetails.count || 10)+ "&desc=" + (options.paginationDetails.desc ?? true)

    try {
      let txPage: Transaction[] = transactions[paginationString]?.pages[page]
      if(!txPage?.length) {
        txPage = await _fillTransactionsPage(address, page, options.paginationDetails)
      }

      return txPage
    }
    catch {
      setTransactionsError("Error getting transactions page.")
      return []
    }
  }

  const _fillTransactionsPage = async (address: string, page: number, paginationDetails: PaginationDetails): Promise<Transaction[]>  => {
    const paginationString = "&count=" + (paginationDetails.count || 10)+ "&desc=" + (paginationDetails.desc ?? true)
    const pageName = address + paginationString
    setTransactionsStatus("loading")
    if(transactions[pageName].total < page * paginationDetails.count - 1) {
      setTransactionsError("Out of bounds.")
      return []
    }

    try {

      const res = await authFetch("/transactions" + "?page=" + page + paginationDetails)
      const txPage = await res.json()
      const newTransactions = { ...transactions }

      newTransactions[pageName] = transactions[pageName]
      newTransactions[pageName].pages[page] = txPage.transactions.map((tx) => {

      })
      newTransactions[pageName].total = txPage.total

      setTransactions(newTransactions)
      setTransactionsError(null)

      return newTransactions[pageName].pages[page]
    }
    catch {
      setTransactionsError("Error fetching new transactions page.")
      return []
    }
    finally {
      setTransactionsStatus("ready")
    }
  }

  return (
    <TransactionContext.Provider
      value={{
        getTransactionsPage
      }}
    >
      {children}
    </TransactionContext.Provider>
  )
}

export function useTransactions() {
  const context = useContext(TransactionContext)
    if (!context) {
    throw new Error("useTransactions must be used within an TransactionProvider");
  }
  return context;
}