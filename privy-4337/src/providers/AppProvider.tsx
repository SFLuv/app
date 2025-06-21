"use client"

import { PrivyProvider, usePrivy, useWallets } from "@privy-io/react-auth"
import { toSimpleSmartAccount } from "permissionless/accounts";
import { createContext, ReactNode, useEffect, useState } from "react";
import { Address, Chain, ChainFormatter, createWalletClient, custom, Hex } from "viem";
import { entryPoint07Address } from "viem/account-abstraction";
import chains from "viem/chains";

interface SmartWalletState {
  address: string;
  owner: string;
  index: bigint;
}

interface UserState {

}

interface AppContextType {
  user: UserState;
  wallet: SmartWalletState;
  ready: boolean;
  authenticated: boolean;
  loading: boolean;
  error: string | null;
  login: () => Promise<void>;
  logout: () => Promise<void>;
  unwrap: (amount: bigint, to: string) => Promise<void>;
  wrap: (amount: bigint) => Promise<void>;
  send: (amount: bigint) => Promise<void>;
}

const defaultSmartWalletState: SmartWalletState = {
  address: "",
  owner: "",
  index: 0n
}

const defaultUserState: UserState = {

}

const AppContext = createContext<AppContextType | null>(null);

export default function AppProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<UserState>(defaultUserState);
  const [wallet, setWallet] = useState<SmartWalletState>(defaultSmartWalletState);
  const [ready, setReady] = useState<boolean>(false);
  const [authenticated, setAuthenticated] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const { getAccessToken, authenticated: privyAuthenticated, ready: privyReady, login: privyLogin, logout: privyLogout } = usePrivy();
  const { wallets } = useWallets();
  const chainName = process.env.NEXT_PUBLIC_CHAIN_NAME as keyof typeof chains
  const chain: Chain = chains[chainName]


  useEffect(() => {
    if(!privyReady) return;

    if(privyAuthenticated) {
      userLogin().finally(() => setReady(true))
    }

  }, [privyReady, privyAuthenticated])

  const userLogin = async () => {
    setLoading(true)
    try {
      await _getUser()
      await _initWallet()
    }
    catch (error) {
      console.error(error)
      await privyLogout()
      setError("error getting user data from backend")
    }

    setLoading(false)
  }

  const _getUser = async () => {

  }

  const _initWallet = async () => {
    const wallet = wallets[0];
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
      index: 1n,
      factoryAddress: process.env.NEXT_PUBLIC_FACTORY_ADDRESS as Address
    })

  }

  const login = async () => {

  }

  const logout = async () => {
    setUser(defaultUserState)
    setWallet(defaultSmartWalletState)
    await privyLogout()
  }

  const wrap = async () => {

  }

  const unwrap = async () => {

  }

  const send = async () => {

  }

  return (
    <AppContext.Provider
      value={{ user, wallet, ready, authenticated, loading, error, login, logout, unwrap, wrap, send}}
    >
      {children}
    </AppContext.Provider>
  )
}