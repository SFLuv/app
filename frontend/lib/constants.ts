import { CommunityConfig } from "@citizenwallet/sdk"
import config, { chain } from "@/app.config"
import { Address, extractChain, ExtractChainParameters } from "viem"
import { polygon } from "viem/chains"

export const CHAIN_ID = config.community.primary_token.chain_id
export const CHAIN = chain
export const TOKEN = config.community.primary_token.address as Address
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
