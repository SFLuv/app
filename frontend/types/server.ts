import { Location } from "./location"

export interface UserResponse {
  id: string
  is_admin: boolean
  is_merchant: boolean
  is_organizer: boolean
  is_improver: boolean
  contact_email?: string
  contact_phone?: string
  contact_name?: string
}

export interface LocationResponse {
  locations: Location[]
}

export interface WalletResponse {
  id: number | null
  owner: string
  name: string
  is_eoa: boolean
  eoa_address: string
  smart_address?: string
  smart_index?: number
}

export interface GetUserResponse {
  user: UserResponse
  wallets: WalletResponse[]
  locations: Location[]
}