/**
 * Enrolment detection: three cheap `eth_call`s against Celo, no session needed.
 *
 * This is the whole of Phase 1 and it is safe to ship before the node fleet is
 * configured — the gate is deployed closed (`allowAll=false`, empty allowlist),
 * so `isAllowed` is false for everyone and every user lands in `not_eligible`.
 */

import { createPublicClient, http, zeroAddress, type Address } from "viem"
import { celo } from "viem/chains"

import { gateAbi, registryAbi, resolverAbi, safeAbi } from "./abi"
import {
  EXPECTED_RESOLVER_VERSION,
  SIGNET_CELO_RPC_URL,
  SIGNET_GATE,
  SIGNET_REGISTRY,
  SIGNET_RESOLVER,
} from "./config"

export type EnrolmentStatus =
  /** Gate says no. Show nothing at all — this is most users. */
  | "not_eligible"
  /** Allowlisted, nothing bound yet. Offer "Explain + Enable". */
  | "eligible"
  /** Bound and/or keyed but the Safe does not own the Signet key yet. Resumable. */
  | "partly_enrolled"
  /** Signet key is a Safe owner. Show the toggle. */
  | "enrolled"

export interface EnrolmentState {
  status: EnrolmentStatus
  /** The Privy EOA. This is what gets allowlisted and bound — never the Safe. */
  eoa: Address
  /** The smart wallet this state was computed for. */
  wallet: Address
  allowed: boolean
  /** Safe recorded in the registry for `eoa`, or null when unbound. */
  boundSafe: Address | null
  /**
   * True when `boundSafe` is set and differs from `wallet`.
   *
   * One Privy EOA has several smart wallets (`smart_index`) but only one
   * binding, so at most one wallet per login is Signet-signed at a time. This
   * is recoverable — the account may rebind — but not free: re-pointing moves
   * the subject, so keys minted under the old one stay addressable only there.
   * Treat it as a deliberate identity move, and make the user confirm it.
   */
  boundElsewhere: boolean
  /** Whether the wallet is deployed. Counterfactual wallets cannot be enrolled. */
  walletDeployed: boolean
  /** Signet key address, once keygen has run. */
  signetAddress: Address | null
  /** Whether `signetAddress` is an owner of the wallet. */
  signetIsOwner: boolean
}

/**
 * The Celo client every Signet read goes through.
 *
 * `lib/signet` constructs it rather than accepting one, and the UI calls this
 * too. The repo resolves four different viem copies (2.23.2, 2.31.0, two
 * 2.41.2s), so a client built against one is not assignable to a
 * `PublicClient` type imported from another — passing clients across that
 * boundary is a compile error waiting for whichever copy a caller happens to
 * import. Owning construction here removes the question.
 */
export function celoClient(rpcUrl: string = SIGNET_CELO_RPC_URL) {
  return createPublicClient({ chain: celo, transport: http(rpcUrl) })
}

/** The client type the rest of `lib/signet` takes. */
export type ChainReader = ReturnType<typeof celoClient>

/**
 * Read enrolment state for one (eoa, wallet) pair.
 *
 * `signetAddress` is not discoverable on-chain — it comes from keygen — so the
 * caller passes whatever it has stored. Without it we can still distinguish
 * eligible from bound, which is all the detection UI needs.
 */
export async function readEnrolmentState(
  client: ChainReader,
  eoa: Address,
  wallet: Address,
  signetAddress?: Address | null,
): Promise<EnrolmentState> {
  const [allowed, safeFor, code] = await Promise.all([
    client.readContract({
      address: SIGNET_GATE, abi: gateAbi, functionName: "isAllowed", args: [eoa],
    }),
    client.readContract({
      address: SIGNET_REGISTRY, abi: registryAbi, functionName: "safeFor", args: [eoa],
    }),
    client.getCode({ address: wallet }),
  ])

  const boundSafe = safeFor === zeroAddress ? null : (safeFor as Address)
  const walletDeployed = !!code && code !== "0x"
  const boundElsewhere =
    boundSafe !== null && boundSafe.toLowerCase() !== wallet.toLowerCase()

  let signetIsOwner = false
  if (signetAddress && walletDeployed) {
    signetIsOwner = await client.readContract({
      address: wallet, abi: safeAbi, functionName: "isOwner", args: [signetAddress],
    })
  }

  let status: EnrolmentStatus = "not_eligible"
  if (allowed) {
    if (signetIsOwner && boundSafe && !boundElsewhere) status = "enrolled"
    else if (boundSafe || signetAddress) status = "partly_enrolled"
    else status = "eligible"
  }

  return {
    status,
    eoa,
    wallet,
    allowed: allowed as boolean,
    boundSafe,
    boundElsewhere,
    walletDeployed,
    signetAddress: signetAddress ?? null,
    signetIsOwner,
  }
}

/**
 * Ask the resolver directly. This is the same call every Signet node makes, so
 * it is the authoritative preflight: if this returns `ok: false`, `/v1/auth`
 * will refuse no matter how the fleet is configured.
 */
export async function resolveSubject(
  client: ChainReader,
  eoa: Address,
): Promise<{ ok: boolean; subject: `0x${string}` }> {
  const [ok, subject] = await client.readContract({
    address: SIGNET_RESOLVER, abi: resolverAbi, functionName: "resolve", args: [eoa],
  })
  return { ok, subject }
}

/** Guards against pointing the app at a resolver the nodes will not accept. */
export async function checkResolverVersion(
  client: ChainReader,
): Promise<{ version: string; accepted: boolean }> {
  const version = await client.readContract({
    address: SIGNET_RESOLVER, abi: resolverAbi, functionName: "typeAndVersion",
  })
  return { version, accepted: version === EXPECTED_RESOLVER_VERSION }
}

/** The block pin the nodes re-read `resolve()` at. Never cache the result. */
export async function getBlockPin(
  client: ChainReader,
): Promise<{ number: number; hash: string }> {
  const block = await client.getBlock({ blockTag: "latest" })
  return { number: Number(block.number), hash: block.hash as string }
}
