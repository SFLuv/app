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
