import { Address } from "viem"

export const ADMIN_ADDRESS =
  (process.env.NEXT_PUBLIC_ADMIN_ADDRESS ||
    "") as Address
export const PRIVY_ID = process.env.NEXT_PUBLIC_PRIVY_APP_ID as string
export const PRIVY_CLIENT_ID =
  process.env.NEXT_PUBLIC_PRIVY_CLIENT_ID?.trim() || undefined
export const BACKEND =
  process.env.NEXT_PUBLIC_BACKEND_URL ||
  process.env.NEXT_PUBLIC_BACKEND_BASE_URL ||
  process.env.NEXT_PUBLIC_APP_BASE_URL ||
  "http://localhost:8080"
export const CW_APP_BASE_URL = process.env.NEXT_PUBLIC_CW_BASE_URL || "https://app.citizenwallet.xyz"
export const GOOGLE_MAPS_API_KEY = process.env.NEXT_PUBLIC_GOOGLE_MAPS_API_KEY as string
export const MAP_ID = process.env.NEXT_PUBLIC_MAP_ID as string
export const MAP_CENTER = { lat: 37.7749, lng: -122.4194 }
export const MAP_RADIUS = 10
export const LAT_DIF = 0.145
export const LNG_DIF = 0.1818
export const IDLE_TIMER_SECONDS = Number(process.env.NEXT_PUBLIC_IDLE_TIMER_TIMEOUT_SECONDS) || 600
export const IDLE_TIMER_PROMPT_SECONDS = Number(process.env.NEXT_PUBLIC_IDLE_TIMER_PROMPT_SECONDS) || 60
