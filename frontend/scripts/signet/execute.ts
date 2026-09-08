/**
 * Sponsored calls from the Safe, for the CLI.
 *
 * This is the harness half of `ExecuteFromSafe`. It deliberately goes through
 * the SAME `BundlerService.call` the app uses in `AppWallet._execTx`, loading
 * the SAME `/config` payload the browser loads via `ChainConfigProvider`, so
 * what the CLI proves about sponsorship, the module and the Safe carries over
 * to the UI unchanged.
 *
 * Why every enrolment write has to come through here: `bind` is callable only
 * by `account` or `safe`, and a Privy EOA has nonce 0 and no gas — it has never
 * sent a transaction and cannot pay for one. There is no EOA path. The call
 * must originate from the Safe, which means the module, which means a sponsored
 * user operation.
 */

import { BundlerService } from "@citizenwallet/sdk"
import { Wallet } from "ethers"
import { hexToBytes, type Address, type Hex } from "viem"

import {
  resolveCommunityConfig,
  type CommunityConfigPayload,
  type ResolvedCommunityConfig,
} from "../../lib/community-config"
import type { ExecuteFromSafe } from "../../lib/signet/enrol"

const DEFAULT_BACKEND = "https://api.sfluv.org"

export function backendUrl(): string {
  const raw =
    process.env.SIGNET_BACKEND_URL ??
    process.env.NEXT_PUBLIC_BACKEND_URL ??
    DEFAULT_BACKEND
  return raw.replace(/\/+$/, "")
}

/** The same payload `ChainConfigProvider` fetches, resolved the same way. */
export async function loadCommunityConfig(): Promise<ResolvedCommunityConfig> {
  const url = `${backendUrl()}/config`
  const res = await fetch(url, { headers: { Accept: "application/json" } })
  if (!res.ok) throw new Error(`${url} returned ${res.status}`)
  return resolveCommunityConfig((await res.json()) as CommunityConfigPayload)
}

export interface SafeExecutor {
  execute: ExecuteFromSafe
  config: ResolvedCommunityConfig
  /** The EOA that signs the user operation — the Safe owner, not the Safe. */
  owner: Address
}

/**
 * Build an executor that sends `data` to `to` with `msg.sender == safe`.
 *
 * `smartAccountIndex` must match the index the Safe was derived at, because the
 * bundler folds it into initCode for an undeployed account. We refuse to send
 * for an undeployed Safe rather than let it derive a different address — the
 * same hazard `SignetSigner` guards against, and the reason enrolment can never
 * be the thing that deploys a wallet.
 */
export async function safeExecutor(params: {
  privateKey: string
  safe: Address
  index?: number
  config?: ResolvedCommunityConfig
  onSubmit?: (label: string) => void
}): Promise<SafeExecutor> {
  const config = params.config ?? (await loadCommunityConfig())
  const pk = params.privateKey.startsWith("0x") ? params.privateKey : `0x${params.privateKey}`
  // No provider attached: `BundlerService.call` uses only getAddress() and
  // signMessage(). The EOA never sends a transaction and needs no gas.
  const signer = new Wallet(pk)
  const owner = (await signer.getAddress()) as Address

  const bundler = new BundlerService(config.community)

  const execute: ExecuteFromSafe = async (to: Address, data: Hex) => {
    params.onSubmit?.(`${to} ${data.slice(0, 10)}`)
    return bundler.call(
      // ethers Signer; the bundler touches getAddress + signMessage only.
      signer as never,
      to,
      params.safe,
      hexToBytes(data),
      undefined,
      undefined,
      undefined,
      { smartAccountIndex: params.index ?? 0 },
    )
  }

  return { execute, config, owner }
}
