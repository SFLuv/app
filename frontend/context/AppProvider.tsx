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
import { UserResponse, GetUserResponse, WalletResponse, LocationResponse } from "@/types/server";

// const mockUser: User = { id: "user3", name: "Bob Johnson", email: "bob@example.com", isMerchant: true, isAdmin: false, isOrganizer: false }
export type UserStatus = "loading" | "authenticated" | "unauthenticated"
export type MerchantApprovalStatus = "pending" | "approved" | "rejected" | null


export interface User {
  id: string
  name: string
  contact_email?: string
  contact_phone?: string
  // avatar?: string
  isAdmin: boolean
  isMerchant: boolean
  isOrganizer: boolean
  // merchantStatus?: MerchantApprovalStatus
  // merchantProfile?: {
  //   businessName: string
  //   description: string
  //   address: {
  //     street: string
  //     city: string
  //     state: string
  //     zip: string
  //   }
  //   contactInfo: {
  //     phone: string
  //     website?: string
  //   }
  //   businessType: string
  // }
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
  userLocations: LocationResponse[]
  login: () => Promise<void>;
  logout: () => Promise<void>;

  // Web3 Functionality
  wallets: AppWallet[];
  tx: TxState;
  updateWallet: (id: number, name: string) => Promise<string | null>
  refreshWallets: () => Promise<void>

  // App Functionality
  mapLocations: LocationResponse[]
  updateUser: (data: Partial<User>) => void
  approveMerchantStatus: () => void
  rejectMerchantStatus: () => void

  //add location fuction signatures
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
  const [mapLocations, setMapLocations] = useState<LocationResponse[]>([])
  const [userLocations, setUserLocations] = useState<LocationResponse[]>([])
  const [status, setStatus] = useState<UserStatus>("loading")
  const [tx, setTx] = useState<TxState>(defaultTxState)
  const [error, setError] = useState<string | null>(null);
  const {
      getAccessToken,
      authenticated: privyAuthenticated,
      ready: privyReady,
      login: privyLogin,
      logout: privyLogout,
      user: privyUser
  } = usePrivy();
  const {
    wallets: privyWallets,
    ready: walletsReady
  } = useWallets();



  useEffect(() => {
    if(!privyReady) return;
    console.log("ready")
    if(!walletsReady) return;
    console.log("wready")

    if(!privyAuthenticated) {
      setStatus("unauthenticated")
      return
    }

    _userLogin()

  }, [privyReady, privyAuthenticated, walletsReady])

  useEffect(() => {
    if(error) console.error(error)
  }, [error])

  const _userResponseToUser = async (r: GetUserResponse) => {
      const u: User = {
        id: r.user.id,
        name: r.user.contact_name || "User",
        contact_email: r.user.contact_email,
        contact_phone: r.user.contact_phone,
        isAdmin: r.user.is_admin,
        isMerchant: r.user.is_merchant,
        isOrganizer: r.user.is_organizer
      }
      setUser(u)
  }

  const _userLogin = async () => {
    let userResponse: GetUserResponse | null
    setStatus("loading")
    try {
      userResponse = await _getUser()
      if(userResponse === null) {
        await _postUser()
        userResponse = await _getUser()
      }
      if(userResponse === null) {
        throw new Error("error posting user")
      }
      await _userResponseToUser(userResponse)
      await _initWallets(userResponse.wallets)
    }
    catch(error) {
      await logout()
      setError("error logging in")
      console.error(error)
    }
    setStatus("authenticated")
  }

  const _resetAppState = async () => {
    setUser(null)
    setWallets([])
    setStatus("unauthenticated")
    setError(null)
  }

  const _authFetch = async (endpoint: string, options: RequestInit = {}): Promise<Response> => {
    const accessToken = await getAccessToken()
    if(!accessToken) throw new Error("no access token")
    const h: HeadersInit = {
      ...options.headers,
      "Access-Token": accessToken,
    }

    return await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + endpoint, { ...options, headers: h })
  }

  const _postUser = async () => {
    let res = await _authFetch("/users", { method: "POST" })
    if(res.status != 201) {
      throw new Error("error posting user")
    }
  }

  const _getUser = async (): Promise<GetUserResponse | null> => {
    const res = await _authFetch("/users")
    if(res.status == 404) {
      return null
    }
    if(res.status != 200) {
      throw new Error("error getting user")
    }
    return await res.json() as GetUserResponse
  }

  const _getWallets = async (): Promise<WalletResponse[]> => {
    const res = await _authFetch("/wallets")
    if(res.status != 200) {
      throw new Error("error getting wallets")
    }
    return await res.json() as WalletResponse[]
  }

  const _getMapLocations = async (): Promise<LocationResponse[]> => {
    const res = await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + "/locations")
    return await res.json() as LocationResponse[]
  }

  const _postWallet = async (wallet: WalletResponse): Promise<number> => {
    const res = await _authFetch("/wallets", {
      method: "POST",
      body: JSON.stringify(wallet)
    })

    if(res.status != 201) {
      throw new Error("error posting wallet")
    }

    return await res.json()
  }

  const _updateWallet = async (w: WalletResponse) => {
    const res = await _authFetch("/wallets", {
      method: "PUT",
      body: JSON.stringify(w)
    })

    if(res.status != 201) {
      throw new Error("error updating wallet")
    }
  }

  const _initWallets = async (extWallets?: WalletResponse[]) => {
    try {
      if(!privyUser?.id) {
        throw new Error("user not authenticated")
      }
      if(extWallets === undefined) {
        extWallets = await _getWallets()
      }

      let wlts: AppWallet[] = [];
      for(const i in privyWallets) {
        const privyWallet = privyWallets[i]

        let extWallet = extWallets.find((w, n) => w.eoa_address == privyWallet.address && w.is_eoa)
        if(!extWallet) {
          extWallet = {
            id: null,
            owner: privyUser.id,
            name: "EOA-" + (i + 1),
            is_eoa: true,
            eoa_address: privyWallet.address
          }

          let id = await _postWallet(extWallet)
          extWallet.id = id
        }

        const eoaName = extWallet.name
        const w = new AppWallet(privyWallet, eoaName, { id: extWallet.id || undefined })
        await w.init()
        wlts.push(w)

        await privyWallet.switchChain(CHAIN_ID);
        let next = true;
        let index = 0n;
        while(next) {
          let extSmartWallet = extWallets.find((w, n) => w.eoa_address == privyWallet.address && w.smart_index != undefined && BigInt(w.smart_index) == index)

          const prefix = extWallet.name.startsWith("EOA") ? "SW-" + (i + 1) : extWallet.name + "-SW"
          const smartWalletName = extSmartWallet?.name || prefix + "-" + (index + 1n).toString()
          const w = new AppWallet(privyWallet, smartWalletName, {index})
          next = await w.init()
          if(next){
            wlts.push(w)
            if(!extSmartWallet) {
              extSmartWallet = {
                id: null,
                owner: privyUser.id,
                name: smartWalletName,
                is_eoa: false,
                eoa_address: privyWallet.address,
                smart_address: w.address,
                smart_index: Number(w.index)
              }

              let id = await _postWallet(extSmartWallet)
              w.setId(id)
            }
            index += 1n
          }
        }
      }

      setWallets(wlts)
      console.log(wlts)
    }
    catch(error) {
      console.error(error)
      throw new Error("error initializing wallets")
    }
  }

  const updateWallet = async (id: number, name: string): Promise<string | null> => {
    const s = status
    let n: string | null = null
    setStatus("loading")
    try {
      if(!user) {
        throw new Error("no user logged in")
      }
      var sWallet: WalletResponse = {
        id: id,
        owner: user.id,
        name: name,
        is_eoa: true,
        eoa_address: "0x"
      }

      await _updateWallet(sWallet)
      n = name
      await refreshWallets()
    }
    catch {
      console.error("error updating wallets")
      setError("error updating wallets")
    }
    setStatus(s)
    return n
  }

  const refreshWallets = async () => {
    const s = status
    setStatus("loading")
    try {
      await _initWallets()
    }
    catch {
      setError("error updating wallets")
    }
    setStatus(s)
  }

  const login = async () => {
    console.log("login", privyReady, privyAuthenticated)
    if(!privyReady) {
      setError("privy not ready")
      return
    }

    if(!privyAuthenticated) {
      try {
        await privyLogin()
        // move user data implementation to helper functions called in useEffect instead of passing into login() for real auth
        // localStorage.setItem("sfluv_user", JSON.stringify(mockUser))
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

  const approveMerchantStatus = () => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "approved" as MerchantApprovalStatus,
        role: "merchant",
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
          userLocations,
          tx,
          updateWallet,
          refreshWallets,
          error,
          login,
          logout,
          mapLocations,
          updateUser,
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