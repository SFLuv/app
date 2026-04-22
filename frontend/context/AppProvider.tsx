"use client";

import Link from "next/link";
import {
  ConnectedWallet,
  EIP1193Provider,
  PrivyProvider,
  useLinkAccount,
  useOAuthTokens,
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
import {
  AppleRecoveryResponse,
  AccountDeletionStatusResponse,
  GetUserResponse,
  UserPolicyStatusResponse,
  UserResponse,
  WalletResponse,
} from "@/types/server";
import { AuthedLocation } from "@/types/location";
import { importWallet as privyImportWallet } from "@/lib/wallets/import";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Contact } from "@/types/contact";
import { useIdleTimer } from "react-idle-timer";
import { IdleModal } from "@/components/idle/idle-modal";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { PonderSubscription, PonderSubscriptionRequest } from "@/types/ponder";
import { base64 } from "@scure/base";
import {
  buildPolicyPageHref,
  buildPolicyReturnTo,
  EMAIL_OPT_IN_POLICY_PATH,
  PRIVACY_POLICY_PATH,
} from "@/lib/policies";

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
  acceptedPrivacyPolicy: boolean;
  acceptedPrivacyPolicyAt?: string | null;
  privacyPolicyVersion: string;
  mailingListOptIn: boolean;
  mailingListOptInAt?: string | null;
  mailingListPolicyVersion: string;
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
  googleLinked: boolean;
  googleLinkedEmail?: string;
  googleActionBusy: boolean;
  googleMessage: string;
  canDisconnectGoogle: boolean;
  googleDisconnectDisabledReason: string;
  unlinkGoogle: () => Promise<void>;
  appleLinked: boolean;
  appleLinkedEmail?: string;
  appleLinkBusy: boolean;
  appleLinkMessage: string;
  canDisconnectApple: boolean;
  appleDisconnectDisabledReason: string;
  linkApple: () => Promise<void>;
  unlinkApple: () => Promise<void>;
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
const REACTIVATED_ACCOUNT_RECOVERY_NOTICE_STORAGE_KEY =
  "sfluv_reactivated_account_recovery_notice";
const ACCOUNT_RECOVERY_SUPPORT_EMAIL = "techsupport@sfluv.org";
const POLICY_REQUIRED_HEADER = "X-SFLUV-Auth-Reason";
const POLICY_REQUIRED_REASON = "privacy-policy-required";

const AppContext = createContext<AppContextType | null>(null);
const AppStatusContext = createContext<UserStatus>("loading");

class DeletedAccountError extends Error {
  readonly deletionStatus: AccountDeletionStatusResponse;

  constructor(deletionStatus: AccountDeletionStatusResponse) {
    super("This account is scheduled for deletion.");
    this.name = "DeletedAccountError";
    this.deletionStatus = deletionStatus;
  }
}

function formatDeletionDate(value?: string | null): string | null {
  if (!value) {
    return null;
  }

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return null;
  }

  return new Intl.DateTimeFormat("en-US", {
    month: "long",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(parsed);
}

function getLinkedOAuthAccount(
  currentUser: unknown,
  accountType: "apple_oauth" | "google_oauth",
): {
  email?: string | null;
  subject?: string;
} | null {
  if (!currentUser || typeof currentUser !== "object") {
    return null;
  }

  const rawLinkedAccounts = Array.isArray(
    (currentUser as { linkedAccounts?: unknown[] }).linkedAccounts,
  )
    ? (currentUser as { linkedAccounts: unknown[] }).linkedAccounts
    : Array.isArray((currentUser as { linked_accounts?: unknown[] }).linked_accounts)
      ? (currentUser as { linked_accounts: unknown[] }).linked_accounts
      : [];

  for (const account of rawLinkedAccounts) {
    if (!account || typeof account !== "object") {
      continue;
    }

    const typedAccount = account as {
      type?: string;
      email?: string | null;
      subject?: string;
    };
    if (typedAccount.type !== accountType) {
      continue;
    }

    return {
      email: typedAccount.email ?? undefined,
      subject: typedAccount.subject,
    };
  }

  return null;
}

function getLinkedAppleAccount(currentUser: unknown) {
  return getLinkedOAuthAccount(currentUser, "apple_oauth");
}

function getLinkedGoogleAccount(currentUser: unknown) {
  return getLinkedOAuthAccount(currentUser, "google_oauth");
}

function getLinkedEmailAccount(currentUser: unknown): {
  address?: string;
} | null {
  if (!currentUser || typeof currentUser !== "object") {
    return null;
  }

  const rawLinkedAccounts = Array.isArray(
    (currentUser as { linkedAccounts?: unknown[] }).linkedAccounts,
  )
    ? (currentUser as { linkedAccounts: unknown[] }).linkedAccounts
    : Array.isArray((currentUser as { linked_accounts?: unknown[] }).linked_accounts)
      ? (currentUser as { linked_accounts: unknown[] }).linked_accounts
      : [];

  for (const account of rawLinkedAccounts) {
    if (!account || typeof account !== "object") {
      continue;
    }

    const typedAccount = account as {
      type?: string;
      address?: string;
    };
    if (typedAccount.type !== "email") {
      continue;
    }

    return {
      address: typedAccount.address?.trim() || undefined,
    };
  }

  return null;
}

function isApplePrivateRelayEmail(email?: string | null): boolean {
  return (email || "").trim().toLowerCase().endsWith("@privaterelay.appleid.com");
}

function describeAppleRecoveryPrompt(
  recovery: AppleRecoveryResponse,
): {
  title: string;
  body: string;
  primaryLabel: string;
  secondaryLabel: string;
} {
  if (recovery.resolution === "recovery_suggested") {
    const existingAccountLabel =
      recovery.suggested_existing_account?.contact_name?.trim() ||
      recovery.suggested_existing_account?.verified_email?.trim() ||
      "your existing SFLUV account";
    return {
      title: "We found an existing account",
      body: `Apple signed you into a new Privy identity, but ${existingAccountLabel} already exists in SFLUV. Go back and sign in with Google or email first, then link Apple in Settings. If you continue here and Apple does not share your real email with us, we will not be able to link the accounts together and you will end up with two separate SFLUV accounts.`,
      primaryLabel: "Use my existing account",
      secondaryLabel: "Continue with Apple anyway",
    };
  }

  if (recovery.is_private_relay || !recovery.apple_email) {
    return {
      title: "Continue with Apple?",
      body: "Apple may have hidden your real email. If you create an Apple account without sharing your real email with us, we will not be able to link it to an existing SFLUV account and you will end up with two separate accounts. Go back and sign in with Google or email first if you already have an account.",
      primaryLabel: "I already have an account",
      secondaryLabel: "Continue with Apple anyway",
    };
  }

  if (recovery.resolution === "ambiguous_match") {
    return {
      title: "Multiple accounts found",
      body: `Apple shared ${recovery.apple_email}, but more than one active SFLUV account is associated with that address. Go back and sign in with the account you want to keep, then link Apple in Settings. If you continue here, you may create a separate Apple account.`,
      primaryLabel: "Go back to sign in",
      secondaryLabel: "Continue with Apple anyway",
    };
  }

  return {
    title: "Continue with Apple?",
    body: "If you already have an SFLUV account, go back and sign in with Google or email first, then link Apple in Settings. Otherwise continue to create a new Apple account here.",
    primaryLabel: "I already have an account",
    secondaryLabel: "Continue with Apple",
  };
}

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
  const [deletedAccountStatus, setDeletedAccountStatus] =
    useState<AccountDeletionStatusResponse | null>(null);
  const [deletedAccountAction, setDeletedAccountAction] = useState<
    "idle" | "reactivating" | "returning"
  >("idle");
  const [deletedAccountError, setDeletedAccountError] = useState("");
  const [appleRecovery, setAppleRecovery] =
    useState<AppleRecoveryResponse | null>(null);
  const [appleRecoveryAction, setAppleRecoveryAction] = useState<
    "idle" | "continuing" | "returning"
  >("idle");
  const [appleRecoveryError, setAppleRecoveryError] = useState("");
  const [appleRecoveryBypassed, setAppleRecoveryBypassed] = useState(false);
  const [pendingAppleTokens, setPendingAppleTokens] = useState<{
    accessToken: string;
    refreshToken?: string;
    accessTokenExpiresInSeconds?: number;
    refreshTokenExpiresInSeconds?: number;
    scopes?: string[];
  } | null>(null);
  const [appleLinkBusy, setAppleLinkBusy] = useState(false);
  const [appleLinkMessage, setAppleLinkMessage] = useState("");
  const [appleUnlinkBusy, setAppleUnlinkBusy] = useState(false);
  const [googleActionBusy, setGoogleActionBusy] = useState(false);
  const [googleMessage, setGoogleMessage] = useState("");
  const [idleModalOpen, setIdleModalOpen] = useState<boolean>(false);
  const [showRecoveryFundsNotice, setShowRecoveryFundsNotice] =
    useState(false);
  const [policyStatus, setPolicyStatus] =
    useState<UserPolicyStatusResponse | null>(null);
  const [policyAction, setPolicyAction] = useState<
    "idle" | "submitting" | "returning"
  >("idle");
  const [policyError, setPolicyError] = useState("");
  const [ponderSubscriptions, setPonderSubscriptions] = useState<
    PonderSubscription[]
  >([]);
  const [idleTimer, setIdleTimer] = useState<NodeJS.Timeout | undefined>();
  const privy = usePrivy();
  const {
    getAccessToken,
    authenticated: privyAuthenticated,
    ready: privyReady,
    login: privyLogin,
    logout: privyLogout,
    user: privyUser,
  } = privy;
  const unlinkOAuth = (
    privy as typeof privy & {
      unlinkOAuth?: (
        provider: "google" | "apple",
        subject: string,
      ) => Promise<unknown>;
    }
  ).unlinkOAuth;
  const linkedAppleAccount = useMemo(
    () => getLinkedAppleAccount(privyUser),
    [privyUser],
  );
  const linkedGoogleAccount = useMemo(
    () => getLinkedGoogleAccount(privyUser),
    [privyUser],
  );
  const linkedEmailAccount = useMemo(
    () => getLinkedEmailAccount(privyUser),
    [privyUser],
  );
  const appleLinked = Boolean(
    linkedAppleAccount?.subject || linkedAppleAccount?.email,
  );
  const googleLinked = Boolean(
    linkedGoogleAccount?.subject || linkedGoogleAccount?.email,
  );
  const emailLinked = Boolean(linkedEmailAccount?.address);
  const signInMethodCount =
    Number(appleLinked) + Number(googleLinked) + Number(emailLinked);
  const canDisconnectApple = appleLinked && signInMethodCount > 1;
  const canDisconnectGoogle = googleLinked && signInMethodCount > 1;
  const appleLinkedEmail =
    linkedAppleAccount?.email?.trim() || undefined;
  const googleLinkedEmail =
    linkedGoogleAccount?.email?.trim() || undefined;
  const appleDisconnectDisabledReason =
    appleLinked && !canDisconnectApple
      ? "Add email or Google before disconnecting Apple."
      : "";
  const googleDisconnectDisabledReason =
    googleLinked && !canDisconnectGoogle
      ? "Add email or Apple before disconnecting Google."
      : "";
  const { linkApple: privyLinkApple } = useLinkAccount({
    onSuccess: ({ linkMethod }) => {
      if (linkMethod !== "apple") {
        return;
      }
      setAppleLinkBusy(false);
      setAppleLinkMessage("Apple is now linked to this account.");
    },
    onError: (_error, details) => {
      if (details.linkMethod !== "apple") {
        return;
      }
      setAppleLinkBusy(false);
      setAppleLinkMessage("Unable to link Apple right now.");
    },
  });
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

  useOAuthTokens({
    onOAuthTokenGrant: ({ oAuthTokens }) => {
      if (oAuthTokens.provider !== "apple") {
        return;
      }
      setPendingAppleTokens({
        accessToken: oAuthTokens.accessToken,
        refreshToken: oAuthTokens.refreshToken,
        accessTokenExpiresInSeconds: oAuthTokens.accessTokenExpiresInSeconds,
        refreshTokenExpiresInSeconds: oAuthTokens.refreshTokenExpiresInSeconds,
        scopes: oAuthTokens.scopes,
      });
    },
  });

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
  const allowPolicyRoute =
    pathname.startsWith(PRIVACY_POLICY_PATH) ||
    pathname.startsWith(EMAIL_OPT_IN_POLICY_PATH);

  const clearAuthenticatedState = (options?: {
    clearDeletedAccount?: boolean;
    redirectToMap?: boolean;
  }) => {
    const clearDeletedAccount = options?.clearDeletedAccount ?? true;
    const redirectToMap = options?.redirectToMap ?? true;
    const allowUnauthedRoute =
      pathname === "/map" ||
      pathname === "/redirect" ||
      pathname.startsWith("/faucet") ||
      pathname.startsWith("/improver/join") ||
      pathname.startsWith(PRIVACY_POLICY_PATH) ||
      pathname.startsWith(EMAIL_OPT_IN_POLICY_PATH) ||
      pathname.startsWith("/photos/") ||
      pathname.startsWith("/photo/");

    if (redirectToMap && !allowUnauthedRoute) {
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
    if (clearDeletedAccount) {
      setDeletedAccountStatus(null);
      setDeletedAccountAction("idle");
      setDeletedAccountError("");
    }
    setAppleRecovery(null);
    setAppleRecoveryAction("idle");
    setAppleRecoveryError("");
    setAppleRecoveryBypassed(false);
    setPendingAppleTokens(null);
    setAppleLinkBusy(false);
    setAppleLinkMessage("");
    setPolicyStatus(null);
    setPolicyAction("idle");
    setPolicyError("");
  };

  const activateDeletedAccountGate = (
    nextDeletedAccountStatus: AccountDeletionStatusResponse,
  ) => {
    clearAuthenticatedState({ clearDeletedAccount: false, redirectToMap: true });
    setDeletedAccountStatus(nextDeletedAccountStatus);
    setDeletedAccountAction("idle");
    setDeletedAccountError("");
  };

  const activatePolicyGate = (
    nextPolicyStatus: UserPolicyStatusResponse,
  ) => {
    setPolicyStatus(nextPolicyStatus);
    setPolicyAction("idle");
    setPolicyError("");
    setStatus("loading");
  };

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

    if (deletedAccountStatus) {
      return;
    }

    if (appleRecovery) {
      return;
    }

    if (policyStatus) {
      return;
    }

    _userLogin();
  }, [
    appleRecovery,
    deletedAccountStatus,
    pathname,
    policyStatus,
    privyAuthenticated,
    privyReady,
    privyUser,
    walletsReady,
  ]);

  useEffect(() => {
    if (error) console.error(error);
  }, [error]);

  useEffect(() => {
    if (appleLinked) {
      setAppleLinkBusy(false);
    }
  }, [appleLinked]);

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

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    if (status !== "authenticated" || deletedAccountStatus || policyStatus) {
      setShowRecoveryFundsNotice(false);
      return;
    }

    try {
      setShowRecoveryFundsNotice(
        window.localStorage.getItem(
          REACTIVATED_ACCOUNT_RECOVERY_NOTICE_STORAGE_KEY,
        ) === "pending",
      );
    } catch (error) {
      console.error("Unable to load the account recovery notice state", error);
    }
  }, [deletedAccountStatus, policyStatus, status, user?.id]);

  const dismissRecoveryFundsNotice = () => {
    setShowRecoveryFundsNotice(false);
    if (typeof window === "undefined") {
      return;
    }

    try {
      window.localStorage.removeItem(
        REACTIVATED_ACCOUNT_RECOVERY_NOTICE_STORAGE_KEY,
      );
    } catch (error) {
      console.error("Unable to clear the account recovery notice state", error);
    }
  };

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
      acceptedPrivacyPolicy: r.user.accepted_privacy_policy,
      acceptedPrivacyPolicyAt: r.user.accepted_privacy_policy_at,
      privacyPolicyVersion: r.user.privacy_policy_version,
      mailingListOptIn: r.user.mailing_list_opt_in,
      mailingListOptInAt: r.user.mailing_list_opt_in_at,
      mailingListPolicyVersion: r.user.mailing_list_policy_version,
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
    let nextPolicyStatus: UserPolicyStatusResponse | null;

    setStatus("loading");

    try {
      nextPolicyStatus = await _getUserPolicyStatusRaw();
      if (nextPolicyStatus === null) {
        if (appleLinked && !appleRecoveryBypassed) {
          try {
            const recovery = await _resolveAppleRecovery({
              providerSubject: linkedAppleAccount?.subject,
              providerEmail: linkedAppleAccount?.email || undefined,
              isPrivateRelay: isApplePrivateRelayEmail(linkedAppleAccount?.email),
            });

            if (
              recovery.resolution !== "current_account_exists" &&
              recovery.resolution !== "no_apple_account"
            ) {
              setAppleRecovery(recovery);
              setStatus("unauthenticated");
              return;
            }
          } catch (error) {
            console.error("error resolving apple recovery", error);
            setAppleRecovery({
              current_user_id: privyUser?.id || "",
              current_user_exists: false,
              apple_linked: true,
              apple_email: linkedAppleAccount?.email || undefined,
              is_private_relay: isApplePrivateRelayEmail(
                linkedAppleAccount?.email,
              ),
              resolution: "no_match",
            });
            setStatus("unauthenticated");
            return;
          }
        }

        await _postUser();
        setAppleRecoveryBypassed(false);
        nextPolicyStatus = await _getUserPolicyStatusRaw();
      }
      if (nextPolicyStatus === null) {
        throw new Error("error loading user policy status");
      }
      if (!nextPolicyStatus.active) {
        const accountDeletionStatus = await _getDeleteAccountStatus();
        if (
          accountDeletionStatus &&
          accountDeletionStatus.status !== "active"
        ) {
          throw new DeletedAccountError(accountDeletionStatus);
        }
      }
      if (!nextPolicyStatus.accepted_privacy_policy) {
        activatePolicyGate(nextPolicyStatus);
        return;
      }

      setAppleRecovery(null);
      setAppleRecoveryBypassed(false);
      setPolicyStatus(null);

      userResponse = await _getUser();
      if (userResponse === null) {
        throw new Error("error getting user");
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
      if (error instanceof DeletedAccountError) {
        activateDeletedAccountGate(error.deletionStatus);
        return;
      }
      setError(error);
      console.error(error);
      await logout();
    }
  };

  const _resetAppState = async () => {
    clearAuthenticatedState({ clearDeletedAccount: true, redirectToMap: true });
  };

  const rawAuthFetch = async (
    endpoint: string,
    options: RequestInit = {},
  ): Promise<Response> => {
    const accessToken = await getAccessToken();
    if (!accessToken) {
      throw new Error("no access token");
    }
    const h: HeadersInit = {
      ...options.headers,
      "Access-Token": accessToken,
    };

    return await fetch(BACKEND + endpoint, { ...options, headers: h });
  };

  const _getUserPolicyStatusRaw =
    async (): Promise<UserPolicyStatusResponse | null> => {
      const res = await rawAuthFetch("/users/policy-status");
      if (res.status === 404) {
        return null;
      }
      if (res.status !== 200) {
        throw new Error("error getting user policy status");
      }
      return (await res.json()) as UserPolicyStatusResponse;
    };

  const _acceptUserPolicies = async (
    mailingListOptIn: boolean,
  ): Promise<UserPolicyStatusResponse> => {
    const res = await rawAuthFetch("/users/policies/accept", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        accepted_privacy_policy: true,
        mailing_list_opt_in: mailingListOptIn,
      }),
    });
    if (res.status !== 200) {
      throw new Error(
        (await res.text()) || "Unable to save your policy preferences.",
      );
    }
    return (await res.json()) as UserPolicyStatusResponse;
  };

  const authFetch = async (
    endpoint: string,
    options: RequestInit = {},
  ): Promise<Response> => {
    const response = await rawAuthFetch(endpoint, options);
    if (
      response.status === 403 &&
      response.headers.get(POLICY_REQUIRED_HEADER) ===
        POLICY_REQUIRED_REASON
    ) {
      try {
        const nextPolicyStatus = await _getUserPolicyStatusRaw();
        if (
          nextPolicyStatus &&
          nextPolicyStatus.accepted_privacy_policy !== true
        ) {
          activatePolicyGate(nextPolicyStatus);
        }
      } catch (error) {
        console.error("Unable to load the user policy status", error);
      }
    }

    return response;
  };

  const _storeAppleOAuthCredential = async (input: {
    accessToken: string;
    refreshToken?: string;
    accessTokenExpiresInSeconds?: number;
    refreshTokenExpiresInSeconds?: number;
    scopes?: string[];
    providerSubject?: string;
    providerEmail?: string;
    isPrivateRelay?: boolean;
  }) => {
    const res = await authFetch("/users/oauth/apple", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        access_token: input.accessToken,
        refresh_token: input.refreshToken ?? "",
        access_token_expires_in_seconds:
          input.accessTokenExpiresInSeconds ?? 0,
        refresh_token_expires_in_seconds:
          input.refreshTokenExpiresInSeconds ?? 0,
        scopes: Array.isArray(input.scopes) ? input.scopes : [],
        provider_subject: input.providerSubject ?? "",
        provider_email: input.providerEmail ?? "",
        is_private_relay: input.isPrivateRelay === true,
      }),
    });

    if (!res.ok) {
      throw new Error("Unable to store Apple OAuth credentials.");
    }
  };

  const _resolveAppleRecovery = async (input?: {
    providerSubject?: string;
    providerEmail?: string;
    isPrivateRelay?: boolean;
  }): Promise<AppleRecoveryResponse> => {
    const res = await authFetch("/users/apple/recovery", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        provider_subject: input?.providerSubject ?? "",
        provider_email: input?.providerEmail ?? "",
        is_private_relay: input?.isPrivateRelay === true,
      }),
    });

    if (!res.ok) {
      throw new Error("Unable to resolve Apple account recovery status.");
    }

    return (await res.json()) as AppleRecoveryResponse;
  };

  const _postUser = async () => {
    let res = await rawAuthFetch("/users", { method: "POST" });
    if (res.status != 201) {
      throw new Error("error posting user");
    }
  };

  const _getDeleteAccountStatus =
    async (): Promise<AccountDeletionStatusResponse | null> => {
      const res = await rawAuthFetch("/users/delete-account/status");
      if (res.status === 404) {
        return null;
      }
      if (res.status !== 200) {
        throw new Error("error getting delete-account status");
      }
      return (await res.json()) as AccountDeletionStatusResponse;
    };

  const _cancelDeleteAccount =
    async (): Promise<AccountDeletionStatusResponse> => {
      const res = await rawAuthFetch("/users/delete-account/cancel", {
        method: "POST",
      });
      if (res.status === 410) {
        throw new Error(
          "This account has already reached the end of its deletion window.",
        );
      }
      if (res.status !== 200) {
        throw new Error("error canceling delete account");
      }
      return (await res.json()) as AccountDeletionStatusResponse;
    };

  const _getUser = async (): Promise<GetUserResponse | null> => {
    const res = await authFetch("/users");
    if (res.status == 404) {
      return null;
    }
    if (
      res.status === 403 &&
      res.headers.get(POLICY_REQUIRED_HEADER) ===
        POLICY_REQUIRED_REASON
    ) {
      throw new Error("privacy policy acceptance required");
    }
    if (res.status === 403) {
      const accountDeletionStatus = await _getDeleteAccountStatus();
      if (
        accountDeletionStatus &&
        accountDeletionStatus.status !== "active"
      ) {
        throw new DeletedAccountError(accountDeletionStatus);
      }
    }
    if (res.status != 200) {
      throw new Error("error getting user");
    }
    const json = await res.json();
    return json as GetUserResponse;
  };

  useEffect(() => {
    if (!user?.id || !pendingAppleTokens) {
      return;
    }

    let cancelled = false;
    const persistAppleTokens = async () => {
      try {
        await _storeAppleOAuthCredential({
          accessToken: pendingAppleTokens.accessToken,
          refreshToken: pendingAppleTokens.refreshToken,
          accessTokenExpiresInSeconds:
            pendingAppleTokens.accessTokenExpiresInSeconds,
          refreshTokenExpiresInSeconds:
            pendingAppleTokens.refreshTokenExpiresInSeconds,
          scopes: pendingAppleTokens.scopes,
          providerSubject: linkedAppleAccount?.subject,
          providerEmail: linkedAppleAccount?.email ?? undefined,
          isPrivateRelay: isApplePrivateRelayEmail(linkedAppleAccount?.email),
        });
        if (!cancelled) {
          setPendingAppleTokens(null);
        }
      } catch (error) {
        console.error("Unable to persist Apple OAuth credentials", error);
      }
    };

    void persistAppleTokens();
    return () => {
      cancelled = true;
    };
  }, [linkedAppleAccount?.email, linkedAppleAccount?.subject, pendingAppleTokens, user?.id]);

  const _getWallets = async (): Promise<WalletResponse[]> => {
    const res = await authFetch("/wallets");
    if (res.status != 200) {
      throw new Error("error getting wallets");
    }
    return (await res.json()) as WalletResponse[];
  };

  useEffect(() => {
    if (status !== "authenticated" || !privyAuthenticated || policyStatus) {
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
        if (error instanceof DeletedAccountError) {
          activateDeletedAccountGate(error.deletionStatus);
          return;
        }
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
  }, [policyStatus, status, privyAuthenticated]);

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
        setAppleRecovery(null);
        setAppleRecoveryAction("idle");
        setAppleRecoveryError("");
        setAppleRecoveryBypassed(false);
        setPolicyStatus(null);
        setPolicyAction("idle");
        setPolicyError("");
        await privyLogin();
        // move user data implementation to helper functions called in useEffect instead of passing into login() for real auth
        // localStorage.setItem("sfluv_user", JSON.stringify(mockUser))
      } catch (error) {
        setError(error);
      }
    }
  };

  const acceptPolicies = async (mailingListOptIn: boolean) => {
    setPolicyAction("submitting");
    setPolicyError("");
    try {
      await _acceptUserPolicies(mailingListOptIn);
      setPolicyStatus(null);
      setStatus("loading");
    } catch (error) {
      setPolicyError(
        (error as Error)?.message?.trim() ||
          "Unable to save your policy preferences right now.",
      );
    } finally {
      setPolicyAction("idle");
    }
  };

  const returnPolicyGateToLogin = async () => {
    setPolicyAction("returning");
    setPolicyError("");
    try {
      await logout();
    } finally {
      setPolicyAction("idle");
    }
  };

  const reactivateDeletedAccount = async () => {
    setDeletedAccountAction("reactivating");
    setDeletedAccountError("");
    try {
      await _cancelDeleteAccount();
      try {
        window.localStorage.setItem(
          REACTIVATED_ACCOUNT_RECOVERY_NOTICE_STORAGE_KEY,
          "pending",
        );
      } catch (storageError) {
        console.error(
          "Unable to persist the account recovery notice state",
          storageError,
        );
      }
      setDeletedAccountStatus(null);
      setStatus("loading");
    } catch (error) {
      setDeletedAccountError(
        (error as Error)?.message?.trim() ||
          "Unable to reactivate this account right now.",
      );
    } finally {
      setDeletedAccountAction("idle");
    }
  };

  const returnDeletedAccountToLogin = async () => {
    setDeletedAccountAction("returning");
    setDeletedAccountError("");
    try {
      await logout();
    } finally {
      setDeletedAccountAction("idle");
    }
  };

  const continueWithAppleAccount = async () => {
    setAppleRecoveryAction("continuing");
    setAppleRecoveryError("");
    setAppleRecoveryBypassed(true);
    setAppleRecovery(null);
    setStatus("unauthenticated");
    setAppleRecoveryAction("idle");
  };

  const returnAppleRecoveryToLogin = async () => {
    setAppleRecoveryAction("returning");
    setAppleRecoveryError("");
    try {
      window.alert(
        "Sign in with Google or email first, then link Apple from Settings.",
      );
      await logout();
    } catch (error) {
      setAppleRecoveryError(
        (error as Error)?.message?.trim() ||
          "Unable to return to the login screen right now.",
      );
    } finally {
      setAppleRecoveryAction("idle");
    }
  };

  const linkApple = async () => {
    if (appleLinked) {
      return;
    }

    try {
      setAppleLinkBusy(true);
      setAppleLinkMessage("");
      privyLinkApple();
    } catch (error) {
      setAppleLinkBusy(false);
      setAppleLinkMessage(
        (error as Error)?.message?.trim() || "Unable to link Apple right now.",
      );
    }
  };

  const unlinkApple = async () => {
    if (!linkedAppleAccount?.subject) {
      setAppleLinkMessage("Apple is not linked to this account.");
      return;
    }
    if (!canDisconnectApple) {
      setAppleLinkMessage(appleDisconnectDisabledReason);
      return;
    }
    const appleSubject = linkedAppleAccount.subject;
    if (
      !window.confirm(
        "Disconnect Apple from this account? Apple will no longer be able to sign in until you link it again.",
      )
    ) {
      return;
    }
    if (!unlinkOAuth) {
      setAppleLinkMessage("Unable to disconnect Apple right now.");
      return;
    }

    try {
      setAppleUnlinkBusy(true);
      setAppleLinkMessage("");
      await unlinkOAuth("apple", appleSubject);
      setAppleLinkMessage("Apple has been disconnected from this account.");
    } catch (error) {
      setAppleLinkMessage(
        (error as Error)?.message?.trim() ||
          "Unable to disconnect Apple right now.",
      );
    } finally {
      setAppleUnlinkBusy(false);
    }
  };

  const unlinkGoogle = async () => {
    if (!linkedGoogleAccount?.subject) {
      setGoogleMessage("Google is not linked to this account.");
      return;
    }
    if (!canDisconnectGoogle) {
      setGoogleMessage(googleDisconnectDisabledReason);
      return;
    }
    const googleSubject = linkedGoogleAccount.subject;
    if (
      !window.confirm(
        "Disconnect Google from this account? Google will no longer be able to sign in until you link it again.",
      )
    ) {
      return;
    }
    if (!unlinkOAuth) {
      setGoogleMessage("Unable to disconnect Google right now.");
      return;
    }

    try {
      setGoogleActionBusy(true);
      setGoogleMessage("");
      await unlinkOAuth("google", googleSubject);
      setGoogleMessage("Google has been disconnected from this account.");
    } catch (error) {
      setGoogleMessage(
        (error as Error)?.message?.trim() ||
          "Unable to disconnect Google right now.",
      );
    } finally {
      setGoogleActionBusy(false);
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
          googleLinked,
          googleLinkedEmail,
          googleActionBusy,
          googleMessage,
          canDisconnectGoogle,
          googleDisconnectDisabledReason,
          unlinkGoogle,
          appleLinked,
          appleLinkedEmail,
          appleLinkBusy: appleLinkBusy || appleUnlinkBusy,
          appleLinkMessage,
          canDisconnectApple,
          appleDisconnectDisabledReason,
          linkApple,
          unlinkApple,
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
          <RecoveryFundsNoticeDialog
            open={showRecoveryFundsNotice}
            onClose={dismissRecoveryFundsNotice}
          />
          {deletedAccountStatus && privyAuthenticated ? (
            <DeletedAccountGate
              account={deletedAccountStatus}
              action={deletedAccountAction}
              error={deletedAccountError}
              onReactivate={() => {
                void reactivateDeletedAccount();
              }}
              onReturnToLogin={() => {
                void returnDeletedAccountToLogin();
              }}
            />
          ) : appleRecovery && privyAuthenticated ? (
            <AppleRecoveryGate
              recovery={appleRecovery}
              action={appleRecoveryAction}
              error={appleRecoveryError}
              onUseExistingAccount={() => {
                void returnAppleRecoveryToLogin();
              }}
              onContinueWithApple={() => {
                void continueWithAppleAccount();
              }}
            />
          ) : (
            <>
              {children}
              {policyStatus && privyAuthenticated && !allowPolicyRoute ? (
                <PolicyAcceptanceOverlay
                  key={policyStatus.user_id}
                  action={policyAction}
                  error={policyError}
                  onAccept={(mailingListOptIn) => {
                    void acceptPolicies(mailingListOptIn);
                  }}
                  onReturnToLogin={() => {
                    void returnPolicyGateToLogin();
                  }}
                />
              ) : null}
            </>
          )}
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

function PolicyAcceptanceOverlay({
  action,
  error,
  onAccept,
  onReturnToLogin,
}: {
  action: "idle" | "submitting" | "returning";
  error: string;
  onAccept: (mailingListOptIn: boolean) => void;
  onReturnToLogin: () => void;
}) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [acceptedPrivacyPolicy, setAcceptedPrivacyPolicy] = useState(false);
  const [mailingListOptIn, setMailingListOptIn] = useState(true);
  const busy = action !== "idle";
  const returnTo = buildPolicyReturnTo(pathname, searchParams);
  const privacyPolicyHref = buildPolicyPageHref(PRIVACY_POLICY_PATH, returnTo);
  const emailOptInPolicyHref = buildPolicyPageHref(
    EMAIL_OPT_IN_POLICY_PATH,
    returnTo,
  );

  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/55 px-4 py-6 backdrop-blur-[2px]">
      <div className="w-full max-w-2xl rounded-3xl border border-border/70 bg-card/95 p-6 shadow-[0_1px_3px_hsl(var(--foreground)/0.08),0_24px_60px_hsl(var(--foreground)/0.16)] sm:p-8">
        <div className="space-y-4">
          <p className="text-sm font-semibold uppercase tracking-[0.24em] text-[#eb6c6c]">
            Privacy Policy
          </p>
          <h2 className="text-3xl font-semibold tracking-tight text-foreground">
            Accept the Privacy Policy to keep using SFLuv
          </h2>
          <p className="text-sm leading-6 text-muted-foreground sm:text-base">
            To use the app, You need to review and accept the{" "}
            <Link
              href={privacyPolicyHref}
              target="_blank"
              rel="noreferrer"
              className="font-semibold text-foreground underline underline-offset-4"
            >
              Privacy Policy
            </Link>
            . You can also choose whether to opt in to SFLuv email updates under
            the{" "}
            <Link
              href={emailOptInPolicyHref}
              target="_blank"
              rel="noreferrer"
              className="font-semibold text-foreground underline underline-offset-4"
            >
              Email Opt-In Policy
            </Link>
            .
          </p>

          <div className="space-y-3 rounded-2xl border border-border/70 bg-muted/30 p-4">
            <label className="flex items-start gap-3">
              <Checkbox
                checked={acceptedPrivacyPolicy}
                disabled={busy}
                onCheckedChange={(checked) =>
                  setAcceptedPrivacyPolicy(Boolean(checked))
                }
              />
              <span className="text-sm leading-6 text-foreground">
                I have read and accept the{" "}
                <Link
                  href={privacyPolicyHref}
                  target="_blank"
                  rel="noreferrer"
                  className="font-semibold underline underline-offset-4"
                >
                  Privacy Policy
                </Link>
                .
              </span>
            </label>

            <label className="flex items-start gap-3">
              <Checkbox
                checked={mailingListOptIn}
                disabled={busy}
                onCheckedChange={(checked) =>
                  setMailingListOptIn(Boolean(checked))
                }
              />
              <span className="text-sm leading-6 text-foreground">
                I want to receive SFLuv emails in line with the{" "}
                <Link
                  href={emailOptInPolicyHref}
                  target="_blank"
                  rel="noreferrer"
                  className="font-semibold underline underline-offset-4"
                >
                  Email Opt-In Policy
                </Link>
                .
              </span>
            </label>
          </div>

          <p className="text-sm text-muted-foreground">
            The Privacy Policy checkbox is required. Email opt-in is optional and
            You can unsubscribe later at any time.
          </p>

          {error ? (
            <p className="rounded-2xl border border-red-400/40 bg-red-100/70 px-4 py-3 text-sm leading-6 text-red-900 dark:bg-red-500/10 dark:text-red-100">
              {error}
            </p>
          ) : null}
        </div>

        <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:justify-end">
          <Button
            type="button"
            variant="outline"
            size="lg"
            className="w-full sm:w-auto sm:min-w-[190px]"
            disabled={busy}
            onClick={onReturnToLogin}
          >
            {action === "returning" ? "Logging out..." : "Log out"}
          </Button>
          <Button
            type="button"
            size="lg"
            className="w-full sm:w-auto sm:min-w-[220px]"
            disabled={busy || !acceptedPrivacyPolicy}
            onClick={() => onAccept(mailingListOptIn)}
          >
            {action === "submitting" ? "Saving..." : "Continue"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function DeletedAccountGate({
  account,
  action,
  error,
  onReactivate,
  onReturnToLogin,
}: {
  account: AccountDeletionStatusResponse;
  action: "idle" | "reactivating" | "returning";
  error: string;
  onReactivate: () => void;
  onReturnToLogin: () => void;
}) {
  const deleteDateLabel = formatDeletionDate(account.delete_date);
  const deleteWindowLabel =
    deleteDateLabel ||
    "the end of the current 30-day deletion window";
  const busy = action !== "idle";

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(235,108,108,0.18),_transparent_45%),linear-gradient(180deg,_hsl(var(--background))_0%,_hsl(var(--background))_100%)] px-4 py-10 sm:px-6">
      <div className="mx-auto flex min-h-[calc(100vh-5rem)] max-w-2xl items-center justify-center">
        <div className="w-full rounded-3xl border border-border/70 bg-card/95 p-8 shadow-[0_1px_3px_hsl(var(--foreground)/0.08),0_24px_60px_hsl(var(--foreground)/0.16)] sm:p-10">
          <div className="space-y-4">
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-[#eb6c6c]">
              Account Recovery
            </p>
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">
              This account has been recently deleted. Do you want to
              re-activate it?
            </h1>
            <p className="text-sm leading-6 text-muted-foreground sm:text-base">
              The account is currently scheduled for permanent deletion on{" "}
              <span className="font-semibold text-foreground">
                {deleteWindowLabel}
              </span>
              . If you reactivate it now, your profile and wallets will become
              active again.
            </p>
            {account.status === "ready_for_manual_purge" ? (
              <p className="rounded-2xl border border-amber-400/40 bg-amber-100/60 px-4 py-3 text-sm leading-6 text-amber-900 dark:bg-amber-500/10 dark:text-amber-100">
                This account is already at the end of its deletion window. If
                reactivation fails, it may need a manual restore.
              </p>
            ) : null}
            {error ? (
              <p className="rounded-2xl border border-red-400/40 bg-red-100/70 px-4 py-3 text-sm leading-6 text-red-900 dark:bg-red-500/10 dark:text-red-100">
                {error}
              </p>
            ) : null}
          </div>

          <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:justify-end">
            <Button
              type="button"
              variant="outline"
              size="lg"
              className="w-full sm:w-auto sm:min-w-[190px]"
              disabled={busy}
              onClick={onReturnToLogin}
            >
              {action === "returning" ? "Returning..." : "No, take me back"}
            </Button>
            <Button
              type="button"
              size="lg"
              className="w-full sm:w-auto sm:min-w-[190px]"
              disabled={busy}
              onClick={onReactivate}
            >
              {action === "reactivating"
                ? "Re-activating..."
                : "Yes, re-activate it"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function AppleRecoveryGate({
  recovery,
  action,
  error,
  onUseExistingAccount,
  onContinueWithApple,
}: {
  recovery: AppleRecoveryResponse;
  action: "idle" | "continuing" | "returning";
  error: string;
  onUseExistingAccount: () => void;
  onContinueWithApple: () => void;
}) {
  const prompt = describeAppleRecoveryPrompt(recovery);
  const busy = action !== "idle";
  const existingWallet =
    recovery.suggested_existing_account?.primary_wallet_address?.trim() || "";

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(235,108,108,0.18),_transparent_45%),linear-gradient(180deg,_hsl(var(--background))_0%,_hsl(var(--background))_100%)] px-4 py-10 sm:px-6">
      <div className="mx-auto flex min-h-[calc(100vh-5rem)] max-w-2xl items-center justify-center">
        <div className="w-full rounded-3xl border border-border/70 bg-card/95 p-8 shadow-[0_1px_3px_hsl(var(--foreground)/0.08),0_24px_60px_hsl(var(--foreground)/0.16)] sm:p-10">
          <div className="space-y-4">
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-[#eb6c6c]">
              Apple Sign-In
            </p>
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">
              {prompt.title}
            </h1>
            <p className="text-sm leading-6 text-muted-foreground sm:text-base">
              {prompt.body}
            </p>
            {existingWallet ? (
              <p className="rounded-2xl border border-border bg-muted/40 px-4 py-3 text-sm leading-6 text-foreground">
                Existing wallet: {existingWallet}
              </p>
            ) : null}
            {error ? (
              <p className="rounded-2xl border border-red-400/40 bg-red-100/70 px-4 py-3 text-sm leading-6 text-red-900 dark:bg-red-500/10 dark:text-red-100">
                {error}
              </p>
            ) : null}
          </div>

          <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:justify-end">
            <Button
              type="button"
              variant="outline"
              size="lg"
              className="w-full sm:w-auto sm:min-w-[220px]"
              disabled={busy}
              onClick={onUseExistingAccount}
            >
              {action === "returning" ? "Returning..." : prompt.primaryLabel}
            </Button>
            <Button
              type="button"
              size="lg"
              className="w-full sm:w-auto sm:min-w-[220px]"
              disabled={busy}
              onClick={onContinueWithApple}
            >
              {action === "continuing"
                ? "Continuing..."
                : prompt.secondaryLabel}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function RecoveryFundsNoticeDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          onClose();
        }
      }}
    >
      <DialogContent className="w-[95vw] max-w-md">
        <DialogHeader className="space-y-2">
          <DialogTitle>Funds recovery</DialogTitle>
          <DialogDescription>
            Your account is active again, but transferred funds do not return
            automatically.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-sm text-muted-foreground">
          <p>
            Any SFLuv transferred out during the deletion request will need to
            be restored manually.
          </p>
          <p>
            Contact{" "}
            <a
              className="font-semibold text-foreground underline underline-offset-4"
              href={`mailto:${ACCOUNT_RECOVERY_SUPPORT_EMAIL}`}
            >
              {ACCOUNT_RECOVERY_SUPPORT_EMAIL}
            </a>{" "}
            to recover your funds.
          </p>
        </div>
        <div className="flex justify-end">
          <Button
            type="button"
            onClick={onClose}
          >
            Understood
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
