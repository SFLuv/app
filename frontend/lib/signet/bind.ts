/**
 * EIP-712 bind authorizations.
 *
 * The registry accepts a binding only from the account it names — either as
 * `msg.sender`, or as an EIP-712 signature the account produced. Privy EOAs
 * hold no CELO and have never sent a transaction, so the signature path is the
 * only one available to us: the user signs, and any relayer submits.
 *
 * This replaced a registry that also accepted `msg.sender == safe`. That was
 * exploitable — the caller chose which contract played the part of the Safe, so
 * `isOwner` was answered by the attacker, letting anyone pin an unbound EOA to
 * an address of their choosing. Hence the rule this module exists to honour:
 * only the account can speak for itself.
 */

import { encodeFunctionData, type Address, type Hex } from "viem"

import { registryAbi } from "./abi"
import { BIND_DEADLINE_SECONDS, BIND_DOMAIN, SIGNET_REGISTRY } from "./config"
import type { ChainReader } from "./state"

/** The EIP-712 types, matching BIND_TYPEHASH on chain. */
export const BIND_TYPES = {
  Bind: [
    { name: "account", type: "address" },
    { name: "safe", type: "address" },
    { name: "nonce", type: "uint256" },
    { name: "deadline", type: "uint256" },
  ],
} as const

export interface BindAuthorization {
  domain: typeof BIND_DOMAIN
  types: typeof BIND_TYPES
  primaryType: "Bind"
  message: {
    account: Address
    safe: Address
    nonce: bigint
    deadline: bigint
  }
}

/** Signs typed data as the account. In the browser this is Privy's EOA. */
export type SignTypedData = (auth: BindAuthorization) => Promise<Hex>

/**
 * Build the typed-data payload for a binding.
 *
 * The nonce is read fresh from the registry: it increments on every successful
 * signed bind, so a stale one produces a signature the contract rejects as
 * invalid rather than as expired.
 */
export async function buildBindAuthorization(
  client: ChainReader,
  account: Address,
  safe: Address,
  deadlineSeconds: number = BIND_DEADLINE_SECONDS,
): Promise<BindAuthorization> {
  const nonce = await client.readContract({
    address: SIGNET_REGISTRY,
    abi: registryAbi,
    functionName: "nonces",
    args: [account],
  })

  return {
    domain: BIND_DOMAIN,
    types: BIND_TYPES,
    primaryType: "Bind",
    message: {
      account,
      safe,
      nonce,
      deadline: BigInt(Math.floor(Date.now() / 1000) + deadlineSeconds),
    },
  }
}

/** Calldata for the relayed write. Anyone may submit this. */
export function encodeBindWithSignature(auth: BindAuthorization, signature: Hex): Hex {
  return encodeFunctionData({
    abi: registryAbi,
    functionName: "bindWithSignature",
    args: [auth.message.account, auth.message.safe, auth.message.deadline, signature],
  })
}
