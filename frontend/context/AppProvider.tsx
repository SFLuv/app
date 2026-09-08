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
import { createContext, Dispatch, ReactNode, SetStateAction, useContext, useEffect, useMemo, useState, useRef, useCallback } from "react";
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
import {
  BACKEND,
  IDLE_TIMER_PROMPT_SECONDS,
  IDLE_TIMER_SECONDS,
} from "@/lib/constants";
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
  AccountDeletionStatusResponse,
  AccountType,
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
import { Loader2, Store, User as UserIcon } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { LucideIcon } from "lucide-react";
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
import { useChainConfig } from "@/context/ChainConfigProvider";
import { isChromeFreeRoute } from "@/lib/app-chrome";
import { MERCHANT_ONBOARDING_PATH } from "@/lib/merchant-onboarding";

// const mockUser: User = { id: "user3", name: "Bob Johnson", email: "bob@example.com", isMerchant: true, isAdmin: false, isOrganizer: false }
export type UserStatus = "loading" | "authenticated" | "unauthenticated";
export type WalletsStatus = "loading" | "available" | "unavailable";
export type MerchantApprovalStatus = "pending" | "approved" | "rejected" | null;

/**
 * The backend's answer to "can this merchant account go back to being a
 * personal one". Any live listing blocks it, in any of the three states; an
 * application that has not been approved can be withdrawn to clear the way.
 */
export interface MerchantRevertEligibility {
  account_type: AccountType;
  approved_locations: number;
  pending_locations: number;
  /** Refused applications block too, and are withdrawn the same way. */
  rejected_locations: number;
  can_revert: boolean;
}

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
  /** The signup answer. isMerchant is a different question — see AccountType. */
  accountType: AccountType;
  /**
   * When the person was actually asked which kind of account this is. Null
   * means nobody ever was: only the web signup puts the question, so a null
   * here is how the web app recognises an account created on the mobile app.
   */
  accountTypeSelectedAt?: string | null;
  /** Stamped once the web app has offered such an account the merchant option. */
  webMerchantPromptSeenAt?: string | null;
  /** Stamped when a merchant lists their first shop; null while they still owe us one. */
  merchantOnboardingCompletedAt?: string | null;
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
  /**
   * The client-side mirror of the backend's read-only gate: a self-declared
   * merchant who has not listed a shop yet. It exists so the app stops offering
   * writes that would come back as 403 merchant-onboarding-required, not to be
   * the thing enforcing them — the server refuses those calls either way.
   */
  merchantOnboardingRequired: boolean;
  /** Pulls the authenticated user record — including locations — right now,
   *  using the same routine the background poll runs. */
  refreshUserRecord: () => Promise<void>;
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
  /**
   * True when the web app should offer this account the merchant option once.
   * It is the mobile-signup case only: somebody who was never asked at signup,
   * is on a personal account, and has not been offered it here before.
   */
  showWebMerchantPrompt: boolean;
  /** Records that the one-time offer has been made, so it is never made again. */
  dismissWebMerchantPrompt: () => Promise<void>;
  /**
   * Switches this account between personal and merchant from the settings
   * screen. Rejects with the backend's message when a merchant account still
   * has locations, which is what makes the choice one-way.
   */
  setOwnAccountType: (accountType: AccountType) => Promise<void>;
  /** What is standing between a merchant account and a personal one, if anything. */
  getMerchantRevertEligibility: () => Promise<MerchantRevertEligibility>;

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
const MERCHANT_ONBOARDING_REQUIRED_REASON = "merchant-onboarding-required";

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

export default function AppProvider({ children }: { children: ReactNode }) {
  const chainConfig = useChainConfig();
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
  // Set by the background refresher below. A ref rather than state so wiring it
  // up cannot itself trigger a render loop.
  const refreshUserRecordRef = useRef<(() => Promise<void>) | null>(null);
  const refreshUserRecord = useCallback(async () => {
    await refreshUserRecordRef.current?.();
  }, []);
  const [status, setStatus] = useState<UserStatus>("loading");
  const [tx, setTx] = useState<TxState>(defaultTxState);
  const [error, setError] = useState<string | unknown | null>(null);
  const [deletedAccountStatus, setDeletedAccountStatus] =
    useState<AccountDeletionStatusResponse | null>(null);
  const [deletedAccountAction, setDeletedAccountAction] = useState<
    "idle" | "reactivating" | "returning"
  >("idle");
  const [deletedAccountError, setDeletedAccountError] = useState("");
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
  const merchantOnboardingRequired =
    status === "authenticated" &&
    user?.accountType === "merchant" &&
    !user?.merchantOnboardingCompletedAt;
  // Which account this session has already sent to the form, so it is sent once
  // and not again. A ref rather than state: nothing renders from it, and it must
  // not itself cause the render that would re-run the effect.
  const merchantFormOfferedForRef = useRef<string | null>(null);
  /**
   * Covers the gap between choosing a merchant account and landing on the form.
   *
   * Those are two separate awaits — the profile reloads, then an effect
   * navigates — and in between the app is authenticated with a route that is
   * still the map. Without this the merchant watches the map paint and vanish
   * on their way to a form they already asked for.
   *
   * State rather than a derived value because it has to outlast the render that
   * starts the navigation: by then the ref below is set and every derived
   * condition has already gone false, while the route has not changed yet.
   */
  const [merchantRedirectPending, setMerchantRedirectPending] = useState(false);

  // There is no wall on the web app any more.
  //
  // A merchant who has not listed a shop can look around like anybody else, and
  // is not thrown back to the form on every sign-in. The form is reached from
  // Locations, which is where a merchant's shops live, and leaving it returns
  // them there.
  //
  // The server-side gate is untouched and still refuses writes for such an
  // account — merchantOnboardingRequired above is its mirror, and the screens
  // that offer a write consult it. Reading was always allowed; walling the
  // whole app was the client's own decision, and it made "look around" mean
  // "look at this one form".
  //
  // Mobile is the opposite and deliberately so: a phone is a till, not a place
  // to browse, so a merchant account there sees an onboarding flow or the till
  // and nothing else.

  /**
   * Takes a merchant with nothing listed to the form, once per sign-in.
   *
   * A navigation, not the wall this replaced: they arrive on the form because
   * it is what they signed up to do, and Cancel leaves for Locations and stays
   * left. The wall re-asserted itself on every path change, which is what made
   * "look around" mean "look at this one form".
   *
   * Fires after status is authenticated, which is set immediately after the
   * locations land — so "has nothing listed" is never read off an empty list
   * that simply has not loaded. It also covers the signup flow without a second
   * mechanism: choosing merchant reloads the profile, and the account arrives
   * here with no locations.
   *
   * Zero locations rather than the onboarding stamp, because a merchant who
   * withdrew their only application has a stamp and nothing listed, and is in
   * exactly the state this exists for.
   */
  useEffect(() => {
    if (status !== "authenticated") return;
    // Every path from here that does not navigate has to lower the cover, or a
    // merchant who turns out not to need the form waits on a spinner for a
    // navigation that is never coming.
    if (!user || user.accountType !== "merchant") {
      setMerchantRedirectPending(false);
      return;
    }
    if (userLocations.length > 0) {
      setMerchantRedirectPending(false);
      return;
    }
    if (merchantFormOfferedForRef.current === user.id) {
      setMerchantRedirectPending(false);
      return;
    }

    // Already there, or somewhere they went deliberately — the account exits and
    // the policy texts. Marked as offered either way, so leaving one of those
    // pages later does not spring the redirect on them.
    if (
      pathname.startsWith(MERCHANT_ONBOARDING_PATH) ||
      isChromeFreeRoute(pathname) ||
      allowPolicyRoute
    ) {
      merchantFormOfferedForRef.current = user.id;
      setMerchantRedirectPending(false);
      return;
    }

    merchantFormOfferedForRef.current = user.id;
    setMerchantRedirectPending(true);
    replace(MERCHANT_ONBOARDING_PATH);
  }, [allowPolicyRoute, pathname, replace, status, user, userLocations.length]);

  // The cover comes down when the form is actually on screen, not when the
  // navigation was requested.
  useEffect(() => {
    if (pathname.startsWith(MERCHANT_ONBOARDING_PATH)) {
      setMerchantRedirectPending(false);
    }
  }, [pathname]);

  // A cover that can outlive its navigation is the same infinite spinner as any
  // other, so it expires on its own. Long enough that a slow route change is
  // never cut short, short enough that a stuck one is a hesitation rather than
  // a dead end.
  useEffect(() => {
    if (!merchantRedirectPending) return;
    const timer = setTimeout(() => setMerchantRedirectPending(false), 8000);
    return () => clearTimeout(timer);
  }, [merchantRedirectPending]);

  const clearAuthenticatedState = (options?: {
    clearDeletedAccount?: boolean;
    redirectToMap?: boolean;
  }) => {
    const clearDeletedAccount = options?.clearDeletedAccount ?? true;
    const redirectToMap = options?.redirectToMap ?? true;
    const allowUnauthedRoute =
      pathname === "/map" ||
      pathname === "/update" ||
      pathname === "/mcp/authorize" ||
      pathname === "/organization/join" ||
      pathname === "/redirect" ||
      pathname === "/delete-account" ||
      pathname === "/recovery" ||
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
    setPendingAppleTokens(null);
    setAppleLinkBusy(false);
    setAppleLinkMessage("");
    setPolicyStatus(null);
    setPolicyAction("idle");
    setPolicyError("");
    // Cleared so signing back in routes to onboarding again rather than
    // remembering that this account was already sent there once.
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

    if (policyStatus) {
      return;
    }

    _userLogin();
  }, [
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
      // Anything that is not an explicit "merchant" reads as a regular
      // account. A backend too old to send the field, or a field that arrives
      // malformed, must not be able to lock somebody into onboarding.
      accountType: r.user.account_type === "merchant" ? "merchant" : "regular",
      accountTypeSelectedAt: r.user.account_type_selected_at ?? null,
      webMerchantPromptSeenAt: r.user.web_merchant_prompt_seen_at ?? null,
      merchantOnboardingCompletedAt: r.user.merchant_onboarding_completed_at,
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
        await _postUser();
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
    merchantFormOfferedForRef.current = null;
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
    accountType: AccountType,
  ): Promise<UserPolicyStatusResponse> => {
    const res = await rawAuthFetch("/users/policies/accept", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        accepted_privacy_policy: true,
        mailing_list_opt_in: mailingListOptIn,
        account_type: accountType,
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

    if (
      response.status === 403 &&
      response.headers.get(POLICY_REQUIRED_HEADER) ===
        MERCHANT_ONBOARDING_REQUIRED_REASON
    ) {
      // Reaching this means the app offered an action the gate was always
      // going to refuse, so what it is rendering from is stale. Reload the
      // profile rather than leave the same button sitting there.
      void refreshUserRecord();
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

    // Expose the same routine the poll uses, so an explicit refetch after a
    // save and the background update cannot drift apart.
    refreshUserRecordRef.current = refreshAuthenticatedUserRecord;

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

        cResults.push(privyWallet.switchChain(chainConfig.chainId));

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
      console.error("error initializing wallets:", error);
      throw new Error(
        `error initializing wallets: ${error instanceof Error ? error.message : String(error)}`,
      );
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
    const w = new AppWallet(privyWallet, eoaName, chainConfig, {
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

    const w = new AppWallet(privyWallet, smartWalletName, chainConfig, {
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
      await primaryPrivyWallet.switchChain(chainConfig.chainId);
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

  const acceptPolicies = async (
    mailingListOptIn: boolean,
    accountType: AccountType,
  ) => {
    setPolicyAction("submitting");
    setPolicyError("");
    try {
      await _acceptUserPolicies(mailingListOptIn, accountType);
      // Raised before the overlay closes, so there is no frame in which the
      // page behind it is uncovered. The redirect effect lowers it — either by
      // reaching the form, or by deciding this account does not need it.
      if (accountType === "merchant") {
        setMerchantRedirectPending(true);
      }
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

  /**
   * The one-time merchant offer for somebody who signed up on the mobile app.
   *
   * Three conditions, and all three matter. account_type_selected_at is null
   * only for an account nobody ever asked — the web signup always asks — which
   * is what makes this the mobile-signup case rather than "any personal
   * account". The account must still be a personal one, and the offer must not
   * have been made before. From then on the option lives in settings, where the
   * person can go looking for it rather than being asked again.
   */
  const showWebMerchantPrompt =
    status === "authenticated" &&
    user?.accountType === "regular" &&
    !user?.accountTypeSelectedAt &&
    !user?.webMerchantPromptSeenAt;

  const dismissWebMerchantPrompt = async () => {
    // Optimistic, and deliberately so: the prompt closing is the whole point,
    // and a failed stamp costs at most one more offer on the next sign-in.
    setUser((current) =>
      current
        ? { ...current, webMerchantPromptSeenAt: new Date().toISOString() }
        : current,
    );
    try {
      await authFetch("/users/web-merchant-prompt-seen", { method: "POST" });
    } catch (error) {
      console.error("Unable to record the merchant prompt as seen", error);
    }
  };

  const getMerchantRevertEligibility =
    async (): Promise<MerchantRevertEligibility> => {
      const res = await authFetch("/users/account-type/revert-eligibility");
      if (!res.ok) throw new Error("Unable to check this account right now.");
      return (await res.json()) as MerchantRevertEligibility;
    };

  const setOwnAccountType = async (accountType: AccountType) => {
    const res = await authFetch("/users/account-type", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ account_type: accountType }),
    });
    if (!res.ok) {
      // The 409 carries why — an approved listing, or applications still in the
      // queue — and that is the only thing worth telling the person, so it is
      // surfaced rather than replaced with a generic failure.
      let message = "Unable to change this account type right now.";
      try {
        const body = await res.json();
        if (body && typeof body.error === "string" && body.error.trim()) {
          message = body.error.trim();
        }
      } catch {
        // not a JSON body
      }
      throw new Error(message);
    }

    // Re-read rather than patching locally: the switch also stamps the two
    // account-type signals, and the merchant wall keys off account_type, so the
    // app must be looking at the server's version of all three at once.
    await refreshUserRecord();
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
          merchantOnboardingRequired,
          refreshUserRecord,
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
          showWebMerchantPrompt,
          dismissWebMerchantPrompt,
          setOwnAccountType,
          getMerchantRevertEligibility,
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
          ) : (
            <>
              {children}
              {/* Opaque, not a scrim: the point is that the map behind it is
                  not seen at all. Above the policy overlay's z-[80] so the
                  handover between the two never shows a seam. */}
              {merchantRedirectPending ? (
                <div className="fixed inset-0 z-[90] flex items-center justify-center bg-background">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              ) : null}
              {policyStatus && privyAuthenticated && !allowPolicyRoute ? (
                <PolicyAcceptanceOverlay
                  key={policyStatus.user_id}
                  action={policyAction}
                  error={policyError}
                  onAccept={(mailingListOptIn, accountType) => {
                    void acceptPolicies(mailingListOptIn, accountType);
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

const ACCOUNT_TYPE_OPTIONS: {
  value: AccountType;
  title: string;
  icon: LucideIcon;
  /** Terse, and shown on hover rather than under the title. */
  hint: string;
}[] = [
  {
    value: "regular",
    title: "Personal",
    icon: UserIcon,
    hint: "Spend and receive SFLuv in your own wallet.",
  },
  {
    value: "merchant",
    title: "Merchant",
    icon: Store,
    // The one consequence somebody cannot discover by clicking, so it is the
    // one thing the hint spends its words on.
    hint: "List your business. Permanent once a location is approved.",
  },
];

function PolicyAcceptanceOverlay({
  action,
  error,
  onAccept,
  onReturnToLogin,
}: {
  action: "idle" | "submitting" | "returning";
  error: string;
  onAccept: (mailingListOptIn: boolean, accountType: AccountType) => void;
  onReturnToLogin: () => void;
}) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [acceptedPrivacyPolicy, setAcceptedPrivacyPolicy] = useState(false);
  const [mailingListOptIn, setMailingListOptIn] = useState(true);
  // Starts unanswered rather than defaulting to "regular". The backend writes
  // this once and will not take a correction from this route, so somebody who
  // skims past the question has to be stopped here — a pre-ticked answer would
  // be a decision made on their behalf.
  const [accountType, setAccountType] = useState<AccountType | null>(null);
  // Two views, one submission. The policies and the account-type question are
  // different kinds of decision — one is a document to read and agree to, the
  // other is a choice about what this account is for — and on a single screen
  // the second was furniture around the first. Nothing is sent until both are
  // answered, so stepping back costs nothing.
  const [step, setStep] = useState<"policies" | "account-type">("policies");
  const busy = action !== "idle";
  const returnTo = buildPolicyReturnTo(pathname, searchParams);
  const privacyPolicyHref = buildPolicyPageHref(PRIVACY_POLICY_PATH, returnTo);
  const emailOptInPolicyHref = buildPolicyPageHref(
    EMAIL_OPT_IN_POLICY_PATH,
    returnTo,
  );

  const policiesStep = step === "policies";

  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/55 px-4 py-6 backdrop-blur-[2px]">
      <div className="max-h-full w-full max-w-2xl overflow-y-auto rounded-3xl border border-border/70 bg-card/95 p-6 shadow-[0_1px_3px_hsl(var(--foreground)/0.08),0_24px_60px_hsl(var(--foreground)/0.16)] sm:p-8">
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-[#eb6c6c]">
              {policiesStep ? "Privacy Policy" : "Account Type"}
            </p>
            <p className="text-xs font-medium text-muted-foreground">
              Step {policiesStep ? 1 : 2} of 2
            </p>
          </div>

          {policiesStep ? (
            <>
              <h2 className="text-3xl font-semibold tracking-tight text-foreground">
                Accept the Privacy Policy to keep using SFLuv
              </h2>

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

            </>
          ) : (
            <>
              <h2 className="text-3xl font-semibold tracking-tight text-foreground">
                What kind of account is this?
              </h2>

              {/* Icon, title, and nothing else on the face of the tile. What a
                  merchant account costs you is real but is not something to
                  read past on the way to a two-option choice, so it is on the
                  hint rather than under the title. */}
              <TooltipProvider delayDuration={200}>
                <div
                  role="radiogroup"
                  aria-label="Account type"
                  className="grid gap-3 sm:grid-cols-2"
                >
                  {ACCOUNT_TYPE_OPTIONS.map((option) => {
                    const selected = accountType === option.value;
                    const Icon = option.icon;
                    return (
                      <Tooltip key={option.value}>
                        <TooltipTrigger asChild>
                          <button
                            type="button"
                            role="radio"
                            aria-checked={selected}
                            aria-label={`${option.title} — ${option.hint}`}
                            disabled={busy}
                            onClick={() => setAccountType(option.value)}
                            className={`flex flex-col items-center gap-3 rounded-2xl border p-6 transition-colors disabled:opacity-60 ${
                              selected
                                ? "border-[#eb6c6c] bg-[#eb6c6c]/10"
                                : "border-border/70 bg-muted/30 hover:border-[#eb6c6c]/60"
                            }`}
                          >
                            <Icon
                              className={`h-8 w-8 ${selected ? "text-[#eb6c6c]" : "text-muted-foreground"}`}
                              strokeWidth={1.5}
                            />
                            <span className="text-sm font-semibold text-foreground">
                              {option.title}
                            </span>
                          </button>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-[15rem]">
                          {option.hint}
                        </TooltipContent>
                      </Tooltip>
                    );
                  })}
                </div>
              </TooltipProvider>

              {/* A failure can only come back from the submission, which only
                  this step can make — showing it on the first step would put an
                  error over a form nobody had touched yet. */}
              {error ? (
                <p className="rounded-2xl border border-red-400/40 bg-red-100/70 px-4 py-3 text-sm leading-6 text-red-900 dark:bg-red-500/10 dark:text-red-100">
                  {error}
                </p>
              ) : null}
            </>
          )}
        </div>

        <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:justify-end">
          <Button
            type="button"
            variant="outline"
            size="lg"
            className="w-full sm:w-auto sm:min-w-[190px]"
            disabled={busy}
            onClick={() => {
              if (policiesStep) {
                onReturnToLogin();
                return;
              }
              setStep("policies");
            }}
          >
            {policiesStep
              ? action === "returning"
                ? "Logging out..."
                : "Log out"
              : "Back"}
          </Button>
          <Button
            type="button"
            size="lg"
            className="w-full sm:w-auto sm:min-w-[220px]"
            disabled={
              busy ||
              (policiesStep ? !acceptedPrivacyPolicy : accountType === null)
            }
            onClick={() => {
              if (policiesStep) {
                setStep("account-type");
                return;
              }
              if (accountType === null) return;
              onAccept(mailingListOptIn, accountType);
            }}
          >
            {policiesStep
              ? "Continue"
              : action === "submitting"
                ? "Saving..."
                : "Finish"}
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
