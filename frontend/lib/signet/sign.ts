/**
 * Threshold-sign a 32-byte hash so an EVM verifier recovers the Signet key.
 *
 * The mechanics now live in the SDK's `signEvmDigest` (0.4.0). This wrapper
 * only supplies SFLuv's node/group/suffix policy and measures latency.
 *
 * Do not reimplement the body. It has three failure modes that are invisible in
 * JavaScript and only surface as a reverted transaction:
 *
 *   1. The EIP-191 envelope. Citizen Wallet's Safe module verifies with
 *      `recoverSigner(toEthSignedMessageHash(userOpHash), sig)`
 *      (UserOpHandler.sol:25), so the bare hash must be wrapped. Skipping it
 *      yields a valid signature by a *different* recovered address.
 *   2. The response field. An ECDSA request returns `ecdsa_signature`;
 *      `ethereum_signature` is the FROST Schnorr field. Reading the wrong one
 *      gets `undefined` or a signature `ecrecover` cannot verify.
 *   3. The recovery id. The node emits `v` in {0,1}; the module rejects
 *      anything outside {27,28} before `ecrecover` is reached
 *      (UserOpHandler.sol:91). viem and ethers both accept 0/1, so a JS
 *      round-trip test passes on a signature that reverts on chain.
 */

import { signEvmDigest as sdkSignEvmDigest } from "@oleary-labs/signet-sdk/signature"
import { recoverAddress, type Address, type Hex } from "viem"

import { SIGNET_GROUP_ID, SIGNET_KEY_SUFFIX } from "./config"
import type { SignetSession } from "./session"

export interface SignEvmDigestResult {
  /** 65 bytes, `v` in {27,28}. Ready for a contract. */
  signature: Hex
  /** Address an EIP-191 verifier will recover. */
  recovered: Address
  /** Round-trip latency of the /v1/sign call, milliseconds. */
  elapsedMs: number
}

/**
 * Sign `hash32` such that a contract calling
 * `ecrecover(toEthSignedMessageHash(hash32), sig)` recovers the Signet key.
 *
 * Mirrors ethers' `signMessage(bytes32)` semantics, which is what
 * `BundlerService.signUserOp` calls.
 */
export async function signEvmDigest(
  session: SignetSession,
  hash32: Uint8Array,
  opts?: { keySuffix?: string; nodeUrl?: string },
): Promise<SignEvmDigestResult> {
  const started = Date.now()
  let result
  try {
    result = await sdkSignEvmDigest(
      { groupId: SIGNET_GROUP_ID, nodeUrl: opts?.nodeUrl ?? session.nodeUrl },
      {
        keypair: session.keypair,
        hash: hash32,
        // Opaque. Echoed verbatim — never rebuilt from what we think it is.
        identity: session.identity,
        keySuffix: opts?.keySuffix ?? SIGNET_KEY_SUFFIX,
      },
    )
  } catch (e) {
    // A 401 here has three causes and only one is fixed by retrying: the
    // participant has not processed the auth broadcast yet (retry), it was
    // unreachable when the broadcast went out (re-auth — there is no backfill),
    // or it cannot reach Celo (neither).
    throw new SignetSignError(
      e instanceof Error ? e.message : String(e),
      opts?.nodeUrl ?? session.nodeUrl,
    )
  }
  const elapsedMs = Date.now() - started

  // Recover against the digest the SDK actually signed, so a wrong preimage or
  // a bad `v` fails here rather than as an opaque on-chain revert.
  const recovered = await recoverAddress({
    hash: result.digest as Hex,
    signature: result.signature as Hex,
  })

  return { signature: result.signature as Hex, recovered, elapsedMs }
}

export class SignetSignError extends Error {
  constructor(readonly detail: string, readonly nodeUrl: string) {
    super(`${nodeUrl}: threshold signing failed — ${detail}`)
    this.name = "SignetSignError"
  }

  /** A 401 may just be broadcast propagation. Worth one retry before re-auth. */
  get maybePropagation(): boolean {
    return / 401\b/.test(this.detail)
  }
}
