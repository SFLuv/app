import { ConnectedWallet, EIP1193Provider } from "@privy-io/react-auth";
import { BrowserProvider, JsonRpcSigner, Signer } from "ethers";
import { toSimpleSmartAccount, ToSimpleSmartAccountReturnType } from "permissionless/accounts";
import { Address, createPublicClient, createWalletClient, custom, encodeFunctionData, formatUnits, Hex, hexToBytes, parseUnits, PublicClient } from "viem";
import { CHAIN, BYUSD_DECIMALS, HONEY_DECIMALS, SFLUV_DECIMALS, FACTORY, SFLUV_TOKEN, BYUSD_TOKEN, HONEY_TOKEN, ZAPPER_CONTRACT_ADDRESS } from "../constants";
import { entryPoint07Address } from "viem/account-abstraction";
import { Hash } from "viem";
import { cw_bundler } from "../paymaster/client";
import { allowance, approve, balanceOf, depositFor, hasRole, minterRole, redeemerRole, transfer, unwrapSwapAndBridge, zapIn } from "../abi";

export type WalletType = "smartwallet" | "eoa"

interface TxState {
  sending: boolean;
  error: string | null;
  hash: string | null;
}

interface AppWalletOptions {
  index?: bigint
  id?: number
  isRedeemer?: boolean
  isMinter?: boolean
}

const MAX_UINT256 = (1n << 256n) - 1n
const SMART_WALLET_DEPLOYMENT_TIMEOUT_MS = 90_000
const SMART_WALLET_DEPLOYMENT_POLL_INTERVAL_MS = 1_500


export class AppWallet {

  owner: ConnectedWallet;
  index?: bigint;

  name: string;
  id?: number;
  isRedeemer: boolean;
  isMinter: boolean;
  type: WalletType;
  address?: Address;
  wallet?: ConnectedWallet | ToSimpleSmartAccountReturnType<"0.7">;
  initialized: boolean;
  // unwrap: (amount: bigint, to: Address) => Promise<TxState | null>;
  // wrap: (amount: bigint) => Promise<TxState | null>;
  // send: (amount: bigint, to: Address) => Promise<TxState | null>;
  // getBalance: () => Promise<bigint>;

  private viemProvider?: EIP1193Provider;
  private ethersProvider?: BrowserProvider;
  private ethersSigner?: JsonRpcSigner;
  private publicClient?: PublicClient;

  constructor(owner: ConnectedWallet, name: string, options?: AppWalletOptions) {
    this.owner = owner
    this.index = options?.index
    this.id = options?.id
    this.isRedeemer = options?.isRedeemer === true
    this.isMinter = options?.isMinter === true
    this.name = name
    this.type = options?.index !== undefined ? "smartwallet" : "eoa"
    this.initialized = false
  }

  async init(): Promise<boolean> {
    this.viemProvider = await this.owner.getEthereumProvider()
    this.ethersProvider = new BrowserProvider(this.viemProvider)
    this.ethersSigner = await this.ethersProvider.getSigner()
    this.publicClient = createPublicClient({
      chain: CHAIN,
      transport: custom(this.viemProvider)
    })

    if(this.index !== undefined) {
      const client = createWalletClient({
        account: this.owner.address as Hex,
        chain: CHAIN,
        transport: custom(this.viemProvider)
      })

      const smartWallet = await toSimpleSmartAccount({
        owner: client,
        client,
        entryPoint: {
          address: entryPoint07Address as Address,
          version: "0.7"
        },
        index: this.index,
        factoryAddress: FACTORY
      })


      const code = await this.publicClient.getCode({
        address: smartWallet.address
      })

      this.wallet = smartWallet
      this.type = "smartwallet"
      this.address = this.wallet.address
      if(code === "0x" || code == null) {
        this.initialized = true
        return false
      }
    }
    else {
      this.wallet = this.owner
      this.type = "eoa"
      this.address = this.wallet.address as Address
    }

    this.initialized = true

    return true
  }

  private _hasCodeAt = async (address: Address): Promise<boolean> => {
    if (!this.publicClient) {
      return false
    }

    try {
      const code = await this.publicClient.getCode({ address })
      return code !== undefined && code !== null && code !== "0x"
    }
    catch (error) {
      console.error("error checking deployed smart wallet code", error)
      return false
    }
  }

  private _waitForCodeAt = async (address: Address): Promise<boolean> => {
    const start = Date.now()

    while (Date.now() - start < SMART_WALLET_DEPLOYMENT_TIMEOUT_MS) {
      if (await this._hasCodeAt(address)) {
        return true
      }
      await new Promise((resolve) => setTimeout(resolve, SMART_WALLET_DEPLOYMENT_POLL_INTERVAL_MS))
    }

    return false
  }

  async ensureSmartWalletDeployed(): Promise<boolean> {
    if (!this.initialized || this.type !== "smartwallet" || !this.address) {
      return false
    }

    if (await this._hasCodeAt(this.address)) {
      return true
    }

    const deploymentReceipt = await this._setTokenAllowance(SFLUV_TOKEN, this.address, 0n)
    if (deploymentReceipt.error || !deploymentReceipt.hash) {
      console.error("error submitting smart wallet deployment user operation", deploymentReceipt.error)
      return false
    }

    const deployed = await this._waitForCodeAt(this.address)
    if (!deployed) {
      console.error("smart wallet deployment confirmation timed out", this.address)
    }

    return deployed
  }

  // async deploy(): Promise<boolean> {
  //   if(!this.initialized) {
  //     console.error("wallet not initialized when attempting to deploy")
  //     return false
  //   }




  //   return true
  // }



  private _beforeTx(): { wallet: ToSimpleSmartAccountReturnType<"0.7">, signer: Signer } | null
  private _beforeTx(options: { allowEOA: true }): { signer: Signer } | null
  private _beforeTx(options?: { allowEOA?: boolean }): { wallet: ToSimpleSmartAccountReturnType<"0.7">, signer: Signer } | { signer: Signer } | null {
    if(!this.initialized) {
      console.error("wallet not yet initialized")
      return null
    }

    if(!this.ethersSigner) {
      console.error("signer not ready")
      return null
    }

    if(this.type === "smartwallet") {
      if(!this.wallet) {
        console.error("smartwallet not available")
        return null
      }

      return {
        wallet: this.wallet as ToSimpleSmartAccountReturnType<"0.7">, signer: this.ethersSigner
      }
    }

    if (options?.allowEOA) {
      return { signer: this.ethersSigner }
    }

    console.error("transaction type requires a smartwallet")
    return null
  }

  private _execTx = async (account: ToSimpleSmartAccountReturnType<"0.7">, signer: Signer, callData: Hash, contract?: Address): Promise<TxState> => {
    let receipt: TxState = {
      sending: false,
      error: null,
      hash: null
    }

    const data = hexToBytes(callData)

    try {
      const hash = await this._withTimeout(
        cw_bundler.call(
          signer,
          contract || SFLUV_TOKEN,
          account.address,
          data,
          undefined,
          undefined,
          undefined,
          { smartAccountIndex: this.index ? Number(this.index) : undefined }
        ),
        60_000,
        "Transaction submission"
      )
      receipt.hash = hash
    }
    catch(error) {
      const message = error instanceof Error ? error.message : "unknown error"
      receipt.error = `error sending transaction: ${message}`
      console.error(error)
    }

    return receipt
  }

  private _getAllowance = async (owner: Address, spender: Address, token: Address = SFLUV_TOKEN): Promise<bigint | null> => {
    if (!this.publicClient) return null

    try {
      return await this.publicClient.readContract({
        address: token,
        abi: [allowance],
        functionName: "allowance",
        args: [owner, spender]
      }) as bigint
    }
    catch (error) {
      console.error("error reading allowance", error)
      return null
    }
  }

  private _waitForAllowance = async (owner: Address, spender: Address, minAllowance: bigint, token: Address = SFLUV_TOKEN): Promise<boolean> => {
    const timeoutMs = 90_000
    const intervalMs = 1_500
    const start = Date.now()

    while (Date.now() - start < timeoutMs) {
      const currentAllowance = await this._getAllowance(owner, spender, token)
      if (currentAllowance !== null && currentAllowance >= minAllowance) {
        return true
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs))
    }

    return false
  }

  private _waitForAllowanceEquals = async (owner: Address, spender: Address, expectedAllowance: bigint, token: Address = SFLUV_TOKEN): Promise<boolean> => {
    const timeoutMs = 90_000
    const intervalMs = 1_500
    const start = Date.now()

    while (Date.now() - start < timeoutMs) {
      const currentAllowance = await this._getAllowance(owner, spender, token)
      if (currentAllowance !== null && currentAllowance === expectedAllowance) {
        return true
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs))
    }

    return false
  }

  private _waitForTokenBalanceAtMost = async (token: Address, owner: Address, maxBalance: bigint): Promise<boolean> => {
    const timeoutMs = 120_000
    const intervalMs = 2_000
    const start = Date.now()

    while (Date.now() - start < timeoutMs) {
      const currentBalance = await this.getBalanceOf(token, owner)
      if (currentBalance !== null && currentBalance <= maxBalance) {
        return true
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs))
    }

    return false
  }

  private _extractRevertSelector = (error: unknown): string | null => {
    const visit = (value: unknown): string | null => {
      if (value === null || value === undefined) return null

      if (typeof value === "string") {
        const dataMatch = value.match(/0x[0-9a-fA-F]{8,}/)
        return dataMatch ? dataMatch[0].slice(0, 10).toLowerCase() : null
      }

      if (Array.isArray(value)) {
        for (const item of value) {
          const found = visit(item)
          if (found) return found
        }
        return null
      }

      if (typeof value === "object") {
        const record = value as Record<string, unknown>
        const data = record.data
        if (typeof data === "string" && /^0x[0-9a-fA-F]{8,}$/.test(data)) {
          return data.slice(0, 10).toLowerCase()
        }

        for (const nested of Object.values(record)) {
          const found = visit(nested)
          if (found) return found
        }
      }

      return null
    }

    return visit(error)
  }

  private _mapUnwrapRevertReason = (selector: string | null): string => {
    if (selector === "0x6ce14a8b") {
      return "Unwrap is currently unavailable: Honey redemption returned UnexpectedBasketModeStatus."
    }

    if (selector) {
      return `Unwrap preflight reverted (${selector}).`
    }

    return "Unwrap preflight reverted."
  }

  private _simulateUnwrap = async (from: Address, amount: bigint, to: Address): Promise<string | null> => {
    if (!this.publicClient) {
      return "Unable to simulate unwrap transaction."
    }

    const data = encodeFunctionData({
      abi: [unwrapSwapAndBridge],
      functionName: "unwrapSwapAndBridge",
      args: [amount, to]
    })

    try {
      await this.publicClient.call({
        account: from,
        to: ZAPPER_CONTRACT_ADDRESS,
        data
      })
      return null
    }
    catch (error) {
      const selector = this._extractRevertSelector(error)
      console.error("unwrap preflight failed", error)
      return this._mapUnwrapRevertReason(selector)
    }
  }

  private _withTimeout = async <T>(promise: Promise<T>, timeoutMs: number, stage: string): Promise<T> => {
    let timeoutHandle: ReturnType<typeof setTimeout> | undefined

    try {
      return await Promise.race([
        promise,
        new Promise<T>((_, reject) => {
          timeoutHandle = setTimeout(() => {
            reject(new Error(`${stage} timed out after ${Math.floor(timeoutMs / 1000)}s`))
          }, timeoutMs)
        })
      ])
    } finally {
      if (timeoutHandle) {
        clearTimeout(timeoutHandle)
      }
    }
  }

  private _hasRedeemerRole = async (account: Address): Promise<boolean | null> => {
    if (!this.publicClient) return null

    try {
      const role = await this.publicClient.readContract({
        address: SFLUV_TOKEN,
        abi: [redeemerRole],
        functionName: "REDEEMER_ROLE"
      }) as Hash

      const walletHasRole = await this.publicClient.readContract({
        address: SFLUV_TOKEN,
        abi: [hasRole],
        functionName: "hasRole",
        args: [role, account]
      }) as boolean

      return walletHasRole
    }
    catch (error) {
      console.error("error verifying REDEEMER_ROLE", error)
      return null
    }
  }

  private _hasMinterRole = async (account: Address): Promise<boolean | null> => {
    if (!this.publicClient) return null

    try {
      const role = await this.publicClient.readContract({
        address: SFLUV_TOKEN,
        abi: [minterRole],
        functionName: "MINTER_ROLE"
      }) as Hash

      const walletHasRole = await this.publicClient.readContract({
        address: SFLUV_TOKEN,
        abi: [hasRole],
        functionName: "hasRole",
        args: [role, account]
      }) as boolean

      return walletHasRole
    }
    catch (error) {
      console.error("error verifying MINTER_ROLE", error)
      return null
    }
  }

  private _submitContractCall = async (contract: Address, callData: Hash, value: bigint = 0n): Promise<TxState> => {
    const receipt: TxState = {
      sending: false,
      error: null,
      hash: null
    }

    if (!this.initialized || !this.address) {
      receipt.error = "wallet not initialized"
      return receipt
    }

    try {
      if (this.type === "smartwallet") {
        const t = this._beforeTx()
        if (!t) {
          receipt.error = "smart wallet not ready"
          return receipt
        }
        const hash = await this._withTimeout(
          cw_bundler.call(
            t.signer,
            contract,
            t.wallet.address,
            hexToBytes(callData),
            value,
            undefined,
            undefined,
            { smartAccountIndex: this.index ? Number(this.index) : undefined }
          ),
          60_000,
          "Transaction submission"
        )
        receipt.hash = hash
        return receipt
      }

      if (!this.ethersSigner) {
        receipt.error = "eoa signer not ready"
        return receipt
      }
      return this._submitEOATransaction(contract, callData, value)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : "unknown error"
      receipt.error = `error sending transaction: ${message}`
      console.error(error)
      return receipt
    }
  }

  private _submitEOATransaction = async (contract: Address, callData: Hash, value: bigint = 0n): Promise<TxState> => {
    const receipt: TxState = {
      sending: false,
      error: null,
      hash: null
    }

    if (!this.initialized || !this.address) {
      receipt.error = "wallet not initialized"
      return receipt
    }

    if (!this.ethersSigner) {
      receipt.error = "eoa signer not ready"
      return receipt
    }

    try {
      const tx = await this._withTimeout(
        this.ethersSigner.sendTransaction({
          to: contract,
          data: callData,
          value
        }),
        60_000,
        "Transaction submission"
      )
      receipt.hash = tx.hash
      return receipt
    }
    catch (error) {
      const message = error instanceof Error ? error.message : "unknown error"
      receipt.error = `error sending transaction: ${message}`
      console.error(error)
      return receipt
    }
  }

  private _setTokenAllowance = async (token: Address, spender: Address, value: bigint): Promise<TxState> => {
    const callData = encodeFunctionData({
      abi: [approve],
      functionName: "approve",
      args: [spender, value]
    })

    return this._submitContractCall(token, callData)
  }

  private _clearTokenAllowance = async (token: Address, owner: Address, spender: Address): Promise<string | null> => {
    const clearReceipt = await this._setTokenAllowance(token, spender, 0n)
    if (clearReceipt.error || !clearReceipt.hash) {
      return "failed to submit allowance reset transaction"
    }

    const allowanceCleared = await this._waitForAllowanceEquals(owner, spender, 0n, token)
    if (!allowanceCleared) {
      return "allowance reset is still pending"
    }

    return null
  }

  private _simulateZapIn = async (from: Address, amount: bigint): Promise<string | null> => {
    if (!this.publicClient) {
      return "Unable to simulate mint transaction."
    }

    const data = encodeFunctionData({
      abi: [zapIn],
      functionName: "zapIn",
      args: [amount]
    })

    try {
      await this.publicClient.call({
        account: from,
        to: ZAPPER_CONTRACT_ADDRESS,
        data
      })
      return null
    }
    catch (error) {
      console.error("mint preflight failed", error)
      return "Mint preflight reverted."
    }
  }

  private _setZapperAllowance = async (account: ToSimpleSmartAccountReturnType<"0.7">, signer: Signer, value: bigint): Promise<TxState> => {
    const approveCallData = encodeFunctionData({
      abi: [approve],
      functionName: "approve",
      args: [ZAPPER_CONTRACT_ADDRESS, value]
    })

    return this._execTx(account, signer, approveCallData, SFLUV_TOKEN)
  }

  private _clearZapperAllowance = async (account: ToSimpleSmartAccountReturnType<"0.7">, signer: Signer): Promise<string | null> => {
    const clearReceipt = await this._setZapperAllowance(account, signer, 0n)
    if (clearReceipt.error || !clearReceipt.hash) {
      return "failed to submit allowance reset transaction"
    }

    const allowanceCleared = await this._waitForAllowanceEquals(account.address, ZAPPER_CONTRACT_ADDRESS, 0n)
    if (!allowanceCleared) {
      return "allowance reset is still pending"
    }

    return null
  }

  hasRedeemerRole = async (): Promise<boolean> => {
    if (this.type !== "smartwallet") {
      return false
    }
    if (!this.address) {
      return false
    }

    const hasRole = await this._hasRedeemerRole(this.address)
    return hasRole === true
  }

  hasMinterRole = async (): Promise<boolean> => {
    if (!this.address) {
      return false
    }

    const hasRole = await this._hasMinterRole(this.address)
    return hasRole === true
  }

  setId = (id: number) => {
    this.id = id
  }

  wrap = async (amount: bigint): Promise<TxState | null>  => {
    const t = this._beforeTx()
    if(!t) return null

    const callData = encodeFunctionData({
      abi: [depositFor],
      functionName: "depositFor",
      args: [t.wallet.address, amount]
    })

    return this._execTx(t.wallet, t.signer, callData)
  }

  mintSFLUVFromBYUSD = async (amount: string): Promise<TxState | null> => {
    if (!this.address) {
      return {
        sending: false,
        error: "Wallet address is not available",
        hash: null
      }
    }

    let sendAmount: bigint
    try {
      sendAmount = parseUnits(amount, BYUSD_DECIMALS)
    }
    catch {
      return {
        sending: false,
        error: "Invalid mint amount",
        hash: null
      }
    }
    if (sendAmount <= 0n) {
      return {
        sending: false,
        error: "Mint amount must be greater than zero",
        hash: null
      }
    }

    const walletHasMinterRole = await this._hasMinterRole(this.address)
    if (walletHasMinterRole === null) {
      return {
        sending: false,
        error: "Unable to verify MINTER_ROLE on SFLUV contract",
        hash: null
      }
    }
    if (!walletHasMinterRole) {
      return {
        sending: false,
        error: "Wallet is missing MINTER_ROLE and cannot mint",
        hash: null
      }
    }

    const currentBalance = await this.getBalance(BYUSD_TOKEN)
    if (currentBalance === null) {
      return {
        sending: false,
        error: "Unable to read BYUSD balance",
        hash: null
      }
    }
    if (currentBalance < sendAmount) {
      return {
        sending: false,
        error: "Insufficient BYUSD balance for mint",
        hash: null
      }
    }

    const currentAllowance = await this._getAllowance(this.address, ZAPPER_CONTRACT_ADDRESS, BYUSD_TOKEN)
    if (currentAllowance === null) {
      return {
        sending: false,
        error: "Unable to verify BYUSD approval status",
        hash: null
      }
    }

    if (currentAllowance > 0n) {
      const clearError = await this._clearTokenAllowance(BYUSD_TOKEN, this.address, ZAPPER_CONTRACT_ADDRESS)
      if (clearError) {
        return {
          sending: false,
          error: `Unable to clear previous approval: ${clearError}`,
          hash: null
        }
      }
    }

    const approveReceipt = await this._setTokenAllowance(BYUSD_TOKEN, ZAPPER_CONTRACT_ADDRESS, MAX_UINT256)
    if (approveReceipt.error || !approveReceipt.hash) {
      return {
        sending: false,
        error: approveReceipt.error ?? "error approving BYUSD spend: check logs",
        hash: approveReceipt.hash
      }
    }

    const allowanceUpdated = await this._waitForAllowance(this.address, ZAPPER_CONTRACT_ADDRESS, MAX_UINT256, BYUSD_TOKEN)
    if (!allowanceUpdated) {
      return {
        sending: false,
        error: "Approval sent, but confirmation is still pending. Please retry in a moment.",
        hash: approveReceipt.hash
      }
    }

    const preflightError = await this._simulateZapIn(this.address, sendAmount)
    if (preflightError) {
      const cleanupError = await this._clearTokenAllowance(BYUSD_TOKEN, this.address, ZAPPER_CONTRACT_ADDRESS)
      return {
        sending: false,
        error: cleanupError
          ? `${preflightError} Also unable to reset approval: ${cleanupError}`
          : preflightError,
        hash: null
      }
    }

    const mintCallData = encodeFunctionData({
      abi: [zapIn],
      functionName: "zapIn",
      args: [sendAmount]
    })

    const mintReceipt = await this._submitContractCall(ZAPPER_CONTRACT_ADDRESS, mintCallData)
    if (mintReceipt.error || !mintReceipt.hash) {
      const cleanupError = await this._clearTokenAllowance(BYUSD_TOKEN, this.address, ZAPPER_CONTRACT_ADDRESS)
      if (cleanupError) {
        mintReceipt.error = `${mintReceipt.error ?? "mint failed"}. Also unable to reset approval: ${cleanupError}`
      }
      return mintReceipt
    }

    const cleanupError = await this._clearTokenAllowance(BYUSD_TOKEN, this.address, ZAPPER_CONTRACT_ADDRESS)
    if (cleanupError) {
      mintReceipt.error = `Mint submitted, but failed to clear leftover approval: ${cleanupError}`
    }

    return mintReceipt
  }

  mintSFLUVFromHONEY = async (amount: string): Promise<TxState | null> => {
    if (!this.address) {
      return {
        sending: false,
        error: "Wallet address is not available",
        hash: null
      }
    }

    let sendAmount: bigint
    try {
      sendAmount = parseUnits(amount, HONEY_DECIMALS)
    }
    catch {
      return {
        sending: false,
        error: "Invalid mint amount",
        hash: null
      }
    }
    if (sendAmount <= 0n) {
      return {
        sending: false,
        error: "Mint amount must be greater than zero",
        hash: null
      }
    }

    const walletHasMinterRole = await this._hasMinterRole(this.address)
    if (walletHasMinterRole === null) {
      return {
        sending: false,
        error: "Unable to verify MINTER_ROLE on SFLUV contract",
        hash: null
      }
    }
    if (!walletHasMinterRole) {
      return {
        sending: false,
        error: "Wallet is missing MINTER_ROLE and cannot mint",
        hash: null
      }
    }

    const currentBalance = await this.getBalance(HONEY_TOKEN)
    if (currentBalance === null) {
      return {
        sending: false,
        error: "Unable to read Honey balance",
        hash: null
      }
    }
    if (currentBalance < sendAmount) {
      return {
        sending: false,
        error: "Insufficient Honey balance for mint",
        hash: null
      }
    }

    const currentAllowance = await this._getAllowance(this.address, SFLUV_TOKEN, HONEY_TOKEN)
    if (currentAllowance === null) {
      return {
        sending: false,
        error: "Unable to verify Honey approval status",
        hash: null
      }
    }
    if (currentAllowance > 0n) {
      const clearError = await this._clearTokenAllowance(HONEY_TOKEN, this.address, SFLUV_TOKEN)
      if (clearError) {
        return {
          sending: false,
          error: `Unable to clear previous approval: ${clearError}`,
          hash: null
        }
      }
    }

    const approveReceipt = await this._setTokenAllowance(HONEY_TOKEN, SFLUV_TOKEN, MAX_UINT256)
    if (approveReceipt.error || !approveReceipt.hash) {
      return {
        sending: false,
        error: approveReceipt.error ?? "error approving Honey spend: check logs",
        hash: approveReceipt.hash
      }
    }

    const allowanceUpdated = await this._waitForAllowance(this.address, SFLUV_TOKEN, MAX_UINT256, HONEY_TOKEN)
    if (!allowanceUpdated) {
      return {
        sending: false,
        error: "Approval sent, but confirmation is still pending. Please retry in a moment.",
        hash: approveReceipt.hash
      }
    }

    const mintCallData = encodeFunctionData({
      abi: [depositFor],
      functionName: "depositFor",
      args: [this.address, sendAmount]
    })
    const mintReceipt = await this._submitContractCall(SFLUV_TOKEN, mintCallData)
    if (mintReceipt.error || !mintReceipt.hash) {
      const cleanupError = await this._clearTokenAllowance(HONEY_TOKEN, this.address, SFLUV_TOKEN)
      if (cleanupError) {
        mintReceipt.error = `${mintReceipt.error ?? "mint failed"}. Also unable to reset approval: ${cleanupError}`
      }
      return mintReceipt
    }

    const cleanupError = await this._clearTokenAllowance(HONEY_TOKEN, this.address, SFLUV_TOKEN)
    if (cleanupError) {
      mintReceipt.error = `Mint submitted, but failed to clear leftover approval: ${cleanupError}`
    }

    return mintReceipt
  }

  bridge = async (amount: number, paypalEthAddress : string): Promise<TxState | null>  => {
     const t = this._beforeTx()
    if(!t) return null

    const sourceAmount = String(amount * (10 ** BYUSD_DECIMALS))
    const destAmountMin = String((amount * (10 ** BYUSD_DECIMALS) * .95))

    const params = new URLSearchParams({
      srcToken: "0x688e72142674041f8f6Af4c808a4045cA1D6aC82",
      srcChainKey: "bera",
      dstToken: "0x6c3ea9036406852006290770BEdFcAbA0e23A0e8",
      dstChainKey: "ethereum",
      srcAddress: t.wallet.address,
      dstAddress: paypalEthAddress,
      srcAmount: sourceAmount,
      dstAmountMin: destAmountMin
      });

      console.log(t.wallet.address)
      console.log("Signer: " + JSON.stringify(t.signer, null, 2))

    const url = `https://stargate.finance/api/v1/quotes?${params.toString()}`;

    const res = await fetch(url, {
      method: "GET",
      headers: {
        "Accept": "application/json"
      }
    });

    const response = await res.json();
    // check if response got anything
    if (!response.quotes || response.quotes.length === 0) {
    console.error("No quotes returned from the API:", response);
    return null;
  }

      // Access route data from API response
    const route = response.quotes[0]; // First route (oft/v2)

    // Access transaction steps
    const bridgeStep = route.steps[0]; // First step (bridge)

    const bridgeTransactionValue = BigInt(bridgeStep.transaction.value)
    const bridgeTransactionData = bridgeStep.transaction.data

    let receipt: TxState = {
        sending: false,
        error: null,
        hash: null
      }

    const data = hexToBytes(bridgeTransactionData)

    try {
      const hash = await cw_bundler.call(t.signer, BYUSD_TOKEN, t.wallet.address, data, bridgeTransactionValue, undefined, undefined, { smartAccountIndex: this.index ? Number(this.index) : undefined })
      receipt.hash = hash
    }
    catch(error) {
      receipt.error = "error sending transaction: check logs"
      console.error(error)
    }

    console.log(receipt)
    return receipt
  }

  unwrapAndBridge = async (amount: string, to: string): Promise<TxState | null> => {
    const t = this._beforeTx()
    if(!t) return null

    let sendAmount: bigint
    try {
      sendAmount = parseUnits(amount, SFLUV_DECIMALS)
    }
    catch {
      return {
        sending: false,
        error: "Invalid cash out amount",
        hash: null
      }
    }

    if (sendAmount <= 0n) {
      return {
        sending: false,
        error: "Cash out amount must be greater than zero",
        hash: null
      }
    }

    if (!to.startsWith("0x") || to.length !== 42) {
      return {
        sending: false,
        error: "Invalid PayPal ETH address",
        hash: null
      }
    }

    const walletHasRedeemerRole = await this._hasRedeemerRole(t.wallet.address)
    if (walletHasRedeemerRole === null) {
      return {
        sending: false,
        error: "Unable to verify REDEEMER_ROLE on SFLUV contract",
        hash: null
      }
    }

    if (!walletHasRedeemerRole) {
      return {
        sending: false,
        error: "Wallet is missing REDEEMER_ROLE and cannot unwrap",
        hash: null
      }
    }

    const currentBalance = await this.getBalance(SFLUV_TOKEN)
    if (currentBalance === null) {
      return {
        sending: false,
        error: "Unable to read SFLUV balance",
        hash: null
      }
    }

    if (currentBalance < sendAmount) {
      return {
        sending: false,
        error: "Insufficient SFLUV balance for unwrap",
        hash: null
      }
    }

    const currentAllowance = await this._getAllowance(t.wallet.address, ZAPPER_CONTRACT_ADDRESS)
    if (currentAllowance === null) {
      return {
        sending: false,
        error: "Unable to verify wallet approval status",
        hash: null
      }
    }

    if (currentAllowance > 0n) {
      const clearError = await this._clearZapperAllowance(t.wallet, t.signer)
      if (clearError) {
        return {
          sending: false,
          error: `Unable to clear previous approval: ${clearError}`,
          hash: null
        }
      }
    }

    const approveReceipt = await this._setZapperAllowance(t.wallet, t.signer, sendAmount)
    if (approveReceipt.error || !approveReceipt.hash) {
      return {
        sending: false,
        error: approveReceipt.error ?? "error approving SFLUV spend: check logs",
        hash: approveReceipt.hash
      }
    }

    const allowanceUpdated = await this._waitForAllowance(t.wallet.address, ZAPPER_CONTRACT_ADDRESS, sendAmount)
    if (!allowanceUpdated) {
      return {
        sending: false,
        error: "Approval sent, but confirmation is still pending. Please retry in a moment.",
        hash: approveReceipt.hash
      }
    }

    const allowanceAfterApprove = await this._getAllowance(t.wallet.address, ZAPPER_CONTRACT_ADDRESS)
    if (allowanceAfterApprove === null || allowanceAfterApprove < sendAmount) {
      return {
        sending: false,
        error: "Unable to verify exact SFLUV approval amount",
        hash: approveReceipt.hash
      }
    }

    const preflightError = await this._simulateUnwrap(t.wallet.address, sendAmount, to as Address)
    if (preflightError) {
      const cleanupError = await this._clearZapperAllowance(t.wallet, t.signer)
      return {
        sending: false,
        error: cleanupError
          ? `${preflightError} Also unable to reset approval: ${cleanupError}`
          : preflightError,
        hash: null
      }
    }

    const callData = encodeFunctionData({
      abi: [unwrapSwapAndBridge],
      functionName: "unwrapSwapAndBridge",
      args: [sendAmount, to]
    })

    const callDataBytes = hexToBytes(callData)

    let receipt: TxState = {
        sending: false,
        error: null,
        hash: null
      }

    console.log("Unwrapping and bridging " + sendAmount + " to: " + to)
    console.log("Index: " + this.index)
    try {
      const hash = await this._withTimeout(
        cw_bundler.call(
          t.signer,
          ZAPPER_CONTRACT_ADDRESS,
          t.wallet.address,
          callDataBytes,
          undefined,
          undefined,
          undefined,
          { smartAccountIndex: this.index ? Number(this.index) : undefined }
        ),
        60_000,
        "Unwrap transaction submission"
      )
      receipt.hash = hash

      const expectedMaxBalance = currentBalance - sendAmount
      const debited = await this._waitForTokenBalanceAtMost(SFLUV_TOKEN, t.wallet.address, expectedMaxBalance)
      if (!debited) {
        receipt.error = `Transaction reverted at hash: ${hash}`
        console.error("unwrap transaction not confirmed by balance check", hash)
        return receipt
      }
    }
    catch(error) {
      const cleanupError = await this._clearZapperAllowance(t.wallet, t.signer)
      const message = error instanceof Error ? error.message : "unknown error"
      receipt.error = `error sending transaction: ${message}`
      if (cleanupError) {
        receipt.error = `${receipt.error}. Also unable to reset approval: ${cleanupError}`
      }
      console.error(error)
      console.error("unwrap failed after approval; attempted to clear allowance", cleanupError)
      return receipt
    }

    const allowanceConsumed = await this._waitForAllowanceEquals(t.wallet.address, ZAPPER_CONTRACT_ADDRESS, 0n)
    if (!allowanceConsumed) {
      const remainingAllowance = await this._getAllowance(t.wallet.address, ZAPPER_CONTRACT_ADDRESS)
      if (remainingAllowance === null) {
        console.warn("unwrap submitted, but allowance reset could not be verified yet")
      } else if (remainingAllowance > 0n) {
        const cleanupError = await this._clearZapperAllowance(t.wallet, t.signer)
        if (cleanupError) {
          console.warn(
            "unwrap submitted, but allowance cleanup is still pending",
            cleanupError
          )
        }
      }
    }

    console.log(receipt)
    return receipt
   }

  send = async (amount: bigint, to: Address): Promise<TxState | null> => {
    if(!this._beforeTx({ allowEOA: true })) return null

    const callData = encodeFunctionData({
      abi: [transfer],
      functionName: "transfer",
      args: [to, amount]
    })

    if (this.type === "eoa") {
      return this._submitEOATransaction(SFLUV_TOKEN, callData)
    }

    return this._submitContractCall(SFLUV_TOKEN, callData)
  }

  sendBYUSD = async (amount: bigint, to: Address): Promise<TxState | null> => {
    if(!this._beforeTx({ allowEOA: true })) return null

    const callData = encodeFunctionData({
      abi: [transfer],
      functionName: "transfer",
      args: [to, amount]
    })

    if (this.type === "eoa") {
      return this._submitEOATransaction(BYUSD_TOKEN, callData)
    }

    return this._submitContractCall(BYUSD_TOKEN, callData)
  }

  getBalance = async (token: Address): Promise<bigint | null> => {
    if(!this.initialized) return null
    if(!this.wallet) return null
    if(!this.publicClient) return null
    if(!this.address) return null

    try {
      const statement = {
        address: token,
        abi: [balanceOf],
        functionName: "balanceOf",
        args: [this.address],
      }
      const balance = await this.publicClient.readContract(statement) as bigint
      return balance
    } catch (error) {
      console.error("error reading token balance", error)
      return null
    }
  }

  getBalanceOf = async (token: Address, address: Address): Promise<bigint | null> => {
    if(!this.initialized) return null
    if(!this.wallet) return null
    if(!this.publicClient) return null
    if(!this.address) return null

    try {
      const statement = {
        address: token,
        abi: [balanceOf],
        functionName: "balanceOf",
        args: [address],
      }
      const balance = await this.publicClient.readContract(statement) as bigint
      return balance
    } catch (error) {
      console.error("error reading token balance", error)
      return null
    }
  }

  getGasTokenBalance = async (): Promise<bigint | null> => {
    if(!this.initialized) return null
    if(!this.publicClient) return null
    if(!this.address) return null

    try {
      const balance = await this.publicClient.getBalance({
        address: this.address
      })
      return balance
    }
    catch (error) {
      console.error("error reading gas token balance", error)
      return null
    }
  }

  getGasTokenBalanceFormatted = async (): Promise<number | null> => {
    const b = await this.getGasTokenBalance()
    if(b === null) return null

    const formatted = Number(formatUnits(b, CHAIN.nativeCurrency.decimals))
    if (!Number.isFinite(formatted)) return null
    return formatted
  }

  getSFLUVBalanceFormatted = async (): Promise<number | null> => {
    const b = await this.getBalance(SFLUV_TOKEN)
    if(b === null) return null

    const d = BigInt(10 ** SFLUV_DECIMALS)
    const q = Number(b * 100n / (d)) / 100

    return q
  }

  getBYUSDBalanceFormatted = async (): Promise<number | null> => {
    const b = await this.getBalance(BYUSD_TOKEN)
    if(b === null) return null

    const d = 10n ** BigInt(BYUSD_DECIMALS)
    const q = Number(b * 100n / (d)) / 100

    return q
  }

  getHoneyBalanceFormatted = async (): Promise<number | null> => {
    if (!HONEY_TOKEN || HONEY_TOKEN.length !== 42) return null
    const b = await this.getBalance(HONEY_TOKEN)
    if(b === null) return null

    const d = 10n ** BigInt(HONEY_DECIMALS)
    const q = Number(b * 100n / (d)) / 100

    return q
  }
}
