import { Contact } from "./contact"
import { Affiliate } from "./affiliate"
import { AuthedLocation, Location } from "./location"

export interface UserResponse {
  id: string
  is_admin: boolean
  is_merchant: boolean
  is_organizer: boolean
  is_improver: boolean
  is_affiliate: boolean
  contact_email?: string
  contact_phone?: string
  contact_name?: string
  paypal_eth: string
  last_redemption: number
}

export interface LocationResponse {
  locations: Location[]
}

export interface AuthedLocationResponse {
  locations: AuthedLocation[]
}

export interface WalletResponse {
  id: number | null
  owner: string
  name: string
  is_eoa: boolean
  is_redeemer: boolean
  eoa_address: string
  smart_address?: string
  smart_index?: number
}

export interface GetUserResponse {
  user: UserResponse
  wallets: WalletResponse[]
  locations: AuthedLocation[]
  contacts: Contact[]
  affiliate?: Affiliate | null
}
