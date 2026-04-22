"use client";

import type React from "react";

import { useEffect, useMemo, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useApp } from "@/context/AppProvider";
import PlaceAutocomplete from "@/components/merchant/google_place_finder";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Check,
  ChevronDown,
  Loader2,
  Upload,
  User,
  Clock,
  XCircle,
  AlertTriangle,
} from "lucide-react";
import {
  AccountDeletionPreview,
  AccountDeletionStatusResponse,
  UserResponse,
  VerifiedEmailResponse,
  WalletResponse,
} from "@/types/server";
import { AuthedLocation } from "@/types/location";
import { GoogleSubLocation } from "@/types/location";
import { ensureGooglePlacesScript, hasGoogleMapsPlaces } from "@/lib/google-places";
import { sweepSFLUVBalancesToAdmin } from "@/lib/account-deletion";
import { getAddress, isAddress } from "viem";

type MerchantStatus = "approved" | "pending" | "rejected" | "none";
type LocationApplicationStatus = "approved" | "pending" | "rejected";
const CUSTOM_REWARDS_ACCOUNT_VALUE = "__custom__";
const CUSTOM_WALLET_VALUE = "__custom_wallet__";
const NONE_WALLET_VALUE = "__none__";
const SELECT_WALLET_PLACEHOLDER_VALUE = "__select_wallet__";

type MerchantLocationWalletDraft = {
  paymentWalletAddresses: string[];
  defaultPaymentWalletAddress: string;
  paymentAddSelection: string;
  paymentAddCustomAddress: string;
  tippingWalletSelection: string;
  tippingWalletCustomAddress: string;
  saving: boolean;
  error: string;
  success: string;
};

type MerchantLocationProfileDraft = {
  googleId: string;
  name: string;
  description: string;
  type: string;
  street: string;
  city: string;
  state: string;
  zip: string;
  lat: number;
  lng: number;
  phone: string;
  website: string;
  imageUrl: string;
  rating: number;
  mapsPage: string;
  openingHours: string[];
  dirty: boolean;
  saving: boolean;
  error: string;
  success: string;
};

const getLocationApplicationStatus = (
  approval?: boolean | null,
): LocationApplicationStatus => {
  if (approval === true) return "approved";
  if (approval === false) return "rejected";
  return "pending";
};

const formatDeletionDate = (value?: string | null): string | null => {
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
};

const getDeletionFallbackDate = () =>
  formatDeletionDate(
    new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
  ) || "30 days from now";
const ACCOUNT_RECOVERY_SUPPORT_EMAIL = "techsupport@sfluv.org";

function GoogleLogo({ className = "h-5 w-5" }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={className}
    >
      <path
        d="M21.805 12.227c0-.817-.073-1.602-.209-2.355H12v4.454h5.489a4.695 4.695 0 0 1-2.036 3.081v2.556h3.296c1.929-1.776 3.056-4.396 3.056-7.736Z"
        fill="#4285F4"
      />
      <path
        d="M12 22.182c2.754 0 5.062-.913 6.749-2.473l-3.296-2.556c-.914.613-2.082.975-3.453.975-2.655 0-4.906-1.793-5.71-4.204H2.883v2.636A10.18 10.18 0 0 0 12 22.182Z"
        fill="#34A853"
      />
      <path
        d="M6.29 13.924A6.119 6.119 0 0 1 5.97 12c0-.668.115-1.315.32-1.924V7.439H2.883A10.18 10.18 0 0 0 1.818 12c0 1.629.39 3.171 1.065 4.561l3.407-2.637Z"
        fill="#FBBC04"
      />
      <path
        d="M12 5.872c1.497 0 2.841.515 3.899 1.526l2.923-2.923C17.056 2.839 14.748 1.818 12 1.818a10.18 10.18 0 0 0-9.117 5.621l3.407 2.637c.804-2.411 3.055-4.204 5.71-4.204Z"
        fill="#EA4335"
      />
    </svg>
  );
}

function AppleLogo({ className = "h-5 w-5" }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={className}
      fill="currentColor"
    >
      <path d="M15.77 12.6c.03 3.02 2.65 4.03 2.68 4.05-.02.07-.41 1.42-1.35 2.81-.81 1.2-1.66 2.39-2.99 2.42-1.31.02-1.73-.78-3.23-.78s-1.97.76-3.2.8c-1.29.05-2.27-1.29-3.09-2.48-1.68-2.43-2.96-6.86-1.24-9.84.85-1.48 2.37-2.42 4.02-2.45 1.25-.03 2.44.84 3.2.84.76 0 2.18-1.03 3.67-.88.62.03 2.37.25 3.49 1.89-.09.06-2.08 1.21-2.06 3.62Zm-2.08-7.96c.68-.82 1.14-1.97 1.02-3.11-.98.04-2.16.65-2.86 1.47-.63.73-1.18 1.9-1.03 3.01 1.09.08 2.19-.56 2.87-1.37Z" />
    </svg>
  );
}

export default function SettingsPage() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const {
    user,
    userLocations,
    setUserLocations,
    status,
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
    updateUser,
    authFetch,
    wallets,
    refreshWallets,
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
    appleLinkBusy,
    appleLinkMessage,
    canDisconnectApple,
    appleDisconnectDisabledReason,
    linkApple,
    unlinkApple,
  } = useApp();
  const userRole = useMemo(
    () => (user?.isAdmin ? "admin" : user?.isMerchant ? "merchant" : "user"),
    [user],
  );

  const merchantStatus: MerchantStatus = useMemo(() => {
    if (userLocations.length === 0) return "none";
    if (
      userLocations.some(
        (loc) => getLocationApplicationStatus(loc.approval) === "approved",
      )
    )
      return "approved";
    if (
      userLocations.some(
        (loc) => getLocationApplicationStatus(loc.approval) === "pending",
      )
    )
      return "pending";
    return "rejected";
  }, [userLocations]);

  const sortedUserLocations = useMemo(() => {
    return [...userLocations].sort((a, b) => b.id - a.id);
  }, [userLocations]);

  const approvedMerchantLocations = useMemo(
    () =>
      sortedUserLocations.filter(
        (location) => getLocationApplicationStatus(location.approval) === "approved",
      ),
    [sortedUserLocations],
  );

  const reviewMerchantLocations = useMemo(
    () =>
      sortedUserLocations.filter(
        (location) => getLocationApplicationStatus(location.approval) !== "approved",
      ),
    [sortedUserLocations],
  );

  const affiliateStatus = useMemo(() => {
    if (user?.isAffiliate) return "approved";
    if (affiliate?.status) return affiliate.status;
    return "none";
  }, [affiliate, user]);

  const proposerStatus = useMemo(() => {
    if (user?.isProposer) return "approved";
    if (proposer?.status) return proposer.status;
    return "none";
  }, [proposer, user]);

  const improverStatus = useMemo(() => {
    if (user?.isImprover) return "approved";
    if (improver?.status) return improver.status;
    return "none";
  }, [improver, user]);

  const issuerStatus = useMemo(() => {
    if (user?.isIssuer) return "approved";
    if (issuer?.status) return issuer.status;
    return "none";
  }, [issuer, user]);

  const supervisorStatus = useMemo(() => {
    if (user?.isSupervisor) return "approved";
    if (supervisor?.status) return supervisor.status;
    return "none";
  }, [supervisor, user]);

  useEffect(() => {
    if (affiliate?.affiliate_logo) {
      setAffiliateLogoPreview(affiliate.affiliate_logo);
    } else {
      setAffiliateLogoPreview("");
    }
  }, [affiliate?.affiliate_logo]);

  // Form states
  const tabFromQuery = searchParams.get("tab");
  const [activeTab, setActiveTab] = useState(tabFromQuery || "account");
  const [isUpdating, setIsUpdating] = useState(false);
  const [isChangingPassword, setIsChangingPassword] = useState(false);
  const [successMessage, setSuccessMessage] = useState("");
  type RoleRequestType =
    | "merchant"
    | "affiliate"
    | "proposer"
    | "improver"
    | "issuer"
    | "supervisor"
    | "";
  const [roleRequestType, setRoleRequestType] = useState<RoleRequestType>("");
  const [roleOrg, setRoleOrg] = useState("");
  const [roleEmail, setRoleEmail] = useState("");
  const [roleFirstName, setRoleFirstName] = useState("");
  const [roleLastName, setRoleLastName] = useState("");
  const [roleSubmitting, setRoleSubmitting] = useState(false);
  const [roleError, setRoleError] = useState("");
  const [roleSuccess, setRoleSuccess] = useState("");
  const [verifiedEmails, setVerifiedEmails] = useState<VerifiedEmailResponse[]>(
    [],
  );
  const [verifiedEmailsLoading, setVerifiedEmailsLoading] = useState(false);
  const [verifiedEmailFormOpen, setVerifiedEmailFormOpen] = useState(false);
  const [newVerifiedEmail, setNewVerifiedEmail] = useState("");
  const [verifiedEmailSubmitting, setVerifiedEmailSubmitting] = useState(false);
  const [verifiedEmailError, setVerifiedEmailError] = useState("");
  const [verifiedEmailSuccess, setVerifiedEmailSuccess] = useState("");
  const [affiliateLogoPreview, setAffiliateLogoPreview] = useState<string>("");
  const [affiliateLogoSaving, setAffiliateLogoSaving] = useState(false);
  const [affiliateLogoError, setAffiliateLogoError] = useState("");
  const [rewardsWallets, setRewardsWallets] = useState<WalletResponse[]>([]);
  const [rewardsWalletsLoading, setRewardsWalletsLoading] = useState(false);
  const [rewardsWalletsError, setRewardsWalletsError] = useState("");
  const [walletVisibilitySavingId, setWalletVisibilitySavingId] = useState<
    number | null
  >(null);
  const [walletVisibilityError, setWalletVisibilityError] = useState("");
  const [walletVisibilitySuccess, setWalletVisibilitySuccess] = useState("");
  const [primaryWalletSelection, setPrimaryWalletSelection] = useState("");
  const [primaryWalletCustomAddress, setPrimaryWalletCustomAddress] =
    useState("");
  const [primaryWalletSaving, setPrimaryWalletSaving] = useState(false);
  const [primaryWalletError, setPrimaryWalletError] = useState("");
  const [primaryWalletSuccess, setPrimaryWalletSuccess] = useState("");
  const [locationWalletDrafts, setLocationWalletDrafts] = useState<
    Record<number, MerchantLocationWalletDraft>
  >({});
  const [locationProfileDrafts, setLocationProfileDrafts] = useState<
    Record<number, MerchantLocationProfileDraft>
  >({});
  const [merchantLocationCardOpen, setMerchantLocationCardOpen] = useState<
    Record<number, boolean>
  >({});
  const [merchantPlacesReady, setMerchantPlacesReady] = useState(false);
  const [merchantPlacesLoadError, setMerchantPlacesLoadError] = useState("");

  const [improverRewardsSelection, setImproverRewardsSelection] = useState("");
  const [improverCustomRewardsAccount, setImproverCustomRewardsAccount] =
    useState("");
  const [improverRewardsSaving, setImproverRewardsSaving] = useState(false);
  const [improverRewardsError, setImproverRewardsError] = useState("");
  const [improverRewardsSuccess, setImproverRewardsSuccess] = useState("");

  const [supervisorRewardsSelection, setSupervisorRewardsSelection] =
    useState("");
  const [supervisorCustomRewardsAccount, setSupervisorCustomRewardsAccount] =
    useState("");
  const [supervisorRewardsSaving, setSupervisorRewardsSaving] = useState(false);
  const [supervisorRewardsError, setSupervisorRewardsError] = useState("");
  const [supervisorRewardsSuccess, setSupervisorRewardsSuccess] = useState("");
  const [deleteAccountDialogOpen, setDeleteAccountDialogOpen] = useState(false);
  const [deleteAccountPreview, setDeleteAccountPreview] =
    useState<AccountDeletionPreview | null>(null);
  const [deleteAccountLoading, setDeleteAccountLoading] = useState(false);
  const [deleteAccountSubmitting, setDeleteAccountSubmitting] = useState(false);
  const [deleteAccountPhase, setDeleteAccountPhase] = useState<
    "idle" | "sweeping" | "deleting"
  >("idle");
  const [deleteAccountError, setDeleteAccountError] = useState("");
  const noopGoogleSubLocationSetter: React.Dispatch<
    React.SetStateAction<GoogleSubLocation | null>
  > = () => undefined;

  // Account form
  const [name, setName] = useState(user?.name || "");

  // Password form
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordError, setPasswordError] = useState("");

  // Notification settings
  const [emailNotifications, setEmailNotifications] = useState(true);
  const [opportunityAlerts, setOpportunityAlerts] = useState(true);
  const [transactionAlerts, setTransactionAlerts] = useState(true);

  const verifiedEmailOptions = useMemo(
    () => verifiedEmails.filter((email) => email.status === "verified"),
    [verifiedEmails],
  );

  const pendingOrExpiredEmailOptions = useMemo(
    () => verifiedEmails.filter((email) => email.status !== "verified"),
    [verifiedEmails],
  );

  const availableTabs = useMemo(() => {
    const tabs = ["account"];
    if (merchantStatus !== "none") tabs.push("merchant");
    if (affiliateStatus === "pending" || affiliateStatus === "approved")
      tabs.push("affiliate");
    if (proposerStatus === "pending" || proposerStatus === "approved")
      tabs.push("proposer");
    if (improverStatus === "pending" || improverStatus === "approved")
      tabs.push("improver");
    if (issuerStatus === "pending" || issuerStatus === "approved")
      tabs.push("issuer");
    if (supervisorStatus === "pending" || supervisorStatus === "approved")
      tabs.push("supervisor");
    return tabs;
  }, [
    affiliateStatus,
    improverStatus,
    issuerStatus,
    merchantStatus,
    proposerStatus,
    supervisorStatus,
  ]);

  useEffect(() => {
    const tab = searchParams.get("tab");
    if (!tab) return;
    if (!availableTabs.includes(tab)) return;
    setActiveTab((prev) => (tab === prev ? prev : tab));
  }, [availableTabs, searchParams]);

  useEffect(() => {
    if (!availableTabs.includes(activeTab)) {
      setActiveTab("account");
      return;
    }

    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", activeTab);
    const nextQuery = params.toString();
    if (nextQuery !== searchParams.toString()) {
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, {
        scroll: false,
      });
    }
  }, [activeTab, availableTabs, pathname, router, searchParams]);

  const rewardsAccountOptions = useMemo(() => {
    const seen = new Set<string>();
    const options: Array<{
      value: string;
      label: string;
      isSmartWalletZero: boolean;
    }> = [];

    for (const wallet of rewardsWallets) {
      if (wallet.is_eoa) continue;
      const rawAddress = (wallet.smart_address || "").trim();
      if (!rawAddress || !isAddress(rawAddress)) continue;
      const normalizedAddress = getAddress(rawAddress);
      const key = normalizedAddress.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);

      const fallbackName =
        wallet.smart_index !== undefined
          ? `Smart Wallet ${wallet.smart_index + 1}`
          : "Smart Wallet";
      const displayName = (wallet.name || "").trim() || fallbackName;
      const shortAddress = `${normalizedAddress.slice(0, 6)}...${normalizedAddress.slice(-4)}`;
      options.push({
        value: normalizedAddress,
        label: `${displayName} (${shortAddress})`,
        isSmartWalletZero: wallet.is_eoa === false && wallet.smart_index === 0,
      });
    }

    return options;
  }, [rewardsWallets]);

  const defaultRewardsAccount = useMemo(() => {
    const currentPrimaryWallet = (user?.primaryWalletAddress || "").trim();
    if (currentPrimaryWallet && isAddress(currentPrimaryWallet)) {
      return getAddress(currentPrimaryWallet);
    }

    const primaryEoaAddress =
      rewardsWallets
        .filter((wallet) => wallet.is_eoa && typeof wallet.id === "number")
        .sort(
          (a, b) =>
            (a.id ?? Number.MAX_SAFE_INTEGER) -
            (b.id ?? Number.MAX_SAFE_INTEGER),
        )[0]
        ?.eoa_address?.trim()
        .toLowerCase() || "";

    if (primaryEoaAddress) {
      const preferredSmartWallet = rewardsWallets.find(
        (wallet) =>
          wallet.is_eoa === false &&
          wallet.smart_index === 0 &&
          wallet.eoa_address?.trim().toLowerCase() === primaryEoaAddress &&
          typeof wallet.smart_address === "string" &&
          isAddress(wallet.smart_address.trim()),
      );
      if (preferredSmartWallet?.smart_address) {
        return getAddress(preferredSmartWallet.smart_address.trim());
      }
    }

    const smartWalletZero = rewardsAccountOptions.find(
      (option) => option.isSmartWalletZero,
    );
    if (smartWalletZero) return smartWalletZero.value;
    return rewardsAccountOptions[0]?.value || "";
  }, [rewardsAccountOptions, rewardsWallets, user?.primaryWalletAddress]);

  const walletOptionLookup = useMemo(() => {
    const lookup = new Map<string, { value: string; label: string }>();
    for (const option of rewardsAccountOptions) {
      lookup.set(option.value.toLowerCase(), {
        value: option.value,
        label: option.label,
      });
    }
    return lookup;
  }, [rewardsAccountOptions]);

  const getWalletSelectionState = (
    currentAddress: string,
    fallbackAddress = "",
  ) => {
    const normalizedCurrent =
      currentAddress.trim() && isAddress(currentAddress.trim())
        ? getAddress(currentAddress.trim())
        : "";
    if (
      normalizedCurrent &&
      rewardsAccountOptions.some(
        (option) =>
          option.value.toLowerCase() === normalizedCurrent.toLowerCase(),
      )
    ) {
      return {
        selection: normalizedCurrent,
        customAddress: "",
      };
    }
    if (normalizedCurrent) {
      return {
        selection: CUSTOM_REWARDS_ACCOUNT_VALUE,
        customAddress: normalizedCurrent,
      };
    }

    const normalizedFallback =
      fallbackAddress.trim() && isAddress(fallbackAddress.trim())
        ? getAddress(fallbackAddress.trim())
        : "";
    if (
      normalizedFallback &&
      rewardsAccountOptions.some(
        (option) =>
          option.value.toLowerCase() === normalizedFallback.toLowerCase(),
      )
    ) {
      return {
        selection: normalizedFallback,
        customAddress: "",
      };
    }
    if (normalizedFallback) {
      return {
        selection: CUSTOM_REWARDS_ACCOUNT_VALUE,
        customAddress: normalizedFallback,
      };
    }

    return {
      selection:
        rewardsAccountOptions[0]?.value || CUSTOM_REWARDS_ACCOUNT_VALUE,
      customAddress: "",
    };
  };

  const formatWalletAddress = (wallet: WalletResponse) => {
    const rawAddress = (
      wallet.smart_address ||
      wallet.eoa_address ||
      ""
    ).trim();
    if (!rawAddress) return "No address";
    if (!isAddress(rawAddress)) return rawAddress;
    const normalized = getAddress(rawAddress);
    return `${normalized.slice(0, 6)}...${normalized.slice(-4)}`;
  };

  const isWalletPrimary = (wallet: WalletResponse) => {
    const walletAddress = (
      wallet.smart_address ||
      wallet.eoa_address ||
      ""
    ).trim();
    const primaryWalletAddress = (user?.primaryWalletAddress || "").trim();
    if (!walletAddress || !primaryWalletAddress) return false;
    if (!isAddress(walletAddress) || !isAddress(primaryWalletAddress))
      return false;
    return (
      getAddress(walletAddress).toLowerCase() ===
      getAddress(primaryWalletAddress).toLowerCase()
    );
  };

  const formatManagedAddress = (address: string) => {
    const trimmedAddress = address.trim();
    if (!trimmedAddress) return "Not set";
    if (!isAddress(trimmedAddress)) return trimmedAddress;
    const normalizedAddress = getAddress(trimmedAddress);
    return `${normalizedAddress.slice(0, 6)}...${normalizedAddress.slice(-4)}`;
  };

  const getManagedWalletLabel = (address: string) => {
    const trimmedAddress = address.trim();
    if (!trimmedAddress || !isAddress(trimmedAddress)) {
      return trimmedAddress || "Wallet";
    }

    const normalizedAddress = getAddress(trimmedAddress);
    const matchedOption = walletOptionLookup.get(
      normalizedAddress.toLowerCase(),
    );
    if (matchedOption) {
      return matchedOption.label;
    }

    return `${normalizedAddress.slice(0, 6)}...${normalizedAddress.slice(-4)}`;
  };

  const buildLocationWalletDraft = (
    location: AuthedLocation,
  ): MerchantLocationWalletDraft => {
    const paymentWalletAddresses = (location.payment_wallets || [])
      .map((wallet) => {
        const rawAddress = (wallet.wallet_address || "").trim();
        if (!rawAddress || !isAddress(rawAddress)) return "";
        return getAddress(rawAddress);
      })
      .filter(Boolean);

    const defaultPaymentWalletAddress = location.payment_wallets
      ?.find((wallet) => wallet.is_default)
      ?.wallet_address?.trim();
    const normalizedDefaultPaymentWalletAddress =
      defaultPaymentWalletAddress && isAddress(defaultPaymentWalletAddress)
        ? getAddress(defaultPaymentWalletAddress)
        : paymentWalletAddresses[0] || "";

    const tippingWalletAddress = (location.tip_to_address || "").trim();
    const normalizedTippingWalletAddress =
      tippingWalletAddress && isAddress(tippingWalletAddress)
        ? getAddress(tippingWalletAddress)
        : "";

    const tippingOption = normalizedTippingWalletAddress
      ? walletOptionLookup.get(normalizedTippingWalletAddress.toLowerCase())
      : null;

    return {
      paymentWalletAddresses,
      defaultPaymentWalletAddress: normalizedDefaultPaymentWalletAddress,
      paymentAddSelection: SELECT_WALLET_PLACEHOLDER_VALUE,
      paymentAddCustomAddress: "",
      tippingWalletSelection: normalizedTippingWalletAddress
        ? tippingOption
          ? tippingOption.value
          : CUSTOM_WALLET_VALUE
        : NONE_WALLET_VALUE,
      tippingWalletCustomAddress:
        normalizedTippingWalletAddress && !tippingOption
          ? normalizedTippingWalletAddress
          : "",
      saving: false,
      error: "",
      success: "",
    };
  };

  const getPersistedMerchantLocation = (locationId: number): AuthedLocation | null => {
    for (const location of sortedUserLocations) {
      if (location.id === locationId) {
        return location;
      }
    }
    return null;
  };

  const buildLocationProfileDraft = (
    location: AuthedLocation,
  ): MerchantLocationProfileDraft => ({
    googleId: location.google_id || "",
    name: location.name || "",
    description: location.description || "",
    type: location.type || "",
    street: location.street || "",
    city: location.city || "",
    state: location.state || "",
    zip: location.zip || "",
    lat: location.lat ?? 0,
    lng: location.lng ?? 0,
    phone: location.phone || "",
    website: location.website || "",
    imageUrl: location.image_url || "",
    rating: location.rating ?? 0,
    mapsPage: location.maps_page || "",
    openingHours: location.opening_hours || [],
    dirty: false,
    saving: false,
    error: "",
    success: "",
  });

  const formatLocationAddressSummary = (location: AuthedLocation) => {
    const cityState = [location.city, location.state].filter(Boolean).join(", ");
    return [location.street, cityState, location.zip].filter(Boolean).join(" ").trim();
  };

  useEffect(() => {
    if (verifiedEmailOptions.length === 0) {
      if (roleEmail !== "") setRoleEmail("");
      return;
    }

    const roleEmailExists = verifiedEmailOptions.some(
      (option) => option.email === roleEmail,
    );
    if (!roleEmailExists) {
      setRoleEmail(verifiedEmailOptions[0].email);
    }
  }, [verifiedEmailOptions, roleEmail]);

  const loadVerifiedEmails = async () => {
    if (status !== "authenticated") return;

    setVerifiedEmailsLoading(true);
    setVerifiedEmailError("");
    try {
      const res = await authFetch("/users/verified-emails");
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to load verified emails.");
      }
      const data = (await res.json()) as VerifiedEmailResponse[];
      setVerifiedEmails(data || []);
    } catch (err) {
      setVerifiedEmailError(
        err instanceof Error ? err.message : "Unable to load verified emails.",
      );
    } finally {
      setVerifiedEmailsLoading(false);
    }
  };

  const loadRewardsWallets = async () => {
    if (status !== "authenticated") return;

    setRewardsWalletsLoading(true);
    setRewardsWalletsError("");
    try {
      const res = await authFetch("/wallets");
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to load accounts.");
      }
      const data = (await res.json()) as WalletResponse[];
      setRewardsWallets(data || []);
    } catch (err) {
      setRewardsWalletsError(
        err instanceof Error ? err.message : "Unable to load accounts.",
      );
    } finally {
      setRewardsWalletsLoading(false);
    }
  };

  useEffect(() => {
    if (status === "authenticated") {
      void loadVerifiedEmails();
      void loadRewardsWallets();
    }
  }, [status]);

  useEffect(() => {
    if (status !== "authenticated") return;
    if (
      activeTab === "account" ||
      activeTab === "improver" ||
      activeTab === "supervisor"
    ) {
      void loadVerifiedEmails();
      void loadRewardsWallets();
    }
  }, [activeTab, status]);

  useEffect(() => {
    const selectionState = getWalletSelectionState(
      user?.primaryWalletAddress || "",
      defaultRewardsAccount,
    );
    setPrimaryWalletSelection(selectionState.selection);
    setPrimaryWalletCustomAddress(selectionState.customAddress);
  }, [
    defaultRewardsAccount,
    rewardsAccountOptions,
    user?.primaryWalletAddress,
  ]);

  useEffect(() => {
    if (!user?.isMerchant) {
      setLocationWalletDrafts({});
      return;
    }

    setLocationWalletDrafts((current) => {
      const nextDrafts: Record<number, MerchantLocationWalletDraft> = {};
      for (const location of sortedUserLocations) {
        const existingDraft = current[location.id];
        const nextDraft = buildLocationWalletDraft(location);
        nextDrafts[location.id] = {
          ...nextDraft,
          saving: existingDraft?.saving ?? false,
          error: existingDraft?.error ?? "",
          success: existingDraft?.success ?? "",
        };
      }
      return nextDrafts;
    });
  }, [
    rewardsAccountOptions,
    sortedUserLocations,
    user?.isMerchant,
    walletOptionLookup,
  ]);

  useEffect(() => {
    if (!user?.isMerchant) {
      setLocationProfileDrafts({});
      return;
    }

    setLocationProfileDrafts((current) => {
      const nextDrafts: Record<number, MerchantLocationProfileDraft> = {};
      for (const location of approvedMerchantLocations) {
        const existingDraft = current[location.id];
        const nextDraft = buildLocationProfileDraft(location);
        nextDrafts[location.id] =
          existingDraft?.dirty || existingDraft?.saving
            ? existingDraft
            : {
                ...nextDraft,
                saving: existingDraft?.saving ?? false,
                error: existingDraft?.error ?? "",
                success: existingDraft?.success ?? "",
              };
      }
      return nextDrafts;
    });
  }, [approvedMerchantLocations, user?.isMerchant]);

  useEffect(() => {
    setMerchantLocationCardOpen((current) => {
      const nextState: Record<number, boolean> = {};
      for (const location of approvedMerchantLocations) {
        nextState[location.id] = current[location.id] ?? false;
      }
      return nextState;
    });
  }, [approvedMerchantLocations]);

  useEffect(() => {
    if (activeTab !== "merchant" || approvedMerchantLocations.length === 0) {
      return;
    }

    let cancelled = false;
    void ensureGooglePlacesScript()
      .then(() => {
        if (cancelled) return;
        if (hasGoogleMapsPlaces()) {
          setMerchantPlacesReady(true);
          setMerchantPlacesLoadError("");
        }
      })
      .catch((error) => {
        if (cancelled) return;
        console.error(error);
        setMerchantPlacesLoadError(
          "Failed to load Google Places search. Please refresh and try again.",
        );
      });

    return () => {
      cancelled = true;
    };
  }, [activeTab, approvedMerchantLocations.length]);

  useEffect(() => {
    if (improverStatus !== "approved") return;

    const selectionState = getWalletSelectionState(
      improver?.primary_rewards_account || "",
      defaultRewardsAccount,
    );
    setImproverRewardsSelection(selectionState.selection);
    setImproverCustomRewardsAccount(selectionState.customAddress);
  }, [
    improverStatus,
    improver?.primary_rewards_account,
    rewardsAccountOptions,
    defaultRewardsAccount,
  ]);

  useEffect(() => {
    if (supervisorStatus !== "approved") return;

    const selectionState = getWalletSelectionState(
      supervisor?.primary_rewards_account || "",
      defaultRewardsAccount,
    );
    setSupervisorRewardsSelection(selectionState.selection);
    setSupervisorCustomRewardsAccount(selectionState.customAddress);
  }, [
    supervisorStatus,
    supervisor?.primary_rewards_account,
    rewardsAccountOptions,
    defaultRewardsAccount,
  ]);

  const handleAddVerifiedEmail = async (e: React.FormEvent) => {
    e.preventDefault();

    const email = newVerifiedEmail.trim();
    if (!email) {
      setVerifiedEmailError("Email is required.");
      setVerifiedEmailSuccess("");
      return;
    }

    setVerifiedEmailSubmitting(true);
    setVerifiedEmailError("");
    setVerifiedEmailSuccess("");
    try {
      const res = await authFetch("/users/verified-emails", {
        method: "POST",
        body: JSON.stringify({ email }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to send verification email.");
      }

      setNewVerifiedEmail("");
      setVerifiedEmailFormOpen(false);
      setVerifiedEmailSuccess(
        "Verification email sent. It expires in 30 minutes.",
      );
      await loadVerifiedEmails();
    } catch (err) {
      setVerifiedEmailError(
        err instanceof Error
          ? err.message
          : "Unable to send verification email.",
      );
    } finally {
      setVerifiedEmailSubmitting(false);
    }
  };

  const handleResendVerification = async (emailId: string) => {
    setVerifiedEmailSubmitting(true);
    setVerifiedEmailError("");
    setVerifiedEmailSuccess("");
    try {
      const res = await authFetch(`/users/verified-emails/${emailId}/resend`, {
        method: "POST",
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to resend verification email.");
      }

      setVerifiedEmailSuccess(
        "Verification email resent. It expires in 30 minutes.",
      );
      await loadVerifiedEmails();
    } catch (err) {
      setVerifiedEmailError(
        err instanceof Error
          ? err.message
          : "Unable to resend verification email.",
      );
    } finally {
      setVerifiedEmailSubmitting(false);
    }
  };

  // Handle account update

  // Handle password change
  const handlePasswordChange = (e: React.FormEvent) => {
    e.preventDefault();

    // Validate passwords
    if (newPassword.length < 8) {
      setPasswordError("Password must be at least 8 characters long");
      return;
    }

    if (newPassword !== confirmPassword) {
      setPasswordError("Passwords do not match");
      return;
    }

    setPasswordError("");
    setIsChangingPassword(true);

    // Simulate API call
    setTimeout(() => {
      setIsChangingPassword(false);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      showSuccessMessage("Password changed successfully");
    }, 1500);
  };

  // Handle notification settings update
  const handleNotificationUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    setIsUpdating(true);

    // Simulate API call
    setTimeout(() => {
      setIsUpdating(false);
      showSuccessMessage("Notification settings updated successfully");
    }, 1500);
  };

  const handleRoleRequest = async (e: React.FormEvent) => {
    e.preventDefault();
    setRoleError("");
    setRoleSuccess("");

    if (!roleRequestType) {
      setRoleError("Please select a role type.");
      return;
    }

    if (roleRequestType === "merchant") {
      router.push("/settings/merchant-approval");
      return;
    }

    if (
      roleRequestType === "affiliate" ||
      roleRequestType === "proposer" ||
      roleRequestType === "issuer" ||
      roleRequestType === "supervisor"
    ) {
      if (!roleOrg.trim()) {
        setRoleError("Organization name is required.");
        return;
      }
    }
    if (
      (roleRequestType === "proposer" ||
        roleRequestType === "issuer" ||
        roleRequestType === "supervisor") &&
      !roleEmail.trim()
    ) {
      setRoleError("Notification email is required.");
      return;
    }
    if (roleRequestType === "improver") {
      if (!roleFirstName.trim() || !roleLastName.trim() || !roleEmail.trim()) {
        setRoleError("First name, last name, and email are required.");
        return;
      }
    }

    setRoleSubmitting(true);
    try {
      let res: Response;
      if (roleRequestType === "affiliate") {
        res = await authFetch("/affiliates/request", {
          method: "POST",
          body: JSON.stringify({ organization: roleOrg.trim() }),
        });
        if (res.ok) {
          const data = await res.json();
          setAffiliate(data);
        }
      } else if (roleRequestType === "proposer") {
        res = await authFetch("/proposers/request", {
          method: "POST",
          body: JSON.stringify({
            organization: roleOrg.trim(),
            email: roleEmail.trim(),
          }),
        });
        if (res.ok) {
          const data = await res.json();
          setProposer(data);
        }
      } else if (roleRequestType === "issuer") {
        res = await authFetch("/issuers/request", {
          method: "POST",
          body: JSON.stringify({
            organization: roleOrg.trim(),
            email: roleEmail.trim(),
          }),
        });
        if (res.ok) {
          const data = await res.json();
          setIssuer(data);
        }
      } else if (roleRequestType === "supervisor") {
        res = await authFetch("/supervisors/request", {
          method: "POST",
          body: JSON.stringify({
            organization: roleOrg.trim(),
            email: roleEmail.trim(),
          }),
        });
        if (res.ok) {
          const data = await res.json();
          setSupervisor(data);
        }
      } else {
        await refreshWallets();
        const walletsRes = await authFetch("/wallets");
        if (!walletsRes.ok) {
          throw new Error("Unable to verify wallet setup.");
        }
        const wallets = (await walletsRes.json()) as WalletResponse[];
        const hasSmartWalletIndexZero = wallets.some(
          (wallet) =>
            wallet.is_eoa === false &&
            wallet.smart_index === 0 &&
            typeof wallet.smart_address === "string" &&
            wallet.smart_address.trim() !== "",
        );
        if (!hasSmartWalletIndexZero) {
          throw new Error(
            "Primary smart wallet is still initializing. Please try again in a few seconds.",
          );
        }

        res = await authFetch("/improvers/request", {
          method: "POST",
          body: JSON.stringify({
            first_name: roleFirstName.trim(),
            last_name: roleLastName.trim(),
            email: roleEmail.trim(),
          }),
        });
        if (res.ok) {
          const data = await res.json();
          setImprover(data);
        }
      }
      if (!res!.ok) {
        if (res!.status === 409) {
          setRoleError("Your status for that role is already approved.");
        } else {
          setRoleError(
            "Unable to submit your request right now. Please try again.",
          );
        }
        return;
      }
      setRoleSuccess("Your request has been submitted and is pending review.");
      setRoleRequestType("");
      setRoleOrg("");
      setRoleEmail(verifiedEmailOptions[0]?.email || "");
      setRoleFirstName("");
      setRoleLastName("");
    } catch (err) {
      setRoleError(
        err instanceof Error && err.message
          ? err.message
          : "Unable to submit your request. Please try again.",
      );
    } finally {
      setRoleSubmitting(false);
    }
  };

  const handleSaveImproverRewardsAccount = async () => {
    setImproverRewardsError("");
    setImproverRewardsSuccess("");

    const selectedValue =
      improverRewardsSelection === CUSTOM_REWARDS_ACCOUNT_VALUE
        ? improverCustomRewardsAccount.trim()
        : improverRewardsSelection.trim();

    if (!selectedValue) {
      setImproverRewardsError("Primary rewards account is required.");
      return;
    }
    if (!isAddress(selectedValue)) {
      setImproverRewardsError("Enter a valid Ethereum address.");
      return;
    }

    const normalized = getAddress(selectedValue);
    setImproverRewardsSaving(true);
    try {
      const res = await authFetch("/improvers/primary-rewards-account", {
        method: "PUT",
        body: JSON.stringify({ primary_rewards_account: normalized }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to update improver rewards account.");
      }

      const data = await res.json();
      setImprover(data);
      setImproverRewardsSuccess("Primary rewards account updated.");
    } catch (err) {
      setImproverRewardsError(
        err instanceof Error
          ? err.message
          : "Unable to update improver rewards account.",
      );
    } finally {
      setImproverRewardsSaving(false);
    }
  };

  const handleSavePrimaryWallet = async () => {
    setPrimaryWalletError("");
    setPrimaryWalletSuccess("");

    const selectedValue =
      primaryWalletSelection === CUSTOM_REWARDS_ACCOUNT_VALUE
        ? primaryWalletCustomAddress.trim()
        : primaryWalletSelection.trim();

    if (!selectedValue) {
      setPrimaryWalletError("Primary wallet is required.");
      return;
    }
    if (!isAddress(selectedValue)) {
      setPrimaryWalletError("Enter a valid Ethereum address.");
      return;
    }

    const normalized = getAddress(selectedValue);
    setPrimaryWalletSaving(true);
    try {
      const res = await authFetch("/users/primary-wallet", {
        method: "PUT",
        body: JSON.stringify({ primary_wallet_address: normalized }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to update primary wallet.");
      }

      const data = (await res.json()) as UserResponse;
      updateUser({
        primaryWalletAddress: data.primary_wallet_address,
      });
      setPrimaryWalletSuccess("Primary wallet updated.");
    } catch (err) {
      setPrimaryWalletError(
        err instanceof Error ? err.message : "Unable to update primary wallet.",
      );
    } finally {
      setPrimaryWalletSaving(false);
    }
  };

  const updateLocationWalletDraft = (
    locationId: number,
    updater: (
      draft: MerchantLocationWalletDraft,
    ) => MerchantLocationWalletDraft,
  ) => {
    setLocationWalletDrafts((current) => {
      const existingDraft = current[locationId];
      if (!existingDraft) return current;
      return {
        ...current,
        [locationId]: updater(existingDraft),
      };
    });
  };

  const updateLocationProfileDraft = (
    locationId: number,
    updater: (
      draft: MerchantLocationProfileDraft,
    ) => MerchantLocationProfileDraft,
  ) => {
    setLocationProfileDrafts((current) => {
      const existingDraft = current[locationId];
      if (!existingDraft) return current;
      return {
        ...current,
        [locationId]: updater(existingDraft),
      };
    });
  };

  const handleSaveLocationProfile = async (locationId: number) => {
    const draft = locationProfileDrafts[locationId];
    const location = approvedMerchantLocations.find((entry) => entry.id === locationId);
    if (!draft || !location || draft.saving) return;

    const nextLocation: AuthedLocation = {
      ...location,
      google_id: draft.googleId.trim(),
      name: draft.name.trim(),
      description: draft.description.trim(),
      type: draft.type.trim(),
      street: draft.street.trim(),
      city: draft.city.trim(),
      state: draft.state.trim(),
      zip: draft.zip.trim(),
      lat: draft.lat,
      lng: draft.lng,
      phone: draft.phone.trim(),
      website: draft.website.trim(),
      image_url: draft.imageUrl.trim(),
      rating: draft.rating,
      maps_page: draft.mapsPage.trim(),
      opening_hours: draft.openingHours,
    };

    updateLocationProfileDraft(locationId, () => ({
      ...draft,
      saving: true,
      error: "",
      success: "",
    }));

    try {
      const res = await authFetch("/locations", {
        method: "PUT",
        body: JSON.stringify(nextLocation),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to update location profile.");
      }

      setUserLocations((current) =>
        current.map((entry) => (entry.id === locationId ? nextLocation : entry)),
      );
      updateLocationProfileDraft(locationId, () => ({
        ...buildLocationProfileDraft(nextLocation),
        dirty: false,
        saving: false,
        error: "",
        success: "Location profile updated.",
      }));
    } catch (err) {
      updateLocationProfileDraft(locationId, () => ({
        ...draft,
        saving: false,
        error:
          err instanceof Error ? err.message : "Unable to update location profile.",
        success: "",
      }));
    }
  };

  const applyGoogleLocationSelection = (
    locationId: number,
    selection: GoogleSubLocation,
  ) => {
    updateLocationProfileDraft(locationId, (current) => ({
      ...current,
      googleId: selection.google_id || current.googleId,
      name: selection.name || current.name,
      type: selection.type || current.type,
      street: selection.street || current.street,
      city: selection.city || current.city,
      state: selection.state || current.state,
      zip: selection.zip || current.zip,
      lat: selection.lat ?? current.lat,
      lng: selection.lng ?? current.lng,
      phone: selection.phone || current.phone,
      website: selection.website || current.website,
      imageUrl: selection.image_url || current.imageUrl,
      rating: selection.rating ?? current.rating,
      mapsPage: selection.maps_page || current.mapsPage,
      openingHours: selection.opening_hours || current.openingHours,
      dirty: true,
      error: "",
      success: "",
    }));
  };

  const resolveDraftAddress = (
    selection: string,
    customAddress: string,
    allowBlank = false,
  ) => {
    if (selection === NONE_WALLET_VALUE) {
      return allowBlank ? "" : null;
    }
    const rawAddress =
      selection === CUSTOM_WALLET_VALUE
        ? customAddress.trim()
        : selection.trim();
    if (!rawAddress) {
      return allowBlank ? "" : null;
    }
    if (!isAddress(rawAddress)) {
      return null;
    }
    return getAddress(rawAddress);
  };

  const commitLocationWalletSettings = async (
    locationId: number,
    nextDraft: MerchantLocationWalletDraft,
    successMessage = "Updated.",
  ) => {
    const tippingWalletAddress = resolveDraftAddress(
      nextDraft.tippingWalletSelection,
      nextDraft.tippingWalletCustomAddress,
      true,
    );
    if (
      nextDraft.tippingWalletSelection !== NONE_WALLET_VALUE &&
      tippingWalletAddress === null
    ) {
      updateLocationWalletDraft(locationId, (current) => ({
        ...current,
        error: "Enter a valid tipping wallet address.",
        success: "",
      }));
      return;
    }

    if (
      tippingWalletAddress &&
      nextDraft.paymentWalletAddresses.some(
        (address) =>
          address.toLowerCase() === tippingWalletAddress.toLowerCase(),
      )
    ) {
      updateLocationWalletDraft(locationId, (current) => ({
        ...current,
        error: "Tipping wallet must be different from every payment wallet.",
        success: "",
      }));
      return;
    }

    updateLocationWalletDraft(locationId, () => ({
      ...nextDraft,
      saving: true,
      error: "",
      success: "",
    }));

    try {
      const res = await authFetch(`/locations/${locationId}/wallet-settings`, {
        method: "PUT",
        body: JSON.stringify({
          payment_wallet_addresses: nextDraft.paymentWalletAddresses,
          default_payment_wallet_address: nextDraft.defaultPaymentWalletAddress,
          tipping_wallet_address: tippingWalletAddress || "",
        }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to update merchant wallet settings.");
      }

      const updatedLocation = (await res.json()) as AuthedLocation;
      setUserLocations((current) =>
        current.map((location) =>
          location.id === updatedLocation.id ? updatedLocation : location,
        ),
      );
      updateLocationWalletDraft(locationId, () => ({
        ...buildLocationWalletDraft(updatedLocation),
        saving: false,
        error: "",
        success: successMessage,
      }));
    } catch (err) {
      const persistedLocation = getPersistedMerchantLocation(locationId);
      updateLocationWalletDraft(locationId, () => ({
        ...(persistedLocation
          ? buildLocationWalletDraft(persistedLocation)
          : nextDraft),
        saving: false,
        error:
          err instanceof Error
            ? err.message
            : "Unable to update merchant wallet settings.",
        success: "",
      }));
    }
  };

  const handleAddLocationPaymentWallet = async (
    locationId: number,
    options?: {
      silentIfEmptyCustom?: boolean;
      draftOverride?: MerchantLocationWalletDraft;
    },
  ) => {
    const draft = options?.draftOverride || locationWalletDrafts[locationId];
    if (!draft || draft.saving) return;

    const normalizedAddress = resolveDraftAddress(
      draft.paymentAddSelection,
      draft.paymentAddCustomAddress,
    );
    if (!normalizedAddress) {
      if (
        options?.silentIfEmptyCustom &&
        draft.paymentAddSelection === CUSTOM_WALLET_VALUE &&
        !draft.paymentAddCustomAddress.trim()
      ) {
        return;
      }
      updateLocationWalletDraft(locationId, (current) => ({
        ...current,
        error: "Enter a valid payment wallet address before adding it.",
        success: "",
      }));
      return;
    }

    const currentTipAddress = resolveDraftAddress(
      draft.tippingWalletSelection,
      draft.tippingWalletCustomAddress,
      true,
    );
    if (
      currentTipAddress &&
      normalizedAddress.toLowerCase() === currentTipAddress.toLowerCase()
    ) {
      updateLocationWalletDraft(locationId, (current) => ({
        ...current,
        error: "Payment wallets must be different from the tipping wallet.",
        success: "",
      }));
      return;
    }

    if (
      draft.paymentWalletAddresses.some(
        (address) => address.toLowerCase() === normalizedAddress.toLowerCase(),
      )
    ) {
      updateLocationWalletDraft(locationId, (current) => ({
        ...current,
        error: "That payment wallet is already added to this location.",
        success: "",
      }));
      return;
    }

    const nextPaymentWalletAddresses = [
      ...draft.paymentWalletAddresses,
      normalizedAddress,
    ];
    const nextDraft: MerchantLocationWalletDraft = {
      ...draft,
      paymentWalletAddresses: nextPaymentWalletAddresses,
      defaultPaymentWalletAddress:
        draft.defaultPaymentWalletAddress || normalizedAddress,
      paymentAddSelection: SELECT_WALLET_PLACEHOLDER_VALUE,
      paymentAddCustomAddress: "",
      error: "",
      success: "",
    };

    await commitLocationWalletSettings(
      locationId,
      nextDraft,
      "Payment wallet added.",
    );
  };

  const handleRemoveLocationPaymentWallet = async (
    locationId: number,
    walletAddress: string,
  ) => {
    const draft = locationWalletDrafts[locationId];
    if (!draft || draft.saving) return;

    const nextPaymentWalletAddresses = draft.paymentWalletAddresses.filter(
      (address) => address.toLowerCase() !== walletAddress.toLowerCase(),
    );

    const nextDraft: MerchantLocationWalletDraft = {
      ...draft,
      paymentWalletAddresses: nextPaymentWalletAddresses,
      defaultPaymentWalletAddress:
        draft.defaultPaymentWalletAddress.toLowerCase() ===
        walletAddress.toLowerCase()
          ? nextPaymentWalletAddresses[0] || ""
          : draft.defaultPaymentWalletAddress,
      error: "",
      success: "",
    };

    await commitLocationWalletSettings(
      locationId,
      nextDraft,
      "Payment wallet removed.",
    );
  };

  const handleSetLocationDefaultPaymentWallet = async (
    locationId: number,
    walletAddress: string,
  ) => {
    const draft = locationWalletDrafts[locationId];
    if (!draft || draft.saving) return;

    const nextDraft: MerchantLocationWalletDraft = {
      ...draft,
      defaultPaymentWalletAddress: walletAddress,
      error: "",
      success: "",
    };
    await commitLocationWalletSettings(
      locationId,
      nextDraft,
      "Default payment wallet updated.",
    );
  };

  const handleApplyLocationTippingWallet = async (
    locationId: number,
    options?: {
      silentIfEmptyCustom?: boolean;
      draftOverride?: MerchantLocationWalletDraft;
    },
  ) => {
    const draft = options?.draftOverride || locationWalletDrafts[locationId];
    if (!draft || draft.saving) return;

    if (
      options?.silentIfEmptyCustom &&
      draft.tippingWalletSelection === CUSTOM_WALLET_VALUE &&
      !draft.tippingWalletCustomAddress.trim()
    ) {
      return;
    }

    await commitLocationWalletSettings(
      locationId,
      {
        ...draft,
        error: "",
        success: "",
      },
      "Tip wallet updated.",
    );
  };

  const handleWalletVisibilityChange = async (
    wallet: WalletResponse,
    shouldShow: boolean,
  ) => {
    if (wallet.id === null) return;

    setWalletVisibilitySavingId(wallet.id);
    setWalletVisibilityError("");
    setWalletVisibilitySuccess("");
    try {
      const res = await authFetch("/wallets", {
        method: "PUT",
        body: JSON.stringify({
          id: wallet.id,
          name: wallet.name,
          is_hidden: !shouldShow,
        }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to update wallet visibility.");
      }

      setRewardsWallets((current) =>
        current.map((entry) =>
          entry.id === wallet.id ? { ...entry, is_hidden: !shouldShow } : entry,
        ),
      );
      await refreshWallets();
      setWalletVisibilitySuccess("Wallet visibility updated.");
    } catch (err) {
      setWalletVisibilityError(
        err instanceof Error
          ? err.message
          : "Unable to update wallet visibility.",
      );
    } finally {
      setWalletVisibilitySavingId(null);
    }
  };

  const handleSaveSupervisorRewardsAccount = async () => {
    setSupervisorRewardsError("");
    setSupervisorRewardsSuccess("");

    const selectedValue =
      supervisorRewardsSelection === CUSTOM_REWARDS_ACCOUNT_VALUE
        ? supervisorCustomRewardsAccount.trim()
        : supervisorRewardsSelection.trim();

    if (!selectedValue) {
      setSupervisorRewardsError("Primary rewards account is required.");
      return;
    }
    if (!isAddress(selectedValue)) {
      setSupervisorRewardsError("Enter a valid Ethereum address.");
      return;
    }

    const normalized = getAddress(selectedValue);
    setSupervisorRewardsSaving(true);
    try {
      const res = await authFetch("/supervisors/primary-rewards-account", {
        method: "PUT",
        body: JSON.stringify({ primary_rewards_account: normalized }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to update supervisor rewards account.");
      }

      const data = await res.json();
      setSupervisor(data);
      setSupervisorRewardsSuccess("Primary rewards account updated.");
    } catch (err) {
      setSupervisorRewardsError(
        err instanceof Error
          ? err.message
          : "Unable to update supervisor rewards account.",
      );
    } finally {
      setSupervisorRewardsSaving(false);
    }
  };

  const handleAffiliateLogoChange = (
    e: React.ChangeEvent<HTMLInputElement>,
  ) => {
    setAffiliateLogoError("");
    const file = e.target.files?.[0];
    if (!file) return;

    if (!file.type.startsWith("image/")) {
      setAffiliateLogoError("Please upload a valid image file.");
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result;
      if (typeof result === "string") {
        setAffiliateLogoPreview(result);
      }
    };
    reader.readAsDataURL(file);
  };

  const handleAffiliateLogoSave = async () => {
    if (!affiliateLogoPreview) return;
    setAffiliateLogoSaving(true);
    setAffiliateLogoError("");

    try {
      const res = await authFetch("/affiliates/logo", {
        method: "PUT",
        body: JSON.stringify({ logo: affiliateLogoPreview }),
      });

      if (!res.ok) {
        throw new Error("Unable to update affiliate logo right now.");
      }

      const updated = await res.json();
      setAffiliate(updated);
    } catch (err) {
      const message =
        err instanceof Error
          ? err.message
          : "Unable to update affiliate logo right now.";
      setAffiliateLogoError(message);
    } finally {
      setAffiliateLogoSaving(false);
    }
  };

  const openDeleteAccountDialog = async () => {
    setDeleteAccountError("");
    setDeleteAccountLoading(true);
    try {
      const res = await authFetch("/users/delete-account/preview");
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Unable to load the account deletion preview.");
      }

      const preview = (await res.json()) as AccountDeletionPreview;
      setDeleteAccountPreview(preview);
      setDeleteAccountDialogOpen(true);
    } catch (err) {
      setDeleteAccountError(
        err instanceof Error
          ? err.message
          : "Unable to load the account deletion preview.",
      );
    } finally {
      setDeleteAccountLoading(false);
    }
  };

  const closeDeleteAccountDialog = () => {
    if (deleteAccountSubmitting) {
      return;
    }
    setDeleteAccountDialogOpen(false);
    setDeleteAccountError("");
  };

  const handleDeleteAccount = async () => {
    setDeleteAccountError("");
    setDeleteAccountSubmitting(true);
    setDeleteAccountPhase("sweeping");
    try {
      await sweepSFLUVBalancesToAdmin(wallets);
      setDeleteAccountPhase("deleting");
      const res = await authFetch("/users/delete-account", {
        method: "POST",
      });
      if (res.status !== 202) {
        const text = await res.text();
        throw new Error(text || "Unable to schedule account deletion.");
      }

      await res.json();
      setDeleteAccountDialogOpen(false);
      await logout();
      router.replace("/map");
    } catch (err) {
      setDeleteAccountError(
        err instanceof Error
          ? err.message
          : "Unable to schedule account deletion.",
      );
    } finally {
      setDeleteAccountSubmitting(false);
      setDeleteAccountPhase("idle");
    }
  };

  // Show success message
  const showSuccessMessage = (message: string) => {
    setSuccessMessage(message);
    setTimeout(() => {
      setSuccessMessage("");
    }, 3000);
  };

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6 w-full">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">
          Account Settings
        </h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">
          Manage your account preferences and settings
        </p>
      </div>

      {/* {successMessage && (
        <div className="bg-green-100 dark:bg-green-900 border border-green-200 dark:border-green-800 text-green-800 dark:text-green-200 px-4 py-3 rounded flex items-center">
          <Check className="h-5 w-5 mr-2" />
          {successMessage}
        </div>
      )} */}

      <Tabs
        defaultValue="account"
        value={activeTab}
        onValueChange={setActiveTab}
        className="w-full"
      >
        <TabsList className="w-full mb-6 bg-secondary flex flex-wrap h-auto gap-1 p-1">
          <TabsTrigger
            value="account"
            className="text-black dark:text-white flex-1"
          >
            Account
          </TabsTrigger>
          {merchantStatus !== "none" && (
            <TabsTrigger
              value="merchant"
              className="text-black dark:text-white flex-1"
            >
              Merchant
            </TabsTrigger>
          )}
          {(affiliateStatus === "pending" ||
            affiliateStatus === "approved") && (
            <TabsTrigger
              value="affiliate"
              className="text-black dark:text-white flex-1"
            >
              Affiliate
            </TabsTrigger>
          )}
          {(proposerStatus === "pending" || proposerStatus === "approved") && (
            <TabsTrigger
              value="proposer"
              className="text-black dark:text-white flex-1"
            >
              Proposer
            </TabsTrigger>
          )}
          {(improverStatus === "pending" || improverStatus === "approved") && (
            <TabsTrigger
              value="improver"
              className="text-black dark:text-white flex-1"
            >
              Improver
            </TabsTrigger>
          )}
          {(issuerStatus === "pending" || issuerStatus === "approved") && (
            <TabsTrigger
              value="issuer"
              className="text-black dark:text-white flex-1"
            >
              Issuer
            </TabsTrigger>
          )}
          {(supervisorStatus === "pending" ||
            supervisorStatus === "approved") && (
            <TabsTrigger
              value="supervisor"
              className="text-black dark:text-white flex-1"
            >
              Supervisor
            </TabsTrigger>
          )}
        </TabsList>

        <TabsContent value="account">
          {/* <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-black dark:text-white">Profile Information</CardTitle>
                <CardDescription>Update your account details</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleAccountUpdate} className="space-y-4">
                  <div className="flex items-center gap-4 mb-6">
                    <Avatar className="h-20 w-20">
                      <AvatarImage src={user?.avatar || "/placeholder.svg"} alt={user?.name} />
                      <AvatarFallback className="bg-[#eb6c6c] text-white text-xl">
                        {user?.name?.charAt(0) || <User />}
                      </AvatarFallback>
                    </Avatar>
                    <div>
                      <Button variant="outline" size="sm" className="text-black dark:text-white bg-secondary">
                        <Upload className="h-4 w-4 mr-2" />
                        Change Avatar
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="name" className="text-black dark:text-white">
                      Full Name
                    </Label>
                    <Input
                      id="name"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="email" className="text-black dark:text-white">
                      Email Address
                    </Label>
                    <Input
                      id="email"
                      type="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      className="text-black dark:text-white bg-secondary rounded-md"
                    />
                  </div>

                  <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isUpdating}>
                    {isUpdating ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Updating...
                      </>
                    ) : (
                      "Update Profile"
                    )}
                  </Button>
                </form>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-black dark:text-white">Change Password</CardTitle>
                <CardDescription>Update your account password</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handlePasswordChange} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="current-password" className="text-black dark:text-white">
                      Current Password
                    </Label>
                    <Input
                      id="current-password"
                      type="password"
                      value={currentPassword}
                      onChange={(e) => setCurrentPassword(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="new-password" className="text-black dark:text-white">
                      New Password
                    </Label>
                    <Input
                      id="new-password"
                      type="password"
                      value={newPassword}
                      onChange={(e) => {
                        setNewPassword(e.target.value)
                        setPasswordError("")
                      }}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="confirm-password" className="text-black dark:text-white">
                      Confirm New Password
                    </Label>
                    <Input
                      id="confirm-password"
                      type="password"
                      value={confirmPassword}
                      onChange={(e) => {
                        setConfirmPassword(e.target.value)
                        setPasswordError("")
                      }}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  {passwordError && <p className="text-red-500 text-sm">{passwordError}</p>}

                  <Button
                    type="submit"
                    className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]"
                    disabled={isChangingPassword || !currentPassword || !newPassword || !confirmPassword}
                  >
                    {isChangingPassword ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Changing Password...
                      </>
                    ) : (
                      "Change Password"
                    )}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </div> */}

          <Card className="mt-6">
            <CardHeader className="flex flex-row items-start justify-between gap-4">
              <div>
                <CardTitle className="text-black dark:text-white">
                  Verified Emails
                </CardTitle>
                <CardDescription>
                  Only verified emails can be used for role requests and wallet
                  notification subscriptions.
                </CardDescription>
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setVerifiedEmailFormOpen((prev) => !prev);
                  setVerifiedEmailError("");
                  setVerifiedEmailSuccess("");
                }}
                className="whitespace-nowrap"
              >
                Add Email
              </Button>
            </CardHeader>
            <CardContent className="space-y-4">
              {verifiedEmailsLoading ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading verified emails...
                </div>
              ) : (
                <>
                  <div className="space-y-2">
                    <p className="text-sm font-medium text-black dark:text-white">
                      Verified
                    </p>
                    {verifiedEmailOptions.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        No verified emails yet.
                      </p>
                    ) : (
                      verifiedEmailOptions.map((entry) => (
                        <div
                          key={entry.id}
                          className="rounded border bg-secondary/30 px-3 py-2 text-sm flex items-center justify-between gap-2"
                        >
                          <span className="break-all">{entry.email}</span>
                          <span className="text-xs text-green-700 dark:text-green-300">
                            Verified
                          </span>
                        </div>
                      ))
                    )}
                  </div>

                  {pendingOrExpiredEmailOptions.length > 0 && (
                    <div className="space-y-2">
                      <p className="text-sm font-medium text-black dark:text-white">
                        Pending Verification
                      </p>
                      {pendingOrExpiredEmailOptions.map((entry) => (
                        <div
                          key={entry.id}
                          className="rounded border bg-secondary/30 px-3 py-2 text-sm space-y-2"
                        >
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <span className="break-all">{entry.email}</span>
                            <span
                              className={`text-xs ${entry.status === "expired" ? "text-red-600 dark:text-red-300" : "text-yellow-700 dark:text-yellow-300"}`}
                            >
                              {entry.status === "expired"
                                ? "Expired"
                                : "Pending"}
                            </span>
                          </div>
                          {entry.verification_token_expires_at && (
                            <p className="text-xs text-muted-foreground">
                              Expires:{" "}
                              {new Date(
                                entry.verification_token_expires_at,
                              ).toLocaleString()}
                            </p>
                          )}
                          {entry.status === "expired" && (
                            <Button
                              type="button"
                              size="sm"
                              variant="secondary"
                              onClick={() =>
                                void handleResendVerification(entry.id)
                              }
                              disabled={verifiedEmailSubmitting}
                            >
                              {verifiedEmailSubmitting
                                ? "Sending..."
                                : "Resend Verification"}
                            </Button>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}

              {verifiedEmailFormOpen && (
                <form
                  onSubmit={handleAddVerifiedEmail}
                  className="space-y-3 rounded border p-3"
                >
                  <div className="space-y-1">
                    <Label htmlFor="verified-email-input">Email</Label>
                    <Input
                      id="verified-email-input"
                      type="email"
                      value={newVerifiedEmail}
                      onChange={(e) => setNewVerifiedEmail(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                      placeholder="name@example.com"
                    />
                  </div>
                  <div className="flex flex-wrap justify-end gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => {
                        setVerifiedEmailFormOpen(false);
                        setNewVerifiedEmail("");
                      }}
                    >
                      Cancel
                    </Button>
                    <Button type="submit" disabled={verifiedEmailSubmitting}>
                      {verifiedEmailSubmitting ? (
                        <>
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          Sending...
                        </>
                      ) : (
                        "Send Verification"
                      )}
                    </Button>
                  </div>
                </form>
              )}

              {verifiedEmailError && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{verifiedEmailError}</span>
                </div>
              )}
              {verifiedEmailSuccess && (
                <p className="text-sm text-green-600">{verifiedEmailSuccess}</p>
              )}
            </CardContent>
          </Card>

          {(merchantStatus === "none" ||
            merchantStatus === "rejected" ||
            affiliateStatus === "none" ||
            affiliateStatus === "rejected" ||
            proposerStatus === "none" ||
            proposerStatus === "rejected" ||
            improverStatus === "none" ||
            improverStatus === "rejected" ||
            issuerStatus === "none" ||
            issuerStatus === "rejected" ||
            supervisorStatus === "none" ||
            supervisorStatus === "rejected") && (
            <Card className="mt-6">
              <CardHeader>
                <CardTitle className="text-black dark:text-white">
                  Request Role Access
                </CardTitle>
                <CardDescription>
                  Apply for merchant, affiliate, proposer, improver, issuer, or
                  supervisor status
                </CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleRoleRequest} className="space-y-4">
                  <div className="space-y-2">
                    <Label className="text-black dark:text-white">
                      Role Type
                    </Label>
                    <Select
                      value={roleRequestType}
                      onValueChange={(v) => {
                        setRoleRequestType(v as RoleRequestType);
                        setRoleError("");
                        setRoleSuccess("");
                      }}
                    >
                      <SelectTrigger className="text-black dark:text-white bg-secondary">
                        <SelectValue placeholder="Select a role to request..." />
                      </SelectTrigger>
                      <SelectContent>
                        {(merchantStatus === "none" ||
                          merchantStatus === "rejected") && (
                          <SelectItem value="merchant">
                            Merchant — accept SFLuv as payment at your business
                          </SelectItem>
                        )}
                        {(affiliateStatus === "none" ||
                          affiliateStatus === "rejected") && (
                          <SelectItem value="affiliate">
                            Affiliate — create funded community events
                          </SelectItem>
                        )}
                        {(proposerStatus === "none" ||
                          proposerStatus === "rejected") && (
                          <SelectItem value="proposer">
                            Proposer — build and submit community workflows
                          </SelectItem>
                        )}
                        {(improverStatus === "none" ||
                          improverStatus === "rejected") && (
                          <SelectItem value="improver">
                            Improver — claim and complete workflow steps
                          </SelectItem>
                        )}
                        {(issuerStatus === "none" ||
                          issuerStatus === "rejected") && (
                          <SelectItem value="issuer">
                            Issuer — issue credentials to community members
                          </SelectItem>
                        )}
                        {(supervisorStatus === "none" ||
                          supervisorStatus === "rejected") && (
                          <SelectItem value="supervisor">
                            Supervisor — review workflow submissions and exports
                          </SelectItem>
                        )}
                      </SelectContent>
                    </Select>
                  </div>

                  {(roleRequestType === "affiliate" ||
                    roleRequestType === "proposer" ||
                    roleRequestType === "issuer" ||
                    roleRequestType === "supervisor") && (
                    <div className="space-y-2">
                      <Label
                        htmlFor="role-org"
                        className="text-black dark:text-white"
                      >
                        Organization Name
                      </Label>
                      <Input
                        id="role-org"
                        value={roleOrg}
                        onChange={(e) => setRoleOrg(e.target.value)}
                        className="text-black dark:text-white bg-secondary"
                        placeholder="Organization or group name"
                      />
                    </div>
                  )}

                  {(roleRequestType === "proposer" ||
                    roleRequestType === "issuer" ||
                    roleRequestType === "supervisor") && (
                    <div className="space-y-2">
                      <Label
                        htmlFor="role-email"
                        className="text-black dark:text-white"
                      >
                        Notification Email
                      </Label>
                      {verifiedEmailOptions.length > 0 ? (
                        <Select value={roleEmail} onValueChange={setRoleEmail}>
                          <SelectTrigger
                            id="role-email"
                            className="text-black dark:text-white bg-secondary"
                          >
                            <SelectValue placeholder="Select a verified email" />
                          </SelectTrigger>
                          <SelectContent>
                            {verifiedEmailOptions.map((entry) => (
                              <SelectItem key={entry.id} value={entry.email}>
                                {entry.email}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <div className="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300 space-y-2">
                          <p>No verified emails available.</p>
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            onClick={() => setActiveTab("account")}
                          >
                            Go to Account Emails
                          </Button>
                        </div>
                      )}
                    </div>
                  )}

                  {roleRequestType === "improver" && (
                    <>
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                        <div className="space-y-2">
                          <Label
                            htmlFor="role-first-name"
                            className="text-black dark:text-white"
                          >
                            First Name
                          </Label>
                          <Input
                            id="role-first-name"
                            value={roleFirstName}
                            onChange={(e) => setRoleFirstName(e.target.value)}
                            className="text-black dark:text-white bg-secondary"
                          />
                        </div>
                        <div className="space-y-2">
                          <Label
                            htmlFor="role-last-name"
                            className="text-black dark:text-white"
                          >
                            Last Name
                          </Label>
                          <Input
                            id="role-last-name"
                            value={roleLastName}
                            onChange={(e) => setRoleLastName(e.target.value)}
                            className="text-black dark:text-white bg-secondary"
                          />
                        </div>
                      </div>
                      <div className="space-y-2">
                        <Label
                          htmlFor="role-email-improver"
                          className="text-black dark:text-white"
                        >
                          Email
                        </Label>
                        {verifiedEmailOptions.length > 0 ? (
                          <Select
                            value={roleEmail}
                            onValueChange={setRoleEmail}
                          >
                            <SelectTrigger
                              id="role-email-improver"
                              className="text-black dark:text-white bg-secondary"
                            >
                              <SelectValue placeholder="Select a verified email" />
                            </SelectTrigger>
                            <SelectContent>
                              {verifiedEmailOptions.map((entry) => (
                                <SelectItem key={entry.id} value={entry.email}>
                                  {entry.email}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        ) : (
                          <div className="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300 space-y-2">
                            <p>No verified emails available.</p>
                            <Button
                              type="button"
                              size="sm"
                              variant="outline"
                              onClick={() => setActiveTab("account")}
                            >
                              Go to Account Emails
                            </Button>
                          </div>
                        )}
                      </div>
                    </>
                  )}

                  {roleError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{roleError}</span>
                    </div>
                  )}

                  {roleSuccess && (
                    <p className="text-sm text-green-600">{roleSuccess}</p>
                  )}

                  <div className="flex justify-end">
                    <Button
                      type="submit"
                      disabled={roleSubmitting || !roleRequestType}
                    >
                      {roleSubmitting ? (
                        <>
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          Submitting...
                        </>
                      ) : roleRequestType === "merchant" ? (
                        "Continue to Application"
                      ) : (
                        "Submit Request"
                      )}
                    </Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          )}

          <Card className="mt-6">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">
                Primary Wallet
              </CardTitle>
              <CardDescription>
                This wallet is used as your default account anywhere a wallet is
                needed and you have not chosen a more specific one.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label
                  htmlFor="primary-wallet"
                  className="text-black dark:text-white"
                >
                  Default Wallet
                </Label>
                <Select
                  value={primaryWalletSelection || CUSTOM_REWARDS_ACCOUNT_VALUE}
                  onValueChange={(value) => {
                    setPrimaryWalletSelection(value);
                    setPrimaryWalletError("");
                    setPrimaryWalletSuccess("");
                  }}
                >
                  <SelectTrigger
                    id="primary-wallet"
                    className="text-black dark:text-white bg-secondary"
                  >
                    <SelectValue
                      placeholder={
                        rewardsWalletsLoading
                          ? "Loading wallets..."
                          : "Select a wallet"
                      }
                    />
                  </SelectTrigger>
                  <SelectContent className="bg-secondary text-black dark:text-white">
                    {rewardsAccountOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                    <SelectItem value={CUSTOM_REWARDS_ACCOUNT_VALUE}>
                      Other
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {primaryWalletSelection === CUSTOM_REWARDS_ACCOUNT_VALUE && (
                <div className="space-y-2">
                  <Label
                    htmlFor="primary-wallet-custom"
                    className="text-black dark:text-white"
                  >
                    Wallet Address
                  </Label>
                  <Input
                    id="primary-wallet-custom"
                    value={primaryWalletCustomAddress}
                    onChange={(e) => {
                      setPrimaryWalletCustomAddress(e.target.value);
                      setPrimaryWalletError("");
                      setPrimaryWalletSuccess("");
                    }}
                    placeholder="0x..."
                    className="text-black dark:text-white bg-secondary"
                  />
                </div>
              )}

              {rewardsWalletsError && (
                <p className="text-sm text-red-600 dark:text-red-400">
                  {rewardsWalletsError}
                </p>
              )}
              {primaryWalletError && (
                <p className="text-sm text-red-600 dark:text-red-400">
                  {primaryWalletError}
                </p>
              )}
              {primaryWalletSuccess && (
                <p className="text-sm text-green-600 dark:text-green-400">
                  {primaryWalletSuccess}
                </p>
              )}

              <Button
                type="button"
                variant="outline"
                className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                onClick={handleSavePrimaryWallet}
                disabled={primaryWalletSaving}
              >
                {primaryWalletSaving ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Saving...
                  </>
                ) : (
                  "Save Primary Wallet"
                )}
              </Button>
            </CardContent>
          </Card>

          <Card className="mt-6">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">
                Wallet Visibility
              </CardTitle>
              <CardDescription>
                Choose which wallets appear on your wallets page.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {rewardsWalletsLoading ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading wallets...
                </div>
              ) : rewardsWallets.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No wallets available yet.
                </p>
              ) : (
                <div className="space-y-3">
                  {rewardsWallets.map((wallet) => (
                    <div
                      key={`${wallet.id ?? "wallet"}-${wallet.smart_address || wallet.eoa_address}`}
                      className="flex flex-col gap-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-900/30 sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="font-medium text-black dark:text-white">
                            {wallet.name}
                          </p>
                          {isWalletPrimary(wallet) && (
                            <span className="rounded-full border border-[#eb6c6c]/40 bg-[#eb6c6c]/10 px-2 py-0.5 text-[11px] font-medium text-[#eb6c6c]">
                              Primary Wallet
                            </span>
                          )}
                        </div>
                        <p className="font-mono text-xs text-gray-500 dark:text-gray-400">
                          {formatWalletAddress(wallet)}
                        </p>
                      </div>

                      <div className="flex items-center justify-between gap-3 sm:justify-end">
                        <div className="text-right">
                          <p className="text-sm font-medium text-black dark:text-white">
                            Show on wallets page
                          </p>
                          <p className="text-xs text-gray-500 dark:text-gray-400">
                            Hidden wallets stay usable elsewhere.
                          </p>
                        </div>
                        <Switch
                          checked={!wallet.is_hidden}
                          disabled={walletVisibilitySavingId === wallet.id}
                          onCheckedChange={(checked) => {
                            void handleWalletVisibilityChange(wallet, checked);
                          }}
                        />
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {walletVisibilityError && (
                <p className="text-sm text-red-600 dark:text-red-400">
                  {walletVisibilityError}
                </p>
              )}
              {walletVisibilitySuccess && (
                <p className="text-sm text-green-600 dark:text-green-400">
                  {walletVisibilitySuccess}
                </p>
              )}
            </CardContent>
          </Card>

          {affiliateStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Affiliate Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">
                  Your affiliate request was not approved. Use the form below to
                  submit another request.
                </p>
              </CardContent>
            </Card>
          )}

          {proposerStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Proposer Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">
                  Your proposer request was not approved. Use the form below to
                  submit another request.
                </p>
              </CardContent>
            </Card>
          )}

          {improverStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Improver Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">
                  Your improver request was not approved. Use the form below to
                  submit another request.
                </p>
              </CardContent>
            </Card>
          )}

          {issuerStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Issuer Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">
                  Your issuer request was not approved. Use the form below to
                  submit another request.
                </p>
              </CardContent>
            </Card>
          )}

          {supervisorStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Supervisor Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">
                  Your supervisor request was not approved. Use the form below
                  to submit another request.
                </p>
              </CardContent>
            </Card>
          )}

          {/* {user?.merchantStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Merchant Application Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400 mb-4">
                  Unfortunately, your merchant application was not approved. You can contact support for more
                  information.
                </p>
                <Button
                  variant="outline"
                  className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                  onClick={() => router.push("/merchant-status")}
                >
                  View Details
                </Button>
              </CardContent>
            </Card>
          )} */}

          <Card className="mt-6">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">
                Link Socials
              </CardTitle>
              <CardDescription>
                Manage the Google and Apple sign-in methods attached to this
                account.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-2">
                  <div className="flex items-center gap-3">
                    <span className="flex h-10 w-10 items-center justify-center rounded-2xl border border-border/70 bg-background shadow-sm">
                      <GoogleLogo />
                    </span>
                    <div>
                      <p className="font-medium text-black dark:text-white">
                        Google
                      </p>
                    </div>
                  </div>
                  <div>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {googleLinked
                        ? "Google is linked to this account for future sign-ins."
                        : "Google links when you sign in with Google on this account."}
                    </p>
                  </div>
                  {googleLinkedEmail ? (
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      Google email: {googleLinkedEmail}
                    </p>
                  ) : null}
                  {googleDisconnectDisabledReason ? (
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {googleDisconnectDisabledReason}
                    </p>
                  ) : null}
                  {googleMessage ? (
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {googleMessage}
                    </p>
                  ) : null}
                </div>
                {googleLinked ? (
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full gap-2 sm:w-auto"
                    disabled={googleActionBusy || !canDisconnectGoogle}
                    onClick={() => {
                      void unlinkGoogle();
                    }}
                  >
                    <GoogleLogo className="h-4 w-4" />
                    {canDisconnectGoogle
                      ? googleActionBusy
                        ? "Disconnecting Google..."
                        : "Disconnect Google"
                      : "Google linked"}
                  </Button>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full gap-2 sm:w-auto"
                    disabled
                  >
                    <GoogleLogo className="h-4 w-4" />
                    Sign in with Google to link
                  </Button>
                )}
              </div>

              <Separator />

              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-2">
                  <div className="flex items-center gap-3">
                    <span className="flex h-10 w-10 items-center justify-center rounded-2xl border border-border/70 bg-background text-black shadow-sm dark:text-white">
                      <AppleLogo />
                    </span>
                    <div>
                      <p className="font-medium text-black dark:text-white">
                        Apple
                      </p>
                    </div>
                  </div>
                  <div>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {appleLinked
                        ? "Apple is linked to this account for future sign-ins."
                        : "Link Apple so future Apple sign-ins land on this account."}
                    </p>
                  </div>
                  {appleLinkedEmail ? (
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      Apple email: {appleLinkedEmail}
                    </p>
                  ) : null}
                  {appleDisconnectDisabledReason ? (
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {appleDisconnectDisabledReason}
                    </p>
                  ) : null}
                  {appleLinkMessage ? (
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {appleLinkMessage}
                    </p>
                  ) : null}
                </div>
                <Button
                  type="button"
                  variant={appleLinked ? "outline" : "default"}
                  className="w-full gap-2 sm:w-auto"
                  disabled={
                    appleLinkBusy || (appleLinked ? !canDisconnectApple : false)
                  }
                  onClick={() => {
                    if (appleLinked) {
                      void unlinkApple();
                      return;
                    }
                    void linkApple();
                  }}
                >
                  <AppleLogo className="h-4 w-4" />
                  {appleLinked
                    ? canDisconnectApple
                      ? appleLinkBusy
                        ? "Disconnecting Apple..."
                        : "Disconnect Apple"
                      : "Apple linked"
                    : appleLinkBusy
                      ? "Linking Apple..."
                      : "Link Apple"}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card className="mt-6 border-red-200/80 bg-red-50/70 dark:border-red-500/30 dark:bg-red-500/10">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">
                Delete Account
              </CardTitle>
              <CardDescription>
                Delete your account and log out. Your account will be
                recoverable for the next 30 days, but any SFLuv in your
                accessible wallets will be transferred out of your account
                before the deletion request is submitted.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {deleteAccountError ? (
                <p className="text-sm text-red-600 dark:text-red-300">
                  {deleteAccountError}
                </p>
              ) : null}
              <Button
                type="button"
                variant="destructive"
                disabled={deleteAccountLoading || deleteAccountSubmitting}
                onClick={() => {
                  void openDeleteAccountDialog();
                }}
              >
                {deleteAccountLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Loading deletion preview...
                  </>
                ) : (
                  "Delete Account"
                )}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {merchantStatus !== "none" && (
          <TabsContent value="merchant" className="space-y-6">
            {approvedMerchantLocations.length > 0 && (
              <div className="space-y-4">
                {approvedMerchantLocations.map((loc) => {
                  const walletDraft = locationWalletDrafts[loc.id];
                  const profileDraft = locationProfileDrafts[loc.id];
                  if (!walletDraft || !profileDraft) return null;

                  const isOpen = merchantLocationCardOpen[loc.id] ?? false;
                  const tippingWalletOptions = rewardsAccountOptions.filter(
                    (option) =>
                      !walletDraft.paymentWalletAddresses.some(
                        (address) =>
                          address.toLowerCase() === option.value.toLowerCase(),
                      ),
                  );
                  const availablePaymentWalletOptions =
                    rewardsAccountOptions.filter(
                      (option) =>
                        !walletDraft.paymentWalletAddresses.some(
                          (address) =>
                            address.toLowerCase() === option.value.toLowerCase(),
                        ),
                    );
                  const effectivePaymentWallet = (loc.pay_to_address || "").trim();
                  const effectiveTippingWallet = (loc.tip_to_address || "").trim();
                  const paymentSummary =
                    walletDraft.paymentWalletAddresses.length === 0
                      ? "Primary wallet fallback"
                      : `${walletDraft.paymentWalletAddresses.length} payment wallet${walletDraft.paymentWalletAddresses.length === 1 ? "" : "s"}`;
                  const profileStatusLabel = profileDraft.saving
                    ? "Saving profile"
                    : profileDraft.success
                      ? "Profile saved"
                      : "Profile";
                  const routingStatusLabel = walletDraft.saving
                    ? "Saving routing"
                    : walletDraft.success
                      ? "Routing saved"
                      : "Routing";

                  return (
                    <Collapsible
                      key={`merchant-location-${loc.id}`}
                      open={isOpen}
                      onOpenChange={(open) =>
                        setMerchantLocationCardOpen((current) => ({
                          ...current,
                          [loc.id]: open,
                        }))
                      }
                    >
                      <Card className="overflow-hidden">
                        <CollapsibleTrigger asChild>
                          <button
                            type="button"
                            className="w-full text-left transition-colors hover:bg-secondary/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-[#eb6c6c] focus-visible:ring-inset"
                          >
                            <CardHeader className="gap-4">
                              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                                <div className="min-w-0 space-y-1">
                                  <CardTitle className="truncate text-black dark:text-white">
                                    {loc.name}
                                  </CardTitle>
                                  <CardDescription className="break-words">
                                    {formatLocationAddressSummary(loc) || "Approved location"}
                                  </CardDescription>
                                </div>
                                <div className="flex items-center gap-2 self-start text-sm font-medium text-muted-foreground">
                                  <span>{isOpen ? "Hide settings" : "Location settings"}</span>
                                  <ChevronDown
                                    className={`h-4 w-4 transition-transform ${isOpen ? "rotate-180" : ""}`}
                                  />
                                </div>
                              </div>
                              <div className="flex flex-wrap gap-2">
                                <span className="rounded-full border px-3 py-1 text-xs text-muted-foreground">
                                  {profileStatusLabel}
                                </span>
                                <span className="rounded-full border px-3 py-1 text-xs text-muted-foreground">
                                  {routingStatusLabel}
                                </span>
                                <span className="rounded-full border px-3 py-1 text-xs text-muted-foreground">
                                  {paymentSummary}
                                </span>
                                <span className="rounded-full border px-3 py-1 text-xs text-muted-foreground">
                                  {effectiveTippingWallet ? "Tips enabled" : "No tip wallet"}
                                </span>
                              </div>
                            </CardHeader>
                          </button>
                        </CollapsibleTrigger>

                        <CollapsibleContent>
                          <CardContent className="space-y-6 pt-0">
                            <div className="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
                              <div className="space-y-4 rounded-xl border bg-background/70 p-4">
                                <div className="space-y-1">
                                  <h3 className="text-sm font-semibold text-black dark:text-white">
                                    Location profile
                                  </h3>
                                  <p className="text-xs text-muted-foreground">
                                    Update the public business details for this approved location.
                                  </p>
                                </div>

                                <div className="space-y-2">
                                  <Label
                                    htmlFor={`merchant-location-name-${loc.id}`}
                                    className="text-black dark:text-white"
                                  >
                                    Business name
                                  </Label>
                                  <Input
                                    id={`merchant-location-name-${loc.id}`}
                                    value={profileDraft.name}
                                    onChange={(e) =>
                                      updateLocationProfileDraft(loc.id, (current) => ({
                                        ...current,
                                        name: e.target.value,
                                        dirty: true,
                                        error: "",
                                        success: "",
                                      }))
                                    }
                                    className="text-black dark:text-white bg-secondary"
                                  />
                                </div>

                                <div className="space-y-2">
                                  <Label
                                    htmlFor={`merchant-location-description-${loc.id}`}
                                    className="text-black dark:text-white"
                                  >
                                    Description
                                  </Label>
                                  <Textarea
                                    id={`merchant-location-description-${loc.id}`}
                                    value={profileDraft.description}
                                    onChange={(e) =>
                                      updateLocationProfileDraft(loc.id, (current) => ({
                                        ...current,
                                        description: e.target.value,
                                        dirty: true,
                                        error: "",
                                        success: "",
                                      }))
                                    }
                                    className="min-h-[110px] text-black dark:text-white bg-secondary"
                                  />
                                </div>

                                <div className="space-y-4">
                                  <div className="space-y-2">
                                    <Label className="text-black dark:text-white">
                                      Search for your location name
                                    </Label>
                                    {merchantPlacesLoadError ? (
                                      <div className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-300">
                                        {merchantPlacesLoadError}
                                      </div>
                                    ) : merchantPlacesReady ? (
                                      <PlaceAutocomplete
                                        key={`merchant-location-place-${loc.id}`}
                                        setGoogleSubLocation={noopGoogleSubLocationSetter}
                                        setBusinessPhone={(value) =>
                                          updateLocationProfileDraft(loc.id, (current) => ({
                                            ...current,
                                            phone: value,
                                            dirty: true,
                                            error: "",
                                            success: "",
                                          }))
                                        }
                                        setStreet={(value) =>
                                          updateLocationProfileDraft(loc.id, (current) => ({
                                            ...current,
                                            street: value,
                                            dirty: true,
                                            error: "",
                                            success: "",
                                          }))
                                        }
                                        onSelect={(selection) =>
                                          applyGoogleLocationSelection(loc.id, selection)
                                        }
                                      />
                                    ) : (
                                      <div className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm text-muted-foreground">
                                        <Loader2 className="h-4 w-4 animate-spin" />
                                        Loading Google Places search...
                                      </div>
                                    )}
                                    <p className="text-xs text-muted-foreground">
                                      Re-select the location here whenever you want to refresh
                                      the Google Maps address, coordinates, and related map
                                      details.
                                    </p>
                                  </div>

                                  <div className="space-y-2">
                                    <Label
                                      htmlFor={`merchant-location-street-${loc.id}`}
                                      className="text-black dark:text-white"
                                    >
                                      Street address
                                    </Label>
                                    <Input
                                      id={`merchant-location-street-${loc.id}`}
                                      value={profileDraft.street}
                                      onChange={(e) =>
                                        updateLocationProfileDraft(loc.id, (current) => ({
                                          ...current,
                                          street: e.target.value,
                                          dirty: true,
                                          error: "",
                                          success: "",
                                        }))
                                      }
                                      className="text-black dark:text-white bg-secondary"
                                    />
                                  </div>

                                  <div className="grid gap-3 sm:grid-cols-3">
                                    <div className="rounded-lg border bg-secondary/20 px-3 py-2">
                                      <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                                        City
                                      </p>
                                      <p className="mt-1 text-sm font-medium text-black dark:text-white">
                                        {profileDraft.city || "Not set"}
                                      </p>
                                    </div>
                                    <div className="rounded-lg border bg-secondary/20 px-3 py-2">
                                      <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                                        State
                                      </p>
                                      <p className="mt-1 text-sm font-medium text-black dark:text-white">
                                        {profileDraft.state || "Not set"}
                                      </p>
                                    </div>
                                    <div className="rounded-lg border bg-secondary/20 px-3 py-2">
                                      <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                                        ZIP
                                      </p>
                                      <p className="mt-1 text-sm font-medium text-black dark:text-white">
                                        {profileDraft.zip || "Not set"}
                                      </p>
                                    </div>
                                  </div>

                                  <p className="text-xs text-muted-foreground">
                                    Latitude, longitude, maps page, and other Google-linked
                                    location metadata update automatically from the selected
                                    place.
                                  </p>
                                </div>

                                <div className="grid gap-4 sm:grid-cols-2">
                                  <div className="space-y-2">
                                    <Label
                                      htmlFor={`merchant-location-phone-${loc.id}`}
                                      className="text-black dark:text-white"
                                    >
                                      Phone
                                    </Label>
                                    <Input
                                      id={`merchant-location-phone-${loc.id}`}
                                      value={profileDraft.phone}
                                      onChange={(e) =>
                                        updateLocationProfileDraft(loc.id, (current) => ({
                                          ...current,
                                          phone: e.target.value,
                                          dirty: true,
                                          error: "",
                                          success: "",
                                        }))
                                      }
                                      className="text-black dark:text-white bg-secondary"
                                    />
                                  </div>

                                  <div className="space-y-2 sm:col-span-2">
                                    <Label
                                      htmlFor={`merchant-location-website-${loc.id}`}
                                      className="text-black dark:text-white"
                                    >
                                      Website
                                    </Label>
                                    <Input
                                      id={`merchant-location-website-${loc.id}`}
                                      value={profileDraft.website}
                                      onChange={(e) =>
                                        updateLocationProfileDraft(loc.id, (current) => ({
                                          ...current,
                                          website: e.target.value,
                                          dirty: true,
                                          error: "",
                                          success: "",
                                        }))
                                      }
                                      className="text-black dark:text-white bg-secondary"
                                    />
                                  </div>
                                </div>

                                {profileDraft.error && (
                                  <p className="text-sm text-red-600 dark:text-red-400">
                                    {profileDraft.error}
                                  </p>
                                )}
                                {profileDraft.success && !profileDraft.saving && (
                                  <p className="text-sm text-green-600 dark:text-green-400">
                                    {profileDraft.success}
                                  </p>
                                )}

                                <Button
                                  type="button"
                                  className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]"
                                  disabled={profileDraft.saving}
                                  onClick={() => void handleSaveLocationProfile(loc.id)}
                                >
                                  {profileDraft.saving ? (
                                    <>
                                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                      Saving...
                                    </>
                                  ) : (
                                    "Save Location Profile"
                                  )}
                                </Button>
                              </div>

                              <div className="space-y-4">
                                <div className="grid gap-3 sm:grid-cols-2">
                                  <div className="rounded-xl border bg-secondary/25 p-4">
                                    <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
                                      Payments
                                    </p>
                                    <p className="mt-2 truncate text-sm font-medium text-black dark:text-white">
                                      {effectivePaymentWallet
                                        ? getManagedWalletLabel(effectivePaymentWallet)
                                        : "Primary wallet fallback"}
                                    </p>
                                    {effectivePaymentWallet && (
                                      <p className="mt-1 break-all font-mono text-[11px] text-muted-foreground">
                                        {formatManagedAddress(effectivePaymentWallet)}
                                      </p>
                                    )}
                                  </div>
                                  <div className="rounded-xl border bg-secondary/25 p-4">
                                    <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
                                      Tips
                                    </p>
                                    <p className="mt-2 truncate text-sm font-medium text-black dark:text-white">
                                      {effectiveTippingWallet
                                        ? getManagedWalletLabel(effectiveTippingWallet)
                                        : "No tip wallet"}
                                    </p>
                                    {effectiveTippingWallet && (
                                      <p className="mt-1 break-all font-mono text-[11px] text-muted-foreground">
                                        {formatManagedAddress(effectiveTippingWallet)}
                                      </p>
                                    )}
                                  </div>
                                </div>

                                <div className="rounded-xl border bg-background/70 p-4">
                                  <div className="space-y-1">
                                    <h3 className="text-sm font-semibold text-black dark:text-white">
                                      Payment wallets
                                    </h3>
                                    <p className="text-xs text-muted-foreground">
                                      Tap a wallet to add it.
                                    </p>
                                  </div>

                                  <div className="mt-4 space-y-3">
                                    {walletDraft.paymentWalletAddresses.length === 0 ? (
                                      <p className="rounded-lg border border-dashed px-3 py-4 text-sm text-muted-foreground">
                                        No location-specific wallets yet.
                                      </p>
                                    ) : (
                                      walletDraft.paymentWalletAddresses.map((walletAddress) => (
                                        <div
                                          key={`${loc.id}-${walletAddress}`}
                                          className="flex flex-col gap-3 rounded-xl border bg-secondary/20 p-3 sm:flex-row sm:items-center sm:justify-between"
                                        >
                                          <div className="min-w-0 flex-1">
                                            <div className="flex flex-wrap items-center gap-2">
                                              <p className="truncate font-medium text-black dark:text-white">
                                                {getManagedWalletLabel(walletAddress)}
                                              </p>
                                              {walletDraft.defaultPaymentWalletAddress.toLowerCase() ===
                                                walletAddress.toLowerCase() && (
                                                <span className="rounded-full bg-[#eb6c6c]/10 px-2 py-0.5 text-[11px] font-medium text-[#eb6c6c]">
                                                  Default
                                                </span>
                                              )}
                                            </div>
                                            <p className="mt-1 break-all font-mono text-[11px] text-muted-foreground">
                                              {formatManagedAddress(walletAddress)}
                                            </p>
                                          </div>
                                          <div className="flex flex-wrap gap-2 sm:justify-end">
                                            {walletDraft.defaultPaymentWalletAddress.toLowerCase() !==
                                              walletAddress.toLowerCase() && (
                                              <Button
                                                type="button"
                                                size="sm"
                                                variant="outline"
                                                onClick={() =>
                                                  void handleSetLocationDefaultPaymentWallet(
                                                    loc.id,
                                                    walletAddress,
                                                  )
                                                }
                                                disabled={walletDraft.saving}
                                              >
                                                Set default
                                              </Button>
                                            )}
                                            <Button
                                              type="button"
                                              size="sm"
                                              variant="outline"
                                              onClick={() =>
                                                void handleRemoveLocationPaymentWallet(
                                                  loc.id,
                                                  walletAddress,
                                                )
                                              }
                                              disabled={walletDraft.saving}
                                            >
                                              Remove
                                            </Button>
                                          </div>
                                        </div>
                                      ))
                                    )}
                                  </div>

                                  <div className="mt-4 space-y-2">
                                    <Label
                                      htmlFor={`payment-wallet-${loc.id}`}
                                      className="text-black dark:text-white"
                                    >
                                      Add wallet
                                    </Label>
                                    <Select
                                      value={
                                        walletDraft.paymentAddSelection ||
                                        SELECT_WALLET_PLACEHOLDER_VALUE
                                      }
                                      onValueChange={(value) => {
                                        if (value === SELECT_WALLET_PLACEHOLDER_VALUE) {
                                          return;
                                        }
                                        if (value === CUSTOM_WALLET_VALUE) {
                                          updateLocationWalletDraft(loc.id, (current) => ({
                                            ...current,
                                            paymentAddSelection: value,
                                            error: "",
                                            success: "",
                                          }));
                                          return;
                                        }
                                        const nextDraft = {
                                          ...walletDraft,
                                          paymentAddSelection: value,
                                          error: "",
                                          success: "",
                                        };
                                        updateLocationWalletDraft(loc.id, () => nextDraft);
                                        void handleAddLocationPaymentWallet(loc.id, {
                                          draftOverride: nextDraft,
                                        });
                                      }}
                                    >
                                      <SelectTrigger
                                        id={`payment-wallet-${loc.id}`}
                                        className="w-full text-black dark:text-white bg-secondary"
                                        disabled={walletDraft.saving}
                                      >
                                        <SelectValue
                                          placeholder={
                                            rewardsWalletsLoading
                                              ? "Loading wallets..."
                                              : "Select wallet"
                                          }
                                        />
                                      </SelectTrigger>
                                      <SelectContent className="bg-secondary text-black dark:text-white">
                                        <SelectItem
                                          value={SELECT_WALLET_PLACEHOLDER_VALUE}
                                          disabled
                                        >
                                          Select wallet
                                        </SelectItem>
                                        {availablePaymentWalletOptions.map((option) => (
                                          <SelectItem
                                            key={`${loc.id}-${option.value}`}
                                            value={option.value}
                                          >
                                            {option.label}
                                          </SelectItem>
                                        ))}
                                        <SelectItem value={CUSTOM_WALLET_VALUE}>
                                          Other
                                        </SelectItem>
                                      </SelectContent>
                                    </Select>
                                    {walletDraft.paymentAddSelection === CUSTOM_WALLET_VALUE && (
                                      <Input
                                        value={walletDraft.paymentAddCustomAddress}
                                        onChange={(e) => {
                                          updateLocationWalletDraft(loc.id, (current) => ({
                                            ...current,
                                            paymentAddCustomAddress: e.target.value,
                                            error: "",
                                            success: "",
                                          }));
                                        }}
                                        onBlur={() =>
                                          void handleAddLocationPaymentWallet(loc.id, {
                                            silentIfEmptyCustom: true,
                                          })
                                        }
                                        onKeyDown={(e) => {
                                          if (e.key === "Enter") {
                                            e.preventDefault();
                                            void handleAddLocationPaymentWallet(loc.id, {
                                              silentIfEmptyCustom: true,
                                            });
                                          }
                                        }}
                                        placeholder="Paste address and press Enter"
                                        className="text-black dark:text-white bg-secondary"
                                        disabled={walletDraft.saving}
                                      />
                                    )}
                                  </div>
                                </div>

                                <div className="rounded-xl border bg-background/70 p-4">
                                  <div className="space-y-1">
                                    <h3 className="text-sm font-semibold text-black dark:text-white">
                                      Tip wallet
                                    </h3>
                                    <p className="text-xs text-muted-foreground">
                                      Must be different from every payment wallet.
                                    </p>
                                  </div>

                                  <div className="mt-4 space-y-2">
                                    <Label
                                      htmlFor={`tipping-wallet-${loc.id}`}
                                      className="text-black dark:text-white"
                                    >
                                      Tip destination
                                    </Label>
                                    <Select
                                      value={walletDraft.tippingWalletSelection}
                                      onValueChange={(value) => {
                                        const nextDraft = {
                                          ...walletDraft,
                                          tippingWalletSelection: value,
                                          error: "",
                                          success: "",
                                        };
                                        updateLocationWalletDraft(loc.id, () => nextDraft);
                                        if (value !== CUSTOM_WALLET_VALUE) {
                                          void handleApplyLocationTippingWallet(loc.id, {
                                            draftOverride: nextDraft,
                                          });
                                        }
                                      }}
                                    >
                                      <SelectTrigger
                                        id={`tipping-wallet-${loc.id}`}
                                        className="w-full text-black dark:text-white bg-secondary"
                                        disabled={walletDraft.saving}
                                      >
                                        <SelectValue
                                          placeholder={
                                            rewardsWalletsLoading
                                              ? "Loading wallets..."
                                              : "Choose a wallet"
                                          }
                                        />
                                      </SelectTrigger>
                                      <SelectContent className="bg-secondary text-black dark:text-white">
                                        <SelectItem value={NONE_WALLET_VALUE}>
                                          No tipping wallet
                                        </SelectItem>
                                        {tippingWalletOptions.map((option) => (
                                          <SelectItem
                                            key={`${loc.id}-tip-${option.value}`}
                                            value={option.value}
                                          >
                                            {option.label}
                                          </SelectItem>
                                        ))}
                                        <SelectItem value={CUSTOM_WALLET_VALUE}>
                                          Other
                                        </SelectItem>
                                      </SelectContent>
                                    </Select>
                                    {walletDraft.tippingWalletSelection === CUSTOM_WALLET_VALUE && (
                                      <Input
                                        value={walletDraft.tippingWalletCustomAddress}
                                        onChange={(e) => {
                                          updateLocationWalletDraft(loc.id, (current) => ({
                                            ...current,
                                            tippingWalletCustomAddress: e.target.value,
                                            error: "",
                                            success: "",
                                          }));
                                        }}
                                        onBlur={() =>
                                          void handleApplyLocationTippingWallet(loc.id, {
                                            silentIfEmptyCustom: true,
                                          })
                                        }
                                        onKeyDown={(e) => {
                                          if (e.key === "Enter") {
                                            e.preventDefault();
                                            void handleApplyLocationTippingWallet(loc.id, {
                                              silentIfEmptyCustom: true,
                                            });
                                          }
                                        }}
                                        placeholder="Paste address and press Enter"
                                        className="text-black dark:text-white bg-secondary"
                                        disabled={walletDraft.saving}
                                      />
                                    )}
                                  </div>
                                </div>

                                {walletDraft.error && (
                                  <p className="text-sm text-red-600 dark:text-red-400">
                                    {walletDraft.error}
                                  </p>
                                )}
                                {walletDraft.success && !walletDraft.saving && (
                                  <p className="text-sm text-green-600 dark:text-green-400">
                                    {walletDraft.success}
                                  </p>
                                )}
                              </div>
                            </div>
                          </CardContent>
                        </CollapsibleContent>
                      </Card>
                    </Collapsible>
                  );
                })}
              </div>
            )}

            {reviewMerchantLocations.length > 0 && (
              <div className="space-y-3">
                <div className="space-y-1">
                  <h2 className="text-lg font-semibold text-black dark:text-white">
                    Location requests
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    Pending and rejected locations stay here until they are approved.
                  </p>
                </div>

                {reviewMerchantLocations.map((loc) => {
                  const applicationStatus = getLocationApplicationStatus(loc.approval);
                  const borderClass =
                    applicationStatus === "rejected"
                      ? "border-red-300 dark:border-red-700"
                      : "border-yellow-300 dark:border-yellow-700";
                  const headerClass =
                    applicationStatus === "rejected"
                      ? "bg-red-50 dark:bg-red-900/20 rounded-t-lg"
                      : "bg-yellow-50 dark:bg-yellow-900/20 rounded-t-lg";
                  const Icon = applicationStatus === "rejected" ? XCircle : Clock;
                  const iconClass =
                    applicationStatus === "rejected"
                      ? "h-5 w-5 text-red-500 mr-2"
                      : "h-5 w-5 text-yellow-500 mr-2";
                  const statusTitle =
                    applicationStatus === "rejected"
                      ? "Location Application Not Approved"
                      : "Location Application Pending";
                  const statusBody =
                    applicationStatus === "rejected"
                      ? `Your application for ${loc.name} was not approved.`
                      : `Your application for ${loc.name} is currently under review.`;

                  return (
                    <Card className={borderClass} key={`merchant-status-${loc.id}`}>
                      <CardHeader className={headerClass}>
                        <CardTitle className="text-black dark:text-white flex items-center">
                          <Icon className={iconClass} />
                          {statusTitle}
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="space-y-2 pt-4">
                        <p className="font-medium text-black dark:text-white">{loc.name}</p>
                        <p className="text-gray-600 dark:text-gray-400">{statusBody}</p>
                      </CardContent>
                    </Card>
                  );
                })}
              </div>
            )}
          </TabsContent>
        )}

        {(affiliateStatus === "pending" || affiliateStatus === "approved") && (
          <TabsContent value="affiliate">
            <Card
              className={
                affiliateStatus === "approved"
                  ? "border-green-300 dark:border-green-700"
                  : "border-yellow-300 dark:border-yellow-700"
              }
            >
              <CardHeader
                className={`rounded-t-lg ${affiliateStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}
              >
                <CardTitle className="text-black dark:text-white flex items-center">
                  {affiliateStatus === "approved" ? (
                    <Check className="h-5 w-5 text-green-500 mr-2" />
                  ) : (
                    <Clock className="h-5 w-5 text-yellow-500 mr-2" />
                  )}
                  Affiliate{" "}
                  {affiliateStatus === "approved"
                    ? "Status Approved"
                    : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {affiliateStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">
                    Your affiliate request for{" "}
                    {affiliate?.organization || "your organization"} is under
                    review.
                  </p>
                )}
                {affiliateStatus === "approved" && (
                  <div className="space-y-4">
                    <p className="text-gray-600 dark:text-gray-400">
                      You are approved to create affiliate events for{" "}
                      {affiliate?.organization || "your organization"}.
                    </p>
                    <div className="space-y-3">
                      <Label className="text-black dark:text-white">
                        Affiliate Logo
                      </Label>
                      <div className="flex items-center gap-4">
                        <div className="h-16 w-16 rounded-xl bg-secondary border border-muted flex items-center justify-center overflow-hidden">
                          {affiliateLogoPreview ? (
                            <img
                              src={affiliateLogoPreview}
                              alt="Affiliate logo"
                              className="h-full w-full object-contain"
                            />
                          ) : (
                            <span className="text-xs text-muted-foreground">
                              No logo
                            </span>
                          )}
                        </div>
                        <div className="space-y-2">
                          <Input
                            type="file"
                            accept="image/*"
                            onChange={handleAffiliateLogoChange}
                            className="text-black dark:text-white bg-secondary"
                          />
                          <Button
                            type="button"
                            variant="outline"
                            className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                            disabled={
                              !affiliateLogoPreview || affiliateLogoSaving
                            }
                            onClick={handleAffiliateLogoSave}
                          >
                            {affiliateLogoSaving ? (
                              <>
                                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                Saving...
                              </>
                            ) : (
                              "Save Logo"
                            )}
                          </Button>
                        </div>
                      </div>
                      {affiliateLogoError && (
                        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                          <span>{affiliateLogoError}</span>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {(proposerStatus === "pending" || proposerStatus === "approved") && (
          <TabsContent value="proposer">
            <Card
              className={
                proposerStatus === "approved"
                  ? "border-green-300 dark:border-green-700"
                  : "border-yellow-300 dark:border-yellow-700"
              }
            >
              <CardHeader
                className={`rounded-t-lg ${proposerStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}
              >
                <CardTitle className="text-black dark:text-white flex items-center">
                  {proposerStatus === "approved" ? (
                    <Check className="h-5 w-5 text-green-500 mr-2" />
                  ) : (
                    <Clock className="h-5 w-5 text-yellow-500 mr-2" />
                  )}
                  Proposer{" "}
                  {proposerStatus === "approved"
                    ? "Status Approved"
                    : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {proposerStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">
                    Your proposer request for{" "}
                    {proposer?.organization || "your organization"} is under
                    review.
                  </p>
                )}
                {proposerStatus === "approved" && (
                  <>
                    <p className="text-gray-600 dark:text-gray-400 mb-4">
                      You are approved to create workflows for{" "}
                      {proposer?.organization || "your organization"}.
                    </p>
                    <Button
                      variant="outline"
                      className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                      onClick={() => router.push("/proposer")}
                    >
                      Open Proposer Panel
                    </Button>
                  </>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {(improverStatus === "pending" || improverStatus === "approved") && (
          <TabsContent value="improver">
            <Card
              className={
                improverStatus === "approved"
                  ? "border-green-300 dark:border-green-700"
                  : "border-yellow-300 dark:border-yellow-700"
              }
            >
              <CardHeader
                className={`rounded-t-lg ${improverStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}
              >
                <CardTitle className="text-black dark:text-white flex items-center">
                  {improverStatus === "approved" ? (
                    <Check className="h-5 w-5 text-green-500 mr-2" />
                  ) : (
                    <Clock className="h-5 w-5 text-yellow-500 mr-2" />
                  )}
                  Improver{" "}
                  {improverStatus === "approved"
                    ? "Status Approved"
                    : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {improverStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">
                    Your improver request is under review.
                  </p>
                )}
                {improverStatus === "approved" && (
                  <div className="space-y-4">
                    <p className="text-gray-600 dark:text-gray-400">
                      You are approved as an improver and can now claim eligible
                      workflow steps.
                    </p>
                    <div className="space-y-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-900/30">
                      <div className="space-y-1">
                        <Label
                          htmlFor="improver-primary-rewards-account"
                          className="text-black dark:text-white"
                        >
                          Primary Rewards Account
                        </Label>
                        <p className="text-xs text-gray-500 dark:text-gray-400">
                          Workflow rewards are paid to this account. Your
                          primary wallet is used by default until you choose a
                          different rewards account here.
                        </p>
                      </div>
                      <Select
                        value={
                          improverRewardsSelection ||
                          CUSTOM_REWARDS_ACCOUNT_VALUE
                        }
                        onValueChange={(value) => {
                          setImproverRewardsSelection(value);
                          setImproverRewardsError("");
                          setImproverRewardsSuccess("");
                        }}
                      >
                        <SelectTrigger
                          id="improver-primary-rewards-account"
                          className="text-black dark:text-white bg-secondary"
                        >
                          <SelectValue
                            placeholder={
                              rewardsWalletsLoading
                                ? "Loading accounts..."
                                : "Select an account"
                            }
                          />
                        </SelectTrigger>
                        <SelectContent className="bg-secondary text-black dark:text-white">
                          {rewardsAccountOptions.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                          <SelectItem value={CUSTOM_REWARDS_ACCOUNT_VALUE}>
                            Other
                          </SelectItem>
                        </SelectContent>
                      </Select>
                      {improverRewardsSelection ===
                        CUSTOM_REWARDS_ACCOUNT_VALUE && (
                        <Input
                          value={improverCustomRewardsAccount}
                          onChange={(e) => {
                            setImproverCustomRewardsAccount(e.target.value);
                            setImproverRewardsError("");
                            setImproverRewardsSuccess("");
                          }}
                          placeholder="0x..."
                          className="text-black dark:text-white bg-secondary"
                        />
                      )}
                      {rewardsWalletsError && (
                        <p className="text-sm text-red-600 dark:text-red-400">
                          {rewardsWalletsError}
                        </p>
                      )}
                      {improverRewardsError && (
                        <p className="text-sm text-red-600 dark:text-red-400">
                          {improverRewardsError}
                        </p>
                      )}
                      {improverRewardsSuccess && (
                        <p className="text-sm text-green-600 dark:text-green-400">
                          {improverRewardsSuccess}
                        </p>
                      )}
                      <Button
                        type="button"
                        variant="outline"
                        className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                        onClick={handleSaveImproverRewardsAccount}
                        disabled={improverRewardsSaving}
                      >
                        {improverRewardsSaving ? (
                          <>
                            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                            Saving...
                          </>
                        ) : (
                          "Save Rewards Account"
                        )}
                      </Button>
                    </div>
                    <Button
                      variant="outline"
                      className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                      onClick={() => router.push("/improver")}
                    >
                      Open Improver Panel
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {(issuerStatus === "pending" || issuerStatus === "approved") && (
          <TabsContent value="issuer">
            <Card
              className={
                issuerStatus === "approved"
                  ? "border-green-300 dark:border-green-700"
                  : "border-yellow-300 dark:border-yellow-700"
              }
            >
              <CardHeader
                className={`rounded-t-lg ${issuerStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}
              >
                <CardTitle className="text-black dark:text-white flex items-center">
                  {issuerStatus === "approved" ? (
                    <Check className="h-5 w-5 text-green-500 mr-2" />
                  ) : (
                    <Clock className="h-5 w-5 text-yellow-500 mr-2" />
                  )}
                  Issuer{" "}
                  {issuerStatus === "approved"
                    ? "Status Approved"
                    : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {issuerStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">
                    Your issuer request for{" "}
                    {issuer?.organization || "your organization"} is under
                    review.
                  </p>
                )}
                {issuerStatus === "approved" && (
                  <p className="text-gray-600 dark:text-gray-400">
                    You are approved to issue credentials on behalf of{" "}
                    {issuer?.organization || "your organization"}.
                  </p>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {(supervisorStatus === "pending" ||
          supervisorStatus === "approved") && (
          <TabsContent value="supervisor">
            <Card
              className={
                supervisorStatus === "approved"
                  ? "border-green-300 dark:border-green-700"
                  : "border-yellow-300 dark:border-yellow-700"
              }
            >
              <CardHeader
                className={`rounded-t-lg ${supervisorStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}
              >
                <CardTitle className="text-black dark:text-white flex items-center">
                  {supervisorStatus === "approved" ? (
                    <Check className="h-5 w-5 text-green-500 mr-2" />
                  ) : (
                    <Clock className="h-5 w-5 text-yellow-500 mr-2" />
                  )}
                  Supervisor{" "}
                  {supervisorStatus === "approved"
                    ? "Status Approved"
                    : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {supervisorStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">
                    Your supervisor request for{" "}
                    {supervisor?.organization || "your organization"} is under
                    review.
                  </p>
                )}
                {supervisorStatus === "approved" && (
                  <div className="space-y-4">
                    <p className="text-gray-600 dark:text-gray-400">
                      You are approved to supervise assigned workflows for{" "}
                      {supervisor?.organization || "your organization"}.
                    </p>
                    <div className="space-y-3 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-900/30">
                      <div className="space-y-1">
                        <Label
                          htmlFor="supervisor-primary-rewards-account"
                          className="text-black dark:text-white"
                        >
                          Primary Rewards Account
                        </Label>
                        <p className="text-xs text-gray-500 dark:text-gray-400">
                          Supervisor workflow rewards are paid to this account.
                          Your primary wallet is used by default until you
                          choose a different rewards account here.
                        </p>
                      </div>
                      <Select
                        value={
                          supervisorRewardsSelection ||
                          CUSTOM_REWARDS_ACCOUNT_VALUE
                        }
                        onValueChange={(value) => {
                          setSupervisorRewardsSelection(value);
                          setSupervisorRewardsError("");
                          setSupervisorRewardsSuccess("");
                        }}
                      >
                        <SelectTrigger
                          id="supervisor-primary-rewards-account"
                          className="text-black dark:text-white bg-secondary"
                        >
                          <SelectValue
                            placeholder={
                              rewardsWalletsLoading
                                ? "Loading accounts..."
                                : "Select an account"
                            }
                          />
                        </SelectTrigger>
                        <SelectContent className="bg-secondary text-black dark:text-white">
                          {rewardsAccountOptions.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                          <SelectItem value={CUSTOM_REWARDS_ACCOUNT_VALUE}>
                            Other
                          </SelectItem>
                        </SelectContent>
                      </Select>
                      {supervisorRewardsSelection ===
                        CUSTOM_REWARDS_ACCOUNT_VALUE && (
                        <Input
                          value={supervisorCustomRewardsAccount}
                          onChange={(e) => {
                            setSupervisorCustomRewardsAccount(e.target.value);
                            setSupervisorRewardsError("");
                            setSupervisorRewardsSuccess("");
                          }}
                          placeholder="0x..."
                          className="text-black dark:text-white bg-secondary"
                        />
                      )}
                      {rewardsWalletsError && (
                        <p className="text-sm text-red-600 dark:text-red-400">
                          {rewardsWalletsError}
                        </p>
                      )}
                      {supervisorRewardsError && (
                        <p className="text-sm text-red-600 dark:text-red-400">
                          {supervisorRewardsError}
                        </p>
                      )}
                      {supervisorRewardsSuccess && (
                        <p className="text-sm text-green-600 dark:text-green-400">
                          {supervisorRewardsSuccess}
                        </p>
                      )}
                      <Button
                        type="button"
                        variant="outline"
                        className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                        onClick={handleSaveSupervisorRewardsAccount}
                        disabled={supervisorRewardsSaving}
                      >
                        {supervisorRewardsSaving ? (
                          <>
                            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                            Saving...
                          </>
                        ) : (
                          "Save Rewards Account"
                        )}
                      </Button>
                    </div>
                    <Button
                      variant="outline"
                      className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                      onClick={() => router.push("/supervisor")}
                    >
                      Open Supervisor Panel
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        <TabsContent value="notifications">
          <Card>
            <CardHeader>
              <CardTitle className="text-black dark:text-white">
                Notification Preferences
              </CardTitle>
              <CardDescription>
                Manage how you receive notifications
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleNotificationUpdate} className="space-y-6">
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-base font-medium text-black dark:text-white">
                        Email Notifications
                      </h3>
                      <p className="text-sm text-gray-500 dark:text-gray-400">
                        Receive email notifications for important updates
                      </p>
                    </div>
                    <Switch
                      id="email-notifications"
                      checked={emailNotifications}
                      onCheckedChange={setEmailNotifications}
                    />
                  </div>

                  <Separator />

                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-base font-medium text-black dark:text-white">
                        Opportunity Alerts
                      </h3>
                      <p className="text-sm text-gray-500 dark:text-gray-400">
                        Get notified about new volunteer opportunities
                      </p>
                    </div>
                    <Switch
                      id="opportunity-alerts"
                      checked={opportunityAlerts}
                      onCheckedChange={setOpportunityAlerts}
                    />
                  </div>

                  <Separator />

                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-base font-medium text-black dark:text-white">
                        Transaction Alerts
                      </h3>
                      <p className="text-sm text-gray-500 dark:text-gray-400">
                        Receive notifications for SFLuv transactions
                      </p>
                    </div>
                    <Switch
                      id="transaction-alerts"
                      checked={transactionAlerts}
                      onCheckedChange={setTransactionAlerts}
                    />
                  </div>
                </div>

                <Button
                  type="submit"
                  className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]"
                  disabled={isUpdating}
                >
                  {isUpdating ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Updating...
                    </>
                  ) : (
                    "Save Notification Preferences"
                  )}
                </Button>
              </form>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog
        open={deleteAccountDialogOpen}
        onOpenChange={(open) => {
          if (deleteAccountSubmitting) {
            return;
          }
          setDeleteAccountDialogOpen(open);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete your account?</DialogTitle>
            <DialogDescription>
              This does not erase your data immediately. Your account will be
              marked inactive right away and scheduled for final deletion after
              30 days.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 text-sm text-muted-foreground">
            <div className="rounded-2xl border border-border bg-muted/40 p-4">
              <p className="font-medium text-foreground">
                Scheduled deletion date
              </p>
              <p className="mt-1">
                {formatDeletionDate(deleteAccountPreview?.delete_date) ||
                  getDeletionFallbackDate()}
              </p>
            </div>

            <p>
              Before the deletion request is submitted, any SFLuv in your
              accessible wallets will be transferred out of your account.
            </p>

            <p>
              If you change your mind later, sign in again and you&apos;ll be
              able to reactivate the account during the grace period.
            </p>

            <p>
              If you later reactivate during the grace period, contact{" "}
              <a
                className="font-semibold text-foreground underline underline-offset-4"
                href={`mailto:${ACCOUNT_RECOVERY_SUPPORT_EMAIL}`}
              >
                {ACCOUNT_RECOVERY_SUPPORT_EMAIL}
              </a>{" "}
              to recover your funds.
            </p>

            {deleteAccountSubmitting ? (
              <p className="rounded-xl border border-border bg-muted/40 px-3 py-2 text-foreground">
                {deleteAccountPhase === "sweeping"
                  ? "Transferring SFLuv out of your accessible wallets before submitting the deletion request."
                  : "Submitting your deletion request now."}
              </p>
            ) : null}

            {deleteAccountError ? (
              <p className="rounded-xl border border-red-400/40 bg-red-100/70 px-3 py-2 text-red-700 dark:bg-red-500/10 dark:text-red-200">
                {deleteAccountError}
              </p>
            ) : null}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={deleteAccountSubmitting}
              onClick={closeDeleteAccountDialog}
            >
              Keep account
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteAccountSubmitting}
              onClick={() => {
                void handleDeleteAccount();
              }}
            >
              {deleteAccountSubmitting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {deleteAccountPhase === "sweeping"
                    ? "Transferring SFLuv..."
                    : "Submitting deletion..."}
                </>
              ) : (
                "Confirm delete"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
