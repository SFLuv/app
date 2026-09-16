// Mirrors backend/structs/merchant_payout.go.

export type KYBStatus =
  | "not_started"
  | "under_review"
  | "incomplete"
  | "awaiting_questionnaire"
  | "awaiting_ubo"
  | "approved"
  | "rejected"
  | "paused"
  | "offboarded"
  | (string & {})

export interface MerchantPayoutProfile {
  owner_id: string
  bridge_customer_id: string
  bridge_kyc_link_id: string
  kyb_status: KYBStatus
  tos_status: string
  kyb_link_url?: string
  kyb_requested_at?: string
  kyb_approved_at?: string
  last_synced_at?: string
  created_at: string
  updated_at: string
}

export interface MerchantBankAccount {
  id: number
  owner_id: string
  bridge_external_account_id: string
  bank_name: string
  last_4: string
  account_owner_name: string
  currency: string
  active: boolean
  created_at: string
}

export interface LocationLiquidationAddress {
  location_id: number
  owner_id: string
  bridge_liquidation_address_id: string
  address: string
  chain: string
  currency: string
  destination_payment_rail: string
  destination_currency: string
  bridge_external_account_id: string
  source: "bridge" | "admin" | (string & {})
  created_at: string
  updated_at: string
}

export type UnwrapStatus =
  | "submitted"
  | "funds_received"
  | "payment_submitted"
  | "payment_processed"
  | "failed"
  | (string & {})

export interface Unwrap {
  id: number
  owner_id: string
  location_id?: number
  wallet_id?: number
  wallet_address: string
  wallet_role: "payment" | "tipping" | (string & {})
  destination_address: string
  amount_wei: string
  tx_hash: string
  status: UnwrapStatus
  bridge_drain_id?: string
  bridge_state?: string
  bank_reference?: string
  last_synced_at?: string
  created_at: string
  updated_at: string
}

export interface MerchantPayoutStatusResponse {
  enabled: boolean
  production: boolean
  profile: MerchantPayoutProfile | null
  bank_accounts: MerchantBankAccount[]
  locations: LocationLiquidationAddress[]
  unwraps: Unwrap[]
}

export interface ProvisionResponse {
  provisioned: LocationLiquidationAddress[]
  skipped_location_ids: number[]
  message?: string
}

// The merchant-facing wording for each ledger status. "Sent to your bank" is
// the terminal happy state; anything failed asks them to contact support.
export function unwrapStatusLabel(status: UnwrapStatus): { label: string; tone: "pending" | "ok" | "bad" } {
  switch (status) {
    case "submitted":
      return { label: "Submitted", tone: "pending" }
    case "funds_received":
      return { label: "Processing", tone: "pending" }
    case "payment_submitted":
      return { label: "Sent to your bank", tone: "pending" }
    case "payment_processed":
      return { label: "Deposited", tone: "ok" }
    case "failed":
      return { label: "Needs attention", tone: "bad" }
    default:
      return { label: status, tone: "pending" }
  }
}

export function kybStatusLabel(status: KYBStatus | undefined): { label: string; done: boolean; failed: boolean } {
  switch (status) {
    case undefined:
    case "not_started":
      return { label: "Not started", done: false, failed: false }
    case "approved":
      return { label: "Verified", done: true, failed: false }
    case "rejected":
    case "offboarded":
      return { label: "Not approved", done: false, failed: true }
    case "paused":
      return { label: "Paused", done: false, failed: true }
    case "incomplete":
      return { label: "Incomplete — finish the form", done: false, failed: false }
    case "awaiting_ubo":
    case "awaiting_questionnaire":
      return { label: "More information needed", done: false, failed: false }
    case "under_review":
      return { label: "Under review", done: false, failed: false }
    default:
      return { label: String(status), done: false, failed: false }
  }
}

// Admin panel shapes (GET /admin/merchant-payouts).
export interface AdminPayoutLocation {
  location_id: number
  name: string
  liquidation_address: LocationLiquidationAddress | null
}

export interface AdminMerchantPayoutBusiness {
  profile: MerchantPayoutProfile
  bank_accounts: MerchantBankAccount[]
  locations: AdminPayoutLocation[]
}

export interface AdminMerchantPayoutsResponse {
  businesses: AdminMerchantPayoutBusiness[]
  unwraps: Unwrap[]
}

// Returned (409) when an attach-by-email matches several accounts.
export interface MerchantOwnerCandidate {
  owner_id: string
  contact_name: string
  contact_email: string
  location_names: string[]
}
