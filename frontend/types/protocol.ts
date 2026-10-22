/** The admin Protocol tab: the backend's own Safes and the keys that sign for them. */

export interface ProtocolSigner {
  id: number
  account_key: string
  eoa_address: string
  created_at: number
  activated_at: number | null
  activation_tx: string
  retired_at: number | null
  retired_reason: string
  rotated_by: string
}

export interface ProtocolAccount {
  key: string
  label: string
  description: string
  role: string
  chain_id: number
  /** Empty until a signer exists to derive it from. */
  safe_address: string
  owner_index: number
  deployed_at: number | null
  deploy_tx: string
  created_at: number
  updated_at: number
  signer: ProtocolSigner | null
}

/** One stretch of time during which an address held a protocol role. */
export interface ProtocolAddressWindow {
  id: number
  address: string
  role: string
  chain_id: number
  account_key: string
  started_at: number
  /** null means it is the current holder. */
  ended_at: number | null
  source: string
  note: string
}

export interface ProtocolMigrationStep {
  step: string
  status: "pending" | "running" | "done" | "failed" | "skipped"
  detail: string
  tx_hash: string
  amount_base: string
  attempts: number
  started_at: number
  updated_at: number
  completed_at: number | null
}

export interface ProtocolPanelResponse {
  chain_id: number
  accounts: ProtocolAccount[]
  history: ProtocolAddressWindow[]
  steps: ProtocolMigrationStep[]
  master_key_configured: boolean
  sweep_enabled: boolean
  can_rotate: boolean
}

export interface ProtocolRotateResponse {
  account_key: string
  safe_address: string
  previous_eoa: string
  new_eoa: string
  operation_hash: string
  message: string
}

export interface ProtocolRotateFailure {
  account_key: string
  error: string
}

export interface ProtocolRotateAllResponse {
  rotated: ProtocolRotateResponse[]
  failed: ProtocolRotateFailure[]
}
