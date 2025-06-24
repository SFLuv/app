import { polygon } from "viem/chains";
import { Address, Chain } from "viem"

// const chainName = process.env.NEXT_PUBLIC_CHAIN_NAME as keyof typeof chains

const engineUrl = process.env.NEXT_PUBLIC_ENGINE_URL
const paymasterAddress = process.env.NEXT_PUBLIC_PAYMASTER_ADDRESS


// EXPORTS
export const chain: Chain = polygon
export const paymasterUrl = engineUrl + "/rpc/" + paymasterAddress
export const token = process.env.NEXT_PUBLIC_TOKEN_ADDRESS as Address