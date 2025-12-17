import { CommunityConfig } from "@citizenwallet/sdk"
import config, { chain } from "@/app.config"
import { Address, extractChain, ExtractChainParameters } from "viem"
import { polygon } from "viem/chains"

export const CHAIN_ID = config.community.primary_token.chain_id
export const CHAIN = chain
export const SFLUV_TOKEN = config.community.primary_token.address as Address
export const BYUSD_TOKEN = process.env.NEXT_PUBLIC_BYUSD_ADDRESS as Address
export const FAUCET_ADDRESS = process.env.NEXT_PUBLIC_FAUCET_ADDRESS as Address
export const PRIVY_ID = process.env.NEXT_PUBLIC_PRIVY_APP_ID as string
export const BACKEND = process.env.NEXT_PUBLIC_BACKEND_URL || "http://localhost:8080"
export const COMMUNITY_TOKEN_INDEX = Object.keys(config.tokens)[0]
export const COMMUNITY_ACCOUNT_INDEX = Object.keys(config.accounts)[0]
export const COMMUNITY = new CommunityConfig(config)
export const COMMUNITY_ACCOUNT = COMMUNITY.accounts[COMMUNITY_ACCOUNT_INDEX]
export const COMMUNITY_TOKEN = COMMUNITY.accounts[COMMUNITY_TOKEN_INDEX]
export const PAYMASTER = COMMUNITY_ACCOUNT.paymaster_address as Address
export const PAYMASTER_TYPE = COMMUNITY_ACCOUNT.paymaster_type
export const FACTORY = COMMUNITY_ACCOUNT.account_factory_address as Address
export const SFLUV_DECIMALS = (config.tokens as any)[COMMUNITY_TOKEN_INDEX].decimals as number
export const BYUSD_DECIMALS = Number(process.env.NEXT_PUBLIC_BYUSD_DECIMALS)
export const SYMBOL = (config.tokens as any)[COMMUNITY_TOKEN_INDEX].symbol as string
export const CW_APP_BASE_URL = process.env.NEXT_PUBLIC_CW_BASE_URL || "https://app.citizenwallet.xyz"
export const GOOGLE_MAPS_API_KEY = process.env.NEXT_PUBLIC_GOOGLE_MAPS_API_KEY as string
export const MAP_ID = process.env.NEXT_PUBLIC_MAP_ID as string
export const MAP_CENTER = { lat: 37.7749, lng: -122.4194 }
export const MAP_RADIUS = 10
export const LAT_DIF = 0.145
export const LNG_DIF = 0.1818
