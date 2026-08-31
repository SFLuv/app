/**
 * Signet threshold-signer configuration.
 *
 * Deliberately framework-free: nothing in `lib/signet/` may import React, Next,
 * or any module marked "use client". The CLI in `scripts/signet/` runs these
 * same files under plain Node, and that shared execution is the whole point —
 * what the harness proves is what the UI ships.
 */

import type { Address } from "viem"

/**
 * The resolver's chain, pinned as a literal.
 *
 * This is NOT `chainConfig.id`. The app has already moved chains once
 * (Berachain -> Celo) and the resolver cannot follow: its address is half of
 * every Signet key id and can never change without orphaning keys. Reading this
 * from app config would make a future chain move fail silently at SIWE time
 * rather than loudly here.
 */
export const SIGNET_CHAIN_ID = 42220

/**
 * Deployed 2026-08-30 on Celo.
 *
 * Registry and resolver were REDEPLOYED after the first registry was found to
 * accept a write from the named Safe, letting anyone pin an unbound EOA to a
 * contract of their choosing. The gate was unaffected and is reused as-is, so
 * the staff allowlist already written to it survives.
 *
 * The resolver address is half of every Signet key id, so changing it again
 * orphans keys. Nothing had been bound when the flaw was found.
 */
export const SIGNET_RESOLVER: Address = "0x903409cB9248b1f0047c5F967a3db8E03Df3E11a"
export const SIGNET_REGISTRY: Address = "0xd35A40c49c6FAfD8a3B193146726A7B3a97e9BBa"
export const SIGNET_GATE: Address = "0x78B405B629e7c27F81d7dF3dCEcC097f58B47053"

/**
 * EIP-712 domain for `bindWithSignature`, verified on chain via
 * `eip712Domain()`. Pins the signature to this chain and this registry.
 */
export const BIND_DOMAIN = {
  name: "SFLuvSafeBindingRegistry",
  version: "1",
  chainId: SIGNET_CHAIN_ID,
  verifyingContract: SIGNET_REGISTRY,
} as const

/** How long a bind authorization stays valid once signed. */
export const BIND_DEADLINE_SECONDS = 30 * 60

/** Exact string the nodes accept-list. A mismatch disables the scheme fleet-wide. */
export const EXPECTED_RESOLVER_VERSION = "SignetAuthResolver 1.0.0"

/** The Signet group, on Ethereum mainnet. Not a Celo address. */
export const SIGNET_GROUP_ID =
  process.env.NEXT_PUBLIC_SIGNET_GROUP_ID ??
  "0x86fe28144034fdaf86d3c964296dd33e4b94ac59"

/**
 * Session lifetime. The node caps this at 24h; we ask for one.
 *
 * The session private key is a bearer credential for /v1/sign for as long as it
 * lives, so the TTL is the mitigation for persisting it at all. Shortening it
 * costs a silent re-auth; lengthening it widens the window on a stolen key.
 */
export const SESSION_TTL_SECONDS = 60 * 60

/**
 * Must equal every node's configured `siwe_domain` EXACTLY.
 *
 * `siwe_domain` is node-level, not per-group: one value per node across every
 * group it serves. That means localhost and preview deploys cannot authenticate
 * unless the fleet is configured for them, and the CLI must sign a message
 * carrying the production domain even when run from a laptop.
 */
export const SIGNET_SIWE_DOMAIN =
  process.env.NEXT_PUBLIC_SIGNET_SIWE_DOMAIN ?? "app.sfluv.org"

/** Fleet: 4 OLL + 2 SFLuv. Auth needs one; keygen initiates on one. */
export const SIGNET_NODE_URLS: string[] = (
  process.env.NEXT_PUBLIC_SIGNET_NODE_URLS ?? ""
)
  .split(",")
  .map((s) => s.trim())
  .filter(Boolean)

/** Celo RPC used for the gate/registry/Safe reads and the block pin. */
export const SIGNET_CELO_RPC_URL =
  process.env.NEXT_PUBLIC_SIGNET_CELO_RPC_URL ??
  process.env.NEXT_PUBLIC_CHAIN_RPC_URL ??
  "https://forno.celo.org"

/**
 * Key suffix. The subject half of the key id comes from the session identity
 * and is never ours to choose; the suffix is the only part a client picks.
 */
export const SIGNET_KEY_SUFFIX = "sfluv-wallet"

export const SIGNET_CURVE = "ecdsa_secp256k1"

/** True once a node fleet is configured. Detection works without it. */
export function hasSignetNodes(): boolean {
  return SIGNET_NODE_URLS.length > 0
}
