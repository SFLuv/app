#!/usr/bin/env tsx
/**
 * Signet plumbing harness.
 *
 *   npm run signet -- <command> [args]        (or: pnpm signet <command>)
 *   ./node_modules/.bin/tsx scripts/signet/cli.ts <command>
 *
 * Drives the SAME `lib/signet/*` modules the UI will import — that is the whole
 * point. A parallel implementation would prove nothing about what ships.
 *
 * Which commands work today depends on the handoff steps outside this repo:
 *
 *   chain half   (status, resolve, version, bind, add-owner) — step A, DONE
 *   signet half  (preflight, auth, keygen, sign-digest, enrol) — needs B + D
 *
 * Until step D binds the resolver to the group, `/v1/auth` answers
 * "no auth resolver configured" for everyone. `preflight` is what tells you
 * which of B or D is missing, per node, without guessing.
 *
 * Env (frontend/.env.local):
 *   SIGNET_PRIVATE_KEY               EOA to act as — the Privy-equivalent key
 *   NEXT_PUBLIC_SIGNET_NODE_URLS     comma-separated node base URLs
 *   NEXT_PUBLIC_SIGNET_SIWE_DOMAIN   must equal the nodes' siwe_domain exactly
 *   NEXT_PUBLIC_SIGNET_CELO_RPC_URL  Celo endpoint (default forno)
 */

import { privateKeyToAccount } from "viem/accounts"
import { celo } from "viem/chains"
import {
  createWalletClient, encodeFunctionData, formatEther, getAddress, http,
  keccak256, toBytes, type Address, type Hex,
} from "viem"

import {
  SIGNET_CHAIN_ID, SIGNET_GATE, SIGNET_GROUP_ID, SIGNET_KEY_SUFFIX,
  SIGNET_NODE_URLS, SIGNET_REGISTRY, SIGNET_RESOLVER, SIGNET_SIWE_DOMAIN,
  SIGNET_CELO_RPC_URL, hasSignetNodes,
} from "../../lib/signet/config"
import {
  celoClient, checkResolverVersion, getBlockPin, readEnrolmentState, resolveSubject,
} from "../../lib/signet/state"
import { openSession, preflightNodes, type SignetSession } from "../../lib/signet/session"
import { signEvmDigest } from "../../lib/signet/sign"
import { enrol } from "../../lib/signet/enrol"
import { buildBindAuthorization, encodeBindWithSignature, type BindAuthorization } from "../../lib/signet/bind"
import { gateAbi, registryAbi, safeAbi } from "../../lib/signet/abi"
import { backendUrl, loadCommunityConfig, safeExecutor } from "./execute"

const client = celoClient()

function eoaAccount() {
  const pk = process.env.SIGNET_PRIVATE_KEY
  if (!pk) throw new Error("SIGNET_PRIVATE_KEY is not set")
  return privateKeyToAccount((pk.startsWith("0x") ? pk : `0x${pk}`) as Hex)
}

/**
 * The EOA to act as. Falls back to the Safe's sole owner when no key is held,
 * so a dry run can be produced by anyone — the key is needed only to broadcast.
 */
async function eoaFor(safe: Address): Promise<Address> {
  if (process.env.SIGNET_PRIVATE_KEY) return eoaAccount().address
  const owners = await client.readContract({
    address: safe, abi: safeAbi, functionName: "getOwners",
  })
  if (owners.length !== 1) {
    throw new Error(
      `${safe} has ${owners.length} owners; set SIGNET_PRIVATE_KEY to choose one`,
    )
  }
  return owners[0]
}

/** Signs the EIP-712 bind authorization as the account. */
async function signBind(auth: BindAuthorization) {
  const account = eoaAccount()
  return account.signTypedData({
    domain: auth.domain,
    types: auth.types,
    primaryType: auth.primaryType,
    message: auth.message,
  })
}

/** The session-opening callbacks, in their Node flavour. */
async function open(nodeUrl?: string): Promise<SignetSession> {
  const account = eoaAccount()
  return openSession({
    eoa: account.address,
    nodeUrl,
    signMessage: (message) => account.signMessage({ message }),
    getBlockPin: () => getBlockPin(client),
  })
}

const commands: Record<string, (args: string[]) => Promise<void>> = {
  /** Config + live contract state. Answers "is anything even wired up". */
  async config() {
    const { version, accepted } = await checkResolverVersion(client)
    const head = await client.getBlockNumber()
    console.log(`chain            ${SIGNET_CHAIN_ID}  head ${head}`)
    console.log(`rpc              ${SIGNET_CELO_RPC_URL}`)
    console.log(`resolver         ${SIGNET_RESOLVER}`)
    console.log(`  typeAndVersion "${version}" ${accepted ? "(accepted)" : "MISMATCH"}`)
    console.log(`registry         ${SIGNET_REGISTRY}`)
    console.log(`gate             ${SIGNET_GATE}`)
    console.log(`group            ${SIGNET_GROUP_ID}`)
    console.log(`siwe domain      ${SIGNET_SIWE_DOMAIN}`)
    console.log(`nodes            ${hasSignetNodes() ? SIGNET_NODE_URLS.join(", ") : "(none configured)"}`)
  },

  /** The four reads behind the detection UI. Chain half — works today. */
  async status([eoa, wallet, signet]) {
    const owner = (eoa ?? eoaAccount().address) as Address
    if (!wallet) throw new Error("usage: status <eoa> <wallet> [signetAddress]")
    const s = await readEnrolmentState(
      client, owner, wallet as Address, (signet as Address) ?? null,
    )
    console.log(JSON.stringify(s, null, 2))
  },

  /**
   * Gate admission — handoff step 1, and the only thing standing between a
   * deployed-but-closed gate and a testable trial.
   *
   * `setAllowlisted` is owner-only, and the owner is
   * `0x762F96819a7705448843E96D63D638Ec2f39403B`. On Celo that address has no
   * code and no balance, so this prints the calldata by default and broadcasts
   * only with `--send` plus `SIGNET_GATE_OWNER_KEY`. Whichever way it is held,
   * the calldata is the same.
   *
   * Allowlist the OWNER EOAs (the Privy addresses), never the Safes: `resolve`
   * gates on `account`, and the Safe is what it returns.
   *
   *   allowlist check <addr...>
   *   allowlist add   <addr...> [--send]
   *   allowlist remove <addr...> [--send]
   */
  async allowlist(args) {
    const send = args.includes("--send")
    const [action, ...rest] = args.filter((a) => a !== "--send")
    const addrs = rest.map((a) => getAddress(a))

    if (!action || !["check", "add", "remove"].includes(action) || !addrs.length) {
      throw new Error("usage: allowlist <check|add|remove> <addr...> [--send]")
    }

    if (action === "check") {
      for (const a of addrs) {
        const allowed = await client.readContract({
          address: SIGNET_GATE, abi: gateAbi, functionName: "isAllowed", args: [a],
        })
        console.log(`${allowed ? "ALLOWED " : "denied  "} ${a}`)
      }
      return
    }

    const allowed = action === "add"
    const data = encodeFunctionData({
      abi: gateAbi, functionName: "setAllowlisted", args: [addrs, allowed],
    })

    // Read the owner rather than trusting a constant — it is transferable.
    const owner = await client.readContract({
      address: SIGNET_GATE, abi: gateAbi, functionName: "owner",
    })

    console.log(`to       ${SIGNET_GATE}`)
    console.log(`from     ${owner}  (gate owner — the call reverts from anyone else)`)
    console.log(`function setAllowlisted([${addrs.join(", ")}], ${allowed})`)
    console.log(`data     ${data}`)

    if (!send) {
      console.log("\n(dry run — pass --send with SIGNET_GATE_OWNER_KEY to broadcast)")
      return
    }

    const pk = process.env.SIGNET_GATE_OWNER_KEY
    if (!pk) throw new Error("--send requires SIGNET_GATE_OWNER_KEY")
    const account = privateKeyToAccount((pk.startsWith("0x") ? pk : `0x${pk}`) as Hex)
    if (account.address.toLowerCase() !== owner.toLowerCase()) {
      throw new Error(
        `SIGNET_GATE_OWNER_KEY is ${account.address} but the gate owner is ${owner}`,
      )
    }
    const balance = await client.getBalance({ address: account.address })
    if (balance === 0n) {
      throw new Error(`${account.address} holds 0 CELO and cannot pay for this call`)
    }
    console.log(`balance  ${formatEther(balance)} CELO`)

    const wallet = createWalletClient({
      account, chain: celo, transport: http(SIGNET_CELO_RPC_URL),
    })
    const hash = await wallet.sendTransaction({ to: SIGNET_GATE, data })
    console.log(`sent     ${hash}`)
    const receipt = await client.waitForTransactionReceipt({ hash })
    console.log(`status   ${receipt.status}`)
  },

  /** What every node independently computes. Authoritative preflight. */
  async resolve([eoa]) {
    const owner = (eoa ?? eoaAccount().address) as Address
    const { ok, subject } = await resolveSubject(client, owner)
    console.log(`resolve(${owner}) -> ok=${ok} subject=${subject}`)
    if (!ok) {
      console.log("  (0x0 subject means the gate denied it or nothing is bound)")
    }
  },

  /**
   * Per-node readiness. The only client-visible signal for handoff step B:
   * /v1/info advertises neither siwe_domain nor configured chains.
   */
  async preflight() {
    const account = eoaAccount()
    const outcomes = await preflightNodes({
      eoa: account.address,
      signMessage: (message) => account.signMessage({ message }),
      getBlockPin: () => getBlockPin(client),
    })
    for (const o of outcomes) {
      if (o.ok) console.log(`OK   ${o.nodeUrl}  identity=${o.identity}`)
      else {
        const tag = o.error?.isNodeMisconfiguration ? "CONFIG" : "FAIL  "
        console.log(`${tag} ${o.nodeUrl}  ${o.error?.code}: ${o.error?.detail}`)
      }
    }
  },

  async auth([nodeUrl]) {
    const t0 = Date.now()
    const session = await open(nodeUrl)
    console.log(`identity   ${session.identity}`)
    console.log(`expires    ${new Date(session.expiresAt * 1000).toISOString()}`)
    console.log(`node       ${session.nodeUrl}`)
    console.log(`elapsed    ${Date.now() - t0}ms`)
  },

  /** Idempotent: 409 from the node is success, and reported as such. */
  async keygen() {
    const { keygen: doKeygen } = await import("@oleary-labs/signet-sdk/keygen")
    const { SIGNET_CURVE } = await import("../../lib/signet/config")
    const session = await open()
    const r = await doKeygen(
      { nodeUrls: SIGNET_NODE_URLS, groupId: SIGNET_GROUP_ID },
      session.keypair, null, SIGNET_KEY_SUFFIX, session.identity, SIGNET_CURVE,
    )
    console.log(JSON.stringify(r, null, 2))
  },

  /**
   * The highest-value command: proves the exact primitive the bundler needs,
   * with no transaction, no gas and no bundler. `-n N` also yields the latency
   * distribution the rollout plan needs as a measurement rather than a guess.
   */
  async "sign-digest"(args) {
    const nFlag = args.indexOf("-n")
    const runs = nFlag >= 0 ? Number(args[nFlag + 1]) : 1
    const input = args.find((a, i) => !a.startsWith("-") && i !== nFlag + 1)
    const hash = input && input.startsWith("0x") && input.length === 66
      ? (input as Hex)
      : keccak256(toBytes(input ?? "signet plumbing check"))

    const session = await open()
    console.log(`identity ${session.identity}`)
    console.log(`digest   ${hash}`)

    const timings: number[] = []
    let recovered = ""
    for (let i = 0; i < runs; i++) {
      const r = await signEvmDigest(session, toBytes(hash))
      timings.push(r.elapsedMs)
      recovered = r.recovered
      if (runs === 1) {
        console.log(`sig      ${r.signature}`)
        console.log(`v        ${parseInt(r.signature.slice(-2), 16)} (must be 27 or 28)`)
      }
    }
    // signEvmDigest already asserts the recovery locally; echo it so the
    // operator sees the address the on-chain module will compare against.
    console.log(`recovers ${recovered}`)
    timings.sort((a, b) => a - b)
    const pct = (p: number) => timings[Math.min(timings.length - 1, Math.floor(timings.length * p))]
    console.log(
      `latency  n=${runs} min=${timings[0]}ms p50=${pct(0.5)}ms p95=${pct(0.95)}ms max=${timings[timings.length - 1]}ms`,
    )
  },

  /**
   * Sanity-check the sponsored path before using it for anything permanent.
   * Reads the live /config, builds the bundler, and reports what it would use.
   */
  async "check-module"() {
    const config = await loadCommunityConfig()
    console.log(`backend        ${backendUrl()}/config`)
    console.log(`chain          ${config.chainId}`)
    console.log(`token          ${config.tokenSymbol} ${config.tokenAddress}`)
    console.log(`factory        ${config.factoryAddress}`)
    console.log(`module/entry   ${config.entrypointAddress}`)
    console.log(`paymaster      ${config.paymasterAddress} (${config.paymasterType})`)
    console.log(`bundler rpc    ${config.bundlerRpcUrl}`)
    console.log(`read rpc       ${config.rpcUrl}`)
  },

  /**
   * Step 2 on its own.
   *
   * Two halves: the ACCOUNT signs an EIP-712 authorization (only it may
   * authorize its own binding), and the Safe relays that signature and pays.
   * Rebinding is allowed, but re-pointing moves the Signet subject.
   */
  async bind([safeArg, indexArg]) {
    if (!safeArg) throw new Error("usage: bind <safe> [smartAccountIndex] [--send]")
    const safe = getAddress(safeArg) as Address
    const eoa = await eoaFor(safe)
    const send = process.argv.includes("--send")

    const state = await readEnrolmentState(client, eoa, safe)
    if (!state.allowed) throw new Error(`${eoa} is not allowlisted`)
    if (!state.walletDeployed) throw new Error(`${safe} is not deployed`)
    if (state.boundSafe && !state.boundElsewhere) {
      console.log(`already bound to ${state.boundSafe} — nothing to do`)
      return
    }

    const auth = await buildBindAuthorization(client, eoa, safe)
    console.log(`account   ${auth.message.account}`)
    console.log(`safe      ${auth.message.safe}`)
    console.log(`nonce     ${auth.message.nonce}`)
    console.log(`deadline  ${auth.message.deadline} (${new Date(Number(auth.message.deadline) * 1000).toISOString()})`)
    console.log(`domain    ${auth.domain.name} v${auth.domain.version} chain ${auth.domain.chainId}`)
    console.log(`registry  ${auth.domain.verifyingContract}`)
    if (state.boundElsewhere) {
      console.log(`REBIND    currently ${state.boundSafe} -> ${safe}; the subject moves with it`)
    }

    if (!send) {
      console.log("\n(dry run — pass --send to sign and relay)")
      return
    }

    const signature = await signBind(auth)
    const data = encodeBindWithSignature(auth, signature)
    console.log(`signature ${signature}`)

    const { execute } = await safeExecutor({
      privateKey: process.env.SIGNET_PRIVATE_KEY as string,
      safe,
      index: indexArg ? Number(indexArg) : 0,
    })
    const hash = await execute(SIGNET_REGISTRY, data)
    console.log(`sent      ${hash}`)
    const after = await readEnrolmentState(client, eoa, safe)
    console.log(`boundSafe now ${after.boundSafe}`)
  },

  /** Step 5 on its own, for when keygen already ran. */
  async "add-owner"([safeArg, signetArg, indexArg]) {
    if (!safeArg || !signetArg) {
      throw new Error("usage: add-owner <safe> <signetAddress> [smartAccountIndex] [--send]")
    }
    const safe = getAddress(safeArg) as Address
    const signetAddress = getAddress(signetArg) as Address
    const send = process.argv.includes("--send")

    const already = await client.readContract({
      address: safe, abi: safeAbi, functionName: "isOwner", args: [signetAddress],
    })
    if (already) {
      console.log(`${signetAddress} is already an owner of ${safe}`)
      return
    }

    // Threshold stays 1: this ADDS a signer, it does not replace the Privy key.
    // Both owners remain independently sufficient, which is what keeps the
    // fallback real rather than nominal.
    const data = encodeFunctionData({
      abi: safeAbi, functionName: "addOwnerWithThreshold", args: [signetAddress, 1n],
    })
    console.log(`from     ${safe}  (self-call via CommunityModule)`)
    console.log(`function addOwnerWithThreshold(${signetAddress}, 1)`)
    console.log(`data     ${data}`)
    if (!send) {
      console.log("\n(dry run — pass --send to broadcast)")
      return
    }

    const { execute } = await safeExecutor({
      privateKey: process.env.SIGNET_PRIVATE_KEY as string,
      safe,
      index: indexArg ? Number(indexArg) : 0,
    })
    const hash = await execute(safe, data)
    console.log(`sent     ${hash}`)
    const owners = await client.readContract({
      address: safe, abi: safeAbi, functionName: "getOwners",
    })
    console.log(`owners now ${owners.join(", ")}`)
  },

  /** Full steps 2-5, resumable. Needs a way to send calls from the Safe. */
  async enrol([wallet]) {
    if (!wallet) throw new Error("usage: enrol <safe>")
    const account = eoaAccount()
    const { execute } = await safeExecutor({
      privateKey: process.env.SIGNET_PRIVATE_KEY as string,
      safe: getAddress(wallet) as Address,
    })
    const session = await open()
    const r = await enrol({
      client,
      session,
      eoa: account.address,
      safe: wallet as Address,
      execute,
      signTypedData: signBind,
      onProgress: (p) =>
        console.log(`  ${p.step.padEnd(10)} ${p.status}${p.hash ? ` ${p.hash}` : ""}${p.detail ? ` ${p.detail}` : ""}`),
    })
    console.log(JSON.stringify(r, null, 2))
  },
}

async function main() {
  const [cmd, ...args] = process.argv.slice(2)
  const fn = cmd ? commands[cmd] : undefined
  if (!fn) {
    console.error(`usage: npm run signet -- <command>\n\n  ${Object.keys(commands).join("\n  ")}`)
    process.exit(1)
  }
  await fn(args)
}

main().catch((e) => {
  console.error(e instanceof Error ? e.message : e)
  process.exit(1)
})
