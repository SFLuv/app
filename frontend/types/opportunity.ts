export interface Opportunity {
  id: string
  title: string
  description: string
  date: string
  organizer: string
  location: {
    address: string
    city: string
    state: string
    zip: string
    coordinates: {
      lat: number
      lng: number
    }
  }
  rewardAmount: number
  volunteersNeeded: number
  volunteersSignedUp: number
  imageUrl?: string
}

export interface UserLocation {
  lat: number
  lng: number
  address?: string
}

export type SortOption = "reward" | "date" | "proximity" | "organizer"
export type SortDirection = "asc" | "desc"
