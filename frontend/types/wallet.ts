export interface Wallet {
  id: string
  name: string
  address: string
  type: WalletType
  isDefault: boolean
  dateAdded: string
  lastUsed: string | null
  balance: number
}

export type WalletType = "metamask" | "coinbase" | "walletconnect" | "trust" | "ledger" | "trezor" | "other"

export const walletTypeLabels: Record<WalletType, string> = {
  metamask: "MetaMask",
  coinbase: "Coinbase Wallet",
  walletconnect: "WalletConnect",
  trust: "Trust Wallet",
  ledger: "Ledger",
  trezor: "Trezor",
  other: "Other",
}

export const walletTypeIcons: Record<WalletType, string> = {
  metamask: "/placeholder.svg?height=40&width=40",
  coinbase: "/placeholder.svg?height=40&width=40",
  walletconnect: "/placeholder.svg?height=40&width=40",
  trust: "/placeholder.svg?height=40&width=40",
  ledger: "/placeholder.svg?height=40&width=40",
  trezor: "/placeholder.svg?height=40&width=40",
  other: "/placeholder.svg?height=40&width=40",
}
