"use client"

import { CommunityConfig } from "@citizenwallet/sdk"
import type { Chain, Address } from "viem"
import { berachain, celo } from "viem/chains"

export type ConfigAddressRef = {
  address: string
  chain_id: number
}

export type CommunityConfigPayload = {
  community: {
    name: string
    alias: string
    custom_domain?: string
    logo?: string
    profile: ConfigAddressRef
    primary_token: ConfigAddressRef
    primary_account_factory: ConfigAddressRef
    [key: string]: unknown
  }
  tokens: Record<string, {
    standard?: string
    name?: string
    address: string
    symbol?: string
    decimals?: number
    chain_id: number
    [key: string]: unknown
  }>
  accounts: Record<string, {
    chain_id: number
    entrypoint_address: string
    paymaster_address: string
    account_factory_address: string
    paymaster_type: string
    [key: string]: unknown
  }>
  chains: Record<string, {
    id: number
    node: {
      url: string
      ws_url?: string
      [key: string]: unknown
    }
    [key: string]: unknown
  }>
  scan?: {
    url?: string
    name?: string
    [key: string]: unknown
  }
  contracts?: Record<string, unknown>
  extras?: {
    honey_token_address?: unknown
    honeyTokenAddress?: unknown
    honey_address?: unknown
    honeyAddress?: unknown
    honey_decimals?: unknown
    honeyDecimals?: unknown
    byusd_token_address?: unknown
    byusdTokenAddress?: unknown
    byusd_address?: unknown
    byusdAddress?: unknown
    byusd_decimals?: unknown
    byusdDecimals?: unknown
    zapper_address?: unknown
    zapperAddress?: unknown
    zapper_contract_address?: unknown
    zapperContractAddress?: unknown
    faucet_address?: unknown
    faucetAddress?: unknown
    backing_assets?: unknown
    backingAssets?: unknown
    [key: string]: unknown
  }
  version?: number
  [key: string]: unknown
}

export type ResolvedCommunityConfigExtras = {
  honeyTokenAddress?: Address
  honeyDecimals?: number
  byusdTokenAddress?: Address
  byusdDecimals?: number
  zapperAddress?: Address
  faucetAddress?: Address
  backingAssets: Address[]
}

export type ResolvedCommunityConfig = {
  raw: CommunityConfigPayload
  community: CommunityConfig
  chain: Chain
  chainId: number
  rpcUrl: string
  engineWsUrl?: string
  tokenAddress: Address
  tokenDecimals: number
  tokenSymbol: string
  factoryAddress: Address
  entrypointAddress: Address
  paymasterAddress: Address
  paymasterType: string
  alias: string
  appOrigin: string
  extras: ResolvedCommunityConfigExtras
  honeyTokenAddress?: Address
  honeyDecimals?: number
  byusdTokenAddress?: Address
  byusdDecimals?: number
  zapperContractAddress?: Address
  faucetAddress?: Address
}

const knownChains: Record<number, Chain> = {
  [berachain.id]: berachain,
  [celo.id]: celo,
}

export function resolveCommunityConfig(payload: CommunityConfigPayload): ResolvedCommunityConfig {
  const primaryTokenRef = payload.community?.primary_token
  const primaryFactoryRef = payload.community?.primary_account_factory
  if (!primaryTokenRef?.address || !primaryTokenRef.chain_id) {
    throw new Error("Config is missing community.primary_token")
  }
  if (!primaryFactoryRef?.address || !primaryFactoryRef.chain_id) {
    throw new Error("Config is missing community.primary_account_factory")
  }

  const token = findToken(payload, primaryTokenRef)
  const account = findAccount(payload, primaryFactoryRef)
  const chainConfig = payload.chains?.[String(primaryTokenRef.chain_id)]
  const nodeUrl = chainConfig?.node?.url
  if (!nodeUrl) {
    throw new Error(`Config is missing chains.${primaryTokenRef.chain_id}.node.url`)
  }
  if (typeof token.decimals !== "number" || !Number.isFinite(token.decimals)) {
    throw new Error(`Config token ${primaryTokenRef.chain_id}:${primaryTokenRef.address} is missing decimals`)
  }

  const community = new CommunityConfig(payload as any)
  // The Citizen Wallet engine serves JSON-RPC (and AA methods) at
  // `${node.url}/v1/rpc/${paymaster}`, not at the bare node.url. Posting to the
  // root returns 401, so use the SDK's canonical RPC URL for all transports.
  const rpcUrl = community.primaryRPCUrl

  const baseChain = knownChains[primaryTokenRef.chain_id] ?? {
    id: primaryTokenRef.chain_id,
    name: `Chain ${primaryTokenRef.chain_id}`,
    nativeCurrency: { name: "Native Token", symbol: "ETH", decimals: 18 },
    rpcUrls: { default: { http: [rpcUrl] } },
  }
  const explorer = payload.scan?.url
    ? { default: { name: payload.scan.name || "Explorer", url: payload.scan.url } }
    : baseChain.blockExplorers
  const chain = {
    ...baseChain,
    id: primaryTokenRef.chain_id,
    rpcUrls: {
      default: { http: [rpcUrl] },
      public: { http: [rpcUrl] },
    },
    blockExplorers: explorer,
  } as Chain
  const byusd = findTokenBySymbol(payload, "BYUSD")
  const honey = findTokenBySymbol(payload, "HONEY")
  const extras = payload.extras
  const byusdTokenAddress = (byusd?.address as Address | undefined) ??
    findExtraAddress(extras, "byusd_token_address", "byusdTokenAddress", "byusd_address", "byusdAddress")
  const byusdDecimals = byusd?.decimals ?? findExtraNumber(extras, "byusd_decimals", "byusdDecimals")
  const honeyTokenAddress = (honey?.address as Address | undefined) ??
    findExtraAddress(extras, "honey_token_address", "honeyTokenAddress", "honey_address", "honeyAddress")
  const honeyDecimals = honey?.decimals ?? findExtraNumber(extras, "honey_decimals", "honeyDecimals")
  const zapperAddress = findConfiguredContract(payload, "zapper") ??
    findExtraAddress(extras, "zapper_address", "zapperAddress", "zapper_contract_address", "zapperContractAddress")
  const faucetAddress = findConfiguredContract(payload, "faucet") ?? findExtraAddress(extras, "faucet_address", "faucetAddress")
  const backingAssets = findExtraAddressList(extras, "backing_assets", "backingAssets")

  return {
    raw: payload,
    community,
    chain,
    chainId: primaryTokenRef.chain_id,
    rpcUrl,
    engineWsUrl: chainConfig?.node?.ws_url,
    tokenAddress: token.address as Address,
    tokenDecimals: token.decimals,
    tokenSymbol: token.symbol || token.name || "Token",
    factoryAddress: account.account_factory_address as Address,
    entrypointAddress: account.entrypoint_address as Address,
    paymasterAddress: account.paymaster_address as Address,
    paymasterType: account.paymaster_type,
    alias: payload.community.alias,
    appOrigin: firstPluginURL(payload) || "https://app.sfluv.org",
    extras: {
      byusdTokenAddress,
      byusdDecimals,
      honeyTokenAddress,
      honeyDecimals,
      zapperAddress,
      faucetAddress,
      backingAssets,
    },
    byusdTokenAddress,
    byusdDecimals,
    honeyTokenAddress,
    honeyDecimals,
    zapperContractAddress: zapperAddress,
    faucetAddress,
  }
}

function findToken(payload: CommunityConfigPayload, ref: ConfigAddressRef) {
  const exact = payload.tokens?.[`${ref.chain_id}:${ref.address.toLowerCase()}`]
  if (exact) return exact
  const token = Object.values(payload.tokens || {}).find(
    (candidate) => candidate.chain_id === ref.chain_id && sameAddress(candidate.address, ref.address),
  )
  if (!token) throw new Error(`Config token ${ref.chain_id}:${ref.address} was not found`)
  return token
}

function findTokenBySymbol(payload: CommunityConfigPayload, symbol: string) {
  return Object.values(payload.tokens || {}).find(
    (candidate) => candidate.symbol?.toLowerCase() === symbol.toLowerCase(),
  )
}

function findAccount(payload: CommunityConfigPayload, ref: ConfigAddressRef) {
  const exact = payload.accounts?.[`${ref.chain_id}:${ref.address.toLowerCase()}`]
  if (exact) return exact
  const account = Object.values(payload.accounts || {}).find(
    (candidate) => candidate.chain_id === ref.chain_id && sameAddress(candidate.account_factory_address, ref.address),
  )
  if (!account) throw new Error(`Config account ${ref.chain_id}:${ref.address} was not found`)
  return account
}

function firstPluginURL(payload: CommunityConfigPayload) {
  const plugins = Array.isArray(payload.plugins) ? payload.plugins : []
  const plugin = plugins.find((entry) => {
    if (!entry || typeof entry !== "object") return false
    const url = (entry as { url?: unknown }).url
    return typeof url === "string" && url.startsWith("http")
  }) as { url?: string } | undefined
  return plugin?.url
}

function findConfiguredContract(payload: CommunityConfigPayload, key: string): Address | undefined {
  const candidates = [
    readAddressish(payload.contracts?.[key]),
    readAddressish(payload.contracts?.[`${key}_address`]),
    readAddressish(payload[key]),
    readAddressish(payload[`${key}_address`]),
  ]
  return candidates.find(Boolean) as Address | undefined
}

function findExtraAddress(
  extras: CommunityConfigPayload["extras"] | undefined,
  ...keys: string[]
): Address | undefined {
  for (const key of keys) {
    const address = readAddressish(extras?.[key])
    if (address) return address
  }
  return undefined
}

function findExtraNumber(
  extras: CommunityConfigPayload["extras"] | undefined,
  ...keys: string[]
): number | undefined {
  for (const key of keys) {
    const value = extras?.[key]
    if (typeof value === "number" && Number.isFinite(value)) return value
    if (typeof value === "string" && value.trim() !== "") {
      const parsed = Number(value)
      if (Number.isFinite(parsed)) return parsed
    }
  }
  return undefined
}

function findExtraAddressList(
  extras: CommunityConfigPayload["extras"] | undefined,
  ...keys: string[]
): Address[] {
  for (const key of keys) {
    const value = extras?.[key]
    if (Array.isArray(value)) {
      return value.map(readAddressish).filter(Boolean) as Address[]
    }
    if (typeof value === "string") {
      return value.split(/[,\s;]+/).map(readAddressish).filter(Boolean) as Address[]
    }
  }
  return []
}

function readAddressish(value: unknown): Address | undefined {
  if (typeof value === "string") {
    return value.trim() ? value.trim() as Address : undefined
  }
  if (value && typeof value === "object") {
    const address = (value as { address?: unknown }).address
    return typeof address === "string" && address.trim() ? address.trim() as Address : undefined
  }
  return undefined
}

function sameAddress(left: string, right: string) {
  return left.trim().toLowerCase() === right.trim().toLowerCase()
}
