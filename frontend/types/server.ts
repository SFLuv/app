import { Contact } from "./contact";
import { Affiliate } from "./affiliate";
import { AuthedLocation, Location } from "./location";
import { Proposer } from "./proposer";
import { Improver } from "./improver";
import { IssuerRecord } from "./issuer";
import { Supervisor } from "./supervisor";

export interface UserResponse {
  id: string;
  is_admin: boolean;
  is_merchant: boolean;
  is_organizer: boolean;
  is_improver: boolean;
  is_proposer: boolean;
  is_voter: boolean;
  is_issuer: boolean;
  is_supervisor: boolean;
  is_affiliate: boolean;
  contact_email?: string;
  contact_phone?: string;
  contact_name?: string;
  primary_wallet_address: string;
  paypal_eth: string;
  last_redemption: number;
  accepted_privacy_policy: boolean;
  accepted_privacy_policy_at?: string | null;
  privacy_policy_version: string;
  mailing_list_opt_in: boolean;
  mailing_list_opt_in_at?: string | null;
  mailing_list_policy_version: string;
}

export interface UserPolicyStatusResponse {
  user_id: string;
  active: boolean;
  accepted_privacy_policy: boolean;
  accepted_privacy_policy_at?: string | null;
  privacy_policy_version: string;
  mailing_list_opt_in: boolean;
  mailing_list_opt_in_at?: string | null;
  mailing_list_policy_version: string;
}

export interface LocationResponse {
  locations: Location[];
}

export interface AuthedLocationResponse {
  locations: AuthedLocation[];
}

export interface WalletResponse {
  id: number | null;
  owner: string;
  name: string;
  is_eoa: boolean;
  is_hidden: boolean;
  is_redeemer: boolean;
  is_minter: boolean;
  eoa_address: string;
  smart_address?: string;
  smart_index?: number;
  last_unwrap_at?: string;
}

export interface GetUserResponse {
  user: UserResponse;
  wallets: WalletResponse[];
  locations: AuthedLocation[];
  contacts: Contact[];
  affiliate?: Affiliate | null;
  proposer?: Proposer | null;
  improver?: Improver | null;
  issuer?: IssuerRecord | null;
  supervisor?: Supervisor | null;
}

export type AccountDeletionStatus =
  | "active"
  | "scheduled_for_deletion"
  | "ready_for_manual_purge";

export interface AccountDeletionCounts {
  wallets: number;
  contacts: number;
  locations: number;
  location_hours: number;
  location_wallets: number;
  ponder_subscriptions: number;
  verified_emails: number;
  memos: number;
}

export interface AccountDeletionPreview {
  user_id: string;
  status: AccountDeletionStatus;
  delete_date?: string | null;
  requested_at?: string | null;
  can_cancel: boolean;
  primary_wallet_address: string;
  wallet_addresses: string[];
  counts: AccountDeletionCounts;
  purge_enabled: boolean;
}

export interface AccountDeletionStatusResponse {
  user_id: string;
  status: AccountDeletionStatus;
  delete_date?: string | null;
  requested_at?: string | null;
  canceled_at?: string | null;
  completed_at?: string | null;
  can_cancel: boolean;
  purge_enabled: boolean;
  purge_enabled_by?: string;
}

export type AppleRecoveryResolution =
  | "current_account_exists"
  | "recovery_suggested"
  | "no_match"
  | "ambiguous_match"
  | "no_apple_account";

export interface AppleRecoverySuggestedAccount {
  user_id: string;
  contact_name?: string;
  verified_email?: string;
  primary_wallet_address?: string;
}

export interface AppleRecoveryResponse {
  current_user_id: string;
  current_user_exists: boolean;
  apple_linked: boolean;
  apple_email?: string;
  is_private_relay: boolean;
  resolution: AppleRecoveryResolution;
  suggested_existing_account?: AppleRecoverySuggestedAccount;
}

export type VerifiedEmailStatus = "verified" | "pending" | "expired";

export interface VerifiedEmailResponse {
  id: string;
  user_id: string;
  email: string;
  status: VerifiedEmailStatus;
  verified_at?: string | null;
  verification_sent_at?: string | null;
  verification_token_expires_at?: string | null;
  created_at: string;
  updated_at: string;
}
