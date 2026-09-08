/**
 * Enrolment, steps 2-5. Step 1 (`setAllowlisted`) is an off-app multisig call.
 *
 * Every step is idempotent and the whole thing is resumable: it re-reads chain
 * state rather than trusting a stored cursor, and keygen treats 409 as success.
 * A user who closes the tab mid-flow resumes where they left off.
 *
 * Binding is REBINDABLE but not free. Only the account may authorize its own
 * binding, and it may re-point it later. Re-pointing changes the subject the
 * account's future sessions land under, so keys minted under the old subject
 * stay addressable only there — a deliberate identity move, not a routine
 * correction. Since one Privy EOA has several smart wallets (`smart_index`),
 * only one of them is the bound one at any time.
 */

import { keygen } from "@oleary-labs/signet-sdk/keygen"
import { encodeFunctionData, type Address, type Hex } from "viem"

import { safeAbi } from "./abi"
import {
  SIGNET_CURVE,
  SIGNET_GROUP_ID,
  SIGNET_KEY_SUFFIX,
  SIGNET_NODE_URLS,
  SIGNET_REGISTRY,
} from "./config"
import { buildBindAuthorization, encodeBindWithSignature, type SignTypedData } from "./bind"
import { readEnrolmentState, type ChainReader, type EnrolmentState } from "./state"
import type { SignetSession } from "./session"

/**
 * Relay a sponsored call. In the app this is `AppWallet`/`CommunityModule`; in
 * the CLI it is a direct module call.
 *
 * `addOwnerWithThreshold` genuinely requires `msg.sender == safe` — it is a Safe
 * self-call. `bindWithSignature` does NOT care who relays it; the account's
 * signature is the authority. Routing it through the Safe is simply convenient,
 * since that is the sponsored path we already have.
 */
export type ExecuteFromSafe = (to: Address, data: Hex) => Promise<string>

export type EnrolStep = "bind" | "keygen" | "add_owner"

export interface EnrolProgress {
  step: EnrolStep
  status: "skipped" | "done"
  /** Transaction or user-operation hash, when the step submitted one. */
  hash?: string
  detail?: string
}

export interface EnrolResult {
  signetAddress: Address
  keyId: string
  progress: EnrolProgress[]
  finalState: EnrolmentState
}

export async function enrol(params: {
  client: ChainReader
  session: SignetSession
  eoa: Address
  safe: Address
  execute: ExecuteFromSafe
  /** Signs the EIP-712 bind authorization as the account (the Privy EOA). */
  signTypedData: SignTypedData
  nodeUrls?: string[]
  keySuffix?: string
  onProgress?: (p: EnrolProgress) => void
}): Promise<EnrolResult> {
  const { client, session, eoa, safe, execute, signTypedData } = params
  const keySuffix = params.keySuffix ?? SIGNET_KEY_SUFFIX
  const progress: EnrolProgress[] = []
  const record = (p: EnrolProgress) => {
    progress.push(p)
    params.onProgress?.(p)
  }

  const before = await readEnrolmentState(client, eoa, safe)

  if (!before.walletDeployed) {
    throw new Error(`${safe} is not deployed; nothing can be enrolled against it`)
  }
  if (!before.allowed) {
    throw new Error(`${eoa} is not allowlisted on the gate`)
  }
  // --- Step 2: bind account -> Safe ----------------------------------------
  // The account signs; the Safe relays and pays. `boundElsewhere` is no longer
  // terminal — rebinding is allowed — but it does move the subject, so callers
  // are expected to have confirmed that with the user first.
  if (before.boundSafe && !before.boundElsewhere) {
    record({ step: "bind", status: "skipped", detail: `already bound to ${before.boundSafe}` })
  } else {
    const auth = await buildBindAuthorization(client, eoa, safe)
    const signature = await signTypedData(auth)
    const hash = await execute(SIGNET_REGISTRY, encodeBindWithSignature(auth, signature))
    record({
      step: "bind",
      status: "done",
      hash,
      detail: before.boundSafe ? `rebound from ${before.boundSafe}` : undefined,
    })
  }

  // --- Step 4: keygen (409 = already exists, which is what makes this resumable)
  const result = await keygen(
    { nodeUrls: params.nodeUrls ?? SIGNET_NODE_URLS, groupId: SIGNET_GROUP_ID },
    session.keypair,
    null,
    keySuffix,
    session.identity,
    SIGNET_CURVE,
  )
  const signetAddress = result.ethereumAddress as Address
  if (!signetAddress) {
    throw new Error("keygen returned no ethereum address")
  }
  record({
    step: "keygen",
    status: result.alreadyExisted ? "skipped" : "done",
    detail: signetAddress,
  })

  // --- Step 5: make the threshold key a Safe owner --------------------------
  const alreadyOwner = await client.readContract({
    address: safe, abi: safeAbi, functionName: "isOwner", args: [signetAddress],
  })
  if (alreadyOwner) {
    record({ step: "add_owner", status: "skipped", detail: `${signetAddress} already an owner` })
  } else {
    // Threshold stays 1: this ADDS a key that can sign, it does not take the
    // Privy key away. Both owners remain independently sufficient, which is
    // what keeps the fallback real.
    const hash = await execute(
      safe,
      encodeFunctionData({
        abi: safeAbi, functionName: "addOwnerWithThreshold", args: [signetAddress, 1n],
      }),
    )
    record({ step: "add_owner", status: "done", hash })
  }

  const finalState = await readEnrolmentState(client, eoa, safe, signetAddress)
  return { signetAddress, keyId: result.keyId, progress, finalState }
}
