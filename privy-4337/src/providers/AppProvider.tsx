"use client"

import { PrivyProvider, usePrivy, useWallets } from "@privy-io/react-auth"
import { toSimpleSmartAccount, ToSimpleSmartAccountReturnType } from "permissionless/accounts";
import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { Address, createWalletClient, custom, encodeFunctionData, Hash, Hex } from "viem";
import { entryPoint07Address, PaymasterClient } from "viem/account-abstraction";
import { depositFor, transfer, withdrawTo } from "@lib/abi";
import { chain, token } from "@config"
import { client } from "@lib/paymaster"

interface UserState {
}

interface TxState {
  sending: boolean;
  error: string | null;
  hash: string | null;
}

interface AppContextType {
  ready: boolean;
  authenticated: boolean;
  loading: boolean;
  user: UserState;
  wallet: ToSimpleSmartAccountReturnType | null;
  tx: TxState;
  error: string | null;
  login: () => Promise<void>;
  logout: () => Promise<void>;
  unwrap: (amount: bigint, to: Address) => Promise<TxState | null>;
  wrap: (amount: bigint) => Promise<TxState | null>;
  send: (amount: bigint, to: Address) => Promise<TxState | null>;
}

const defaultUserState: UserState = {
}

const defaultTxState: TxState = {
  sending: false,
  error: null,
  hash: null
}

const AppContext = createContext<AppContextType | null>(null);

export default function AppProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState<boolean>(false);
  const [authenticated, setAuthenticated] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);
  const [user, setUser] = useState<UserState>(defaultUserState);
  const [wallet, setWallet] = useState<ToSimpleSmartAccountReturnType | null>(null);
  const [tx, setTx] = useState<TxState>(defaultTxState)
  const [error, setError] = useState<string | null>(null);
  const { getAccessToken, authenticated: privyAuthenticated, ready: privyReady, login: privyLogin, logout: privyLogout } = usePrivy();
  const { wallets } = useWallets();



  useEffect(() => {
    if(!privyReady) return;

    if(privyAuthenticated) {
      _userLogin().then(() => setAuthenticated(true)).finally(() => setReady(true))
    }
    else {
      setLoading(false)
      setReady(true)
    }

  }, [privyReady, privyAuthenticated])

  useEffect(() => {
    if(error) console.error(error)
  }, [error])

  const _userLogin = async () => {
    setLoading(true)
    try {
      await _getUser()
      await _initWallet()
    }
    catch (error) {
      console.error(error)
      await privyLogout()
      setError("error getting user data from backend")
      throw new Error("error logging user in")
    }

    setLoading(false)
  }

  const _resetAppState = async () => {
    setUser(defaultUserState)
    setWallet(null)
    setAuthenticated(false)
    setError(null)
  }

  const _getUser = async () => {

  }

  const _initWallet = async () => {
    try {
      const index = 0n;
      const wallet = wallets[0];
      console.log(wallets)
      await wallet.switchChain(chain.id)
      const provider = await wallet.getEthereumProvider()
      const client = createWalletClient({
        account: wallet.address as Hex,
        chain: chain,
        transport: custom(provider)
      })
      const smartWallet = await toSimpleSmartAccount({
        owner: client,
        client,
        entryPoint: {
          address: entryPoint07Address,
          version: "0.7"
        },
        index,
        factoryAddress: process.env.NEXT_PUBLIC_FACTORY_ADDRESS as Address
      })

      setWallet(smartWallet)
    }
    catch {
      throw new Error("error initializing wallet")
    }
  }

  const _beforeTx = (): ToSimpleSmartAccountReturnType | null => {
    if(!wallet) {
      setError("no wallet initialized")
      return null
    }

    if(tx.sending) {
      setError("tx already in progress")
      return null
    }

    setTx({
      sending: true,
      error: null,
      hash: null
    })

    return wallet
  }

  const _execTx = async (account: ToSimpleSmartAccountReturnType, callData: Hash): Promise<TxState> => {
    let receipt: TxState = {
      sending: false,
      error: null,
      hash: null
    }

    try {
      const userOp = await client.sendUserOperation({
        account,
        calls: [{
          to: token,
          value: 0n,
          data: callData
        }],
        paymaster: true
      })

      receipt.hash = userOp
    }
    catch(error) {
      receipt.error = "error sending transaction: check logs"
      console.error(error)
    }

    setTx(receipt)
    return receipt
  }

  const login = async () => {
    if(!privyReady) {
      setError("privy not ready")
      return
    }

    if(!privyAuthenticated) {
      try {
        await privyLogin()
      }
      catch {
        setError("error logging in with privy")
      }
    }
  }

  const logout = async () => {
    _resetAppState()
    await privyLogout()
  }

  const wrap = async (amount: bigint): Promise<TxState | null>  => {
    const wallet = _beforeTx()
    if(!wallet) return null

    const callData = encodeFunctionData({
      abi: [depositFor],
      functionName: "depositFor",
      args: [wallet.address, amount]
    })

    return _execTx(wallet, callData)
  }

  const unwrap = async (amount: bigint, to: Address): Promise<TxState | null>  => {
    const wallet = _beforeTx()
    if(!wallet) return null

    const callData = encodeFunctionData({
      abi: [withdrawTo],
      functionName: "withdrawTo",
      args: [to || wallet.address, amount]
    })

    return _execTx(wallet, callData)
  }

  const send = async (amount: bigint, to: Address): Promise<TxState | null> => {
    const wallet = _beforeTx()
    if(!wallet) return null

    const callData = encodeFunctionData({
      abi: [transfer],
      functionName: "transfer",
      args: [to, amount]
    })

    return _execTx(wallet, callData)
  }

  return (
      <AppContext.Provider
        value={{ ready, authenticated, loading, user, wallet, tx, error, login, logout, unwrap, wrap, send}}
      >
        {children}
      </AppContext.Provider>
  )
}

export function useApp() {
  const context = useContext(AppContext);
  if (!context) {
    throw new Error("useGame must be used within a GameProvider");
  }
  return context;
}