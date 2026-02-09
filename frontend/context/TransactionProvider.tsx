import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { useApp } from "./AppProvider";
import { ServerTransaction, Transaction, TransactionStatus, TransactionType } from "@/types/transaction";
import { Options } from "react-to-pdf";
import { useContacts } from "./ContactsProvider";
import { Server } from "http";
import { FAUCET_ADDRESS, HONEY_TOKEN, SFLUV_DECIMALS } from "@/lib/constants";
import { WalletTransaction } from "@/types/privy-wallet";

interface TransactionContext {
  transactionsStatus: TransactionsStatus
  transactionsError: string | null

  getTransactionsPage: (address: string, page: number, options: TransactionOptions) => Promise<WalletPage>
  toWalletTransaction: (owner: string, tx: Transaction) => WalletTransaction
  refreshTransactions: (address: string, page: number, options: TransactionOptions) => Promise<WalletPage>
}


interface WalletTransactions {
  pages: Transaction[][]
  total: number
}

interface WalletPage {
  txs: Transaction[]
  page: number
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
  const { status, authFetch } = useApp()
  const { contacts, contactsStatus } = useContacts()
  const [transactions, setTransactions] = useState<Record<string, WalletTransactions>>({})
  const [transactionsError, setTransactionsError] = useState<string | null>(null)
  const [transactionsStatus, setTransactionsStatus] = useState<TransactionsStatus>("ready")


  useEffect(() => {
    if(status == "unauthenticated") {
      setTransactions({})
      setTransactionsError(null)
    }
  }, [status])

  useEffect(() => {
    if(transactionsError) {
      console.error(transactionsError)
    }
  }, [transactionsError])

  const getTransactionsPage = async (address: string, page: number, options: TransactionOptions): Promise<WalletPage> => {
    const paginationString = "&count=" + (options.paginationDetails.count || 10)+ "&desc=" + (options.paginationDetails.desc ?? true)

    try {
      let txPage: WalletPage = {
        txs: transactions[paginationString]?.pages[page],
        page,
        total: transactions[paginationString]?.total
      }
      if(!txPage?.txs?.length) {
        txPage = await _fillTransactionsPage(address, page, options.paginationDetails)
        console.log(txPage)
      }

      return txPage
    }
    catch(error) {
      console.error(error)
      setTransactionsError("Error getting transactions page.")
      return {
        txs: [],
        page,
        total: 0
      }
    }
  }

  const refreshTransactions = async (address: string, page: number, options: TransactionOptions): Promise<WalletPage> => {
    setTransactions({})
    setTransactionsError(null)
    return getTransactionsPage(address, page, options)
  }

  const toWalletTransaction = (owner: string, tx: Transaction): WalletTransaction => {
    const id = tx.id
    const type = owner.toLowerCase() === tx.fromAddress.toLowerCase() ? "send" : "receive"
    const amount = tx.amount
    const currency = "SFLUV"
    const fromAddress = tx.fromAddress
    const toAddress = tx.toAddress
    const status = "confirmed"
    const timestamp = tx.timestamp
    const hash = tx.transactionId

    return {
      id,
      type,
      amount,
      currency,
      fromAddress,
      toAddress,
      status,
      timestamp,
      hash
    }
  }

  const _fillTransactionsPage = async (address: string, page: number, paginationDetails: PaginationDetails): Promise<WalletPage>  => {
    const paginationString = "&count=" + (paginationDetails.count || 10)+ "&desc=" + (paginationDetails.desc ?? true)
    const pageName = address + paginationString
    setTransactionsStatus("loading")
    if(transactions[pageName]?.total < page * paginationDetails.count - 1) {
      setTransactionsError("Out of bounds.")
      return {
        txs: [],
        page,
        total: 0
      }
    }
    try {
      const res = await fetchTransactionsWithRetry("/transactions" + "?address=" + address + "&page=" + page + paginationString)
      const txPage = await res.json()
      const newTransactions = { ...transactions }

      newTransactions[pageName] = transactions[pageName] || { pages: [], total: 0 }
      newTransactions[pageName].pages[page] = txPage.transactions.map(_txResponseToAppTx)
      newTransactions[pageName].total = txPage.total

      setTransactions(newTransactions)
      setTransactionsError(null)

      return {
        txs: newTransactions[pageName]?.pages[page],
        page,
        total: txPage.total
      }
    }
    catch(error: any) {
      console.error(error)
      setTransactionsError(error?.message || "Error fetching new transactions page.")
      return {
        txs: [],
        page,
        total: 0
      }
    }
    finally {
      setTransactionsStatus("ready")
    }
  }

  const fetchTransactionsWithRetry = async (endpoint: string): Promise<Response> => {
    const maxAttempts = 3
    for(let attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        return await authFetch(endpoint)
      }
      catch(error: any) {
        const message = error?.message || ""
        if(message.includes("no access token") && attempt < maxAttempts - 1) {
          await new Promise((resolve) => setTimeout(resolve, 400))
          continue
        }
        throw error
      }
    }
    throw new Error("Error fetching transactions.")
  }


  const _txResponseToAppTx = (tx: ServerTransaction): Transaction => {
    const id = tx.id
    const type = _getTxType(tx)
    const amount = Number(BigInt(tx.amount) / BigInt(10 ** (SFLUV_DECIMALS - 2))) / 100
    const timestamp = String(tx.timestamp)
    const status = "completed"
    const fromName = contacts.find((c) => c.address.toLowerCase() === tx.from.toLowerCase())?.name
    const fromAddress = tx.from
    const toName = contacts.find((c) => c.address.toLowerCase() === tx.to.toLowerCase())?.name
    const toAddress = tx.to
    const transactionId = tx.hash

    return {
      id,
      type,
      amount,
      timestamp,
      status,
      fromName,
      fromAddress,
      toName,
      toAddress,
      transactionId
    }
  }


  const _getTxType = (tx: ServerTransaction): TransactionType => {
    const to = tx?.to?.toLowerCase ? tx.to.toLowerCase() : ""
    const from = tx?.from?.toLowerCase ? tx.from.toLowerCase() : ""
    if(to === "0x0000000000000000000000000000000000000000") {
      return "currency_unwrap"
    }
    if(FAUCET_ADDRESS && from === FAUCET_ADDRESS.toLowerCase()) {
      return "volunteer_reward"
    }

    return "currency_transfer"
  }


  return (
    <TransactionContext.Provider
      value={{
        transactionsStatus,
        transactionsError,
        getTransactionsPage,
        toWalletTransaction,
        refreshTransactions
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
