export interface Partner {
  id: string
  name: string
  link_url: string
  logo_url: string | null
  logo_width: number
  logo_height: number
  position: number
  active: boolean
  created_at: number
  updated_at: number
}

export interface PartnersResponse {
  partners: Partner[]
}
