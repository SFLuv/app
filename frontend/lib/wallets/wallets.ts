import { ConnectedWallet, EIP1193Provider } from "@privy-io/react-auth";
import { BrowserProvider, JsonRpcSigner, Signer } from "ethers";
import { toSimpleSmartAccount, ToSimpleSmartAccountReturnType } from "permissionless/accounts";
import { Address, createPublicClient, createWalletClient, custom, encodeFunctionData, Hex, hexToBytes, http, PublicClient } from "viem";
import { CHAIN, BYUSD_DECIMALS, SFLUV_DECIMALS, FACTORY, SFLUV_TOKEN, BYUSD_TOKEN, ZAPPER_CONTRACT_ADDRESS } from "../constants";
import { entryPoint07Address } from "viem/account-abstraction";
import { Hash } from "viem";
import { cw_bundler } from "../paymaster/client";
import { balanceOf, depositFor, transfer, withdrawTo, unwrapSwapAndBridge } from "../abi";
import { useApp } from "@/context/AppProvider";

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
      const hash = await cw_bundler.call(signer, contract || SFLUV_TOKEN, account.address, data, undefined, undefined, undefined, { smartAccountIndex: this.index ? Number(this.index) : undefined })
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

  unwrapAndBridge = async (amount: number, to: string): Promise<TxState | null> => {
  const sendAmount = amount * (10 ** SFLUV_DECIMALS)
    const t = this._beforeTx()
    if(!t) return null

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
      const hash = await cw_bundler.call(t.signer, ZAPPER_CONTRACT_ADDRESS, t.wallet.address, callDataBytes, undefined, undefined, undefined, { smartAccountIndex: this.index ? Number(this.index) : undefined })
      receipt.hash = hash
    }
    catch(error) {
      receipt.error = "error sending transaction: check logs"
      console.error(error)
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
