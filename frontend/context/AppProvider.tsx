"use client";

import {
  ConnectedWallet,
  EIP1193Provider,
  PrivyProvider,
  useImportWallet,
  usePrivy,
  useWallets,
  Wallet,
} from "@privy-io/react-auth";
import {
  toSimpleSmartAccount,
  ToSimpleSmartAccountReturnType,
} from "permissionless/accounts";
import {
  createContext,
  Dispatch,
  ReactNode,
  SetStateAction,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  Address,
  createWalletClient,
  custom,
  encodeFunctionData,
  Hash,
  Hex,
  hexToBytes,
  RpcUserOperation,
} from "viem";
import {
  entryPoint07Address,
  entryPoint08Address,
  formatUserOperation,
  PaymasterClient,
  toPackedUserOperation,
  ToSmartAccountReturnType,
  UserOperation,
} from "viem/account-abstraction";
import { depositFor, execute, transfer, withdrawTo } from "@/lib/abi";
import { client } from "@/lib/paymaster";
import {
  BACKEND,
  CHAIN,
  CHAIN_ID,
  COMMUNITY,
  COMMUNITY_ACCOUNT,
  FACTORY,
  IDLE_TIMER_PROMPT_SECONDS,
  IDLE_TIMER_SECONDS,
  PAYMASTER,
} from "@/lib/constants";
import { bundler, cw_bundler } from "@/lib/paymaster/client";
import config from "@/app.config";
import { UserOp } from "@citizenwallet/sdk";
import { JsonRpcSigner, Signer } from "ethers";
import { BrowserProvider } from "ethers";
import { AppWallet } from "@/lib/wallets/wallets";
import { Affiliate } from "@/types/affiliate";
import { Proposer } from "@/types/proposer";
import { Improver } from "@/types/improver";
import { IssuerRecord } from "@/types/issuer";
import { Supervisor } from "@/types/supervisor";
import { UserResponse, GetUserResponse, WalletResponse } from "@/types/server";
import { AuthedLocation } from "@/types/location";
import { importWallet as privyImportWallet } from "@/lib/wallets/import";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Contact } from "@/types/contact";
import { useIdleTimer } from "react-idle-timer";
import { IdleModal } from "@/components/idle/idle-modal";
import { PonderSubscription, PonderSubscriptionRequest } from "@/types/ponder";
import { base64 } from "@scure/base";

// const mockUser: User = { id: "user3", name: "Bob Johnson", email: "bob@example.com", isMerchant: true, isAdmin: false, isOrganizer: false }
export type UserStatus = "loading" | "authenticated" | "unauthenticated";
export type WalletsStatus = "loading" | "available" | "unavailable";
export type MerchantApprovalStatus = "pending" | "approved" | "rejected" | null;

export interface User {
  id: string;
  name: string;
  contact_email?: string;
  contact_phone?: string;
  isAdmin: boolean;
  isMerchant: boolean;
  isOrganizer: boolean;
  isImprover: boolean;
  isProposer: boolean;
  isVoter: boolean;
  isIssuer: boolean;
  isSupervisor: boolean;
  primaryWalletAddress: string;
  paypalEthAddress: string;
  lastRedemption: number;
  isAffiliate: boolean;
}

interface TxState {
  sending: boolean;
  error: string | null;
  hash: string | null;
}

interface AppContextType {
  error: string | unknown | null;
  setError: Dispatch<unknown>;

  // Authentication
  status: UserStatus;
  user: User | null;
  affiliate: Affiliate | null;
  setAffiliate: Dispatch<SetStateAction<Affiliate | null>>;
  proposer: Proposer | null;
  setProposer: Dispatch<SetStateAction<Proposer | null>>;
  improver: Improver | null;
  setImprover: Dispatch<SetStateAction<Improver | null>>;
  issuer: IssuerRecord | null;
  setIssuer: Dispatch<SetStateAction<IssuerRecord | null>>;
  supervisor: Supervisor | null;
  setSupervisor: Dispatch<SetStateAction<Supervisor | null>>;
  userLocations: AuthedLocation[];
  setUserLocations: Dispatch<SetStateAction<AuthedLocation[]>>;
  login: () => Promise<void>;
  logout: () => Promise<void>;
  authFetch: (endpoint: string, options?: RequestInit) => Promise<Response>;

  // Web3 Functionality
  wallets: AppWallet[];
  walletsStatus: WalletsStatus;
  tx: TxState;
  addWallet: (walletName: string) => Promise<void>;
  importWallet: (walletName: string, privateKey: string) => Promise<void>;
  updateWallet: (id: number, name: string) => Promise<string | null>;
  refreshWallets: () => Promise<void>;
  ensurePrimarySmartWallet: () => Promise<boolean>;

  // App Functionality
  mapLocations: Location[];
  updateUser: (data: Partial<User>) => void;
  approveMerchantStatus: () => void;
  rejectMerchantStatus: () => void;

  //cashout functionality
  updatePayPalAddress: (payPalAddress: string) => Promise<void>;

  //add location fuction signatures
  // Ponder Functionality
  ponderSubscriptions: PonderSubscription[];
  addPonderSubscription: (email: string, address: string) => Promise<void>;
  getPonderSubscriptions: () => Promise<void>;
  deletePonderSubscription: (id: number) => Promise<void>;
}

const defaultTxState: TxState = {
  sending: false,
  error: null,
  hash: null,
};

const AppContext = createContext<AppContextType | null>(null);
const AppStatusContext = createContext<UserStatus>("loading");

export default function AppProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [affiliate, setAffiliate] = useState<Affiliate | null>(null);
  const [proposer, setProposer] = useState<Proposer | null>(null);
  const [improver, setImprover] = useState<Improver | null>(null);
  const [issuer, setIssuer] = useState<IssuerRecord | null>(null);
  const [supervisor, setSupervisor] = useState<Supervisor | null>(null);
  const [wallets, setWallets] = useState<AppWallet[]>([]);
  const [walletsStatus, setWalletsStatus] = useState<WalletsStatus>("loading");
  const [mapLocations, setMapLocations] = useState<Location[]>([]);
  const [userLocations, setUserLocations] = useState<AuthedLocation[]>([]);
  const [status, setStatus] = useState<UserStatus>("loading");
  const [tx, setTx] = useState<TxState>(defaultTxState);
  const [error, setError] = useState<string | unknown | null>(null);
  const [idleModalOpen, setIdleModalOpen] = useState<boolean>(false);
  const [ponderSubscriptions, setPonderSubscriptions] = useState<
    PonderSubscription[]
  >([]);
  const [idleTimer, setIdleTimer] = useState<NodeJS.Timeout | undefined>();
  const {
    getAccessToken,
    authenticated: privyAuthenticated,
    ready: privyReady,
    login: privyLogin,
    logout: privyLogout,
    user: privyUser,
  } = usePrivy();
  const { wallets: privyWallets, ready: walletsReady } = useWallets();
  const { replace } = useRouter();
  const pathname = usePathname();

  const linkedWalletAddresses = useMemo(() => {
    const linked = new Set<string>();
    for (const account of privyUser?.linkedAccounts ?? []) {
      if (account.type !== "wallet") continue;
      if (!("address" in account) || typeof account.address !== "string")
        continue;
      linked.add(account.address.toLowerCase());
    }
    return linked;
  }, [privyUser]);

  const getManagedPrivyWallets = (): ConnectedWallet[] => {
    const walletsByAddress = new Map<string, ConnectedWallet>();
    for (const wallet of privyWallets) {
      const address = wallet.address?.toLowerCase();
      if (!address) continue;
      const walletClientType = (
        (wallet as unknown as { walletClientType?: string }).walletClientType ||
        ""
      ).toLowerCase();
      const connectorType = (
        (wallet as unknown as { connectorType?: string }).connectorType || ""
      ).toLowerCase();
      const isLinkedWallet = linkedWalletAddresses.has(address);
      const isEmbeddedWallet =
        walletClientType === "privy" ||
        walletClientType === "privy-v2" ||
        connectorType === "embedded" ||
        connectorType === "embedded_imported";

      if (!isLinkedWallet && !isEmbeddedWallet) continue;
      if (!walletsByAddress.has(address)) {
        walletsByAddress.set(address, wallet);
      }
    }
    return Array.from(walletsByAddress.values());
  };

  const onIdle = () => {
    if (status === "authenticated") {
      logout();
    }
  };
  const onPrompt = () => {
    if (status === "authenticated") {
      setIdleModalOpen(true);
    }
  };
  const {
    getRemainingTime,
    start: startIdleTimer,
    pause: pauseIdleTimer,
    reset: resetIdleTimer,
  } = useIdleTimer({
    onIdle,
    onPrompt,
    promptBeforeIdle: IDLE_TIMER_PROMPT_SECONDS * 1000,
    timeout: IDLE_TIMER_SECONDS * 1000,
    throttle: 500,
    startManually: true,
  });

  const toggleIdleModal = () => {
    setIdleModalOpen(!idleModalOpen);
    startIdleTimer();
  };

  useEffect(() => {
    if (!privyReady) return;
    if (!walletsReady) return;

    if (!privyAuthenticated) {
      _resetAppState();
      return;
    }

    _userLogin();
  }, [privyReady, privyAuthenticated, walletsReady, privyUser, pathname]);

  useEffect(() => {
    if (error) console.error(error);
  }, [error]);

  useEffect(() => {
    setError(null);
  }, [pathname]);

  useEffect(() => {
    if (status === "authenticated") {
      resetIdleTimer();
      startIdleTimer();
    } else {
      pauseIdleTimer();
    }
  }, [status]);

  const _userResponseToUser = async (r: GetUserResponse) => {
    const u: User = {
      id: r.user.id,
      name: r.user.contact_name || "User",
      contact_email: r.user.contact_email,
      contact_phone: r.user.contact_phone,
      isAdmin: r.user.is_admin,
      isMerchant: r.user.is_merchant,
      isOrganizer: r.user.is_organizer,
      isImprover: r.user.is_improver,
      isProposer: r.user.is_proposer,
      isVoter: r.user.is_voter,
      isIssuer: r.user.is_issuer,
      isSupervisor: r.user.is_supervisor,
      primaryWalletAddress: r.user.primary_wallet_address,
      paypalEthAddress: r.user.paypal_eth,
      lastRedemption: r.user.last_redemption,
      isAffiliate: r.user.is_affiliate,
    };
    setUser(u);
    setAffiliate(r.affiliate ?? null);
    setProposer(r.proposer ?? null);
    setImprover(r.improver ?? null);
    setIssuer(r.issuer ?? null);
    setSupervisor(r.supervisor ?? null);
  };

  const _userLogin = async () => {
    if (status === "authenticated") return;

    let userResponse: GetUserResponse | null;

    setStatus("loading");

    try {
      userResponse = await _getUser();
      if (userResponse === null) {
        await _postUser();
        userResponse = await _getUser();
      }
      if (userResponse === null) {
        throw new Error("error posting user");
      }

      await _initWallets(userResponse.wallets);
      const latestWallets = await _getWallets();
      try {
        const defaultPrimaryWallet = await _ensureDefaultPrimaryWallet(
          userResponse.user,
          latestWallets,
        );
        if (defaultPrimaryWallet) {
          userResponse.user.primary_wallet_address = defaultPrimaryWallet;
        }
      } catch (error) {
        console.error("error ensuring default primary wallet", error);
      }
      userResponse.wallets = latestWallets;
      await _userResponseToUser(userResponse);
      await getPonderSubscriptions();
      setUserLocations(userResponse.locations);

      setStatus("authenticated");
    } catch (error) {
      setError(error);
      console.error(error);
      await logout();
    }
  };

  const _resetAppState = async () => {
    const allowUnauthedRoute =
      pathname === "/map" ||
      pathname === "/redirect" ||
      pathname.startsWith("/faucet") ||
      pathname.startsWith("/improver/join") ||
      pathname.startsWith("/photos/") ||
      pathname.startsWith("/photo/");
    if (!allowUnauthedRoute) {
      replace("/map");
    }
    setUser(null);
    setAffiliate(null);
    setProposer(null);
    setImprover(null);
    setIssuer(null);
    setSupervisor(null);
    setStatus("unauthenticated");
    setWallets([]);
    setWalletsStatus("unavailable");
    setError(null);
    setUserLocations([]);
  };

  const authFetch = async (
    endpoint: string,
    options: RequestInit = {},
  ): Promise<Response> => {
    const accessToken = await getAccessToken();
    if (!accessToken) throw new Error("no access token");
    const h: HeadersInit = {
      ...options.headers,
      "Access-Token": accessToken,
    };

    return await fetch(BACKEND + endpoint, { ...options, headers: h });
  };

  const _postUser = async () => {
    let res = await authFetch("/users", { method: "POST" });
    if (res.status != 201) {
      throw new Error("error posting user");
    }
  };

  const _getUser = async (): Promise<GetUserResponse | null> => {
    const res = await authFetch("/users");
    if (res.status == 404) {
      return null;
    }
    if (res.status != 200) {
      throw new Error("error getting user");
    }
    const json = await res.json();
    return json as GetUserResponse;
  };

  const _getWallets = async (): Promise<WalletResponse[]> => {
    const res = await authFetch("/wallets");
    if (res.status != 200) {
      throw new Error("error getting wallets");
    }
    return (await res.json()) as WalletResponse[];
  };

  useEffect(() => {
    if (status !== "authenticated" || !privyAuthenticated) {
      return;
    }

    let cancelled = false;
    let inFlight = false;

    const refreshAuthenticatedUserRecord = async () => {
      if (cancelled || inFlight) {
        return;
      }

      inFlight = true;
      try {
        const response = await _getUser();
        if (cancelled || response === null) {
          return;
        }
        await _userResponseToUser(response);
        if (!cancelled) {
          setUserLocations(response.locations);
        }
      } catch (error) {
        console.error("error refreshing authenticated user record", error);
      } finally {
        inFlight = false;
      }
    };

    const handleWindowFocus = () => {
      void refreshAuthenticatedUserRecord();
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        void refreshAuthenticatedUserRecord();
      }
    };

    const refreshInterval = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        void refreshAuthenticatedUserRecord();
      }
    }, 15000);

    window.addEventListener("focus", handleWindowFocus);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    void refreshAuthenticatedUserRecord();

    return () => {
      cancelled = true;
      window.clearInterval(refreshInterval);
      window.removeEventListener("focus", handleWindowFocus);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [status, privyAuthenticated]);

  const _ensureDefaultPrimaryWallet = async (
    currentUser: GetUserResponse["user"],
    walletList: WalletResponse[],
  ): Promise<string> => {
    const existingPrimaryWallet = (
      currentUser.primary_wallet_address || ""
    ).trim();
    if (existingPrimaryWallet) {
      return existingPrimaryWallet;
    }

    const managedPrivyWallets = getManagedPrivyWallets();
    const primaryPrivyWallet = managedPrivyWallets[0];
    const preferredSmartWallet = primaryPrivyWallet?.address
      ? walletList.find(
          (wallet) =>
            wallet.is_eoa === false &&
            wallet.smart_index === 0 &&
            wallet.eoa_address?.toLowerCase() ===
              primaryPrivyWallet.address.toLowerCase() &&
            typeof wallet.smart_address === "string" &&
            wallet.smart_address.trim() !== "",
        )
      : undefined;

    const fallbackSmartWallet = walletList.find(
      (wallet) =>
        wallet.is_eoa === false &&
        wallet.smart_index === 0 &&
        typeof wallet.smart_address === "string" &&
        wallet.smart_address.trim() !== "",
    );

    const rawDefaultAddress = (
      preferredSmartWallet?.smart_address ||
      fallbackSmartWallet?.smart_address ||
      ""
    ).trim();
    if (!rawDefaultAddress) {
      return "";
    }

    const res = await authFetch("/users/primary-wallet", {
      method: "PUT",
      body: JSON.stringify({ primary_wallet_address: rawDefaultAddress }),
    });
    if (res.status !== 200) {
      throw new Error("error setting default primary wallet");
    }

    const updatedUser = (await res.json()) as UserResponse;
    return updatedUser.primary_wallet_address;
  };

  const _postWallet = async (wallet: WalletResponse): Promise<number> => {
    const res = await authFetch("/wallets", {
      method: "POST",
      body: JSON.stringify(wallet),
    });

    if (res.status != 201) {
      throw new Error("error posting wallet");
    }

    return await res.json();
  };

  const _updateWallet = async (
    w: Partial<WalletResponse> & Pick<WalletResponse, "id" | "owner" | "name">,
  ) => {
    const res = await authFetch("/wallets", {
      method: "PUT",
      body: JSON.stringify(w),
    });

    if (res.status != 201) {
      throw new Error("error updating wallet");
    }
  };

  const _initWallets = async (extWallets?: WalletResponse[]) => {
    setWalletsStatus("loading");
    try {
      if (!privyUser?.id) {
        throw new Error("user not authenticated");
      }
      if (extWallets === undefined) {
        extWallets = await _getWallets();
      }
      const isNewAccount = extWallets.length === 0;

      let wResults: Promise<AppWallet>[] = [];
      let cResults: Promise<void>[] = [];
      const managedPrivyWallets = getManagedPrivyWallets();
      for (let i = 0; i < managedPrivyWallets.length; i++) {
        const privyWallet = managedPrivyWallets[i];

        cResults.push(privyWallet.switchChain(CHAIN_ID));

        let extWallet = extWallets.find(
          (w) => w.eoa_address == privyWallet.address && w.is_eoa === true,
        );
        wResults.push(_initEOAWallet(privyWallet, extWallet, i));

        let smartWallets = extWallets.filter(
          (w) =>
            w.eoa_address == privyWallet.address &&
            w.is_eoa === false &&
            w.smart_index !== undefined,
        );
        if (smartWallets.length === 0) {
          smartWallets.push({
            id: null,
            owner: privyUser.id,
            name: "",
            is_eoa: false,
            is_hidden: false,
            is_redeemer: false,
            is_minter: false,
            eoa_address: privyWallet.address,
            smart_index: 0,
          });
        }

        for (let index = 0n; index < BigInt(smartWallets.length); index++) {
          let extSmartWallet = smartWallets.find((w) => {
            if (w.smart_index === undefined) w.smart_index = 10000;
            if (w.smart_index === null) w.smart_index = 10000;
            return (
              w.eoa_address == privyWallet.address &&
              w.is_eoa === false &&
              BigInt(w.smart_index) === index
            );
          });
          if (!extSmartWallet) continue;

          wResults.push(
            _initSmartWallet(
              privyWallet,
              extSmartWallet,
              index,
              i,
              isNewAccount,
            ),
          );
        }
      }
      await Promise.all(cResults);
      let wlts = await Promise.all(wResults);

      setWallets(wlts);
      setWalletsStatus("available");
    } catch (error) {
      setWalletsStatus("unavailable");
      throw new Error("error initializing wallets");
    }
  };

  const _initEOAWallet = async (
    privyWallet: ConnectedWallet,
    wallet: WalletResponse | undefined,
    i: number,
  ): Promise<AppWallet> => {
    if (!privyUser) throw new Error("user not logged in");

    if (!wallet) {
      wallet = {
        id: null,
        owner: privyUser.id,
        name: "EOA-" + (i + 1),
        is_eoa: true,
        is_hidden: false,
        is_redeemer: false,
        is_minter: false,
        eoa_address: privyWallet.address,
      };

      let id = await _postWallet(wallet);
      wallet.id = id;
    }

    const resolvedWallet = wallet;
    const eoaName = resolvedWallet.name;
    const w = new AppWallet(privyWallet, eoaName, {
      id: resolvedWallet.id || undefined,
      isHidden: resolvedWallet.is_hidden,
      isRedeemer: resolvedWallet.is_redeemer,
      isMinter: resolvedWallet.is_minter,
    });
    await w.init();
    return w;
  };

  const _initSmartWallet = async (
    privyWallet: ConnectedWallet,
    wallet: WalletResponse,
    index: bigint,
    i: number,
    isNewAccount: boolean,
  ): Promise<AppWallet> => {
    if (wallet.is_eoa)
      throw new Error("trying to initialize smart wallet with eoa");
    const existingName = wallet?.name?.trim() || "";
    const defaultSmartWalletName =
      "SW-" + (i + 1) + "-" + (index + 1n).toString();
    const generatedSmartWalletName =
      index === 0n && isNewAccount ? "Primary Wallet" : defaultSmartWalletName;
    const smartWalletName =
      existingName ||
      (wallet.id === null ? generatedSmartWalletName : defaultSmartWalletName);
    if (!existingName && wallet.id === null) {
      wallet.name = generatedSmartWalletName;
    }

    const w = new AppWallet(privyWallet, smartWalletName, {
      index,
      id: wallet.id || undefined,
      isHidden: wallet.is_hidden,
      isRedeemer: wallet.is_redeemer,
      isMinter: wallet.is_minter,
    });
    const isDeployed = await w.init();
    if (wallet.id === null) {
      wallet.smart_address = w.address;
      wallet.smart_index = Number(index);
      let id = await _postWallet(wallet);
      w.setId(id);
    }

    if (!isDeployed) {
      const deployedNow = await w.ensureSmartWalletDeployed();
      if (!deployedNow) {
        console.error("smart wallet remained undeployed after initialization", {
          owner: wallet.owner,
          eoaAddress: wallet.eoa_address,
          smartIndex: index.toString(),
          smartAddress: w.address,
          walletId: wallet.id,
        });
      }
    }

    return w;
  };

  const _updatePayPalAddress = async (payPalAddress: string) => {
    const res = await authFetch("/paypaleth", {
      method: "PUT",
      body: payPalAddress,
    });

    if (res.status != 201) {
      throw new Error("error updating paypal address");
    }
  };

  const addWallet = async (walletName: string) => {
    if (!privyUser) throw new Error("no user logged in");
    const privyWallet = privyWallets[0];
    const n = wallets.filter(
      (w) =>
        w.owner.address === privyWallet.address && w.type === "smartwallet",
    ).length;

    const wallet: WalletResponse = {
      id: null,
      owner: privyUser.id,
      name: walletName,
      is_eoa: false,
      is_hidden: false,
      is_redeemer: false,
      is_minter: false,
      eoa_address: privyWallet.address,
    };

    const w = await _initSmartWallet(privyWallet, wallet, BigInt(n), 1, false);
    setWallets([...wallets, w]);
  };

  const importWallet = async (walletName: string, privateKey: string) => {
    if (!privyUser) {
      setError("no user authenticated");
      return;
    }
    let s = walletsStatus;
    setWalletsStatus("loading");

    let w: WalletResponse;
    try {
      const accessToken = await getAccessToken();
      if (!accessToken) throw new Error("no access token available");
      const address = await privyImportWallet(privateKey, accessToken);
      w = {
        id: 0,
        owner: privyUser.id,
        name: walletName,
        is_eoa: true,
        is_hidden: false,
        is_redeemer: false,
        is_minter: false,
        eoa_address: address,
      };
    } catch (error) {
      setWalletsStatus(s);
      throw error;
    }
    try {
      await _postWallet(w);
      await _initWallets();
      setWalletsStatus(s);
    } catch (error) {
      setWalletsStatus(s);
      setError(error);
    }
  };

  const updateWallet = async (
    id: number,
    name: string,
  ): Promise<string | null> => {
    const s = walletsStatus;
    let n: string | null = null;
    setWalletsStatus("loading");
    try {
      if (!user) {
        throw new Error("no user logged in");
      }
      const existingWallet = wallets.find((wallet) => wallet.id === id);
      const sWallet = {
        id,
        owner: user.id,
        name,
        is_hidden: existingWallet?.isHidden === true,
      };

      await _updateWallet(sWallet);
      n = name;
      if (existingWallet) {
        existingWallet.name = name;
      }
      await refreshWallets();
    } catch (error) {
      setError(error);
      throw new Error("error updating wallet");
    }
    setWalletsStatus(s);
    return n;
  };

  const refreshWallets = async () => {
    const s = walletsStatus;
    setWalletsStatus("loading");
    try {
      await _initWallets();
    } catch (error) {
      setError(error);
    }
    setWalletsStatus(s);
  };

  const ensurePrimarySmartWallet = async (): Promise<boolean> => {
    if (!privyUser?.id) {
      return false;
    }

    const managedPrivyWallets = getManagedPrivyWallets();
    const primaryPrivyWallet = managedPrivyWallets[0];
    if (!primaryPrivyWallet?.address) {
      return false;
    }
    const primaryEoaAddress = primaryPrivyWallet.address.toLowerCase();

    const hasPrimaryWallet = (walletList: WalletResponse[]) =>
      walletList.some(
        (wallet) =>
          wallet.is_eoa === false &&
          wallet.smart_index === 0 &&
          wallet.eoa_address?.toLowerCase() === primaryEoaAddress &&
          typeof wallet.smart_address === "string" &&
          wallet.smart_address.trim() !== "",
      );

    await refreshWallets();
    let backendWallets = await _getWallets();
    const isNewAccount = backendWallets.length === 0;
    if (hasPrimaryWallet(backendWallets)) {
      return true;
    }

    try {
      await primaryPrivyWallet.switchChain(CHAIN_ID);
    } catch (error) {
      console.error(
        "error switching chain while ensuring primary smart wallet",
        error,
      );
    }

    const existingEOAWallet = backendWallets.find(
      (wallet) =>
        wallet.is_eoa === true &&
        wallet.eoa_address?.toLowerCase() === primaryEoaAddress,
    );
    if (!existingEOAWallet) {
      try {
        await _initEOAWallet(primaryPrivyWallet, undefined, 0);
      } catch (error) {
        console.error(
          "error creating missing eoa wallet while ensuring primary smart wallet",
          error,
        );
      }
    }

    const smartWalletTemplate: WalletResponse = {
      id: null,
      owner: privyUser.id,
      name: "",
      is_eoa: false,
      is_hidden: false,
      is_redeemer: false,
      is_minter: false,
      eoa_address: primaryPrivyWallet.address,
      smart_index: 0,
    };

    try {
      await _initSmartWallet(
        primaryPrivyWallet,
        smartWalletTemplate,
        0n,
        0,
        isNewAccount,
      );
    } catch (error) {
      console.error(
        "error upserting primary smart wallet while ensuring wallet availability",
        error,
      );
    }

    await refreshWallets();
    backendWallets = await _getWallets();
    return hasPrimaryWallet(backendWallets);
  };

  const login = async () => {
    if (!privyReady) {
      setError("privy not ready");
      console.log("Should be returning rn");
      return;
    }

    if (!privyAuthenticated) {
      try {
        await privyLogin();
        // move user data implementation to helper functions called in useEffect instead of passing into login() for real auth
        // localStorage.setItem("sfluv_user", JSON.stringify(mockUser))
      } catch (error) {
        setError(error);
      }
    }
  };

  const logout = async () => {
    _resetAppState();
    await privyLogout();
  };

  const updateUser = (data: Partial<User>) => {
    if (user) {
      const updatedUser = { ...user, ...data };
      setUser(updatedUser);
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser));
    }
  };

  const approveMerchantStatus = () => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "approved" as MerchantApprovalStatus,
        role: "merchant",
      };
      setUser(updatedUser);
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser));
    }
  };

  const rejectMerchantStatus = () => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "rejected" as MerchantApprovalStatus,
      };
      setUser(updatedUser);
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser));
    }
  };

  const updatePayPalAddress = async (payPalAddress: string) => {
    if (!user) {
      throw new Error("no user logged in");
    }
    await _updatePayPalAddress(payPalAddress);
    setUser({
      ...user,
      paypalEthAddress: payPalAddress,
    });
  };

  const addPonderSubscription = async (email: string, address: string) => {
    const body: PonderSubscriptionRequest = {
      email,
      address,
    };

    const res = await authFetch("/ponder", {
      body: JSON.stringify(body),
      method: "POST",
    });

    if (!res.ok)
      throw new Error("error adding ponder subscription for " + address);
  };

  const deletePonderSubscription = async (id: number) => {
    const res = await authFetch("/ponder?id=" + id, {
      method: "DELETE",
    });

    if (!res.ok) throw new Error("error deleting ponder subscription " + id);
  };

  const getPonderSubscriptions = async () => {
    try {
      const res = await authFetch("/ponder");
      let body = (await res.json()) as PonderSubscription[];
      body = body?.map((sub) => {
        if (sub.type === "merchant") {
          sub.data = new TextDecoder("utf-8").decode(base64.decode(sub.data));
        }
        return sub;
      });

      setPonderSubscriptions(body || []);
    } catch (error) {
      setError(error);
    }
  };

  return (
    <AppStatusContext.Provider value={status}>
      <AppContext.Provider
        value={{
          status,
          user,
          affiliate,
          setAffiliate,
          proposer,
          setProposer,
          improver,
          setImprover,
          issuer,
          setIssuer,
          supervisor,
          setSupervisor,
          wallets,
          walletsStatus,
          userLocations,
          setUserLocations,
          tx,
          addWallet,
          importWallet,
          updateWallet,
          refreshWallets,
          ensurePrimarySmartWallet,
          error,
          setError,
          login,
          logout,
          authFetch,
          mapLocations,
          updateUser,
          approveMerchantStatus,
          rejectMerchantStatus,
          updatePayPalAddress,
          ponderSubscriptions,
          addPonderSubscription,
          getPonderSubscriptions,
          deletePonderSubscription,
        }}
      >
        <>
          <IdleModal
            open={idleModalOpen}
            onOpenChange={toggleIdleModal}
            getRemainingTime={getRemainingTime}
          />
          {children}
        </>
      </AppContext.Provider>
    </AppStatusContext.Provider>
  );
}

export function useApp() {
  const context = useContext(AppContext);
  if (!context) {
    throw new Error("useApp must be used within an AppProvider");
  }
  return context;
}

export function useAppStatus() {
  return useContext(AppStatusContext);
}
