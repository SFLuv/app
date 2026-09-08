/**
 * The signer that replaces Privy's inside `AppWallet._execTx`.
 *
 * The seam is far narrower than "an ethers Signer". `BundlerService.call`
 * touches the signer exactly twice (@citizenwallet/sdk dist/src/bundler/index.js):
 *
 *   const owner = await signer.getAddress()                      // :285
 *   const signature = ethers.getBytes(await signer.signMessage(userOpHash))  // :257
 *
 * No provider, no `sendTransaction`, no `populateTransaction`. Two methods.
 *
 * THE COUNTERFACTUAL HAZARD. `getAddress()` is not only informational: when the
 * account is not yet deployed, `generateUserOp` puts it into initCode as
 * `createAccount(signerAddress, index)` (:208-226). With the Signet key as
 * signer that derives a DIFFERENT account than `sender`, and the user operation
 * fails. So the swap is unsound for undeployed wallets by construction — not
 * merely because `resolve()` refuses them. This signer therefore refuses to
 * exist for an undeployed account, and must never be the key that deploys one.
 */

import type { Address } from "viem"

import type { ChainReader } from "./state"

import { signEvmDigest, SignetSignError } from "./sign"
import { isSessionExpired, type SignetSession } from "./session"

/** The whole of what the bundler needs. Structurally an ethers Signer subset. */
export interface UserOpSigner {
  getAddress(): Promise<string>
  signMessage(message: string | Uint8Array): Promise<string>
}

export interface SignetSignerOptions {
  /**
   * Re-establish a session. Called at most once per signature, when the node
   * answers 401 and a retry does not clear it.
   */
  reauthenticate?: () => Promise<SignetSession>
  keySuffix?: string
  onLatency?: (ms: number) => void
}

export class SignetSigner implements UserOpSigner {
  private constructor(
    private session: SignetSession,
    /** The threshold key's EVM address — a Safe owner holding no funds. */
    readonly signetAddress: Address,
    readonly account: Address,
    private readonly opts: SignetSignerOptions,
  ) {}

  /**
   * @throws when `account` has no code. See the counterfactual note above.
   */
  static async create(
    client: ChainReader,
    session: SignetSession,
    signetAddress: Address,
    account: Address,
    opts: SignetSignerOptions = {},
  ): Promise<SignetSigner> {
    const code = await client.getCode({ address: account })
    if (!code || code === "0x") {
      throw new Error(
        `refusing to build a Signet signer for undeployed account ${account}: ` +
          `the bundler would derive initCode from the signer address and target ` +
          `a different account. Deploy with the Privy key first.`,
      )
    }
    return new SignetSigner(session, signetAddress, account, opts)
  }

  async getAddress(): Promise<string> {
    return this.signetAddress
  }

  /**
   * Ethers semantics: the argument is hashed under EIP-191 before signing. The
   * bundler always passes a 32-byte user-op hash, and `eip191Digest` encodes
   * the `:\n32` length prefix, so anything else is rejected rather than signed
   * against a prefix that does not match.
   */
  async signMessage(message: string | Uint8Array): Promise<string> {
    if (typeof message === "string") {
      throw new Error(
        "SignetSigner.signMessage accepts a 32-byte digest only; " +
          "string messages are not part of the user-operation path",
      )
    }
    if (message.length !== 32) {
      throw new Error(
        `SignetSigner.signMessage expected a 32-byte hash, got ${message.length}`,
      )
    }

    if (isSessionExpired(this.session) && this.opts.reauthenticate) {
      this.session = await this.opts.reauthenticate()
    }

    const result = await this.signOnce(message)
    this.opts.onLatency?.(result.elapsedMs)

    if (result.recovered.toLowerCase() !== this.signetAddress.toLowerCase()) {
      throw new Error(
        `Signet signature recovers to ${result.recovered}, expected ` +
          `${this.signetAddress} — the module would reject this on chain`,
      )
    }
    return result.signature
  }

  /**
   * A 401 means one of three things and only the first is fixed by retrying:
   * the participant has not processed the auth broadcast yet, it was
   * unreachable when the broadcast went out (needs a fresh session — there is
   * no backfill), or it cannot reach Celo. Retry once, then re-auth once.
   */
  private async signOnce(hash: Uint8Array) {
    const opts = { keySuffix: this.opts.keySuffix }
    try {
      return await signEvmDigest(this.session, hash, opts)
    } catch (e) {
      if (!(e instanceof SignetSignError) || !e.maybePropagation) throw e

      try {
        return await signEvmDigest(this.session, hash, opts)
      } catch (retryErr) {
        if (
          !(retryErr instanceof SignetSignError) ||
          !retryErr.maybePropagation ||
          !this.opts.reauthenticate
        ) {
          throw retryErr
        }
        this.session = await this.opts.reauthenticate()
        return await signEvmDigest(this.session, hash, opts)
      }
    }
  }
}
