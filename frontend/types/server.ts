
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
  id: number
  google_id: string
  owner_id: string
  name: string
  description: string
  type: string
  approval: boolean
  street: string
  city: string
  state: string
  zip: string
  lat: number
  lng: number
  phone: string
  email: string
  website: string
  image_url: string
  rating: number
  maps_page: string
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
  locations: LocationResponse[]
}