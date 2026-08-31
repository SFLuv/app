/**
 * Signet sessions over the `onchain_resolver` scheme.
 *
 * Framework-free by design: signing and the block pin arrive as callbacks, so
 * the browser can hand in Privy and the CLI can hand in a raw key, and both
 * exercise identical protocol code.
 */

import { generateSessionKeypair } from "@oleary-labs/signet-sdk/session"
import {
  authenticateWithResolver,
  preflightResolverNodes,
  ResolverAuthError,
  type BlockPin,
  type NodeAuthOutcome,
} from "@oleary-labs/signet-sdk/resolver-session"
import type { SessionKeypair } from "@oleary-labs/signet-sdk/types"
import type { Address } from "viem"

import {
  SESSION_TTL_SECONDS,
  SIGNET_CHAIN_ID,
  SIGNET_GROUP_ID,
  SIGNET_NODE_URLS,
  SIGNET_SIWE_DOMAIN,
} from "./config"

export interface SignetSession {
  keypair: SessionKeypair
  /** Opaque subject from `resolve()`, as the node rendered it. Echo verbatim. */
  identity: string
  /** Unix seconds. */
  expiresAt: number
  nodeUrl: string
  eoa: Address
}

export interface OpenSessionParams {
  eoa: Address
  /** Personal-sign the SIWE message. Called exactly once. */
  signMessage: (message: string) => Promise<string>
  getBlockPin: () => Promise<BlockPin>
  /** Defaults to the first configured node. One is enough. */
  nodeUrl?: string
  statement?: string
  /** Wait for these nodes to hold the session before returning. */
  barrierNodeUrls?: string[]
  ttlSeconds?: number
}

export async function openSession(params: OpenSessionParams): Promise<SignetSession> {
  const nodeUrl = params.nodeUrl ?? SIGNET_NODE_URLS[0]
  if (!nodeUrl) {
    throw new Error(
      "no Signet node configured — set NEXT_PUBLIC_SIGNET_NODE_URLS",
    )
  }

  const keypair = await generateSessionKeypair()
  const issuedAt = new Date()
  const ttl = params.ttlSeconds ?? SESSION_TTL_SECONDS

  const result = await authenticateWithResolver(
    { groupId: SIGNET_GROUP_ID, nodeUrl, barrierNodeUrls: params.barrierNodeUrls },
    {
      sessionPubHex: keypair.publicKeyHex,
      siwe: {
        domain: SIGNET_SIWE_DOMAIN,
        address: params.eoa,
        // The RESOLVER's chain, not the app's. They coincide today; that is a
        // coincidence, not an invariant.
        chainId: SIGNET_CHAIN_ID,
        statement:
          params.statement ??
          "Authorize SFLUV to sign your transactions with your Signet key.",
        issuedAt,
        expirationTime: new Date(issuedAt.getTime() + ttl * 1000),
      },
      signMessage: params.signMessage,
      getBlockPin: params.getBlockPin,
    },
  )

  return {
    keypair,
    identity: result.identity,
    expiresAt: result.expiresAt,
    nodeUrl: result.nodeUrl,
    eoa: params.eoa,
  }
}

/** True when the session is gone or close enough to it to re-auth first. */
export function isSessionExpired(session: SignetSession, skewSeconds = 60): boolean {
  return Date.now() / 1000 >= session.expiresAt - skewSeconds
}

/**
 * Ask every node individually whether it can serve this scheme.
 *
 * The only readiness signal available client-side: `/v1/info` advertises
 * neither `siwe_domain` nor which chains a node has RPC for, and the initiating
 * node returns 200 on its own verification regardless of whether the others can
 * do the resolver read. This is how a half-configured fleet becomes visible
 * before it matters.
 */
export async function preflightNodes(
  params: Omit<OpenSessionParams, "nodeUrl" | "barrierNodeUrls"> & {
    nodeUrls?: string[]
  },
): Promise<NodeAuthOutcome[]> {
  const keypair = await generateSessionKeypair()
  const issuedAt = new Date()
  const ttl = params.ttlSeconds ?? SESSION_TTL_SECONDS

  return preflightResolverNodes(
    { groupId: SIGNET_GROUP_ID, nodeUrls: params.nodeUrls ?? SIGNET_NODE_URLS },
    {
      sessionPubHex: keypair.publicKeyHex,
      siwe: {
        domain: SIGNET_SIWE_DOMAIN,
        address: params.eoa,
        chainId: SIGNET_CHAIN_ID,
        statement: params.statement ?? "SFLUV Signet node preflight.",
        issuedAt,
        expirationTime: new Date(issuedAt.getTime() + ttl * 1000),
      },
      signMessage: params.signMessage,
      getBlockPin: params.getBlockPin,
    },
  )
}

export { ResolverAuthError }
export type { BlockPin, NodeAuthOutcome }
