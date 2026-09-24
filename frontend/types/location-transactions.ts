// A location's transaction history, as the backend renders it with the refund
// ledger already applied. The refund marks are computed server-side so the web
// panel and the till app cannot disagree about what is fully refunded.

export type RefundRecord = {
  id: number
  original_tx_hash: string
  refund_tx_hash: string
  location_id?: number
  owner_id: string
  from_address: string
  to_address: string
  amount: string
  chain_id?: number
  created_at: string
}

export type RefundStatus = "none" | "partially_refunded" | "refunded" | "refund"

export type RefundState = {
  status: RefundStatus
  refunded_base?: string
  remaining_base?: string
  refunds?: RefundRecord[]
  /** Set on a refund line: the payment this refund undid. */
  refunds_original_hash?: string
}

export type LocationTransaction = {
  hash: string
  from: string
  to: string
  amount_base: string
  timestamp: number
  direction: "in" | "out"
  wallet: "payment" | "tipping"
  refund: RefundState
}

export type LocationTransactionsResponse = {
  location_id: number
  token_decimals: number
  transactions: LocationTransaction[]
  total: number
  page: number
}
