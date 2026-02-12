import { ConnectedWallet, EIP1193Provider } from "@privy-io/react-auth";
import { BrowserProvider, JsonRpcSigner, Signer } from "ethers";
import { toSimpleSmartAccount, ToSimpleSmartAccountReturnType } from "permissionless/accounts";
import { Address, createPublicClient, createWalletClient, custom, encodeFunctionData, Hex, hexToBytes, parseUnits, PublicClient } from "viem";
import { CHAIN, BYUSD_DECIMALS, SFLUV_DECIMALS, FACTORY, SFLUV_TOKEN, BYUSD_TOKEN, ZAPPER_CONTRACT_ADDRESS } from "../constants";
import { entryPoint07Address } from "viem/account-abstraction";
import { Hash } from "viem";
import { cw_bundler } from "../paymaster/client";
import { allowance, approve, balanceOf, depositFor, hasRole, redeemerRole, transfer, unwrapSwapAndBridge } from "../abi";

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
}



export class AppWallet {

  owner: ConnectedWallet;
  index?: bigint;

  name: string;
  id?: number;
  isRedeemer: boolean;
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

  // async deploy(): Promise<boolean> {
  //   if(!this.initialized) {
  //     console.error("wallet not initialized when attempting to deploy")
  //     return false
  //   }




  //   return true
  // }



  private _beforeTx = (): { wallet: ToSimpleSmartAccountReturnType<"0.7">, signer: Signer } | null => {
    if(!this.initialized) {
      console.error("wallet not yet initialized")
      return null
    }

    if(!(this.type === "smartwallet")) {
      console.error("eoa transactions not yet implemented")
      return null
    }

    if(!this.ethersSigner) {
      return null
    }

    return {
      wallet: this.wallet as ToSimpleSmartAccountReturnType<"0.7">, signer: this.ethersSigner
    }
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

  private _getAllowance = async (owner: Address, spender: Address): Promise<bigint | null> => {
    if (!this.publicClient) return null

    try {
      return await this.publicClient.readContract({
        address: SFLUV_TOKEN,
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

  private _waitForAllowance = async (owner: Address, spender: Address, minAllowance: bigint): Promise<boolean> => {
    const timeoutMs = 90_000
    const intervalMs = 1_500
    const start = Date.now()

    while (Date.now() - start < timeoutMs) {
      const currentAllowance = await this._getAllowance(owner, spender)
      if (currentAllowance !== null && currentAllowance >= minAllowance) {
        return true
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs))
    }

    return false
  }

  private _waitForAllowanceEquals = async (owner: Address, spender: Address, expectedAllowance: bigint): Promise<boolean> => {
    const timeoutMs = 90_000
    const intervalMs = 1_500
    const start = Date.now()

    while (Date.now() - start < timeoutMs) {
      const currentAllowance = await this._getAllowance(owner, spender)
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
    const t = this._beforeTx()
    if(!t) return null

    const callData = encodeFunctionData({
      abi: [transfer],
      functionName: "transfer",
      args: [to, amount]
    })

    return this._execTx(t.wallet, t.signer, callData)
  }

  sendBYUSD = async (amount: bigint, to: Address): Promise<TxState | null> => {
    const t = this._beforeTx()
    if(!t) return null

    const callData = encodeFunctionData({
      abi: [transfer],
      functionName: "transfer",
      args: [to, amount]
    })

    return this._execTx(t.wallet, t.signer, callData, BYUSD_TOKEN)
  }

  getBalance = async (token: Address): Promise<bigint | null> => {
    if(!this.initialized) return null
    if(!this.wallet) return null
    if(!this.publicClient) return null
    if(!this.address) return null


    const statement = {
      address: token,
      abi: [balanceOf],
      functionName: "balanceOf",
      args: [this.address],
    }
    const balance = await this.publicClient.readContract(statement) as bigint

    return balance
  }

  getBalanceOf = async (token: Address, address: Address): Promise<bigint | null> => {
    if(!this.initialized) return null
    if(!this.wallet) return null
    if(!this.publicClient) return null
    if(!this.address) return null


    const statement = {
      address: token,
      abi: [balanceOf],
      functionName: "balanceOf",
      args: [address],
    }
    const balance = await this.publicClient.readContract(statement) as bigint

    return balance
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

    const d = BigInt(10 ** BYUSD_DECIMALS)
    const q = Number(b * 100n / (d)) / 100

    return q
  }
}
