"use client"

import { ConnectedWallet, EIP1193Provider, PrivyProvider, usePrivy, useWallets } from "@privy-io/react-auth"
import { toSimpleSmartAccount, ToSimpleSmartAccountReturnType } from "permissionless/accounts";
import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { Address, createWalletClient, custom, encodeFunctionData, Hash, Hex, hexToBytes, RpcUserOperation } from "viem";
import { entryPoint07Address, entryPoint08Address, formatUserOperation, PaymasterClient, toPackedUserOperation, ToSmartAccountReturnType, UserOperation } from "viem/account-abstraction";
import { depositFor, execute, transfer, withdrawTo } from "@/lib/abi";
import { client } from "@/lib/paymaster"
import { CHAIN, CHAIN_ID, COMMUNITY, COMMUNITY_ACCOUNT, FACTORY, PAYMASTER, TOKEN } from "@/lib/constants";
import { bundler, cw_bundler } from "@/lib/paymaster/client";
import config from "@/app.config";
import { UserOp } from "@citizenwallet/sdk";
import { JsonRpcSigner, Signer } from "ethers";
import { BrowserProvider } from "ethers";
import { AppWallet } from "@/lib/wallets/wallets";

const mockUser: User = { id: "user3", name: "Bob Johnson", email: "bob@example.com", role: "merchant", isOrganizer: true }
export type UserRole = "user" | "merchant" | "admin" | null
export type UserStatus = "loading" | "authenticated" | "unauthenticated"
export type MerchantApprovalStatus = "pending" | "approved" | "rejected" | null


export interface User {
  id: string
  name: string
  email: string
  role: UserRole
  avatar?: string
  isOrganizer?: boolean
  merchantStatus?: MerchantApprovalStatus
  merchantProfile?: {
    businessName: string
    description: string
    address: {
      street: string
      city: string
      state: string
      zip: string
    }
    contactInfo: {
      phone: string
      website?: string
    }
    businessType: string
  }
}

interface TxState {
  sending: boolean;
  error: string | null;
  hash: string | null;
}

interface AppContextType {
  error: string | null;

  // Authentication
  status: UserStatus;
  user: User | null;
  login: () => Promise<void>;
  logout: () => Promise<void>;

  // Web3 Functionality
  wallets: AppWallet[];
  tx: TxState;

  // App Functionality
  updateUser: (data: Partial<User>) => void
  requestMerchantStatus: (merchantProfile: User["merchantProfile"]) => void
  approveMerchantStatus: () => void
  rejectMerchantStatus: () => void
}


const defaultTxState: TxState = {
  sending: false,
  error: null,
  hash: null
}

const AppContext = createContext<AppContextType | null>(null);

export default function AppProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [wallets, setWallets] = useState<AppWallet[]>([]);
  const [status, setStatus] = useState<UserStatus>("loading")
  const [tx, setTx] = useState<TxState>(defaultTxState)
  const [error, setError] = useState<string | null>(null);
  const { getAccessToken, authenticated: privyAuthenticated, ready: privyReady, login: privyLogin, logout: privyLogout } = usePrivy();
  const { wallets: privyWallets, ready: walletsReady } = useWallets();



  useEffect(() => {
    if(!privyReady) return;
    console.log("wallet: ", walletsReady)

    if(privyAuthenticated) {
      if(!walletsReady) return;
      _userLogin().then(() => setStatus("authenticated"))
    }
    else {
      setStatus("unauthenticated")
    }

  }, [privyReady, privyAuthenticated, walletsReady])

  useEffect(() => {
    if(error) console.error(error)
  }, [error])

  const _userLogin = async () => {
    setStatus("loading")
    try {
      await _getUser()
    }
    catch(error) {
      await privyLogout()
      setError("error getting user data from backend")
      console.error(error)
      throw new Error("error logging user in")
    }
    try {
      await _initWallets()
    }
    catch(error) {
      await privyLogout()
      setError("error initializing wallet")
      console.error(error)
      throw new Error("error logging user in")
    }

    setStatus("loading")
  }

  const _resetAppState = async () => {
    setUser(null)
    setWallets([])
    setStatus("unauthenticated")
    setError(null)
  }

  const _getUser = async () => {
    // In a real app, this would be an API call to verify the session
    const storedUser = localStorage.getItem("sfluv_user")

    if (storedUser) {
      const userData = JSON.parse(storedUser)
      setUser(userData)
      setStatus("authenticated")
    } else {
      logout()
    }
  }

  const _initWallets = async () => {
    try {
      let wlts: AppWallet[] = [];
      let index = 0n;
      for(const i in privyWallets) {
        const privyWallet = privyWallets[i]
        const eoaName = "EOA-" + (i + 1)
        const w = new AppWallet(privyWallet, eoaName)
        await w.init()
        wlts.push(w)

        await privyWallet.switchChain(CHAIN_ID);
        let next = true;
        while(next) {
          const smartWalletName = "SW-" + (i + 1) + "-" + (index + 1n).toString()
          const w = new AppWallet(privyWallet, smartWalletName, index)
          next = await w.init()
          if(next) wlts.push(w)
          index += 1n
        }
        index = 0n;
      }

      setWallets(wlts)
      console.log(wlts)
    }
    catch(error) {
      console.error(error)
      throw new Error("error initializing wallets")
    }
  }

  const login = async () => {
    if(!privyReady) {
      setError("privy not ready")
      return
    }

    if(!privyAuthenticated) {
      try {
        await privyLogin()
        // move user data implementation to helper functions called in useEffect instead of passing into login() for real auth
        localStorage.setItem("sfluv_user", JSON.stringify(mockUser))
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

  const updateUser = (data: Partial<User>) => {
    if (user) {
      const updatedUser = { ...user, ...data }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  const requestMerchantStatus = (merchantProfile: User["merchantProfile"]) => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "pending" as MerchantApprovalStatus,
        merchantProfile,
      }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  const approveMerchantStatus = () => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "approved" as MerchantApprovalStatus,
        role: "merchant" as UserRole,
      }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  const rejectMerchantStatus = () => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "rejected" as MerchantApprovalStatus,
      }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  return (
      <AppContext.Provider
        value={{
          status,
          user,
          wallets,
          tx,
          error,
          login,
          logout,
          updateUser,
          requestMerchantStatus,
          approveMerchantStatus,
          rejectMerchantStatus,
         }}
      >
        {children}
      </AppContext.Provider>
  )
}

export function useApp() {
  const context = useContext(AppContext);
  if (!context) {
    throw new Error("useApp must be used within an AppProvider");
  }
  return context;
}