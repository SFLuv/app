import { CommunityConfig } from "@citizenwallet/sdk"
import config, { chain } from "@config"
import { Address, extractChain, ExtractChainParameters } from "viem"
import { polygon } from "viem/chains"

export const CHAIN_ID = config.community.primary_token.chain_id
export const CHAIN = chain
export const TOKEN = config.community.primary_token.address as Address
export const FACTORY = process.env.NEXT_PUBLIC_FACTORY_ADDRESS as Address
export const PRIVY_ID = process.env.NEXT_PUBLIC_PRIVY_APP_ID as string
export const BACKEND = process.env.NEXT_PUBLIC_BACKEND_URL || "http://localhost:8080"
export const PAYMASTER = config.accounts["137:0x5e987a6c4bb4239d498E78c34e986acf29c81E8e"].paymaster_address as Address
export const PAYMASTER_TYPE = config.accounts["137:0x5e987a6c4bb4239d498E78c34e986acf29c81E8e"].paymaster_type
export const COMMUNITY = new CommunityConfig(config)
