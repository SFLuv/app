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

interface AppWalletOptions {
  index?: bigint
  id?: number
}


export class AppWallet {
  owner: ConnectedWallet;
  index?: bigint;

  name: string;
  id?: number;
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
      const hash = await cw_bundler.call(signer, contract || TOKEN, account.address, data, undefined, undefined, undefined, { smartAccountIndex: this.index ? Number(this.index) : undefined })
      receipt.hash = hash
    }
    catch(error) {
      receipt.error = "error sending transaction: check logs"
      console.error(error)
    }

    return receipt
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
    if(!this.wallet) return null
    if(!this.publicClient) return null
    if(!this.address) return null

    const statement = {
      address: TOKEN,
      abi: [balanceOf],
      functionName: "balanceOf",
      args: [this.address],
    }
    console.log(statement)
    const balance = await this.publicClient.readContract(statement) as bigint
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