"use client"

import { ConnectedWallet, EIP1193Provider, PrivyProvider, useImportWallet, usePrivy, useWallets, Wallet } from "@privy-io/react-auth"
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
import { importWallet as privyImportWallet } from "@/lib/wallets/import";
import { useRouter } from "next/navigation";

// const mockUser: User = { id: "user3", name: "Bob Johnson", email: "bob@example.com", isMerchant: true, isAdmin: false, isOrganizer: false }
export type UserStatus = "loading" | "authenticated" | "unauthenticated"
export type WalletsStatus = "loading" | "available" | "unavailable"
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
  userLocations: Location[]
  login: () => Promise<void>;
  logout: () => Promise<void>;
  authFetch: (endpoint: string, options?: RequestInit) => Promise<Response>;

  // Web3 Functionality
  wallets: AppWallet[];
  walletsStatus: WalletsStatus
  tx: TxState;
  addWallet: (walletName: string) => Promise<void>
  importWallet: (walletName: string, privateKey: string) => Promise<void>
  updateWallet: (id: number, name: string) => Promise<string | null>
  refreshWallets: () => Promise<void>

  // App Functionality
  mapLocations: Location[]
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
  const [walletsStatus, setWalletsStatus] = useState<WalletsStatus>("loading")
  const [mapLocations, setMapLocations] = useState<Location[]>([])
  const [userLocations, setUserLocations] = useState<Location[]>([])
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
  const {
    replace
  } = useRouter()



  useEffect(() => {
    if(!privyReady) return;
    if(!walletsReady) return;

    console.log(privyAuthenticated)
    if(!privyAuthenticated) {
      _resetAppState()
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

      setStatus("authenticated")
    }
    catch(error) {
      await logout()
      setError("error logging in")
      console.error(error)
    }
  }

  const _resetAppState = async () => {
    replace("/")
    setUser(null)
    setWallets([])
    setStatus("unauthenticated")
    setWalletsStatus("unavailable")
    setError(null)
  }

  const authFetch = async (endpoint: string, options: RequestInit = {}): Promise<Response> => {
    const accessToken = await getAccessToken()
    if(!accessToken) throw new Error("no access token")
    const h: HeadersInit = {
      ...options.headers,
      "Access-Token": accessToken,
    }

    return await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + endpoint, { ...options, headers: h })
  }

  const _postUser = async () => {
    let res = await authFetch("/users", { method: "POST" })
    if(res.status != 201) {
      throw new Error("error posting user")
    }
  }

  const _getUser = async (): Promise<GetUserResponse | null> => {
    const res = await authFetch("/users")
    if(res.status == 404) {
      return null
    }
    if(res.status != 200) {
      throw new Error("error getting user")
    }
    return await res.json() as GetUserResponse
  }

  const _getWallets = async (): Promise<WalletResponse[]> => {
    const res = await authFetch("/wallets")
    if(res.status != 200) {
      throw new Error("error getting wallets")
    }
    return await res.json() as WalletResponse[]
  }


  const _postWallet = async (wallet: WalletResponse): Promise<number> => {
    const res = await authFetch("/wallets", {
      method: "POST",
      body: JSON.stringify(wallet)
    })

    if(res.status != 201) {
      throw new Error("error posting wallet")
    }

    return await res.json()
  }

  const _updateWallet = async (w: WalletResponse) => {
    const res = await authFetch("/wallets", {
      method: "PUT",
      body: JSON.stringify(w)
    })

    if(res.status != 201) {
      throw new Error("error updating wallet")
    }
  }

  const _initWallets = async (extWallets?: WalletResponse[]) => {
    setWalletsStatus("loading")
    try {
      if(!privyUser?.id) {
        throw new Error("user not authenticated")
      }
      if(extWallets === undefined) {
        extWallets = await _getWallets()
      }

      let wResults: Promise<AppWallet>[] = []
      let cResults: Promise<void>[] = []
      for(let i = 0; i < privyWallets.length; i++) {
        const privyWallet = privyWallets[i]

        cResults.push(privyWallet.switchChain(CHAIN_ID));

        let extWallet = extWallets.find((w) => w.eoa_address == privyWallet.address && w.is_eoa === true)
        wResults.push(_initEOAWallet(privyWallet, extWallet, i))

        let smartWallets = extWallets.filter((w) => w.eoa_address == privyWallet.address && w.is_eoa === false && w.smart_index !== undefined)
        console.log(smartWallets)
        console.log(smartWallets.length)
        if(smartWallets.length === 0) {
          smartWallets.push({
            id: null,
            owner: privyUser.id,
            name: "",
            is_eoa: false,
            eoa_address: privyWallet.address,
            smart_index: 0
          })
        }
        console.log(smartWallets)

        for(let index = 0n; index < BigInt(smartWallets.length); index++) {
          let extSmartWallet = smartWallets.find((w) => {
            if(w.smart_index === undefined) w.smart_index = 10000
            if(w.smart_index === null) w.smart_index = 10000
            return w.eoa_address == privyWallet.address && w.is_eoa === false && BigInt(w.smart_index) === index
          })
          console.log("w", extSmartWallet)
          if(!extSmartWallet) continue

          wResults.push(_initSmartWallet(privyWallet, extSmartWallet, index, i))
        }
      }
      await Promise.all(cResults)
      let wlts = await Promise.all(wResults)

      setWallets(wlts)
      setWalletsStatus("available")
      console.log(wlts)
    }
    catch(error) {
      console.error(error)
      setWalletsStatus("unavailable")
      throw new Error("error initializing wallets")
    }
  }

  const _initEOAWallet = async (privyWallet: ConnectedWallet, wallet: WalletResponse | undefined, i: number): Promise<AppWallet> => {
    if(!privyUser) throw new Error("user not logged in")

    if(!wallet) {
      wallet = {
        id: null,
        owner: privyUser.id,
        name: "EOA-" + (i + 1),
        is_eoa: true,
        eoa_address: privyWallet.address
      }

      let id = await _postWallet(wallet)
      wallet.id = id
    }

    const eoaName = wallet.name
    const w = new AppWallet(privyWallet, eoaName, { id: wallet.id || undefined })
    await w.init()
    return w
  }

  const _initSmartWallet = async (privyWallet: ConnectedWallet, wallet: WalletResponse, index: bigint, i: number): Promise<AppWallet> => {
    if(wallet.is_eoa) throw new Error("trying to initialize smart wallet with eoa")
    const smartWalletName = wallet?.name || "SW-" + (i+1) + "-" + (index + 1n).toString()

    const w = new AppWallet(privyWallet, smartWalletName, {index, id: wallet.id || undefined})
    await w.init()

    if(wallet.id === null) {
      wallet.smart_address = w.address
      wallet.smart_index = Number(index)
      let id = await _postWallet(wallet)
      w.setId(id)
    }

    return w
  }

  const addWallet = async (walletName: string) => {
    if(!privyUser) throw new Error("no user logged in")
    const privyWallet = privyWallets[0]
    const n = wallets.filter((w) => w.owner.address === privyWallet.address && w.type === "smartwallet").length

    console.log(n)

    const wallet: WalletResponse = {
      id: null,
      owner: privyUser.id,
      name: walletName,
      is_eoa: false,
      eoa_address: privyWallet.address,
    }

    const w = await _initSmartWallet(privyWallet, wallet, BigInt(n), 1)
    setWallets([...wallets, w])
  }

  const importWallet = async (walletName: string, privateKey: string) => {
    if(!privyUser) {
      setError("no user authenticated")
      return
    }
    let s = walletsStatus
    setWalletsStatus("loading")

    let w: WalletResponse
    try {
      const accessToken = await getAccessToken()
      if(!accessToken) throw new Error("no access token available")
      const address = await privyImportWallet(privateKey, accessToken)
      w = {
        id: 0,
        owner: privyUser.id,
        name: walletName,
        is_eoa: true,
        eoa_address: address
      }
    }
    catch(error) {
      setWalletsStatus(s)
      throw error
    }
    try {
        await _postWallet(w)
        await _initWallets()
        setWalletsStatus(s)
    }
    catch(error) {
      setWalletsStatus(s)
      console.error(error)
      setError("error updating wallets after import")
    }
  }

  const updateWallet = async (id: number, name: string): Promise<string | null> => {
    const s = walletsStatus
    let n: string | null = null
    setWalletsStatus("loading")
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
      throw new Error("error updating wallet")
    }
    setWalletsStatus(s)
    return n
  }

  const refreshWallets = async () => {
    const s = walletsStatus
    setWalletsStatus("loading")
    try {
      await _initWallets()
    }
    catch {
      setError("error updating wallets")
    }
    setWalletsStatus(s)
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
          walletsStatus,
          userLocations,
          tx,
          addWallet,
          importWallet,
          updateWallet,
          refreshWallets,
          error,
          login,
          logout,
          authFetch,
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
