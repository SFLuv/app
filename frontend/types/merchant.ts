export type MerchantStatus = "pending" | "approved" | "rejected" | "revoked"

export interface Merchant {
  name: string
  id?: string
  description: string
  type: string
  status?: boolean
  address: {
    street: string
    city: string
    state: string
    zip: string
    coordinates: {
      lat: number
      lng: number
    }
  }
  contactInfo: {
    phone: string
    email?: string
    website?: string
  }
  imageUrl?: string
  acceptsSFLuv?: boolean
  rating: number // 1-5 stars
  opening_hours?: []
  mapsPage: string
}


export type MerchantType =
  | "restaurant"
  | "cafe"
  | "retail"
  | "grocery"
  | "service"
  | "entertainment"
  | "health"
  | "beauty"
  | "other"

export const merchantTypeLabels: Record<MerchantType, string> = {
  restaurant: "Restaurant",
  cafe: "Café",
  retail: "Retail Store",
  grocery: "Grocery Store",
  service: "Service Provider",
  entertainment: "Entertainment",
  health: "Health & Wellness",
  beauty: "Beauty & Spa",
  other: "Other",
}

export const merchantStatusLabels: Record<MerchantStatus, string> = {
  pending: "Pending Review",
  approved: "Approved",
  rejected: "Rejected",
  revoked: "Approval Revoked",
}

export interface UserLocation {
  lat: number
  lng: number
  address?: string
}