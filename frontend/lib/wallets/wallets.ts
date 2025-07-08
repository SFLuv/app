import { ConnectedWallet, EIP1193Provider } from "@privy-io/react-auth";
import { BrowserProvider, JsonRpcSigner, Signer } from "ethers";
import { toSimpleSmartAccount, ToSimpleSmartAccountReturnType } from "permissionless/accounts";
import { Address, createPublicClient, createWalletClient, custom, encodeFunctionData, Hex, hexToBytes, http, PublicClient } from "viem";
import { CHAIN, DECIMALS, FACTORY, TOKEN } from "../constants";
import { entryPoint07Address } from "viem/account-abstraction";
import { Hash } from "viem";
import { cw_bundler } from "../paymaster/client";
import { balanceOf, depositFor, transfer, withdrawTo } from "../abi";

export type WalletType = "smartwallet" | "eoa"

interface TxState {
  sending: boolean;
  error: string | null;
  hash: string | null;
}


export class AppWallet {
  private owner: ConnectedWallet;
  index?: bigint;

  name: string;
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

  constructor(owner: ConnectedWallet, name: string, index?: bigint) {
    this.owner = owner
    this.index = index
    this.name = name
    this.type = index !== undefined ? "smartwallet" : "eoa"
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
      if(code === "0x" || code == null) {
        return false
      }

      this.wallet = smartWallet
      this.type = "smartwallet"
      this.address = this.wallet.address
      console.log(this.address, code)
    }
    else {
      this.wallet = this.owner
      this.type = "eoa"
      this.address = this.wallet.address as Address
    }

    this.initialized = true

    return true
  }

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
      const hash = await cw_bundler.call(signer, contract || TOKEN, account.address, data)
      receipt.hash = hash
    }
    catch(error) {
      receipt.error = "error sending transaction: check logs"
      console.error(error)
    }

    return receipt
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

  unwrap = async (amount: bigint, to: Address): Promise<TxState | null>  => {
    const t = this._beforeTx()
    if(!t) return null

    const callData = encodeFunctionData({
      abi: [withdrawTo],
      functionName: "withdrawTo",
      args: [to || t.wallet.address, amount]
    })

    return this._execTx(t.wallet, t.signer, callData)
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

  getBalance = async (): Promise<bigint | null> => {
    if(!this.initialized) return null
    const balance = await this.publicClient?.readContract({
      address: TOKEN,
      abi: [balanceOf],
      functionName: "balanceOf",
      args: [this.wallet?.address as Address],
    }) as unknown as bigint

    console.log(balance)


    return balance
  }

  getBalanceFormatted = async (): Promise<number | null> => {
    const b = await this.getBalance()
    if(b === null) return null

    const d = BigInt(10 ** DECIMALS)
    const q = Number(b * 100n / (d)) / 100

    return q
  }
}