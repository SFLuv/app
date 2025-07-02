// Re-export Privy's ConnectedWallet type and extend it if needed
export type { ConnectedWallet } from "@privy-io/react-auth"

// Additional wallet-related types that extend Privy's types
export interface WalletTransaction {
  id: string
  type: "send" | "receive"
  amount: number
  currency: string
  fromAddress?: string
  toAddress?: string
  status: "pending" | "confirmed" | "failed"
  timestamp: string
  hash?: string
  memo?: string
}

export interface WalletBalance {
  currency: string
  balance: number
  usdValue: number
  priceChange24h: number
}
