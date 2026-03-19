"use client"

import React, { useEffect, useMemo, useState } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { useMerchants } from "@/hooks/api/use-merchants"
import { useToast } from "@/hooks/use-toast"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { buildCredentialBadgeDataUrl, buildCredentialLabelMap, formatCredentialLabel } from "@/lib/credential-labels"
import { formatStatusLabel } from "@/lib/status-labels"
import { formatWorkflowDisplayStatus } from "@/lib/workflow-status"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Calendar } from "@/components/ui/calendar"
import { cn } from "@/lib/utils"
import {
  Coins,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Users,
  FileCheck,
  Building2,
  Mail,
  Phone,
  MapPin,
  Check,
  X,
  Loader2,
  CreditCard,
  Wallet,
  Plus,
  Eye,
  EyeOff,
  QrCode,
  Download,
  CalendarIcon,
  Leaf,
  AlertTriangle,
  Search,
  ChevronLeft,
  ChevronRight,
  Copy,
} from "lucide-react"
import { useLocation } from "@/context/LocationProvider"
import { AuthedLocation, UpdateLocationApprovalRequest } from "@/types/location"
import { AppWallet } from "@/lib/wallets/wallets"
import { FAUCET_ADDRESS, SFLUV_DECIMALS, SFLUV_TOKEN } from "@/lib/constants"
import { Affiliate } from "@/types/affiliate"
import { Proposer } from "@/types/proposer"
import { Improver } from "@/types/improver"
import { Supervisor } from "@/types/supervisor"
import { IssuerRecord, IssuerWithScopes } from "@/types/issuer"
import {
  AdminWorkflowListItem,
  AdminWorkflowListResponse,
  CredentialVisibility,
  CredentialType,
  GlobalCredentialType,
  Workflow,
  WorkflowSeriesClaimant,
  WorkflowSeriesClaimRevokeResult,
} from "@/types/workflow"
import { Event, EventsStatus } from "@/types/event"
import { AddEventModal } from "@/components/events/add-event-modal"
import { EventModal } from "@/components/events/event-modal"
import { DrainFaucetModal } from "@/components/events/drain-faucet-modal"
import { WorkflowDetailsModal } from "@/components/workflows/workflow-details-modal"
import EventCard from "@/components/events/event-card"
import type { W9Submission } from "@/types/w9"

// Mock PayPal accounts
const mockPaypalAccounts = [
  {
    id: "paypal-1",
    email: "admin@sfluv.com",
    name: "SFLuv Admin Account",
    isVerified: true,
    isDefault: true,
  },
  {
    id: "paypal-2",
    email: "business@sfluv.com",
    name: "SFLuv Business Account",
    isVerified: true,
    isDefault: false,
  },
]

type ApprovalStatus = "pending" | "approved" | "rejected"

const approvalToStatus = (approval?: boolean | null): ApprovalStatus => {
  if (approval === true) return "approved"
  if (approval === false) return "rejected"
  return "pending"
}

const statusToApproval = (status: ApprovalStatus): boolean | null => {
  if (status === "approved") return true
  if (status === "rejected") return false
  return null
}

type AdminWorkflowSeriesGroup = {
  seriesId: string
  workflows: AdminWorkflowListItem[]
}

const maxCredentialBadgeUploadBytes = 2 * 1024 * 1024
const maxCredentialBadgeUploadLabel = "2MB"
const credentialTypesPageSize = 5

const credentialVisibilityOptions: Array<{
  value: CredentialVisibility
  label: string
  description: string
}> = [
  {
    value: "public",
    label: "Public",
    description: "Any improver can request this credential at any time.",
  },
  {
    value: "unlisted",
    label: "Unlisted",
    description: "Only requestable via a direct /improvers/join link.",
  },
  {
    value: "private",
    label: "Private",
    description: "Never requestable by improvers.",
  },
]

const normalizeCredentialVisibility = (value?: string | null): CredentialVisibility => {
  if (value === "private" || value === "unlisted") return value
  return "public"
}

const getCredentialVisibilityLabel = (value?: string | null): string => {
  const visibility = normalizeCredentialVisibility(value)
  const option = credentialVisibilityOptions.find((candidate) => candidate.value === visibility)
  return option?.label || "Public"
}

const getCredentialVisibilityBadgeClassName = (value?: string | null): string => {
  const visibility = normalizeCredentialVisibility(value)
  if (visibility === "private") return "border-red-300 text-red-700 bg-red-50 dark:border-red-900/60 dark:text-red-300 dark:bg-red-950/30"
  if (visibility === "unlisted") return "border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-900/60 dark:text-amber-300 dark:bg-amber-950/30"
  return "border-emerald-300 text-emerald-700 bg-emerald-50 dark:border-emerald-900/60 dark:text-emerald-300 dark:bg-emerald-950/30"
}

export default function AdminPage() {
  const { user, wallets, authFetch, status } = useApp()
  const { getAuthedMapLocations, authedMapLocations} = useLocation()
  const { toast } = useToast()
  const pathname = usePathname()
  const router = useRouter()
  const searchParams = useSearchParams()

  const readQueryNumber = (key: string, fallback: number) => {
    const rawValue = searchParams.get(key)
    if (rawValue === null || rawValue.trim() === "") return fallback
    const raw = Number(rawValue)
    if (!Number.isFinite(raw)) return fallback
    return raw >= 0 ? raw : fallback
  }

  const readQueryBoolean = (key: string, fallback: boolean) => {
    const raw = searchParams.get(key)
    if (raw === "true") return true
    if (raw === "false") return false
    return fallback
  }

  const readQueryText = (key: string, fallback: string) => {
    const raw = searchParams.get(key)
    return raw === null ? fallback : raw
  }

  const tabFromQuery = searchParams.get("tab")
  const isValidAdminTab = (value: string | null): value is string => {
    if (!value) return false
    return [
      "events",
      "w9",
      "merchants",
      "affiliates",
      "proposers",
      "improvers",
      "supervisors",
      "workflows",
      "issuers",
      "credential-types",
    ].includes(value)
  }

  // Global wallet selection
  const [selectedWallet, setSelectedWallet] = useState<AppWallet | null>(null)
  const [selectedWalletBYUSDBalance, setSelectedWalletBYUSDBalance] = useState<number>(0)
  const [selectedWalletSFLUVBalance, setSelectedWalletSFLUVBalance] = useState<number>(0)

  const [pendingW9Submissions, setPendingW9Submissions] = useState<W9Submission[]>([])
  const [w9Loading, setW9Loading] = useState<boolean>(false)


  // Token management state
  const [amount, setAmount] = useState("")
  const [conversionType, setConversionType] = useState<"wrap" | "unwrap">("wrap")
  const [isProcessing, setIsProcessing] = useState(false)

  // PayPal conversion state
  const [paypalAmount, setPaypalAmount] = useState("")
  const [selectedPaypalAccount, setSelectedPaypalAccount] = useState<string>("")
  const [isConvertingToPaypal, setIsConvertingToPaypal] = useState(false)
  const [paypalAccounts, setPaypalAccounts] = useState(mockPaypalAccounts)

  // PayPal account modal state
  const [isPaypalModalOpen, setIsPaypalModalOpen] = useState(false)
  const [newPaypalEmail, setNewPaypalEmail] = useState("")
  const [newPaypalPassword, setNewPaypalPassword] = useState("")
  const [newPaypalFirstName, setNewPaypalFirstName] = useState("")
  const [newPaypalLastName, setNewPaypalLastName] = useState("")
  const [newPaypalPhone, setNewPaypalPhone] = useState("")
  const [newPaypalAccountName, setNewPaypalAccountName] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const [isConnectingPaypal, setIsConnectingPaypal] = useState(false)

  // Merchant review modal state
  const [selectedLocationForReview, setselectedLocationForReview] = useState<any>(null)
  const [isLocationReviewModalOpen, setisLocationReviewModalOpen] = useState(false)
  const [merchantStatusFilter, setMerchantStatusFilter] = useState<string>(readQueryText("merchant_status", "all"))
  const [merchantSearch, setMerchantSearch] = useState<string>(readQueryText("merchant_search", ""))
  const [merchantStatusDraft, setMerchantStatusDraft] = useState<ApprovalStatus>("pending")
  const [merchantModalSaving, setMerchantModalSaving] = useState<boolean>(false)
  const [merchantModalError, setMerchantModalError] = useState<string>("")

  // QR code generation state
  const [eventStartDate, setEventStartDate] = useState<Date>()
  const [eventEndDate, setEventEndDate] = useState<Date>()
  const [eventStartTime, setEventStartTime] = useState("")
  const [eventEndTime, setEventEndTime] = useState("")
  const [sfluvPerCode, setSfluvPerCode] = useState("")
  const [numberOfCodes, setNumberOfCodes] = useState("")
  const [isGeneratingCodes, setIsGeneratingCodes] = useState(false)
  const [generatedCodes, setGeneratedCodes] = useState<any[]>([])
  const [pendingLocations, setPendingLocations] = useState<AuthedLocation[]>([])
  const [faucetBalance, setFaucetBalance] = useState<string | bigint>("-")
  const [unallocatedBalance, setUnallocatedBalance] = useState<number | undefined>(undefined)
  const [activeTab, setActiveTab] = useState<string>(isValidAdminTab(tabFromQuery) ? (tabFromQuery as string) : "merchants")

  // Events
  const [events, setEvents] = useState<Event[]>([])
  const [eventsStatus, setEventsStatus] = useState<EventsStatus>("loading")
  const [eventsError, setEventsError] = useState<string>("")
  const [eventsSearch, setEventsSearch] = useState<string>(readQueryText("events_search", ""))
  const [eventsPage, setEventsPage] = useState<number>(readQueryNumber("events_page", 0))
  const [eventsCount, setEventsCount] = useState<number>(readQueryNumber("events_count", 10))
  const [eventsExpired, setEventsExpired] = useState<boolean>(readQueryBoolean("events_expired", false))
  const [eventsModalOpen, setEventsModalOpen] = useState<boolean>(false)
  const [eventDetailModalOpen, setEventDetailModalOpen] = useState<boolean>(false)
  const [deleteEventError, setDeleteEventError] = useState<string | undefined>(undefined)
  const [eventDetailsEvent, setEventDetailsEvent] = useState<Event | undefined>(undefined)
  const [drainFaucetModalOpen, setDrainFaucetModalOpen] = useState<boolean>(false)
  const [drainFaucetError, setDrainFaucetError] = useState<boolean>(false)

  // Affiliates
  const [affiliates, setAffiliates] = useState<Affiliate[]>([])
  const [affiliatesError, setAffiliatesError] = useState<string>("")
  const [affiliateModalOpen, setAffiliateModalOpen] = useState<boolean>(false)
  const [selectedAffiliate, setSelectedAffiliate] = useState<Affiliate | null>(null)
  const [affiliateNickname, setAffiliateNickname] = useState<string>("")
  const [affiliateWeeklyBalance, setAffiliateWeeklyBalance] = useState<string>("")
  const [affiliateBonus, setAffiliateBonus] = useState<string>("")
  const [affiliateUpdating, setAffiliateUpdating] = useState<boolean>(false)
  const [affiliateModalError, setAffiliateModalError] = useState<string>("")
  const [eventsOwnerFilter, setEventsOwnerFilter] = useState<string>(readQueryText("events_owner", "all"))
  const [affiliateStatusFilter, setAffiliateStatusFilter] = useState<string>(readQueryText("affiliate_status", "all"))
  const [affiliateStatusDraft, setAffiliateStatusDraft] = useState<Affiliate["status"]>("pending")
  const [affiliateSearch, setAffiliateSearch] = useState<string>(readQueryText("affiliate_search", ""))
  const [affiliatePage, setAffiliatePage] = useState<number>(readQueryNumber("affiliate_page", 0))

  // Proposers
  const [proposers, setProposers] = useState<Proposer[]>([])
  const [proposersError, setProposersError] = useState<string>("")
  const [proposerModalOpen, setProposerModalOpen] = useState<boolean>(false)
  const [selectedProposer, setSelectedProposer] = useState<Proposer | null>(null)
  const [proposerNickname, setProposerNickname] = useState<string>("")
  const [proposerUpdating, setProposerUpdating] = useState<boolean>(false)
  const [proposerModalError, setProposerModalError] = useState<string>("")
  const [proposerStatusFilter, setProposerStatusFilter] = useState<string>(readQueryText("proposer_status", "all"))
  const [proposerStatusDraft, setProposerStatusDraft] = useState<Proposer["status"]>("pending")
  const [proposerSearch, setProposerSearch] = useState<string>(readQueryText("proposer_search", ""))
  const [proposerPage, setProposerPage] = useState<number>(readQueryNumber("proposer_page", 0))

  // Improvers
  const [improvers, setImprovers] = useState<Improver[]>([])
  const [improversError, setImproversError] = useState<string>("")
  const [improverStatusFilter, setImproverStatusFilter] = useState<string>(readQueryText("improver_status", "all"))
  const [improverModalOpen, setImproverModalOpen] = useState<boolean>(false)
  const [selectedImprover, setSelectedImprover] = useState<Improver | null>(null)
  const [improverStatusDraft, setImproverStatusDraft] = useState<Improver["status"]>("pending")
  const [improverModalUpdating, setImproverModalUpdating] = useState<boolean>(false)
  const [improverModalError, setImproverModalError] = useState<string>("")
  const [improverSearch, setImproverSearch] = useState<string>(readQueryText("improver_search", ""))
  const [improverPage, setImproverPage] = useState<number>(readQueryNumber("improver_page", 0))

  // Supervisors
  const [supervisors, setSupervisors] = useState<Supervisor[]>([])
  const [supervisorsError, setSupervisorsError] = useState<string>("")
  const [supervisorStatusFilter, setSupervisorStatusFilter] = useState<string>(readQueryText("supervisor_status", "all"))
  const [supervisorModalOpen, setSupervisorModalOpen] = useState<boolean>(false)
  const [selectedSupervisor, setSelectedSupervisor] = useState<Supervisor | null>(null)
  const [supervisorNickname, setSupervisorNickname] = useState<string>("")
  const [supervisorStatusDraft, setSupervisorStatusDraft] = useState<Supervisor["status"]>("pending")
  const [supervisorModalUpdating, setSupervisorModalUpdating] = useState<boolean>(false)
  const [supervisorModalError, setSupervisorModalError] = useState<string>("")
  const [supervisorSearch, setSupervisorSearch] = useState<string>(readQueryText("supervisor_search", ""))
  const [supervisorPage, setSupervisorPage] = useState<number>(readQueryNumber("supervisor_page", 0))

  // Issuer requests
  const [issuerRequests, setIssuerRequests] = useState<IssuerRecord[]>([])
  const [issuerRequestsError, setIssuerRequestsError] = useState<string>("")
  const [issuerRequestSaving, setIssuerRequestSaving] = useState<Record<string, boolean>>({})
  const [issuerRequestModalOpen, setIssuerRequestModalOpen] = useState<boolean>(false)
  const [selectedIssuerRequest, setSelectedIssuerRequest] = useState<IssuerRecord | null>(null)
  const [issuerRequestNickname, setIssuerRequestNickname] = useState<string>("")
  const [issuerRequestStatusDraft, setIssuerRequestStatusDraft] = useState<string>("pending")
  const [issuerRequestModalError, setIssuerRequestModalError] = useState<string>("")
  const [issuerStatusFilter, setIssuerStatusFilter] = useState<string>(readQueryText("issuer_status", "all"))
  const [issuerRequestSearch, setIssuerRequestSearch] = useState<string>(readQueryText("issuer_search", ""))
  const [issuerRequestPage, setIssuerRequestPage] = useState<number>(readQueryNumber("issuer_page", 0))

  // Issuer credential scopes
  const [issuers, setIssuers] = useState<IssuerWithScopes[]>([])
  const [issuersError, setIssuersError] = useState<string>("")
  const [issuerScopes, setIssuerScopes] = useState<CredentialType[]>([])
  const [issuerScopePicker, setIssuerScopePicker] = useState<string>("")
  const [issuerSaving, setIssuerSaving] = useState<boolean>(false)

  // Credential types
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [credentialTypesError, setCredentialTypesError] = useState<string>("")
  const [newCredentialValue, setNewCredentialValue] = useState<string>("")
  const [newCredentialLabel, setNewCredentialLabel] = useState<string>("")
  const [newCredentialVisibility, setNewCredentialVisibility] = useState<CredentialVisibility>("public")
  const [credentialTypeSaving, setCredentialTypeSaving] = useState<boolean>(false)
  const [credentialTypeSearch, setCredentialTypeSearch] = useState<string>("")
  const [credentialTypePage, setCredentialTypePage] = useState<number>(0)
  const [credentialTypeModalOpen, setCredentialTypeModalOpen] = useState<boolean>(false)
  const [selectedCredentialType, setSelectedCredentialType] = useState<GlobalCredentialType | null>(null)
  const [credentialTypeDraftLabel, setCredentialTypeDraftLabel] = useState<string>("")
  const [credentialTypeDraftVisibility, setCredentialTypeDraftVisibility] = useState<CredentialVisibility>("public")
  const [credentialTypeDraftBadgeDataBase64, setCredentialTypeDraftBadgeDataBase64] = useState<string>("")
  const [credentialTypeDraftBadgeContentType, setCredentialTypeDraftBadgeContentType] = useState<string>("")
  const [credentialTypeDraftClearBadge, setCredentialTypeDraftClearBadge] = useState<boolean>(false)
  const [credentialTypeModalSaving, setCredentialTypeModalSaving] = useState<boolean>(false)
  const [credentialTypeModalError, setCredentialTypeModalError] = useState<string>("")

  // Workflows (admin)
  const [adminWorkflows, setAdminWorkflows] = useState<AdminWorkflowListItem[]>([])
  const [adminWorkflowsTotal, setAdminWorkflowsTotal] = useState<number>(0)
  const [adminWorkflowsPage, setAdminWorkflowsPage] = useState<number>(readQueryNumber("workflow_page", 0))
  const [adminWorkflowsCount] = useState<number>(20)
  const [adminWorkflowsSearch, setAdminWorkflowsSearch] = useState<string>(readQueryText("workflow_search", ""))
  const [adminWorkflowsIncludeArchived, setAdminWorkflowsIncludeArchived] = useState<boolean>(readQueryBoolean("workflow_include_archived", false))
  const [adminWorkflowsError, setAdminWorkflowsError] = useState<string>("")
  const [adminWorkflowDetail, setAdminWorkflowDetail] = useState<Workflow | null>(null)
  const [adminWorkflowDetailOpen, setAdminWorkflowDetailOpen] = useState<boolean>(false)
  const [adminWorkflowDetailLoading, setAdminWorkflowDetailLoading] = useState<boolean>(false)
  const [adminSeriesCardIndexById, setAdminSeriesCardIndexById] = useState<Record<string, number>>({})
  const [adminDetailSeriesContext, setAdminDetailSeriesContext] = useState<{
    seriesId: string
    workflowIds: string[]
    index: number
  } | null>(null)
  const [adminRevokeModalOpen, setAdminRevokeModalOpen] = useState<boolean>(false)
  const [adminRevokeClaimants, setAdminRevokeClaimants] = useState<WorkflowSeriesClaimant[]>([])
  const [adminRevokeLoading, setAdminRevokeLoading] = useState<boolean>(false)
  const [adminRevokeSeriesId, setAdminRevokeSeriesId] = useState<string>("")
  const [adminRevokeImproverId, setAdminRevokeImproverId] = useState<string>("")
  const [adminRevokeError, setAdminRevokeError] = useState<string>("")
  const [adminRevokeSubmitting, setAdminRevokeSubmitting] = useState<boolean>(false)

  const credentialLabelMap = useMemo(
    () => buildCredentialLabelMap(credentialTypes),
    [credentialTypes],
  )

  const toggleNewEventModal = () => {
    setEventsModalOpen(!eventsModalOpen)
  }

  const toggleEventDetailModal = () => {
    setEventDetailModalOpen(!eventDetailModalOpen)
  }

  const toggleDrainFaucetModal = () => {
    setDrainFaucetModalOpen(!drainFaucetModalOpen)
  }

  const handleDrainFaucet = async () => {
    if(BigInt(unallocatedBalance || 0) != faucetBalance) {
      setEventsError("Delete active events before draining faucet balance.")
      return
    }
    const url = "/drain"
    try {
      const res = await authFetch(url, {
        method: "POST"
      })
      if(res.status !== 201) throw new Error()
      setDrainFaucetError(false)
      getFaucetBalance()
    }
    catch {
      setDrainFaucetError(true)
      setEventsError("")
    }
  }

  const handleDeleteEvent = async (id: string) => {
    const url = "/events/" + id
    try {
      const res = await authFetch(url, {
        method: "DELETE",
      })
      if(res.status !== 200) throw new Error()
    }
    catch {
      setEventsStatus("error")
      setEventsError("Error adding event. Please try again later.")
    }

    await getEvents()
    toggleEventDetailModal()
    getUnallocatedBalance()
  }

  const handleAddEvent = async (ev: Event): Promise<boolean> => {
    const url = "/events"
    try {
      const res = await authFetch(url, {
        method: "POST",
        body: JSON.stringify(ev)
      })
      if (!res.ok) {
        const message = (await res.text()).trim()
        throw new Error(message || "Error adding event. Please try again later.")
      }
      setEventsError("")
      await getEvents()
      getUnallocatedBalance()
      return true
    }
    catch (error) {
      const message = error instanceof Error ? error.message : "Error adding event. Please try again later."
      setEventsStatus("error")
      setEventsError(message)
      return false
    }
  }

  const getFaucetBalance = async () => {
    const decimals = 10 ** SFLUV_DECIMALS
    const bal = await wallets[0]?.getBalanceOf(SFLUV_TOKEN, FAUCET_ADDRESS)

    setFaucetBalance(bal ? bal / BigInt(decimals) : "-")
    getUnallocatedBalance()
  }



  const getEvents = async () => {
    const url = "/events"
      + "?page=" + eventsPage
      + "&count=" + eventsCount
      + "&expired=" + eventsExpired
      + (eventsSearch ? "&search=" + eventsSearch : "")
    try {
      const res = await authFetch(url)

      const e = await res.json()
      console.log(e)
      setEvents(e)
    }
    catch {
      setEventsStatus("error")
      setEventsError("Error fetching events. Please try again later.")
    }
  }

  const getUnallocatedBalance = async () => {
    try {
      const res = await authFetch("/balance")
      const bal = await res.json()
      const decimals = 10 ** SFLUV_DECIMALS
      setUnallocatedBalance(Number((BigInt(bal) / BigInt(decimals))))
    }
    catch {
      setEventsError("Error getting unallocated faucet balance.")
    }
  }

  const affiliateLabelMap = useMemo(() => {
    const map = new Map<string, string>()
    affiliates.forEach((affiliate) => {
      map.set(affiliate.user_id, affiliate.nickname || affiliate.organization)
    })
    return map
  }, [affiliates])

  const getOwnerLabel = (owner?: string) => {
    if (!owner) return "Admin"
    const label = affiliateLabelMap.get(owner)
    if (label) return label
    return `Admin`
  }

  const ownerOptions = useMemo(() => {
    const owners = new Map<string, string>()
    events.forEach((event) => {
      if (event.owner) {
        owners.set(event.owner, getOwnerLabel(event.owner))
      }
    })
    return Array.from(owners.entries())
      .map(([value, label]) => ({ value, label }))
      .sort((a, b) => a.label.localeCompare(b.label))
  }, [events, affiliateLabelMap])

  const filteredEvents = useMemo(() => {
    if (eventsOwnerFilter === "all") return events
    return events.filter((event) => event.owner === eventsOwnerFilter)
  }, [events, eventsOwnerFilter])

  const filteredMerchants = useMemo(() => {
    const s = merchantSearch.trim().toLowerCase()
    return authedMapLocations.filter((location) => {
      const matchesStatus = merchantStatusFilter === "all" || approvalToStatus(location.approval) === merchantStatusFilter
      const matchesSearch = !s || location.name.toLowerCase().includes(s) || (location.city || "").toLowerCase().includes(s)
      return matchesStatus && matchesSearch
    })
  }, [authedMapLocations, merchantStatusFilter, merchantSearch])

  const filteredAffiliates = useMemo(() => {
    if (affiliateStatusFilter === "all") return affiliates
    return affiliates.filter((affiliate) => affiliate.status === affiliateStatusFilter)
  }, [affiliates, affiliateStatusFilter])

  const filteredProposers = useMemo(() => {
    if (proposerStatusFilter === "all") return proposers
    return proposers.filter((proposer) => proposer.status === proposerStatusFilter)
  }, [proposers, proposerStatusFilter])

  const filteredImprovers = useMemo(() => {
    if (improverStatusFilter === "all") return improvers
    return improvers.filter((improver) => improver.status === improverStatusFilter)
  }, [improvers, improverStatusFilter])

  const filteredSupervisors = useMemo(() => {
    if (supervisorStatusFilter === "all") return supervisors
    return supervisors.filter((supervisor) => supervisor.status === supervisorStatusFilter)
  }, [supervisors, supervisorStatusFilter])

  const filteredIssuerRequests = useMemo(() => {
    if (issuerStatusFilter === "all") return issuerRequests
    return issuerRequests.filter((issuer) => issuer.status === issuerStatusFilter)
  }, [issuerRequests, issuerStatusFilter])

  const filteredCredentialTypes = useMemo(() => {
    const search = credentialTypeSearch.trim().toLowerCase()
    if (!search) return credentialTypes
    return credentialTypes.filter((credentialType) => {
      const label = credentialType.label.toLowerCase()
      const slug = credentialType.value.toLowerCase()
      return label.includes(search) || slug.includes(search)
    })
  }, [credentialTypeSearch, credentialTypes])

  const credentialTypesTotalPages = useMemo(
    () => Math.max(1, Math.ceil(filteredCredentialTypes.length / credentialTypesPageSize)),
    [filteredCredentialTypes.length],
  )

  const paginatedCredentialTypes = useMemo(() => {
    const start = credentialTypePage * credentialTypesPageSize
    return filteredCredentialTypes.slice(start, start + credentialTypesPageSize)
  }, [credentialTypePage, filteredCredentialTypes])

  const credentialTypeDraftBadgePreview = useMemo(() => {
    if (credentialTypeDraftClearBadge) return null
    if (credentialTypeDraftBadgeDataBase64 && credentialTypeDraftBadgeContentType) {
      return `data:${credentialTypeDraftBadgeContentType};base64,${credentialTypeDraftBadgeDataBase64}`
    }
    return buildCredentialBadgeDataUrl(selectedCredentialType)
  }, [
    credentialTypeDraftBadgeContentType,
    credentialTypeDraftBadgeDataBase64,
    credentialTypeDraftClearBadge,
    selectedCredentialType,
  ])

  const groupedAdminWorkflows = useMemo<AdminWorkflowSeriesGroup[]>(() => {
    const groups = new Map<string, AdminWorkflowSeriesGroup>()
    adminWorkflows.forEach((workflow) => {
      const existing = groups.get(workflow.series_id)
      if (!existing) {
        groups.set(workflow.series_id, {
          seriesId: workflow.series_id,
          workflows: [workflow],
        })
        return
      }
      groups.set(workflow.series_id, {
        ...existing,
        workflows: [...existing.workflows, workflow],
      })
    })

    return Array.from(groups.values())
      .map((group) => ({
        ...group,
        workflows: [...group.workflows].sort((a, b) => a.start_at - b.start_at),
      }))
      .sort((a, b) => {
        const aStart = a.workflows[0]?.start_at || 0
        const bStart = b.workflows[0]?.start_at || 0
        return bStart - aStart
      })
  }, [adminWorkflows])

  const getAffiliates = async (search = affiliateSearch, page = affiliatePage) => {
    try {
      const params = new URLSearchParams({ page: String(page), count: "20" })
      if (search) params.set("search", search)
      const res = await authFetch(`/admin/affiliates?${params}`)
      if (!res.ok) throw new Error()
      const data = await res.json()
      setAffiliates(data || [])
      setAffiliatesError("")
    } catch {
      setAffiliatesError("Error fetching affiliates. Please try again later.")
    }
  }

  const openAffiliateModal = (affiliate: Affiliate) => {
    setSelectedAffiliate(affiliate)
    setAffiliateNickname(affiliate.nickname || "")
    setAffiliateWeeklyBalance(String(affiliate.weekly_allocation ?? affiliate.weekly_balance ?? 0))
    setAffiliateBonus(String(affiliate.one_time_balance ?? 0))
    setAffiliateStatusDraft(affiliate.status)
    setAffiliateModalError("")
    setAffiliateModalOpen(true)
  }

  const submitAffiliateUpdate = async (payload: Record<string, unknown>) => {
    setAffiliateUpdating(true)
    setAffiliateModalError("")
    try {
      const res = await authFetch("/admin/affiliates", {
        method: "PUT",
        body: JSON.stringify(payload),
      })
      if (!res.ok) throw new Error()

      const updated = await res.json()
      setAffiliates((prev) =>
        prev.map((affiliate) => (affiliate.user_id === updated.user_id ? updated : affiliate)),
      )
      setSelectedAffiliate(updated)
      setAffiliateBonus(String(updated?.one_time_balance ?? 0))
      setAffiliateStatusDraft(updated.status)
    } catch {
      setAffiliateModalError("Unable to update affiliate right now. Please try again.")
    } finally {
      setAffiliateUpdating(false)
    }
  }

  const handleAffiliateSave = async () => {
    if (!selectedAffiliate) return

    const payload: Record<string, unknown> = {
      user_id: selectedAffiliate.user_id,
      status: affiliateStatusDraft,
    }

    const weekly = Number(affiliateWeeklyBalance)
    if (!Number.isNaN(weekly)) {
      payload.weekly_balance = weekly
    }

    payload.nickname = affiliateNickname

    const bonusValue = affiliateBonus.trim()
    if (bonusValue !== "") {
      const bonus = Number(bonusValue)
      if (!Number.isNaN(bonus)) {
        payload.one_time_balance = bonus
      }
    }

    await submitAffiliateUpdate(payload)
  }

  const getProposers = async (search = proposerSearch, page = proposerPage) => {
    try {
      const params = new URLSearchParams({ page: String(page), count: "20" })
      if (search) params.set("search", search)
      const res = await authFetch(`/admin/proposers?${params}`)
      if (!res.ok) throw new Error()
      const data = await res.json()
      setProposers(data || [])
      setProposersError("")
    } catch {
      setProposersError("Error fetching proposers. Please try again later.")
    }
  }

  const openProposerModal = (proposer: Proposer) => {
    setSelectedProposer(proposer)
    setProposerNickname(proposer.nickname || "")
    setProposerStatusDraft(proposer.status)
    setProposerModalError("")
    setProposerModalOpen(true)
  }

  const submitProposerUpdate = async (payload: Record<string, unknown>) => {
    setProposerUpdating(true)
    setProposerModalError("")
    try {
      const res = await authFetch("/admin/proposers", {
        method: "PUT",
        body: JSON.stringify(payload),
      })
      if (!res.ok) throw new Error()

      const updated = await res.json()
      setProposers((prev) =>
        prev.map((proposer) => (proposer.user_id === updated.user_id ? updated : proposer)),
      )
      setSelectedProposer(updated)
      setProposerStatusDraft(updated.status)
    } catch {
      setProposerModalError("Unable to update proposer right now. Please try again.")
    } finally {
      setProposerUpdating(false)
    }
  }

  const handleProposerSave = async () => {
    if (!selectedProposer) return

    const payload: Record<string, unknown> = {
      user_id: selectedProposer.user_id,
      nickname: proposerNickname,
      status: proposerStatusDraft,
    }

    await submitProposerUpdate(payload)
  }

  const getImprovers = async (search = improverSearch, page = improverPage) => {
    try {
      const params = new URLSearchParams({ page: String(page), count: "20" })
      if (search) params.set("search", search)
      const res = await authFetch(`/admin/improvers?${params}`)
      if (!res.ok) throw new Error()
      const data = await res.json()
      setImprovers(data || [])
      setImproversError("")
    } catch {
      setImproversError("Error fetching improvers. Please try again later.")
    }
  }

  const openImproverModal = (improver: Improver) => {
    setSelectedImprover(improver)
    setImproverStatusDraft(improver.status)
    setImproverModalError("")
    setImproverModalOpen(true)
  }

  const updateImproverStatus = async (user_id: string, status: Improver["status"]) => {
    setImproverModalUpdating(true)
    setImproversError("")
    setImproverModalError("")
    try {
      const res = await authFetch("/admin/improvers", {
        method: "PUT",
        body: JSON.stringify({ user_id, status }),
      })
      if (!res.ok) throw new Error()
      const updated = await res.json()
      setImprovers((prev) => prev.map((improver) => (improver.user_id === updated.user_id ? updated : improver)))
      setSelectedImprover((prev) => (prev?.user_id === updated.user_id ? updated : prev))
      return true
    } catch {
      setImproversError("Unable to update improver right now. Please try again.")
      setImproverModalError("Unable to update improver right now. Please try again.")
      return false
    } finally {
      setImproverModalUpdating(false)
    }
  }

  const saveImproverModal = async () => {
    if (!selectedImprover) return
    const ok = await updateImproverStatus(selectedImprover.user_id, improverStatusDraft)
    if (ok) setImproverModalOpen(false)
  }

  const getSupervisors = async (search = supervisorSearch, page = supervisorPage) => {
    try {
      const params = new URLSearchParams({ page: String(page), count: "20" })
      if (search) params.set("search", search)
      const res = await authFetch(`/admin/supervisors?${params}`)
      if (!res.ok) throw new Error()
      const data = await res.json()
      setSupervisors(data || [])
      setSupervisorsError("")
    } catch {
      setSupervisorsError("Error fetching supervisors. Please try again later.")
    }
  }

  const openSupervisorModal = (supervisor: Supervisor) => {
    setSelectedSupervisor(supervisor)
    setSupervisorNickname(supervisor.nickname || "")
    setSupervisorStatusDraft(supervisor.status)
    setSupervisorModalError("")
    setSupervisorModalOpen(true)
  }

  const updateSupervisor = async (payload: { user_id: string; status: Supervisor["status"]; nickname: string }) => {
    setSupervisorModalUpdating(true)
    setSupervisorsError("")
    setSupervisorModalError("")
    try {
      const res = await authFetch("/admin/supervisors", {
        method: "PUT",
        body: JSON.stringify(payload),
      })
      if (!res.ok) throw new Error()
      const updated = await res.json()
      setSupervisors((prev) => prev.map((supervisor) => (supervisor.user_id === updated.user_id ? updated : supervisor)))
      setSelectedSupervisor((prev) => (prev?.user_id === updated.user_id ? updated : prev))
      return true
    } catch {
      setSupervisorsError("Unable to update supervisor right now. Please try again.")
      setSupervisorModalError("Unable to update supervisor right now. Please try again.")
      return false
    } finally {
      setSupervisorModalUpdating(false)
    }
  }

  const saveSupervisorModal = async () => {
    if (!selectedSupervisor) return
    const ok = await updateSupervisor({
      user_id: selectedSupervisor.user_id,
      status: supervisorStatusDraft,
      nickname: supervisorNickname,
    })
    if (ok) setSupervisorModalOpen(false)
  }

  const getIssuers = async () => {
    try {
      const res = await authFetch("/admin/issuers")
      if (!res.ok) throw new Error()
      const data = await res.json()
      setIssuers(data || [])
      setIssuersError("")
    } catch {
      setIssuersError("Error fetching issuers. Please try again later.")
    }
  }

  const getIssuerRequests = async (search = issuerRequestSearch, page = issuerRequestPage) => {
    try {
      const params = new URLSearchParams({ page: String(page), count: "20" })
      if (search) params.set("search", search)
      const res = await authFetch(`/admin/issuer-requests?${params}`)
      if (!res.ok) throw new Error()
      const data = await res.json()
      setIssuerRequests(data || [])
      setIssuerRequestsError("")
    } catch {
      setIssuerRequestsError("Error fetching issuer requests. Please try again later.")
    }
  }

  const updateIssuerRequest = async (user_id: string, payload: { status?: string; nickname?: string }) => {
    setIssuerRequestSaving((prev) => ({ ...prev, [user_id]: true }))
    setIssuerRequestsError("")
    try {
      const res = await authFetch("/admin/issuer-requests", {
        method: "PUT",
        body: JSON.stringify({ user_id, ...payload }),
      })
      if (!res.ok) throw new Error()
      const updated = await res.json() as IssuerRecord
      setIssuerRequests((prev) => prev.map((r) => (r.user_id === updated.user_id ? updated : r)))
      setSelectedIssuerRequest((prev) => (prev?.user_id === updated.user_id ? updated : prev))
      await getIssuers()
      return true
    } catch {
      setIssuerRequestsError("Unable to update issuer request right now. Please try again.")
      return false
    } finally {
      setIssuerRequestSaving((prev) => ({ ...prev, [user_id]: false }))
    }
  }

  const openIssuerRequestModal = (req: IssuerRecord) => {
    setSelectedIssuerRequest(req)
    setIssuerRequestNickname(req.nickname || "")
    setIssuerRequestStatusDraft(req.status || "pending")
    const issuer = issuers.find((candidate) => candidate.user_id === req.user_id)
    setIssuerScopes(issuer?.allowed_credentials || [])
    setIssuerScopePicker("")
    setIssuerRequestModalError("")
    setIssuerRequestModalOpen(true)
  }

  const getCredentialTypes = async () => {
    try {
      const res = await authFetch("/admin/credential-types")
      if (!res.ok) throw new Error()
      const data = await res.json()
      setCredentialTypes(data || [])
      setCredentialTypesError("")
    } catch {
      setCredentialTypesError("Error fetching credential types. Please try again later.")
    }
  }

  const fileToBase64 = (file: File) =>
    new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result
        if (typeof result !== "string") {
          reject(new Error("Unable to process selected image."))
          return
        }
        const commaIndex = result.indexOf(",")
        resolve(commaIndex >= 0 ? result.slice(commaIndex + 1) : result)
      }
      reader.onerror = () => reject(new Error("Unable to read selected image."))
      reader.readAsDataURL(file)
    })

  const createCredentialType = async (e: React.FormEvent) => {
    e.preventDefault()
    const value = newCredentialValue.trim()
    const label = newCredentialLabel.trim()
    const visibility = normalizeCredentialVisibility(newCredentialVisibility)
    if (!value || !label) return
    setCredentialTypeSaving(true)
    setCredentialTypesError("")
    try {
      const res = await authFetch("/admin/credential-types", {
        method: "POST",
        body: JSON.stringify({ value, label, visibility }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to create credential type.")
      }
      const created = await res.json() as GlobalCredentialType
      setCredentialTypes((prev) => [...prev, created])
      setNewCredentialValue("")
      setNewCredentialLabel("")
      setNewCredentialVisibility("public")
      setCredentialTypeSearch("")
      setCredentialTypePage(0)
    } catch (err) {
      setCredentialTypesError(err instanceof Error ? err.message : "Unable to create credential type.")
    } finally {
      setCredentialTypeSaving(false)
    }
  }

  const openCredentialTypeModal = (credentialType: GlobalCredentialType) => {
    setSelectedCredentialType(credentialType)
    setCredentialTypeDraftLabel(credentialType.label)
    setCredentialTypeDraftVisibility(normalizeCredentialVisibility(credentialType.visibility))
    setCredentialTypeDraftBadgeDataBase64("")
    setCredentialTypeDraftBadgeContentType("")
    setCredentialTypeDraftClearBadge(false)
    setCredentialTypeModalError("")
    setCredentialTypeModalOpen(true)
  }

  const closeCredentialTypeModal = () => {
    setCredentialTypeModalOpen(false)
    setCredentialTypeModalSaving(false)
    setCredentialTypeModalError("")
    setCredentialTypeDraftVisibility("public")
    setCredentialTypeDraftBadgeDataBase64("")
    setCredentialTypeDraftBadgeContentType("")
    setCredentialTypeDraftClearBadge(false)
  }

  const uploadCredentialBadge = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) return

    if (!file.type.startsWith("image/")) {
      setCredentialTypeModalError("Please upload an image file.")
      return
    }

    if (file.size > maxCredentialBadgeUploadBytes) {
      setCredentialTypeModalError(`Badge image must be ${maxCredentialBadgeUploadLabel} or smaller.`)
      return
    }

    try {
      const base64 = await fileToBase64(file)
      setCredentialTypeDraftBadgeDataBase64(base64)
      setCredentialTypeDraftBadgeContentType(file.type || "image/jpeg")
      setCredentialTypeDraftClearBadge(false)
      setCredentialTypeModalError("")
    } catch (err) {
      setCredentialTypeModalError(err instanceof Error ? err.message : "Unable to read selected image.")
    }
  }

  const clearCredentialBadge = () => {
    setCredentialTypeDraftBadgeDataBase64("")
    setCredentialTypeDraftBadgeContentType("")
    setCredentialTypeDraftClearBadge(true)
    setCredentialTypeModalError("")
  }

  const copyCredentialSlug = async () => {
    if (!selectedCredentialType) return
    try {
      await navigator.clipboard.writeText(selectedCredentialType.value)
      toast({
        title: "Copied",
        description: "Credential slug copied to clipboard.",
      })
    } catch {
      setCredentialTypeModalError("Unable to copy slug automatically.")
    }
  }

  const saveCredentialTypeModal = async () => {
    if (!selectedCredentialType) return

    const label = credentialTypeDraftLabel.trim()
    if (!label) {
      setCredentialTypeModalError("Credential title is required.")
      return
    }

    setCredentialTypeModalSaving(true)
    setCredentialTypeModalError("")
    setCredentialTypesError("")
    try {
      const payload: {
        label: string
        visibility: CredentialVisibility
        badge_content_type?: string
        badge_data_base64?: string
        clear_badge?: boolean
      } = {
        label,
        visibility: normalizeCredentialVisibility(credentialTypeDraftVisibility),
      }

      if (credentialTypeDraftClearBadge) payload.clear_badge = true
      if (credentialTypeDraftBadgeDataBase64 && credentialTypeDraftBadgeContentType) {
        payload.badge_data_base64 = credentialTypeDraftBadgeDataBase64
        payload.badge_content_type = credentialTypeDraftBadgeContentType
      }

      const res = await authFetch(`/admin/credential-types/${encodeURIComponent(selectedCredentialType.value)}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to update credential type.")
      }

      const updated = await res.json() as GlobalCredentialType
      setCredentialTypes((prev) => prev.map((credentialType) => (
        credentialType.value === updated.value ? updated : credentialType
      )))
      setSelectedCredentialType(updated)
      setCredentialTypeDraftLabel(updated.label)
      setCredentialTypeDraftVisibility(normalizeCredentialVisibility(updated.visibility))
      setCredentialTypeDraftBadgeDataBase64("")
      setCredentialTypeDraftBadgeContentType("")
      setCredentialTypeDraftClearBadge(false)
      toast({
        title: "Credential updated",
        description: "Credential type changes were saved.",
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unable to update credential type."
      setCredentialTypeModalError(message)
      setCredentialTypesError(message)
    } finally {
      setCredentialTypeModalSaving(false)
    }
  }

  const deleteCredentialType = async (credentialType: GlobalCredentialType) => {
    if (!window.confirm(`Delete credential type "${credentialType.label}"? This does not revoke existing grants.`)) return
    setCredentialTypesError("")
    setCredentialTypeModalError("")
    try {
      const res = await authFetch(`/admin/credential-types/${encodeURIComponent(credentialType.value)}`, { method: "DELETE" })
      if (!res.ok) throw new Error()
      setCredentialTypes((prev) => prev.filter((ct) => ct.value !== credentialType.value))
      if (selectedCredentialType?.value === credentialType.value) {
        setSelectedCredentialType(null)
        closeCredentialTypeModal()
      }
      toast({
        title: "Credential deleted",
        description: `${credentialType.label} was deleted.`,
      })
    } catch {
      const message = "Unable to delete credential type right now. Please try again."
      setCredentialTypesError(message)
      setCredentialTypeModalError(message)
    }
  }

  const addIssuerScope = (credential: CredentialType) => {
    setIssuerScopes((prev) => (prev.includes(credential) ? prev : [...prev, credential]))
    setIssuerScopePicker("")
  }

  const removeIssuerScope = (credential: CredentialType) => {
    setIssuerScopes((prev) => prev.filter((value) => value !== credential))
  }

  const saveIssuerScopes = async (user_id: string, allowedCredentials: CredentialType[], makeIssuer: boolean) => {
    const normalizedUserId = user_id.trim()
    if (!normalizedUserId) {
      setIssuersError("User ID is required.")
      return false
    }

    setIssuersError("")
    setIssuerRequestModalError("")

    setIssuerSaving(true)
    try {
      const res = await authFetch("/admin/issuers", {
        method: "PUT",
        body: JSON.stringify({
          user_id: normalizedUserId,
          allowed_credentials: allowedCredentials,
          make_issuer: makeIssuer,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to update issuer scopes.")
      }

      const updated = (await res.json()) as IssuerWithScopes
      setIssuers((prev) => {
        const existingIndex = prev.findIndex((issuer) => issuer.user_id === updated.user_id)
        if (existingIndex === -1) return [updated, ...prev]
        const next = [...prev]
        next[existingIndex] = updated
        return next
      })
      setIssuersError("")
      return true
    } catch (error) {
      if (error instanceof Error && error.message) {
        setIssuersError(error.message)
      } else {
        setIssuersError("Unable to update issuer scopes right now. Please try again.")
      }
      return false
    } finally {
      setIssuerSaving(false)
    }
  }

  const saveIssuerRequestModal = async () => {
    if (!selectedIssuerRequest) return
    setIssuerRequestModalError("")

    const userId = selectedIssuerRequest.user_id
    const nextStatus = issuerRequestStatusDraft

    const requestSaved = await updateIssuerRequest(userId, {
      nickname: issuerRequestNickname,
      status: nextStatus,
    })
    if (!requestSaved) {
      setIssuerRequestModalError("Unable to save issuer request changes.")
      return
    }

    const scopesSaved = await saveIssuerScopes(userId, issuerScopes, nextStatus === "approved")
    if (!scopesSaved) {
      setIssuerRequestModalError("Issuer details were saved, but credential scopes could not be updated.")
      return
    }

    setIssuerRequestModalOpen(false)
  }

  const getAdminWorkflows = async (
    search = adminWorkflowsSearch,
    page = adminWorkflowsPage,
    includeArchived = adminWorkflowsIncludeArchived,
  ) => {
    try {
      const params = new URLSearchParams({
        search,
        page: String(page),
        count: String(adminWorkflowsCount),
      })
      if (includeArchived) {
        params.set("include_archived", "true")
      }
      const res = await authFetch(`/admin/workflows?${params}`)
      if (!res.ok) throw new Error()
      const data = (await res.json()) as AdminWorkflowListResponse
      setAdminWorkflows(data.items || [])
      setAdminWorkflowsTotal(data.total || 0)
      setAdminWorkflowsError("")
    } catch {
      setAdminWorkflowsError("Unable to load workflows right now. Please try again.")
    }
  }

  const openAdminWorkflowDetails = async (
    workflowId: string,
    workflow?: Workflow,
    seriesContext?: { seriesId: string; workflowIds: string[]; index: number } | null,
  ) => {
    setAdminWorkflowsError("")
    setAdminDetailSeriesContext(seriesContext || null)

    if (workflow) {
      setAdminWorkflowDetail(workflow)
      setAdminWorkflowDetailLoading(false)
      setAdminWorkflowDetailOpen(true)
      return
    }

    setAdminWorkflowDetail(null)
    setAdminWorkflowDetailLoading(true)
    setAdminWorkflowDetailOpen(true)
    try {
      const res = await authFetch(`/workflows/${workflowId}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load workflow details.")
      }
      const data = (await res.json()) as Workflow
      setAdminWorkflowDetail(data)
    } catch (err) {
      setAdminWorkflowDetailOpen(false)
      setAdminWorkflowsError(err instanceof Error ? err.message : "Unable to load workflow details.")
    } finally {
      setAdminWorkflowDetailLoading(false)
    }
  }

  const loadAdminSeriesClaimants = async (seriesId: string) => {
    setAdminRevokeLoading(true)
    setAdminRevokeError("")
    try {
      const res = await authFetch(`/admin/workflow-series/${encodeURIComponent(seriesId)}/claimants`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load claimants.")
      }
      const data = (await res.json()) as WorkflowSeriesClaimant[]
      setAdminRevokeClaimants(data || [])
      setAdminRevokeImproverId((current) => current || data?.[0]?.user_id || "")
    } catch (err) {
      setAdminRevokeClaimants([])
      setAdminRevokeImproverId("")
      setAdminRevokeError(err instanceof Error ? err.message : "Unable to load claimants.")
    } finally {
      setAdminRevokeLoading(false)
    }
  }

  const openAdminRevokeModal = async (seriesId: string) => {
    setAdminRevokeSeriesId(seriesId)
    setAdminRevokeClaimants([])
    setAdminRevokeImproverId("")
    setAdminRevokeError("")
    setAdminRevokeModalOpen(true)
    await loadAdminSeriesClaimants(seriesId)
  }

  const revokeAdminSeriesClaim = async () => {
    if (!adminRevokeSeriesId || !adminRevokeImproverId) {
      setAdminRevokeError("Select an improver to revoke.")
      return
    }
    setAdminRevokeSubmitting(true)
    setAdminRevokeError("")
    try {
      const res = await authFetch(`/admin/workflow-series/${encodeURIComponent(adminRevokeSeriesId)}/revoke-claim`, {
        method: "POST",
        body: JSON.stringify({
          improver_user_id: adminRevokeImproverId,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to revoke claim.")
      }
      const result = (await res.json()) as WorkflowSeriesClaimRevokeResult
      const skipped =
        result.skipped_count > 0 ? ` ${result.skipped_count} started assignment(s) were not revoked.` : ""
      toast({
        title: "Claims revoked",
        description: `Released ${result.released_count} assignment(s).${skipped}`,
      })
      setAdminRevokeModalOpen(false)
      await getAdminWorkflows(adminWorkflowsSearch, adminWorkflowsPage, adminWorkflowsIncludeArchived)
      if (adminWorkflowDetail?.id) {
        const resWorkflow = await authFetch(`/workflows/${adminWorkflowDetail.id}`)
        if (resWorkflow.ok) {
          const refreshed = (await resWorkflow.json()) as Workflow
          setAdminWorkflowDetail(refreshed)
        }
      }
    } catch (err) {
      setAdminRevokeError(err instanceof Error ? err.message : "Unable to revoke claim.")
    } finally {
      setAdminRevokeSubmitting(false)
    }
  }

  useEffect(() => {
    setCredentialTypePage((prev) => {
      const maxPage = Math.max(0, credentialTypesTotalPages - 1)
      return prev > maxPage ? maxPage : prev
    })
  }, [credentialTypesTotalPages])

  useEffect(() => {
    if (!selectedCredentialType) return
    const current = credentialTypes.find((credentialType) => credentialType.value === selectedCredentialType.value)
    if (!current) {
      setSelectedCredentialType(null)
      setCredentialTypeModalOpen(false)
      setCredentialTypeModalSaving(false)
      setCredentialTypeModalError("")
      setCredentialTypeDraftVisibility("public")
      setCredentialTypeDraftBadgeDataBase64("")
      setCredentialTypeDraftBadgeContentType("")
      setCredentialTypeDraftClearBadge(false)
      return
    }
    if (
      current.label !== selectedCredentialType.label
      || current.visibility !== selectedCredentialType.visibility
      || current.badge_content_type !== selectedCredentialType.badge_content_type
      || current.badge_data_base64 !== selectedCredentialType.badge_data_base64
    ) {
      setSelectedCredentialType(current)
    }
    if (!credentialTypeModalSaving) {
      setCredentialTypeDraftLabel(current.label)
      setCredentialTypeDraftVisibility(normalizeCredentialVisibility(current.visibility))
    }
  }, [credentialTypeModalSaving, credentialTypes, selectedCredentialType])

  useEffect(() => {
    const nextTab = searchParams.get("tab")
    if (isValidAdminTab(nextTab) && nextTab !== activeTab) {
      setActiveTab(nextTab)
    }

    const nextMerchantSearch = readQueryText("merchant_search", "")
    if (nextMerchantSearch !== merchantSearch) setMerchantSearch(nextMerchantSearch)
    const nextMerchantStatus = readQueryText("merchant_status", "all")
    if (nextMerchantStatus !== merchantStatusFilter) setMerchantStatusFilter(nextMerchantStatus)

    const nextEventsSearch = readQueryText("events_search", "")
    if (nextEventsSearch !== eventsSearch) setEventsSearch(nextEventsSearch)
    const nextEventsPage = readQueryNumber("events_page", 0)
    if (nextEventsPage !== eventsPage) setEventsPage(nextEventsPage)
    const nextEventsCount = readQueryNumber("events_count", 10)
    if (nextEventsCount !== eventsCount) setEventsCount(nextEventsCount)
    const nextEventsExpired = readQueryBoolean("events_expired", false)
    if (nextEventsExpired !== eventsExpired) setEventsExpired(nextEventsExpired)
    const nextEventsOwner = readQueryText("events_owner", "all")
    if (nextEventsOwner !== eventsOwnerFilter) setEventsOwnerFilter(nextEventsOwner)

    const nextAffiliateSearch = readQueryText("affiliate_search", "")
    if (nextAffiliateSearch !== affiliateSearch) setAffiliateSearch(nextAffiliateSearch)
    const nextAffiliatePage = readQueryNumber("affiliate_page", 0)
    if (nextAffiliatePage !== affiliatePage) setAffiliatePage(nextAffiliatePage)
    const nextAffiliateStatus = readQueryText("affiliate_status", "all")
    if (nextAffiliateStatus !== affiliateStatusFilter) setAffiliateStatusFilter(nextAffiliateStatus)

    const nextProposerSearch = readQueryText("proposer_search", "")
    if (nextProposerSearch !== proposerSearch) setProposerSearch(nextProposerSearch)
    const nextProposerPage = readQueryNumber("proposer_page", 0)
    if (nextProposerPage !== proposerPage) setProposerPage(nextProposerPage)
    const nextProposerStatus = readQueryText("proposer_status", "all")
    if (nextProposerStatus !== proposerStatusFilter) setProposerStatusFilter(nextProposerStatus)

    const nextImproverSearch = readQueryText("improver_search", "")
    if (nextImproverSearch !== improverSearch) setImproverSearch(nextImproverSearch)
    const nextImproverPage = readQueryNumber("improver_page", 0)
    if (nextImproverPage !== improverPage) setImproverPage(nextImproverPage)
    const nextImproverStatus = readQueryText("improver_status", "all")
    if (nextImproverStatus !== improverStatusFilter) setImproverStatusFilter(nextImproverStatus)

    const nextSupervisorSearch = readQueryText("supervisor_search", "")
    if (nextSupervisorSearch !== supervisorSearch) setSupervisorSearch(nextSupervisorSearch)
    const nextSupervisorPage = readQueryNumber("supervisor_page", 0)
    if (nextSupervisorPage !== supervisorPage) setSupervisorPage(nextSupervisorPage)
    const nextSupervisorStatus = readQueryText("supervisor_status", "all")
    if (nextSupervisorStatus !== supervisorStatusFilter) setSupervisorStatusFilter(nextSupervisorStatus)

    const nextIssuerSearch = readQueryText("issuer_search", "")
    if (nextIssuerSearch !== issuerRequestSearch) setIssuerRequestSearch(nextIssuerSearch)
    const nextIssuerPage = readQueryNumber("issuer_page", 0)
    if (nextIssuerPage !== issuerRequestPage) setIssuerRequestPage(nextIssuerPage)
    const nextIssuerStatus = readQueryText("issuer_status", "all")
    if (nextIssuerStatus !== issuerStatusFilter) setIssuerStatusFilter(nextIssuerStatus)

    const nextWorkflowSearch = readQueryText("workflow_search", "")
    if (nextWorkflowSearch !== adminWorkflowsSearch) setAdminWorkflowsSearch(nextWorkflowSearch)
    const nextWorkflowPage = readQueryNumber("workflow_page", 0)
    if (nextWorkflowPage !== adminWorkflowsPage) setAdminWorkflowsPage(nextWorkflowPage)
    const nextWorkflowIncludeArchived = readQueryBoolean("workflow_include_archived", false)
    if (nextWorkflowIncludeArchived !== adminWorkflowsIncludeArchived) setAdminWorkflowsIncludeArchived(nextWorkflowIncludeArchived)
  }, [searchParams])

  useEffect(() => {
    const params = new URLSearchParams(searchParams.toString())

    params.set("tab", activeTab)

    if (merchantSearch) params.set("merchant_search", merchantSearch)
    else params.delete("merchant_search")
    if (merchantStatusFilter !== "all") params.set("merchant_status", merchantStatusFilter)
    else params.delete("merchant_status")

    if (eventsSearch) params.set("events_search", eventsSearch)
    else params.delete("events_search")
    if (eventsPage > 0) params.set("events_page", String(eventsPage))
    else params.delete("events_page")
    if (eventsCount !== 10) params.set("events_count", String(eventsCount))
    else params.delete("events_count")
    if (eventsExpired) params.set("events_expired", "true")
    else params.delete("events_expired")
    if (eventsOwnerFilter !== "all") params.set("events_owner", eventsOwnerFilter)
    else params.delete("events_owner")

    if (affiliateSearch) params.set("affiliate_search", affiliateSearch)
    else params.delete("affiliate_search")
    if (affiliatePage > 0) params.set("affiliate_page", String(affiliatePage))
    else params.delete("affiliate_page")
    if (affiliateStatusFilter !== "all") params.set("affiliate_status", affiliateStatusFilter)
    else params.delete("affiliate_status")

    if (proposerSearch) params.set("proposer_search", proposerSearch)
    else params.delete("proposer_search")
    if (proposerPage > 0) params.set("proposer_page", String(proposerPage))
    else params.delete("proposer_page")
    if (proposerStatusFilter !== "all") params.set("proposer_status", proposerStatusFilter)
    else params.delete("proposer_status")

    if (improverSearch) params.set("improver_search", improverSearch)
    else params.delete("improver_search")
    if (improverPage > 0) params.set("improver_page", String(improverPage))
    else params.delete("improver_page")
    if (improverStatusFilter !== "all") params.set("improver_status", improverStatusFilter)
    else params.delete("improver_status")

    if (supervisorSearch) params.set("supervisor_search", supervisorSearch)
    else params.delete("supervisor_search")
    if (supervisorPage > 0) params.set("supervisor_page", String(supervisorPage))
    else params.delete("supervisor_page")
    if (supervisorStatusFilter !== "all") params.set("supervisor_status", supervisorStatusFilter)
    else params.delete("supervisor_status")

    if (issuerRequestSearch) params.set("issuer_search", issuerRequestSearch)
    else params.delete("issuer_search")
    if (issuerRequestPage > 0) params.set("issuer_page", String(issuerRequestPage))
    else params.delete("issuer_page")
    if (issuerStatusFilter !== "all") params.set("issuer_status", issuerStatusFilter)
    else params.delete("issuer_status")

    if (adminWorkflowsSearch) params.set("workflow_search", adminWorkflowsSearch)
    else params.delete("workflow_search")
    if (adminWorkflowsPage > 0) params.set("workflow_page", String(adminWorkflowsPage))
    else params.delete("workflow_page")
    if (adminWorkflowsIncludeArchived) params.set("workflow_include_archived", "true")
    else params.delete("workflow_include_archived")

    const nextQuery = params.toString()
    if (nextQuery !== searchParams.toString()) {
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false })
    }
  }, [
    activeTab,
    merchantSearch,
    merchantStatusFilter,
    eventsSearch,
    eventsPage,
    eventsCount,
    eventsExpired,
    eventsOwnerFilter,
    affiliateSearch,
    affiliatePage,
    affiliateStatusFilter,
    proposerSearch,
    proposerPage,
    proposerStatusFilter,
    improverSearch,
    improverPage,
    improverStatusFilter,
    supervisorSearch,
    supervisorPage,
    supervisorStatusFilter,
    issuerRequestSearch,
    issuerRequestPage,
    issuerStatusFilter,
    adminWorkflowsSearch,
    adminWorkflowsPage,
    adminWorkflowsIncludeArchived,
    pathname,
    router,
    searchParams,
  ])

  useEffect(() => {
    if(wallets.length) {
      getFaucetBalance()
    }
  }, [wallets])

  useEffect(() => {
    getAuthedMapLocations()
    getEvents()
    getIssuers()
    getCredentialTypes()
  }, [])

  useEffect(() => { getAffiliates(affiliateSearch, affiliatePage) }, [affiliateSearch, affiliatePage])
  useEffect(() => { getProposers(proposerSearch, proposerPage) }, [proposerSearch, proposerPage])
  useEffect(() => { getImprovers(improverSearch, improverPage) }, [improverSearch, improverPage])
  useEffect(() => { getSupervisors(supervisorSearch, supervisorPage) }, [supervisorSearch, supervisorPage])
  useEffect(() => { getIssuerRequests(issuerRequestSearch, issuerRequestPage) }, [issuerRequestSearch, issuerRequestPage])
  useEffect(() => { getAdminWorkflows(adminWorkflowsSearch, adminWorkflowsPage, adminWorkflowsIncludeArchived) }, [adminWorkflowsSearch, adminWorkflowsPage, adminWorkflowsIncludeArchived])

  useEffect(() => {
    if (status !== "authenticated") return

    switch (activeTab) {
      case "merchants":
        void getAuthedMapLocations()
        break
      case "events":
        void getEvents()
        void getUnallocatedBalance()
        break
      case "w9":
        void fetchPendingW9Submissions()
        break
      case "affiliates":
        void getAffiliates(affiliateSearch, affiliatePage)
        break
      case "proposers":
        void getProposers(proposerSearch, proposerPage)
        break
      case "improvers":
        void getImprovers(improverSearch, improverPage)
        break
      case "supervisors":
        void getSupervisors(supervisorSearch, supervisorPage)
        break
      case "workflows":
        void getAdminWorkflows(adminWorkflowsSearch, adminWorkflowsPage, adminWorkflowsIncludeArchived)
        break
      case "issuers":
        void getIssuerRequests(issuerRequestSearch, issuerRequestPage)
        void getIssuers()
        break
      case "credential-types":
        void getCredentialTypes()
        break
      default:
        break
    }
  }, [
    activeTab,
    status,
    affiliateSearch,
    affiliatePage,
    proposerSearch,
    proposerPage,
    improverSearch,
    improverPage,
    supervisorSearch,
    supervisorPage,
    adminWorkflowsSearch,
    adminWorkflowsPage,
    adminWorkflowsIncludeArchived,
    issuerRequestSearch,
    issuerRequestPage,
  ])

  const fetchPendingW9Submissions = async () => {
    if (!user?.isAdmin) return
    setW9Loading(true)
    try {
      const res = await authFetch("/admin/w9/pending")
      if (res.status !== 200) {
        throw new Error("failed to fetch w9 submissions")
      }
      const data = await res.json()
      setPendingW9Submissions(data.submissions || [])
    } catch {
      toast({
        title: "Error",
        description: "Failed to load W9 submissions.",
        variant: "destructive",
      })
    } finally {
      setW9Loading(false)
    }
  }

  useEffect(() => {
    if (status === "authenticated" && user?.isAdmin) {
      fetchPendingW9Submissions()
    }
  }, [status, user?.isAdmin])

  const handleApproveW9 = async (id: number) => {
    try {
      const res = await authFetch("/admin/w9/approve", {
        method: "PUT",
        body: JSON.stringify({ id }),
      })
      if (res.status !== 200) {
        throw new Error("failed to approve w9")
      }
      setPendingW9Submissions((prev) => prev.filter((submission) => submission.id !== id))
      toast({
        title: "W9 Approved",
        description: "The W9 submission has been approved.",
      })
    } catch {
      toast({
        title: "Approval Failed",
        description: "Failed to approve W9 submission. Please try again.",
        variant: "destructive",
      })
    }
  }

  const handleRejectW9 = async (id: number) => {
    const confirmed = window.confirm("Reject this W9 submission? The user will need to resubmit.")
    if (!confirmed) return
    try {
      const res = await authFetch("/admin/w9/reject", {
        method: "PUT",
        body: JSON.stringify({ id }),
      })
      if (res.status !== 200) {
        throw new Error("failed to reject w9")
      }
      setPendingW9Submissions((prev) => prev.filter((submission) => submission.id !== id))
      toast({
        title: "W9 Rejected",
        description: "The W9 submission has been rejected.",
      })
    } catch {
      toast({
        title: "Rejection Failed",
        description: "Failed to reject W9 submission. Please try again.",
        variant: "destructive",
      })
    }
  }

  useEffect(() => {
      setPendingLocations(authedMapLocations.filter((location) => location.approval === null))
  }, [authedMapLocations])

  useEffect(() => {
    setSelectedWalletBalances()
  }, [selectedWallet])




  async function setSelectedWalletBalances() {
    if (selectedWallet !== null) {
    const SFLuvPromise = selectedWallet.getSFLUVBalanceFormatted()
    const BYUSDPromise = selectedWallet.getBYUSDBalanceFormatted()

    const SFLuvBalance = await SFLuvPromise
    if (SFLuvBalance != null) {
      setSelectedWalletSFLUVBalance(SFLuvBalance)
      }

    const BYUSDBalance = await BYUSDPromise
    if (BYUSDBalance != null) {
      setSelectedWalletBYUSDBalance(BYUSDBalance)
      }
    }
  }


  // Get selected PayPal account data
  const getSelectedPaypalAccount = () => {
    if (!selectedPaypalAccount) return null
    return paypalAccounts.find((account) => account.id === selectedPaypalAccount)
  }

  const selectedPaypalData = getSelectedPaypalAccount()

  // Format number to 2 decimal places and prevent negative values
  const formatAmount = (value: string): string => {
    // Remove any non-numeric characters except decimal point
    let cleaned = value.replace(/[^0-9.]/g, "")

    // Prevent multiple decimal points
    const parts = cleaned.split(".")
    if (parts.length > 2) {
      cleaned = parts[0] + "." + parts.slice(1).join("")
    }

    // Limit to 2 decimal places
    if (parts[1] && parts[1].length > 2) {
      cleaned = parts[0] + "." + parts[1].substring(0, 2)
    }

    // Prevent negative values
    if (cleaned.startsWith("-")) {
      cleaned = cleaned.substring(1)
    }

    return cleaned
  }

  const handleAmountChange = (value: string) => {
    const formatted = formatAmount(value)
    setAmount(formatted)
  }

  const handlePaypalAmountChange = (value: string) => {
    const formatted = formatAmount(value)
    setPaypalAmount(formatted)
  }

  const handleTokenConversion = async () => {
    const convertAmount = Number.parseFloat(amount)

    if (!convertAmount || convertAmount <= 0) {
      toast({
        title: "Invalid Amount",
        description: "Please enter a valid amount to convert.",
        variant: "destructive",
      })
      return
    }

    if (!selectedWallet) {
      toast({
        title: "No Wallet Selected",
        description: "Please select a wallet to perform the conversion.",
        variant: "destructive",
      })
      return
    }

    const walletData = selectedWallet
    if (!walletData) {
      toast({
        title: "Wallet Not Found",
        description: "Selected wallet could not be found.",
        variant: "destructive",
      })
      return
    }

    if (conversionType === "wrap" && convertAmount > selectedWalletBYUSDBalance) {
      toast({
        title: "Insufficient Balance",
        description: "You don't have enough BYUSD in this wallet to wrap this amount.",
        variant: "destructive",
      })
      return
    }

    if (conversionType === "unwrap" && convertAmount > selectedWalletSFLUVBalance) {
      toast({
        title: "Insufficient Balance",
        description: "You don't have enough SFLUV in this wallet to unwrap this amount.",
        variant: "destructive",
      })
      return
    }

    setIsProcessing(true)
    try {
      //Token conversion call goes here
      console.log("Token Conversion")
    } catch (error) {
      toast({
        title: "Conversion Failed",
        description: "Failed to convert tokens. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsProcessing(false)
    }
  }

  const handlePaypalConversion = async () => {
    const convertAmount = Number.parseFloat(paypalAmount)
    if (!convertAmount || convertAmount <= 0) {
      toast({
        title: "Invalid Amount",
        description: "Please enter a valid amount to convert.",
        variant: "destructive",
      })
      return
    }

    if (!selectedWallet) {
      toast({
        title: "No Wallet Selected",
        description: "Please select a wallet to cash out from.",
        variant: "destructive",
      })
      return
    }

    if (!selectedPaypalAccount) {
      toast({
        title: "No PayPal Account Selected",
        description: "Please select a PayPal account to cash out to.",
        variant: "destructive",
      })
      return
    }

    const walletData = selectedWallet
    if (!walletData) {
      toast({
        title: "Wallet Not Found",
        description: "Selected wallet could not be found.",
        variant: "destructive",
      })
      return
    }

    if (convertAmount > selectedWalletBYUSDBalance) {
      toast({
        title: "Insufficient Balance",
        description: "You don't have enough BYUSD in this wallet to convert to cash.",
        variant: "destructive",
      })
      return
    }

    const selectedAccount = paypalAccounts.find((account) => account.id === selectedPaypalAccount)

    setIsConvertingToPaypal(true)
    try {
      // PayPal offload called here
      console.log("PayPal offload")
    } catch (error) {
      toast({
        title: "PayPal Conversion Failed",
        description: "Failed to convert to PayPal cash. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsConvertingToPaypal(false)
    }
  }

  const handleConnectPaypalAccount = async () => {
    if (!newPaypalEmail || !newPaypalPassword || !newPaypalFirstName || !newPaypalLastName || !newPaypalAccountName) {
      toast({
        title: "Missing Information",
        description: "Please fill in all required PayPal account details.",
        variant: "destructive",
      })
      return
    }

    // Basic email validation
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
    if (!emailRegex.test(newPaypalEmail)) {
      toast({
        title: "Invalid Email",
        description: "Please enter a valid email address.",
        variant: "destructive",
      })
      return
    }

    // Basic phone validation (if provided)
    if (newPaypalPhone && !/^\+?[\d\s\-$$$$]+$/.test(newPaypalPhone)) {
      toast({
        title: "Invalid Phone Number",
        description: "Please enter a valid phone number.",
        variant: "destructive",
      })
      return
    }

    // Check if account already exists
    const existingAccount = paypalAccounts.find((account) => account.email === newPaypalEmail)
    if (existingAccount) {
      toast({
        title: "Account Already Exists",
        description: "This PayPal account is already connected.",
        variant: "destructive",
      })
      return
    }

    setIsConnectingPaypal(true)
    try {
      // Simulate PayPal authentication and connection API call
      await new Promise((resolve) => setTimeout(resolve, 3000))

      const newAccount = {
        id: `paypal-${Date.now()}`,
        email: newPaypalEmail,
        name: newPaypalAccountName,
        isVerified: true, // In real implementation, this would be false initially
        isDefault: paypalAccounts.length === 0, // First account becomes default
      }

      setPaypalAccounts((prev) => [...prev, newAccount])
      setSelectedPaypalAccount(newAccount.id)

      toast({
        title: "PayPal Account Connected",
        description: `Successfully connected ${newPaypalEmail} to your account.`,
      })

      // Reset form and close modal
      setNewPaypalEmail("")
      setNewPaypalPassword("")
      setNewPaypalFirstName("")
      setNewPaypalLastName("")
      setNewPaypalPhone("")
      setNewPaypalAccountName("")
      setIsPaypalModalOpen(false)
    } catch (error) {
      toast({
        title: "Connection Failed",
        description: "Failed to connect PayPal account. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsConnectingPaypal(false)
    }
  }

  const openMerchantModal = (location: AuthedLocation) => {
    setselectedLocationForReview(location)
    setMerchantStatusDraft(approvalToStatus(location.approval))
    setMerchantModalError("")
    setisLocationReviewModalOpen(true)
  }

  const saveMerchantModal = async () => {
    if (!selectedLocationForReview) return

    setMerchantModalSaving(true)
    setMerchantModalError("")
    const update: UpdateLocationApprovalRequest = {
      id: selectedLocationForReview.id,
      approval: statusToApproval(merchantStatusDraft),
    }

    try {
      const res = await authFetch("/admin/locations", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(update),
      })
      if (res.status !== 201) {
        throw new Error("Unable to update merchant status")
      }

      await getAuthedMapLocations()
      toast({
        title: "Merchant Updated",
        description: `Status set to ${formatStatusLabel(merchantStatusDraft)}.`,
      })
      setisLocationReviewModalOpen(false)
    } catch {
      setMerchantModalError("Unable to update merchant right now. Please try again.")
    } finally {
      setMerchantModalSaving(false)
    }
  }

  const handleGenerateQRCodes = async () => {
    if (!eventStartDate || !eventEndDate || !eventStartTime || !eventEndTime || !sfluvPerCode || !numberOfCodes) {
      toast({
        title: "Missing Information",
        description: "Please fill in all required fields to generate QR codes.",
        variant: "destructive",
      })
      return
    }

    const sfluvAmount = Number.parseFloat(sfluvPerCode)
    const codeCount = Number.parseInt(numberOfCodes)

    if (sfluvAmount <= 0 || codeCount <= 0 || codeCount > 1000) {
      toast({
        title: "Invalid Values",
        description: "Please enter valid amounts. Maximum 1000 QR codes per event.",
        variant: "destructive",
      })
      return
    }

    if (eventStartDate >= eventEndDate) {
      toast({
        title: "Invalid Date Range",
        description: "Event end date must be after start date.",
        variant: "destructive",
      })
      return
    }

    setIsGeneratingCodes(true)
    try {
      // Simulate QR code generation
      await new Promise((resolve) => setTimeout(resolve, 2000))

      const codes = Array.from({ length: codeCount }, (_, index) => ({
        id: `qr-${Date.now()}-${index}`,
        eventId: `event-${Date.now()}`,
        codeNumber: index + 1,
        sfluvAmount,
        startDate: eventStartDate,
        endDate: eventEndDate,
        startTime: eventStartTime,
        endTime: eventEndTime,
        isRedeemed: false,
        qrData: `sfluv://redeem?event=${Date.now()}&code=${index + 1}&amount=${sfluvAmount}`,
      }))

      setGeneratedCodes(codes)

      toast({
        title: "QR Codes Generated",
        description: `Successfully generated ${codeCount} QR codes.`,
      })
    } catch (error) {
      toast({
        title: "Generation Failed",
        description: "Failed to generate QR codes. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsGeneratingCodes(false)
    }
  }

  const handleDownloadQRCodes = () => {
    if (generatedCodes.length === 0) {
      toast({
        title: "No QR Codes",
        description: "Please generate QR codes first before downloading.",
        variant: "destructive",
      })
      return
    }

    // Create CSV content
    const csvContent = [
      "Code Number,SFLUV Amount,QR Data,Start Date,End Date,Start Time,End Time",
      ...generatedCodes.map(
        (code) =>
          `${code.codeNumber},${code.sfluvAmount},"${code.qrData}","${code.startDate.toLocaleDateString()}","${code.endDate.toLocaleDateString()}","${code.startTime}","${code.endTime}"`,
      ),
    ].join("\n")

    // Create and download file
    const blob = new Blob([csvContent], { type: "text/csv" })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement("a")
    link.href = url
    link.download = `QR_Codes_${new Date().toISOString().split("T")[0]}.csv`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(url)

    toast({
      title: "Download Started",
      description: `QR codes have been downloaded as CSV.`,
    })
  }

  // Get available balance for conversion type
  const getAvailableBalance = () => {
    if (selectedWallet == null) return "No wallet selected"

    if (conversionType === "wrap") {
      return `$${selectedWalletBYUSDBalance.toLocaleString()} BYUSD`
    } else {
      return `${selectedWalletSFLUVBalance.toLocaleString()} SFLUV`
    }
  }

  // Get available balance for PayPal conversion
  const getPaypalAvailableBalance = () => {
    if (selectedWallet == null) return "No wallet selected"
    return `$${selectedWalletBYUSDBalance.toLocaleString()} BYUSD`
  }

  const getAdminSeriesCardIndex = (group: AdminWorkflowSeriesGroup) => {
    if (group.workflows.length === 0) return 0
    const defaultIndex = group.workflows.length - 1
    const currentIndex = adminSeriesCardIndexById[group.seriesId] ?? defaultIndex
    return Math.max(0, Math.min(group.workflows.length - 1, currentIndex))
  }

  const shiftAdminSeriesCardIndex = (group: AdminWorkflowSeriesGroup, direction: number) => {
    if (group.workflows.length <= 1) return
    setAdminSeriesCardIndexById((prev) => {
      const defaultIndex = group.workflows.length - 1
      const currentIndex = prev[group.seriesId] ?? defaultIndex
      const nextIndex = Math.max(0, Math.min(group.workflows.length - 1, currentIndex + direction))
      if (nextIndex === currentIndex) return prev
      return {
        ...prev,
        [group.seriesId]: nextIndex,
      }
    })
  }

  const openAdminSeriesWorkflowDetails = async (group: AdminWorkflowSeriesGroup, index: number) => {
    if (group.workflows.length === 0) return
    const safeIndex = Math.max(0, Math.min(group.workflows.length - 1, index))
    const selectedWorkflow = group.workflows[safeIndex]
    await openAdminWorkflowDetails(selectedWorkflow.id, undefined, {
      seriesId: group.seriesId,
      workflowIds: group.workflows.map((workflow) => workflow.id),
      index: safeIndex,
    })
  }

  const shiftAdminDetailSeriesWorkflow = async (direction: number) => {
    if (!adminDetailSeriesContext || adminDetailSeriesContext.workflowIds.length <= 1) return
    const nextIndex = adminDetailSeriesContext.index + direction
    if (nextIndex < 0 || nextIndex >= adminDetailSeriesContext.workflowIds.length) return
    const nextWorkflowId = adminDetailSeriesContext.workflowIds[nextIndex]
    await openAdminWorkflowDetails(nextWorkflowId, undefined, {
      ...adminDetailSeriesContext,
      index: nextIndex,
    })
  }

  if(status === "loading") {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="container mx-auto space-y-6 px-3 py-4 sm:p-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold sm:text-3xl">Admin Panel</h1>
          <p className="text-muted-foreground">Manage tokens and merchant approvals</p>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4 lg:space-y-0">
        <div className="grid gap-4 lg:grid-cols-[280px_minmax(0,1fr)] lg:items-start lg:gap-6">
          <TabsList className="h-fit w-full flex-col items-stretch gap-2 rounded-xl bg-secondary p-3 lg:sticky lg:top-4 lg:min-w-[280px]">
            <TabsTrigger value="events" className="w-full justify-between px-3 py-2">
              <span>Events</span>
            </TabsTrigger>
            <TabsTrigger value="w9" className="w-full justify-between px-3 py-2">
              <span>W9 Approvals</span>
              {pendingW9Submissions.length > 0 && (
                <Badge variant="destructive" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {pendingW9Submissions.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="merchants" className="w-full justify-between px-3 py-2">
              <span>Merchants</span>
              {pendingLocations.length > 0 && (
                <Badge variant="destructive" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {pendingLocations.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="affiliates" className="w-full justify-between px-3 py-2">
              <span>Affiliates</span>
              {affiliates.filter((affiliate) => affiliate.status === "pending").length > 0 && (
                <Badge variant="destructive" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {affiliates.filter((affiliate) => affiliate.status === "pending").length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="proposers" className="w-full justify-between px-3 py-2">
              <span>Proposers</span>
              {proposers.filter((proposer) => proposer.status === "pending").length > 0 && (
                <Badge variant="destructive" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {proposers.filter((proposer) => proposer.status === "pending").length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="improvers" className="w-full justify-between px-3 py-2">
              <span>Improvers</span>
              {improvers.filter((improver) => improver.status === "pending").length > 0 && (
                <Badge variant="destructive" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {improvers.filter((improver) => improver.status === "pending").length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="supervisors" className="w-full justify-between px-3 py-2">
              <span>Supervisors</span>
              {supervisors.filter((supervisor) => supervisor.status === "pending").length > 0 && (
                <Badge variant="destructive" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {supervisors.filter((supervisor) => supervisor.status === "pending").length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="workflows" className="w-full justify-between px-3 py-2">
              <span>Workflows</span>
            </TabsTrigger>
            <TabsTrigger value="issuers" className="w-full justify-between px-3 py-2">
              <span>Issuers</span>
              {issuerRequests.filter((r) => r.status === "pending").length > 0 && (
                <Badge variant="destructive" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {issuerRequests.filter((r) => r.status === "pending").length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="credential-types" className="w-full justify-between px-3 py-2">
              <span>Credential Types</span>
            </TabsTrigger>
          </TabsList>

          <div className="min-w-0">

        <TabsContent value="tokens" className="space-y-6">
          {/* Global Wallet Selection */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Wallet className="h-5 w-5" />
                Select Wallet
              </CardTitle>
              <CardDescription>Choose which wallet to use for all operations</CardDescription>
            </CardHeader>
            <CardContent>
              <Select value={selectedWallet ? String(selectedWallet?.id) : ""} onValueChange={(id) => {
                const wallet = wallets.find((w) => String(w.id) === id)
                if (wallet) {
                setSelectedWallet(wallet)
                }
              }}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Choose a wallet to manage" />
                </SelectTrigger>
                <SelectContent>
                  {wallets.map((wallet) => (
                    <SelectItem key={wallet.name} value={String(wallet.id)}>
                      <div className="flex items-center gap-3">
                        <Wallet className="h-4 w-4" />
                        <div className="flex flex-col">
                          <span className="font-medium">{wallet.name}</span>
                          <span className="text-xs text-muted-foreground">{wallet.address}</span>
                        </div>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </CardContent>
          </Card>

          {/* Balance Overview */}
          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">
                  {selectedWallet ? `${selectedWallet.name} BYUSD Balance` : "BYUSD Balance"}
                </CardTitle>
                <Coins className="h-4 w-4 text-blue-600" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  ${selectedWallet ? selectedWalletBYUSDBalance.toLocaleString() + " BYUSD" : "0"}
                </div>
                <p className="text-xs text-muted-foreground">
                  {selectedWallet ? "Available in selected wallet" : "Select a wallet to view balance"}
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">
                  {selectedWallet ? `${selectedWallet.name} SFLUV Balance` : "SFLUV Balance"}
                </CardTitle>
                <Coins className="h-4 w-4 text-green-600" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  {selectedWallet ? selectedWalletSFLUVBalance.toLocaleString() + " SFLUV": "0"}
                </div>
                <p className="text-xs text-muted-foreground">
                  {selectedWallet ? "Available in selected wallet" : "Select a wallet to view balance"}
                </p>
              </CardContent>
            </Card>
          </div>

          {/* Token Conversion Interface */}
          <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <ArrowUpDown className="h-5 w-5 text-green-600" />
                  Token Conversion
                </CardTitle>
                <CardDescription>Convert between BYUSD stablecoins and SFLUV tokens</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="conversion-type">Conversion Type</Label>
                  <Select value={conversionType} onValueChange={(value: "wrap" | "unwrap") => setConversionType(value)}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select conversion type" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="wrap">
                        <div className="flex items-center gap-2">
                          <ArrowUp className="h-4 w-4 text-green-600" />
                          Wrap
                        </div>
                      </SelectItem>
                      <SelectItem value="unwrap">
                        <div className="flex items-center gap-2">
                          <ArrowDown className="h-4 w-4 text-blue-600" />
                          Unwrap
                        </div>
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="conversion-amount">Amount to {conversionType === "wrap" ? "Wrap" : "Unwrap"}</Label>
                  <Input
                    id="conversion-amount"
                    type="text"
                    placeholder={`Enter ${conversionType === "wrap" ? "BYUSD" : "SFLUV"} amount`}
                    value={amount}
                    onChange={(e) => handleAmountChange(e.target.value)}
                    disabled={isProcessing}
                  />
                  <p className="text-sm text-muted-foreground">Available: {getAvailableBalance()}</p>
                </div>

                <Button
                  onClick={handleTokenConversion}
                  disabled={isProcessing || !amount || !selectedWallet}
                  className="w-full"
                >
                  {isProcessing ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      {conversionType === "wrap" ? "Wrapping..." : "Unwrapping..."}
                    </>
                  ) : (
                    <>
                      {conversionType === "wrap" ? (
                        <ArrowUp className="mr-2 h-4 w-4" />
                      ) : (
                        <ArrowDown className="mr-2 h-4 w-4" />
                      )}
                      {conversionType === "wrap" ? "Wrap to SFLUV" : "Unwrap to BYUSD"}
                    </>
                  )}
                </Button>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <CreditCard className="h-5 w-5 text-purple-600" />
                  PayPal Cash Conversion
                </CardTitle>
                <CardDescription>Convert BYUSD stablecoins to cash in your PayPal account</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="paypal-account">PayPal Account</Label>
                  <div className="flex gap-2">
                    <Select value={selectedPaypalAccount} onValueChange={setSelectedPaypalAccount}>
                      <SelectTrigger className="flex-1">
                        <SelectValue placeholder="Select PayPal account">
                          {selectedPaypalData && (
                            <div className="flex items-center gap-2 truncate">
                              <CreditCard className="h-4 w-4 flex-shrink-0" />
                              <span className="truncate">{selectedPaypalData.name}</span>
                            </div>
                          )}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {paypalAccounts.map((account) => (
                          <SelectItem key={account.id} value={account.id}>
                            <div className="flex items-center gap-2">
                              <CreditCard className="h-4 w-4" />
                              <div className="flex flex-col">
                                <span className="font-medium">{account.name}</span>
                                <span className="text-xs text-muted-foreground">{account.email}</span>
                              </div>
                              {account.isDefault && (
                                <Badge variant="secondary" className="ml-2 text-xs">
                                  Default
                                </Badge>
                              )}
                            </div>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Dialog open={isPaypalModalOpen} onOpenChange={setIsPaypalModalOpen}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="icon" className="shrink-0">
                          <Plus className="h-4 w-4" />
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-[500px]">
                        <DialogHeader>
                          <DialogTitle>Connect PayPal Account</DialogTitle>
                          <DialogDescription>
                            Enter your PayPal credentials to connect your account for cash conversions.
                          </DialogDescription>
                        </DialogHeader>
                        <div className="grid gap-4 py-4 max-h-[60vh] overflow-y-auto">
                          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                            <div className="space-y-2">
                              <Label htmlFor="paypal-first-name">
                                First Name <span className="text-red-500">*</span>
                              </Label>
                              <Input
                                id="paypal-first-name"
                                type="text"
                                placeholder="John"
                                value={newPaypalFirstName}
                                onChange={(e) => setNewPaypalFirstName(e.target.value)}
                                disabled={isConnectingPaypal}
                              />
                            </div>
                            <div className="space-y-2">
                              <Label htmlFor="paypal-last-name">
                                Last Name <span className="text-red-500">*</span>
                              </Label>
                              <Input
                                id="paypal-last-name"
                                type="text"
                                placeholder="Doe"
                                value={newPaypalLastName}
                                onChange={(e) => setNewPaypalLastName(e.target.value)}
                                disabled={isConnectingPaypal}
                              />
                            </div>
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-email">
                              PayPal Email Address <span className="text-red-500">*</span>
                            </Label>
                            <Input
                              id="paypal-email"
                              type="email"
                              placeholder="your-email@example.com"
                              value={newPaypalEmail}
                              onChange={(e) => setNewPaypalEmail(e.target.value)}
                              disabled={isConnectingPaypal}
                            />
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-password">
                              PayPal Password <span className="text-red-500">*</span>
                            </Label>
                            <div className="relative">
                              <Input
                                id="paypal-password"
                                type={showPassword ? "text" : "password"}
                                placeholder="Enter your PayPal password"
                                value={newPaypalPassword}
                                onChange={(e) => setNewPaypalPassword(e.target.value)}
                                disabled={isConnectingPaypal}
                                className="pr-10"
                              />
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="absolute right-0 top-0 h-full px-3 py-2 hover:bg-transparent"
                                onClick={() => setShowPassword(!showPassword)}
                                disabled={isConnectingPaypal}
                              >
                                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                              </Button>
                            </div>
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-phone">Phone Number (Optional)</Label>
                            <Input
                              id="paypal-phone"
                              type="tel"
                              placeholder="+1 (555) 123-4567"
                              value={newPaypalPhone}
                              onChange={(e) => setNewPaypalPhone(e.target.value)}
                              disabled={isConnectingPaypal}
                            />
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="paypal-account-name">
                              Account Display Name <span className="text-red-500">*</span>
                            </Label>
                            <Input
                              id="paypal-account-name"
                              type="text"
                              placeholder="My PayPal Account"
                              value={newPaypalAccountName}
                              onChange={(e) => setNewPaypalAccountName(e.target.value)}
                              disabled={isConnectingPaypal}
                            />
                            <p className="text-xs text-muted-foreground">
                              This name will be displayed in the account selection dropdown.
                            </p>
                          </div>
                          <div className="p-4 bg-blue-50 dark:bg-blue-950 rounded-lg">
                            <div className="flex items-start gap-2">
                              <CreditCard className="h-4 w-4 text-blue-600 mt-0.5 flex-shrink-0" />
                              <div className="text-xs text-blue-700 dark:text-blue-300">
                                <p className="font-medium mb-1">Secure Connection</p>
                                <p>
                                  Your PayPal credentials are encrypted and securely transmitted. We use PayPal's
                                  official API to verify and connect your account.
                                </p>
                              </div>
                            </div>
                          </div>
                        </div>
                        <DialogFooter className="gap-2">
                          <Button
                            className="w-full sm:w-auto"
                            variant="outline"
                            onClick={() => {
                              setIsPaypalModalOpen(false)
                              // Reset form when closing
                              setNewPaypalEmail("")
                              setNewPaypalPassword("")
                              setNewPaypalFirstName("")
                              setNewPaypalLastName("")
                              setNewPaypalPhone("")
                              setNewPaypalAccountName("")
                              setShowPassword(false)
                            }}
                            disabled={isConnectingPaypal}
                          >
                            Cancel
                          </Button>
                          <Button className="w-full sm:w-auto" onClick={handleConnectPaypalAccount} disabled={isConnectingPaypal}>
                            {isConnectingPaypal ? (
                              <>
                                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                Connecting...
                              </>
                            ) : (
                              <>
                                <CreditCard className="mr-2 h-4 w-4" />
                                Connect Account
                              </>
                            )}
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>
                  </div>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="paypal-amount">Amount to Convert</Label>
                  <Input
                    id="paypal-amount"
                    type="text"
                    placeholder="0.00"
                    value={paypalAmount}
                    onChange={(e) => handlePaypalAmountChange(e.target.value)}
                    disabled={isConvertingToPaypal}
                  />
                  <p className="text-sm text-muted-foreground">Available: {getPaypalAvailableBalance()}</p>
                </div>

                <div className="p-3 bg-blue-50 dark:bg-blue-950 rounded-lg">
                  <p className="text-xs text-blue-700 dark:text-blue-300">
                    <strong>Note:</strong> Funds will be transferred to your selected PayPal account. Processing may
                    take 1-3 business days.
                  </p>
                </div>

                <Button
                  onClick={handlePaypalConversion}
                  disabled={isConvertingToPaypal || !paypalAmount || !selectedWallet || !selectedPaypalAccount}
                  variant="outline"
                  className="w-full bg-transparent border-purple-200 hover:bg-purple-50"
                >
                  {isConvertingToPaypal ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Converting to PayPal...
                    </>
                  ) : (
                    <>
                      <CreditCard className="mr-2 h-4 w-4" />
                      Convert to PayPal Cash
                    </>
                  )}
                </Button>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="merchants" className="space-y-6">
          <Card>
            <CardHeader className="pb-6">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Users className="h-6 w-6" />
                Manage Merchants
              </CardTitle>
              <CardDescription className="text-base mt-2">Review and manage merchant application status</CardDescription>
              <div className="mt-4 flex flex-col gap-3">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search merchants..."
                    value={merchantSearch}
                    onChange={(e) => setMerchantSearch(e.target.value)}
                    className="pl-9"
                  />
                </div>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                  <div className="text-sm text-muted-foreground">
                    Showing {filteredMerchants.length} of {authedMapLocations.length} merchant applications
                  </div>
                  <div className="w-full sm:w-[220px] space-y-1">
                    <Label className="text-xs text-muted-foreground">Filter by status</Label>
                    <Select value={merchantStatusFilter} onValueChange={setMerchantStatusFilter}>
                      <SelectTrigger>
                        <SelectValue placeholder="All statuses" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">All</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {filteredMerchants.length === 0 ? (
                <div className="text-center py-8">
                  <Building2 className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Merchant Applications</h3>
                  <p className="text-muted-foreground">No merchants match the selected status filter.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {filteredMerchants.map((location) => (
                    <Card
                      key={location.id}
                      className="cursor-pointer hover:shadow-md transition-shadow"
                      onClick={() => openMerchantModal(location)}
                    >
                      <CardContent className="p-4">
                        <div className="flex flex-col items-start gap-4 sm:flex-row sm:justify-between">
                          <div className="flex min-w-0 flex-1 items-start gap-4">
                            <Avatar className="h-12 w-12">
                              <AvatarImage src={location.image_url || "/placeholder.svg"} alt={location.name} />
                              <AvatarFallback>
                                {location.name
                                  .split(" ")
                                  .map((n) => n[0])
                                  .join("")}
                              </AvatarFallback>
                            </Avatar>
                            <div className="flex-1 space-y-2">
                              <div>
                                <h4 className="font-semibold">{location.name}</h4>
                                <p className="text-sm text-muted-foreground">{location.type}</p>
                              </div>
                              <div className="grid gap-1 text-sm">
                                <div className="flex items-center gap-2">
                                  <Mail className="h-3 w-3" />
                                  <span className="break-all">{location.email}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                  <Phone className="h-3 w-3" />
                                  <span>{location.phone}</span>
                                </div>
                                <div className="flex items-center gap-2">
                                  <MapPin className="h-3 w-3" />
                                  <span className="break-words">
                                    {location.street}, {location.city}, {location.state}{" "}
                                    {location.zip}
                                  </span>
                                </div>
                              </div>
                            </div>
                          </div>
                          <Badge
                            variant={
                              approvalToStatus(location.approval) === "approved"
                                ? "default"
                                : approvalToStatus(location.approval) === "rejected"
                                  ? "destructive"
                                  : "outline"
                            }
                          >
                            {formatStatusLabel(approvalToStatus(location.approval))}
                          </Badge>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="w9" className="space-y-6">
          <Card>
            <CardHeader className="pb-6">
              <CardTitle className="flex items-center gap-2 text-xl">
                <FileCheck className="h-6 w-6" />
                W9 Submissions Pending Approval
              </CardTitle>
              <CardDescription className="text-base mt-2">Review and approve W9 submissions</CardDescription>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <Badge variant="destructive" className="text-sm px-3 py-1">
                  {pendingW9Submissions.length} Pending
                </Badge>
                <span className="text-sm text-muted-foreground">submissions awaiting review</span>
              </div>
            </CardHeader>
            <CardContent>
              {w9Loading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin" />
                </div>
              ) : pendingW9Submissions.length === 0 ? (
                <div className="text-center py-8">
                  <FileCheck className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Pending W9 Submissions</h3>
                  <p className="text-muted-foreground">All W9 submissions have been processed.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {pendingW9Submissions.map((submission) => (
                    <Card key={submission.id} className="border-l-4 border-l-yellow-500">
                      <CardContent className="p-4">
                        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                          <div className="flex-1 space-y-2">
                            <div>
                              <h4 className="font-semibold">Wallet</h4>
                              <p className="text-sm text-muted-foreground break-all">{submission.wallet_address}</p>
                            </div>
                            <div className="grid gap-1 text-sm">
                              <div className="flex items-center gap-2">
                                <Mail className="h-3 w-3" />
                                <span className="break-all">{submission.email}</span>
                              </div>
                              {submission.user_contact_email && submission.user_contact_email !== submission.email && (
                                <div className="flex items-center gap-2 text-yellow-700">
                                  <AlertTriangle className="h-3 w-3" />
                                  <span className="text-xs">
                                    Email on user profile differs: {submission.user_contact_email}
                                  </span>
                                </div>
                              )}
                              <div className="flex items-center gap-2">
                                <CalendarIcon className="h-3 w-3" />
                                <span>Year {submission.year}</span>
                              </div>
                            </div>
                          </div>
                          <div className="flex w-full flex-wrap gap-2 md:w-auto md:justify-end">
                            <Button className="w-full sm:w-auto" size="sm" onClick={() => handleApproveW9(submission.id)}>
                              <Check className="h-4 w-4" />
                              Approve
                            </Button>
                            <Button className="w-full sm:w-auto" size="sm" variant="outline" onClick={() => handleRejectW9(submission.id)}>
                              <X className="h-4 w-4" />
                              Reject
                            </Button>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="affiliates" className="space-y-6">
          {affiliatesError && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{affiliatesError}</span>
            </div>
          )}

          <Dialog open={affiliateModalOpen} onOpenChange={setAffiliateModalOpen}>
            <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Manage Affiliate</DialogTitle>
                <DialogDescription>
                  {selectedAffiliate?.nickname || selectedAffiliate?.organization || "Affiliate"} ·{" "}
                  {formatStatusLabel(selectedAffiliate?.status || "pending")}
                </DialogDescription>
              </DialogHeader>
              {selectedAffiliate && (
                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label>Nickname</Label>
                    <Input
                      value={affiliateNickname}
                      onChange={(e) => setAffiliateNickname(e.target.value)}
                      placeholder="Display name"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Weekly Allocation (SFLuv)</Label>
                    <Input
                      type="number"
                      min="0"
                      value={affiliateWeeklyBalance}
                      onChange={(e) => setAffiliateWeeklyBalance(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>One-Time Balance (SFLuv)</Label>
                    <Input
                      type="number"
                      min="0"
                      value={affiliateBonus}
                      onChange={(e) => setAffiliateBonus(e.target.value)}
                      placeholder="Set one-time balance"
                    />
                    <p className="text-xs text-muted-foreground">
                      Current one-time balance: {selectedAffiliate.one_time_balance}
                    </p>
                  </div>
                  <div className="space-y-2">
                    <Label>Change Approval Status</Label>
                    <Select
                      value={affiliateStatusDraft}
                      onValueChange={(value) => setAffiliateStatusDraft(value as Affiliate["status"])}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {affiliateModalError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{affiliateModalError}</span>
                    </div>
                  )}

                  <div className="flex justify-end">
                    <Button
                      className="w-full sm:w-auto"
                      variant="secondary"
                      disabled={affiliateUpdating}
                      onClick={handleAffiliateSave}
                    >
                      {affiliateUpdating ? "Saving..." : "Save Changes"}
                    </Button>
                  </div>
                </div>
              )}
            </DialogContent>
          </Dialog>

          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Users className="h-5 w-5" />
                Manage Affiliates
              </CardTitle>
              <CardDescription>Approve requests and manage affiliate balances</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-3">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search affiliates..."
                    value={affiliateSearch}
                    onChange={(e) => { setAffiliateSearch(e.target.value); setAffiliatePage(0) }}
                    className="pl-9"
                  />
                </div>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                  <div className="text-sm text-muted-foreground">
                    Showing {filteredAffiliates.length} affiliates
                  </div>
                  <div className="w-full sm:w-[220px] space-y-1">
                    <Label className="text-xs text-muted-foreground">Filter by status</Label>
                    <Select value={affiliateStatusFilter} onValueChange={setAffiliateStatusFilter}>
                      <SelectTrigger>
                        <SelectValue placeholder="All statuses" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">All</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
              {filteredAffiliates.length === 0 ? (
                <div className="text-center py-8">
                  <Users className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Affiliates Found</h3>
                  <p className="text-muted-foreground">No affiliates match the selected filter.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {filteredAffiliates.map((affiliate) => (
                    <Card
                      key={affiliate.user_id}
                      className="cursor-pointer hover:shadow-md transition-shadow"
                      onClick={() => openAffiliateModal(affiliate)}
                    >
                      <CardContent className="flex flex-col items-start gap-2 p-4 sm:flex-row sm:items-center sm:justify-between">
                        <div>
                          <p className="font-medium text-black dark:text-white">
                            {affiliate.nickname || affiliate.organization}
                          </p>
                          {affiliate.nickname && (
                            <p className="text-xs text-muted-foreground">{affiliate.organization}</p>
                          )}
                        </div>
                        <Badge variant={affiliate.status === "approved" ? "default" : affiliate.status === "rejected" ? "destructive" : "outline"}>
                          {formatStatusLabel(affiliate.status)}
                        </Badge>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
              <div className="flex items-center justify-between pt-2">
                <Button variant="outline" size="sm" onClick={() => setAffiliatePage((p) => Math.max(0, p - 1))} disabled={affiliatePage === 0}>
                  <ChevronLeft className="h-4 w-4" />Previous
                </Button>
                <span className="text-sm text-muted-foreground">Page {affiliatePage + 1}</span>
                <Button variant="outline" size="sm" onClick={() => setAffiliatePage((p) => p + 1)} disabled={affiliates.length < 20}>
                  Next<ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="proposers" className="space-y-6">
          {proposersError && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{proposersError}</span>
            </div>
          )}

          <Dialog open={proposerModalOpen} onOpenChange={setProposerModalOpen}>
            <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Manage Proposer</DialogTitle>
                <DialogDescription>
                  {selectedProposer?.nickname || selectedProposer?.organization || "Proposer"} ·{" "}
                  {formatStatusLabel(selectedProposer?.status || "pending")}
                </DialogDescription>
              </DialogHeader>
              {selectedProposer && (
                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label>Nickname</Label>
                    <Input
                      value={proposerNickname}
                      onChange={(e) => setProposerNickname(e.target.value)}
                      placeholder="Display name"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Notification Email</Label>
                    <Input value={selectedProposer.email} disabled />
                  </div>
                  <div className="space-y-2">
                    <Label>Change Approval Status</Label>
                    <Select
                      value={proposerStatusDraft}
                      onValueChange={(value) => setProposerStatusDraft(value as Proposer["status"])}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {proposerModalError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{proposerModalError}</span>
                    </div>
                  )}

                  <div className="flex justify-end">
                    <Button
                      className="w-full sm:w-auto"
                      variant="secondary"
                      disabled={proposerUpdating}
                      onClick={handleProposerSave}
                    >
                      {proposerUpdating ? "Saving..." : "Save Changes"}
                    </Button>
                  </div>
                </div>
              )}
            </DialogContent>
          </Dialog>

          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Users className="h-5 w-5" />
                Manage Proposers
              </CardTitle>
              <CardDescription>Approve proposer requests and manage proposer access.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-3">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search proposers..."
                    value={proposerSearch}
                    onChange={(e) => { setProposerSearch(e.target.value); setProposerPage(0) }}
                    className="pl-9"
                  />
                </div>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                  <div className="text-sm text-muted-foreground">
                    Showing {filteredProposers.length} proposers
                  </div>
                  <div className="w-full sm:w-[220px] space-y-1">
                    <Label className="text-xs text-muted-foreground">Filter by status</Label>
                    <Select value={proposerStatusFilter} onValueChange={setProposerStatusFilter}>
                      <SelectTrigger>
                        <SelectValue placeholder="All statuses" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">All</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
              {filteredProposers.length === 0 ? (
                <div className="text-center py-8">
                  <Users className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Proposers Found</h3>
                  <p className="text-muted-foreground">No proposers match the selected filter.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {filteredProposers.map((proposer) => (
                    <Card
                      key={proposer.user_id}
                      className="cursor-pointer hover:shadow-md transition-shadow"
                      onClick={() => openProposerModal(proposer)}
                    >
                      <CardContent className="flex flex-col items-start gap-2 p-4 sm:flex-row sm:items-center sm:justify-between">
                        <div>
                          <p className="font-medium text-black dark:text-white">
                            {proposer.nickname || proposer.organization}
                          </p>
                          <p className="text-xs text-muted-foreground">{proposer.organization}</p>
                          <p className="text-xs text-muted-foreground">{proposer.email}</p>
                        </div>
                        <Badge variant={proposer.status === "approved" ? "default" : proposer.status === "rejected" ? "destructive" : "outline"}>
                          {formatStatusLabel(proposer.status)}
                        </Badge>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
              <div className="flex items-center justify-between pt-2">
                <Button variant="outline" size="sm" onClick={() => setProposerPage((p) => Math.max(0, p - 1))} disabled={proposerPage === 0}>
                  <ChevronLeft className="h-4 w-4" />Previous
                </Button>
                <span className="text-sm text-muted-foreground">Page {proposerPage + 1}</span>
                <Button variant="outline" size="sm" onClick={() => setProposerPage((p) => p + 1)} disabled={proposers.length < 20}>
                  Next<ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="improvers" className="space-y-6">
          {improversError && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{improversError}</span>
            </div>
          )}

          <Dialog open={improverModalOpen} onOpenChange={setImproverModalOpen}>
            <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Manage Improver</DialogTitle>
                <DialogDescription>
                  Update approval status for this improver request.
                </DialogDescription>
              </DialogHeader>
              {selectedImprover && (
                <div className="space-y-4">
                  <div className="space-y-1">
                    <Label>Name</Label>
                    <Input value={`${selectedImprover.first_name} ${selectedImprover.last_name}`} disabled />
                  </div>
                  <div className="space-y-1">
                    <Label>Email</Label>
                    <Input value={selectedImprover.email} disabled />
                  </div>
                  <div className="space-y-1">
                    <Label>User ID</Label>
                    <Input value={selectedImprover.user_id} disabled className="font-mono text-xs" />
                  </div>
                  <div className="space-y-1">
                    <Label>Change Approval Status</Label>
                    <Select
                      value={improverStatusDraft}
                      onValueChange={(value) => setImproverStatusDraft(value as Improver["status"])}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {improverModalError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{improverModalError}</span>
                    </div>
                  )}

                  <div className="flex justify-end">
                    <Button
                      className="w-full sm:w-auto"
                      onClick={saveImproverModal}
                      disabled={improverModalUpdating}
                    >
                      {improverModalUpdating ? "Saving..." : "Save Changes"}
                    </Button>
                  </div>
                </div>
              )}
            </DialogContent>
          </Dialog>

          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Users className="h-5 w-5" />
                Manage Improvers
              </CardTitle>
              <CardDescription>Approve or reject improver access requests</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search improvers..."
                    value={improverSearch}
                    onChange={(e) => { setImproverSearch(e.target.value); setImproverPage(0) }}
                    className="pl-9"
                  />
                </div>
                <div className="w-full sm:w-[220px] space-y-1">
                  <Label className="text-xs text-muted-foreground">Filter by status</Label>
                  <Select value={improverStatusFilter} onValueChange={setImproverStatusFilter}>
                    <SelectTrigger>
                      <SelectValue placeholder="All statuses" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All</SelectItem>
                      <SelectItem value="approved">Approved</SelectItem>
                      <SelectItem value="pending">Pending</SelectItem>
                      <SelectItem value="rejected">Rejected</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              {filteredImprovers.length === 0 ? (
                <div className="text-center py-8">
                  <Users className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Improvers Found</h3>
                  <p className="text-muted-foreground">No improvers match the selected status filter.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {filteredImprovers.map((improver) => (
                    <Card
                      key={improver.user_id}
                      className="cursor-pointer hover:shadow-md transition-shadow"
                      onClick={() => openImproverModal(improver)}
                    >
                      <CardContent className="p-4">
                        <div className="flex flex-col items-start gap-3 md:flex-row md:items-center md:justify-between">
                          <div>
                            <p className="font-medium text-black dark:text-white">
                              {improver.first_name} {improver.last_name}
                            </p>
                            <p className="text-sm text-muted-foreground">{improver.email}</p>
                            <p className="text-xs text-muted-foreground break-all">User: {improver.user_id}</p>
                          </div>
                          <Badge variant={improver.status === "approved" ? "default" : improver.status === "rejected" ? "destructive" : "outline"}>
                            {formatStatusLabel(improver.status)}
                          </Badge>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
              <div className="flex items-center justify-between pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setImproverPage((p) => Math.max(0, p - 1))}
                  disabled={improverPage === 0}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">Page {improverPage + 1}</span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setImproverPage((p) => p + 1)}
                  disabled={improvers.length < 20}
                >
                  Next
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="supervisors" className="space-y-6">
          {supervisorsError && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{supervisorsError}</span>
            </div>
          )}

          <Dialog open={supervisorModalOpen} onOpenChange={setSupervisorModalOpen}>
            <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Manage Supervisor</DialogTitle>
                <DialogDescription>
                  Update approval status and nickname for this supervisor request.
                </DialogDescription>
              </DialogHeader>
              {selectedSupervisor && (
                <div className="space-y-4">
                  <div className="space-y-1">
                    <Label>Organization</Label>
                    <Input value={selectedSupervisor.organization} disabled />
                  </div>
                  <div className="space-y-1">
                    <Label>Email</Label>
                    <Input value={selectedSupervisor.email} disabled />
                  </div>
                  <div className="space-y-1">
                    <Label>User ID</Label>
                    <Input value={selectedSupervisor.user_id} disabled className="font-mono text-xs" />
                  </div>
                  <div className="space-y-1">
                    <Label>Nickname</Label>
                    <Input
                      value={supervisorNickname}
                      onChange={(e) => setSupervisorNickname(e.target.value)}
                      placeholder="Nickname (optional)"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label>Change Approval Status</Label>
                    <Select
                      value={supervisorStatusDraft}
                      onValueChange={(value) => setSupervisorStatusDraft(value as Supervisor["status"])}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {supervisorModalError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{supervisorModalError}</span>
                    </div>
                  )}

                  <div className="flex justify-end">
                    <Button
                      className="w-full sm:w-auto"
                      onClick={saveSupervisorModal}
                      disabled={supervisorModalUpdating}
                    >
                      {supervisorModalUpdating ? "Saving..." : "Save Changes"}
                    </Button>
                  </div>
                </div>
              )}
            </DialogContent>
          </Dialog>

          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Users className="h-5 w-5" />
                Manage Supervisors
              </CardTitle>
              <CardDescription>Approve or reject supervisor access requests</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search supervisors..."
                    value={supervisorSearch}
                    onChange={(e) => { setSupervisorSearch(e.target.value); setSupervisorPage(0) }}
                    className="pl-9"
                  />
                </div>
                <div className="w-full sm:w-[220px] space-y-1">
                  <Label className="text-xs text-muted-foreground">Filter by status</Label>
                  <Select value={supervisorStatusFilter} onValueChange={setSupervisorStatusFilter}>
                    <SelectTrigger>
                      <SelectValue placeholder="All statuses" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All</SelectItem>
                      <SelectItem value="approved">Approved</SelectItem>
                      <SelectItem value="pending">Pending</SelectItem>
                      <SelectItem value="rejected">Rejected</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              {filteredSupervisors.length === 0 ? (
                <div className="text-center py-8">
                  <Users className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Supervisors Found</h3>
                  <p className="text-muted-foreground">No supervisors match the selected status filter.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {filteredSupervisors.map((supervisor) => (
                    <Card
                      key={supervisor.user_id}
                      className="cursor-pointer hover:shadow-md transition-shadow"
                      onClick={() => openSupervisorModal(supervisor)}
                    >
                      <CardContent className="p-4">
                        <div className="flex flex-col items-start gap-3 md:flex-row md:items-center md:justify-between">
                          <div>
                            <p className="font-medium text-black dark:text-white">
                              {supervisor.nickname || supervisor.organization}
                            </p>
                            <p className="text-sm text-muted-foreground">{supervisor.organization}</p>
                            <p className="text-sm text-muted-foreground">{supervisor.email}</p>
                            <p className="text-xs text-muted-foreground break-all">User: {supervisor.user_id}</p>
                          </div>
                          <Badge variant={supervisor.status === "approved" ? "default" : supervisor.status === "rejected" ? "destructive" : "outline"}>
                            {formatStatusLabel(supervisor.status)}
                          </Badge>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
              <div className="flex items-center justify-between pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setSupervisorPage((p) => Math.max(0, p - 1))}
                  disabled={supervisorPage === 0}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">Page {supervisorPage + 1}</span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setSupervisorPage((p) => p + 1)}
                  disabled={supervisors.length < 20}
                >
                  Next
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="workflows" className="space-y-6">
          {adminWorkflowsError && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{adminWorkflowsError}</span>
            </div>
          )}

          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <FileCheck className="h-5 w-5" />
                Manage Workflows
              </CardTitle>
              <CardDescription>Search workflows by title or assigned improver email, then manage series claims.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-3">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search workflows by title or improver email..."
                    value={adminWorkflowsSearch}
                    onChange={(event) => {
                      setAdminWorkflowsSearch(event.target.value)
                      setAdminWorkflowsPage(0)
                    }}
                    className="pl-9"
                  />
                </div>
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div className="text-sm text-muted-foreground">
                    {groupedAdminWorkflows.length} of {adminWorkflowsTotal} series
                  </div>
                  <div className="flex items-center gap-2">
                    <Label htmlFor="workflow-include-archived" className="text-sm font-normal text-muted-foreground">
                      Include archived
                    </Label>
                    <Switch
                      id="workflow-include-archived"
                      checked={adminWorkflowsIncludeArchived}
                      onCheckedChange={(checked) => {
                        setAdminWorkflowsIncludeArchived(Boolean(checked))
                        setAdminWorkflowsPage(0)
                      }}
                    />
                  </div>
                </div>
              </div>

              {groupedAdminWorkflows.length === 0 ? (
                <div className="text-center py-8">
                  <FileCheck className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No Workflows Found</h3>
                  <p className="text-muted-foreground">No workflows match this search.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {groupedAdminWorkflows.map((group) => {
                    if (group.workflows.length === 0) return null
                    const cardIndex = getAdminSeriesCardIndex(group)
                    const workflow = group.workflows[cardIndex]
                    const canShiftBackward = cardIndex > 0
                    const canShiftForward = cardIndex < group.workflows.length - 1

                    return (
                      <Card
                        key={`admin-series-${group.seriesId}`}
                        className="cursor-pointer transition-shadow hover:shadow-md"
                        onClick={() => void openAdminSeriesWorkflowDetails(group, cardIndex)}
                      >
                        <CardContent className="p-4 space-y-3">
                          <div className="flex flex-col items-start gap-3 md:flex-row md:items-center md:justify-between">
                            <div>
                              <p className="font-medium text-black dark:text-white">{workflow.title}</p>
                              <p className="text-xs text-muted-foreground">
                                Start: {new Date(workflow.start_at * 1000).toLocaleString()}
                              </p>
                            </div>
                            <Badge
                              variant={
                                workflow.status === "approved" || workflow.status === "in_progress"
                                  ? "default"
                                  : workflow.status === "blocked"
                                    ? "secondary"
                                    : "outline"
                              }
                            >
                              {formatWorkflowDisplayStatus(workflow)}
                            </Badge>
                          </div>

                          <p className="text-sm text-muted-foreground line-clamp-2">{workflow.description || "No description provided."}</p>
                          <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                            <span>Series workflow {cardIndex + 1} of {group.workflows.length}</span>
                            <span>Recurrence: {workflow.recurrence.replace("_", " ")}</span>
                          </div>
                          <div className="flex flex-wrap gap-1">
                            {workflow.assigned_improver_emails.length === 0 ? (
                              <Badge variant="outline">No assigned improvers</Badge>
                            ) : (
                              workflow.assigned_improver_emails.map((email) => (
                                <Badge key={`${workflow.id}-${email}`} variant="secondary" className="max-w-full truncate">
                                  {email}
                                </Badge>
                              ))
                            )}
                          </div>

                          <div className="space-y-2 border-t border-border/60 pt-3">
                            <div className="grid grid-cols-2 gap-2 sm:w-fit">
                              <Button
                                className="w-full"
                                size="sm"
                                variant="outline"
                                onClick={(event) => {
                                  event.stopPropagation()
                                  shiftAdminSeriesCardIndex(group, -1)
                                }}
                                disabled={!canShiftBackward}
                                aria-label="Show previous workflow in this series"
                              >
                                <ChevronLeft className="h-4 w-4" />
                              </Button>
                              <Button
                                className="w-full"
                                size="sm"
                                variant="outline"
                                onClick={(event) => {
                                  event.stopPropagation()
                                  shiftAdminSeriesCardIndex(group, 1)
                                }}
                                disabled={!canShiftForward}
                                aria-label="Show next workflow in this series"
                              >
                                <ChevronRight className="h-4 w-4" />
                              </Button>
                            </div>
                            <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
                              <Button
                                className="w-full sm:w-auto"
                                size="sm"
                                variant="outline"
                                onClick={(event) => {
                                  event.stopPropagation()
                                  void openAdminSeriesWorkflowDetails(group, cardIndex)
                                }}
                              >
                                View Details
                              </Button>
                              <Button
                                className="w-full sm:w-auto"
                                size="sm"
                                variant="destructive"
                                onClick={(event) => {
                                  event.stopPropagation()
                                  void openAdminRevokeModal(group.seriesId)
                                }}
                              >
                                Revoke Improver Claim
                              </Button>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    )
                  })}
                </div>
              )}

              <div className="flex items-center justify-between pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setAdminWorkflowsPage((page) => Math.max(0, page - 1))}
                  disabled={adminWorkflowsPage === 0}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">Page {adminWorkflowsPage + 1}</span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setAdminWorkflowsPage((page) => page + 1)}
                  disabled={(adminWorkflowsPage + 1) * adminWorkflowsCount >= adminWorkflowsTotal}
                >
                  Next
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="issuers" className="space-y-6">
          {(issuerRequestsError || issuersError) && (
            <div className="flex items-center gap-2 rounded-lg bg-red-50 p-3 text-sm text-red-600 dark:bg-red-900/20">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{issuerRequestsError || issuersError}</span>
            </div>
          )}

          <Dialog open={issuerRequestModalOpen} onOpenChange={setIssuerRequestModalOpen}>
            <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Manage Issuer</DialogTitle>
                <DialogDescription>
                  Update approval status, nickname, and allowed credentials.
                </DialogDescription>
              </DialogHeader>
              {selectedIssuerRequest && (
                <div className="space-y-4">
                  <div className="space-y-1">
                    <Label>Organization</Label>
                    <Input value={selectedIssuerRequest.organization} disabled />
                  </div>
                  <div className="space-y-1">
                    <Label>Email</Label>
                    <Input value={selectedIssuerRequest.email} disabled />
                  </div>
                  <div className="space-y-1">
                    <Label>User ID</Label>
                    <Input value={selectedIssuerRequest.user_id} disabled className="font-mono text-xs" />
                  </div>
                  <div className="space-y-1">
                    <Label>Nickname</Label>
                    <Input
                      value={issuerRequestNickname}
                      onChange={(e) => setIssuerRequestNickname(e.target.value)}
                      placeholder="Nickname (optional)"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label>Change Approval Status</Label>
                    <Select value={issuerRequestStatusDraft} onValueChange={setIssuerRequestStatusDraft}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="pending">Pending</SelectItem>
                        <SelectItem value="approved">Approved</SelectItem>
                        <SelectItem value="rejected">Rejected</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-2">
                    <Label>Allowed Credentials</Label>
                    {credentialTypes.length === 0 ? (
                      <p className="text-sm text-muted-foreground">No credential types defined. Add types in the Credential Types tab.</p>
                    ) : (
                      <>
                        <Select value={issuerScopePicker} onValueChange={(value) => addIssuerScope(value)}>
                          <SelectTrigger>
                            <SelectValue placeholder="Select a credential to add" />
                          </SelectTrigger>
                          <SelectContent>
                            {credentialTypes
                              .filter((ct) => !issuerScopes.includes(ct.value))
                              .map((ct) => (
                                <SelectItem key={ct.value} value={ct.value}>
                                  {ct.label}
                                </SelectItem>
                              ))}
                          </SelectContent>
                        </Select>

                        <div className="flex flex-wrap gap-2 pt-2">
                          {issuerScopes.length === 0 ? (
                            <span className="text-xs text-muted-foreground">No credential scopes selected</span>
                          ) : (
                            issuerScopes.map((credential) => {
                              const credentialLabel = formatCredentialLabel(credential, credentialLabelMap)
                              return (
                                <Badge key={credential} variant="secondary" className="gap-1">
                                  {credentialLabel}
                                  <button
                                    type="button"
                                    className="ml-1"
                                    onClick={() => removeIssuerScope(credential)}
                                    aria-label={`Remove ${credentialLabel}`}
                                  >
                                    <X className="h-3 w-3" />
                                  </button>
                                </Badge>
                              )
                            })
                          )}
                        </div>
                      </>
                    )}
                  </div>

                  {issuerRequestModalError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{issuerRequestModalError}</span>
                    </div>
                  )}

                  <div className="flex justify-end">
                    <Button
                      className="w-full sm:w-auto"
                      onClick={saveIssuerRequestModal}
                      disabled={!!issuerRequestSaving[selectedIssuerRequest.user_id] || issuerSaving}
                    >
                      {!!issuerRequestSaving[selectedIssuerRequest.user_id] || issuerSaving ? "Saving..." : "Save Changes"}
                    </Button>
                  </div>
                </div>
              )}
            </DialogContent>
          </Dialog>

          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <Users className="h-5 w-5" />
                Manage Issuers
              </CardTitle>
              <CardDescription>Approve requests and manage issuer credential scopes.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search issuers..."
                    value={issuerRequestSearch}
                    onChange={(e) => { setIssuerRequestSearch(e.target.value); setIssuerRequestPage(0) }}
                    className="pl-9"
                  />
                </div>
                <div className="w-full sm:w-[220px] space-y-1">
                  <Label className="text-xs text-muted-foreground">Filter by status</Label>
                  <Select value={issuerStatusFilter} onValueChange={setIssuerStatusFilter}>
                    <SelectTrigger>
                      <SelectValue placeholder="All statuses" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All</SelectItem>
                      <SelectItem value="approved">Approved</SelectItem>
                      <SelectItem value="pending">Pending</SelectItem>
                      <SelectItem value="rejected">Rejected</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              {filteredIssuerRequests.length === 0 ? (
                <div className="py-8 text-center">
                  <Users className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                  <h3 className="text-lg font-medium">No Issuer Requests</h3>
                  <p className="text-muted-foreground">No issuers match the selected status filter.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {filteredIssuerRequests.map((req) => (
                    <Card
                      key={req.user_id}
                      className={`cursor-pointer hover:shadow-md transition-shadow ${selectedIssuerRequest?.user_id === req.user_id ? "ring-2 ring-primary" : ""}`}
                      onClick={() => openIssuerRequestModal(req)}
                    >
                      <CardContent className="flex flex-col items-start gap-2 p-4 sm:flex-row sm:items-center sm:justify-between">
                        <div>
                          <p className="font-medium text-black dark:text-white break-words">{req.nickname || req.organization}</p>
                          {req.nickname && <p className="text-xs text-muted-foreground">{req.organization}</p>}
                        </div>
                        <Badge variant={req.status === "approved" ? "default" : req.status === "rejected" ? "destructive" : "outline"}>
                          {formatStatusLabel(req.status)}
                        </Badge>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
              <div className="flex items-center justify-between pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setIssuerRequestPage((p) => Math.max(0, p - 1))}
                  disabled={issuerRequestPage === 0}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">Page {issuerRequestPage + 1}</span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setIssuerRequestPage((p) => p + 1)}
                  disabled={issuerRequests.length < 20}
                >
                  Next
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="credential-types" className="space-y-6">
          {credentialTypesError && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{credentialTypesError}</span>
            </div>
          )}

          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="flex items-center gap-2 text-xl">
                <FileCheck className="h-5 w-5" />
                Credential Types
              </CardTitle>
              <CardDescription>Define the credential types that issuers can grant to users.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <form onSubmit={createCredentialType} className="space-y-3">
                <p className="text-sm font-medium">Add New Credential Type</p>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div className="space-y-1">
                    <Label htmlFor="cred-value" className="text-xs">Value (slug)</Label>
                    <Input
                      id="cred-value"
                      value={newCredentialValue}
                      onChange={(e) => setNewCredentialValue(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ""))}
                      placeholder="e.g. dpw_certified"
                      className="font-mono text-sm"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="cred-label" className="text-xs">Display Label</Label>
                    <Input
                      id="cred-label"
                      value={newCredentialLabel}
                      onChange={(e) => setNewCredentialLabel(e.target.value)}
                      placeholder="e.g. DPW Certified"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="cred-visibility" className="text-xs">Visibility</Label>
                    <Select
                      value={newCredentialVisibility}
                      onValueChange={(value) => setNewCredentialVisibility(normalizeCredentialVisibility(value))}
                    >
                      <SelectTrigger id="cred-visibility">
                        <SelectValue placeholder="Select visibility" />
                      </SelectTrigger>
                      <SelectContent>
                        {credentialVisibilityOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <p className="text-xs text-muted-foreground">
                  {credentialVisibilityOptions.find((option) => option.value === newCredentialVisibility)?.description}
                </p>
                <p className="text-xs text-muted-foreground">Note: Deleting a credential type does not revoke existing grants.</p>
                <div className="flex justify-end">
                  <Button className="w-full sm:w-auto" type="submit" disabled={credentialTypeSaving || !newCredentialValue || !newCredentialLabel}>
                    {credentialTypeSaving ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" />Adding...</> : <><Plus className="mr-2 h-4 w-4" />Add Type</>}
                  </Button>
                </div>
              </form>

              <div className="space-y-4 border-t pt-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                  <div className="relative flex-1">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                      placeholder="Search by credential title or slug..."
                      value={credentialTypeSearch}
                      onChange={(event) => {
                        setCredentialTypeSearch(event.target.value)
                        setCredentialTypePage(0)
                      }}
                      className="pl-9"
                    />
                  </div>
                  <p className="text-xs text-muted-foreground sm:text-right">
                    {filteredCredentialTypes.length} credential{filteredCredentialTypes.length === 1 ? "" : "s"} found
                  </p>
                </div>

                {filteredCredentialTypes.length === 0 ? (
                  <p className="text-sm text-muted-foreground py-4 text-center">No credential types match your search.</p>
                ) : (
                  <div className="space-y-2">
                    {paginatedCredentialTypes.map((credentialType) => {
                      const badgePreview = buildCredentialBadgeDataUrl(credentialType)
                      return (
                        <Card
                          key={credentialType.value}
                          className={cn(
                            "cursor-pointer transition-shadow hover:shadow-md",
                            selectedCredentialType?.value === credentialType.value && "ring-2 ring-primary",
                          )}
                          onClick={() => openCredentialTypeModal(credentialType)}
                        >
                          <CardContent className="flex flex-col gap-3 p-3 sm:flex-row sm:items-center sm:justify-between">
                            <div className="flex items-center gap-3 min-w-0">
                              <div className="h-12 w-12 shrink-0 overflow-hidden rounded-md border bg-secondary/40">
                                {badgePreview ? (
                                  <img
                                    src={badgePreview}
                                    alt={`${credentialType.label} badge`}
                                    className="h-full w-full object-cover"
                                  />
                                ) : (
                                  <div className="flex h-full w-full items-center justify-center text-muted-foreground">
                                    <FileCheck className="h-4 w-4" />
                                  </div>
                                )}
                              </div>
                              <div className="min-w-0">
                                <p className="font-medium text-sm truncate">{credentialType.label}</p>
                                <p className="font-mono text-xs text-muted-foreground truncate">{credentialType.value}</p>
                                <Badge
                                  variant="outline"
                                  className={cn(
                                    "mt-2 capitalize text-[10px] leading-none",
                                    getCredentialVisibilityBadgeClassName(credentialType.visibility),
                                  )}
                                >
                                  {getCredentialVisibilityLabel(credentialType.visibility)}
                                </Badge>
                              </div>
                            </div>
                            <Button
                              size="sm"
                              variant="outline"
                              className="w-full text-red-600 border-red-300 hover:bg-red-50 sm:w-auto"
                              onClick={(event) => {
                                event.stopPropagation()
                                void deleteCredentialType(credentialType)
                              }}
                            >
                              <X className="h-4 w-4" />
                              Delete
                            </Button>
                          </CardContent>
                        </Card>
                      )
                    })}
                  </div>
                )}

                <div className="flex items-center justify-between pt-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setCredentialTypePage((page) => Math.max(0, page - 1))}
                    disabled={credentialTypePage === 0}
                  >
                    <ChevronLeft className="h-4 w-4" />
                    Previous
                  </Button>
                  <span className="text-sm text-muted-foreground">
                    Page {Math.min(credentialTypePage + 1, credentialTypesTotalPages)} of {credentialTypesTotalPages}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setCredentialTypePage((page) => page + 1)}
                    disabled={credentialTypePage >= credentialTypesTotalPages - 1 || filteredCredentialTypes.length === 0}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          <Dialog
            open={credentialTypeModalOpen}
            onOpenChange={(open) => {
              if (open) {
                setCredentialTypeModalOpen(true)
                return
              }
              closeCredentialTypeModal()
            }}
          >
            <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Credential Type Details</DialogTitle>
                <DialogDescription>View and edit credential title, slug, visibility, and badge image.</DialogDescription>
              </DialogHeader>

              {selectedCredentialType && (
                <div className="space-y-4">
                  <div className="space-y-1">
                    <Label htmlFor="credential-type-title">Credential Title</Label>
                    <Input
                      id="credential-type-title"
                      value={credentialTypeDraftLabel}
                      onChange={(event) => setCredentialTypeDraftLabel(event.target.value)}
                      placeholder="Credential title"
                    />
                  </div>

                  <div className="space-y-1">
                    <Label htmlFor="credential-type-slug">Credential Slug</Label>
                    <div className="flex gap-2">
                      <Input
                        id="credential-type-slug"
                        value={selectedCredentialType.value}
                        readOnly
                        className="font-mono text-sm"
                      />
                      <Button
                        type="button"
                        variant="outline"
                        onClick={copyCredentialSlug}
                        className="shrink-0"
                      >
                        <Copy className="mr-2 h-4 w-4" />
                        Copy
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-1">
                    <Label htmlFor="credential-type-visibility">Visibility</Label>
                    <Select
                      value={credentialTypeDraftVisibility}
                      onValueChange={(value) => setCredentialTypeDraftVisibility(normalizeCredentialVisibility(value))}
                    >
                      <SelectTrigger id="credential-type-visibility">
                        <SelectValue placeholder="Select visibility" />
                      </SelectTrigger>
                      <SelectContent>
                        {credentialVisibilityOptions.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <p className="text-xs text-muted-foreground">
                      {credentialVisibilityOptions.find((option) => option.value === credentialTypeDraftVisibility)?.description}
                    </p>
                  </div>

                  <div className="space-y-2">
                    <Label>Badge Image</Label>
                    <div className="flex items-center gap-3">
                      <div className="h-20 w-20 overflow-hidden rounded-md border bg-secondary/40">
                        {credentialTypeDraftBadgePreview ? (
                          <img
                            src={credentialTypeDraftBadgePreview}
                            alt={`${selectedCredentialType.label} badge`}
                            className="h-full w-full object-cover"
                          />
                        ) : (
                          <div className="flex h-full w-full items-center justify-center text-xs text-muted-foreground text-center px-2">
                            No badge
                          </div>
                        )}
                      </div>
                      <div className="space-y-2">
                        <Input type="file" accept="image/*" onChange={uploadCredentialBadge} />
                        <p className="text-xs text-muted-foreground">PNG, JPG, GIF, and WebP up to {maxCredentialBadgeUploadLabel}.</p>
                        {credentialTypeDraftBadgePreview && (
                          <Button type="button" variant="outline" size="sm" onClick={clearCredentialBadge}>
                            Remove Badge
                          </Button>
                        )}
                      </div>
                    </div>
                  </div>

                  {credentialTypeModalError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{credentialTypeModalError}</span>
                    </div>
                  )}

                  <DialogFooter className="flex-col-reverse gap-2 sm:flex-row sm:justify-between">
                    <Button
                      type="button"
                      variant="outline"
                      className="w-full text-red-600 border-red-300 hover:bg-red-50 sm:w-auto"
                      disabled={credentialTypeModalSaving}
                      onClick={() => void deleteCredentialType(selectedCredentialType)}
                    >
                      <X className="mr-2 h-4 w-4" />
                      Delete Type
                    </Button>
                    <Button
                      type="button"
                      className="w-full sm:w-auto"
                      onClick={saveCredentialTypeModal}
                      disabled={credentialTypeModalSaving || !credentialTypeDraftLabel.trim()}
                    >
                      {credentialTypeModalSaving ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" />Saving...</> : "Save Changes"}
                    </Button>
                  </DialogFooter>
                </div>
              )}
            </DialogContent>
          </Dialog>
        </TabsContent>

        <TabsContent value="events" className="space-y-6">
          {eventsError != "" && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{eventsError}</span>
            </div>
          )}
          <AddEventModal
            open={eventsModalOpen}
            onOpenChange={toggleNewEventModal}
            handleAddEvent={handleAddEvent}
            addEventError={eventsError}
            currentBalance={faucetBalance == "-" ? 0 : Number(faucetBalance)}
          />
          <EventModal
            event={eventDetailsEvent}
            open={eventDetailModalOpen}
            onOpenChange={toggleEventDetailModal}
            handleDeleteEvent={handleDeleteEvent}
            deleteEventError={deleteEventError}
            ownerLabel={eventDetailsEvent ? getOwnerLabel(eventDetailsEvent.owner) : undefined}
          />
          <DrainFaucetModal
            open={drainFaucetModalOpen}
            onOpenChange={toggleDrainFaucetModal}
            handleDrainFaucet={handleDrainFaucet}
            drainFaucetError={drainFaucetError}
          />
          <Card>
            <CardHeader className="pb-6 flex flex-col gap-4 md:grid md:grid-cols-[2fr,1fr]">
              <div>
                <CardTitle className="flex items-center gap-2 text-xl">
                  <CalendarIcon className="h-6 w-6" />
                  Volunteer Events
                </CardTitle>
                <CardDescription className="text-base mt-2">Create and Manage Volunteer Events</CardDescription>
                <div className="flex flex-wrap items-center gap-2 mt-3">
                  <Badge className="text-xs sm:text-sm px-3 py-1 cursor-pointer" onClick={toggleDrainFaucetModal}>
                    {unallocatedBalance !== undefined
                      ? `${unallocatedBalance} / ${faucetBalance} SFLuv Available`
                      : `${faucetBalance} SFLuv`}
                  </Badge>
                  <span className="text-xs sm:text-sm text-muted-foreground">in faucet</span>
                </div>
                <div className="flex flex-col gap-2 mt-4 sm:flex-row sm:flex-wrap sm:items-center">
                  <Label className="text-xs text-muted-foreground">Filter by owner</Label>
                  <Select value={eventsOwnerFilter} onValueChange={setEventsOwnerFilter}>
                    <SelectTrigger className="w-full sm:w-[220px]">
                      <SelectValue placeholder="All owners" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All owners</SelectItem>
                      {ownerOptions.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="text-left md:text-right">
                <Button onClick={toggleNewEventModal} className="w-full md:w-auto">
                  + New Event
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {filteredEvents.length === 0 ? (
                <div className="text-center py-8">
                  <Leaf className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                  <h3 className="text-lg font-medium">No {eventsExpired ? "" : "Active"} Events</h3>
                  <p className="text-muted-foreground">Create a new event to see it here.</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {filteredEvents.map((event: Event) => (
                    <EventCard
                      key={event.id}
                      event={event}
                      toggleEventModal={toggleEventDetailModal}
                      setEventModalEvent={setEventDetailsEvent}
                      ownerLabel={getOwnerLabel(event.owner)}
                    />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="qrcodes" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <QrCode className="h-5 w-5" />
                Generate Event QR Codes
              </CardTitle>
              <CardDescription>
                Create QR codes for volunteer events that can be redeemed for SFLuv tokens
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Event Information */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold">Event Information</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>Event Start Date</Label>
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button
                          variant="outline"
                          className={cn(
                            "w-full justify-start text-left font-normal",
                            !eventStartDate && "text-muted-foreground",
                          )}
                          disabled={isGeneratingCodes}
                        >
                          <CalendarIcon className="mr-2 h-4 w-4" />
                          {eventStartDate ? eventStartDate.toLocaleDateString() : "Select start date"}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent className="w-auto p-0">
                        <Calendar
                          mode="single"
                          selected={eventStartDate}
                          onSelect={setEventStartDate}
                          initialFocus
                          className="border-none shadow-md"
                        />
                      </PopoverContent>
                    </Popover>
                  </div>

                  <div className="space-y-2">
                    <Label>Event End Date</Label>
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button
                          variant="outline"
                          className={cn(
                            "w-full justify-start text-left font-normal",
                            !eventEndDate && "text-muted-foreground",
                          )}
                          disabled={isGeneratingCodes}
                        >
                          <CalendarIcon className="mr-2 h-4 w-4" />
                          {eventEndDate ? eventEndDate.toLocaleDateString() : "Select end date"}
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent className="w-auto p-0">
                        <Calendar
                          mode="single"
                          selected={eventEndDate}
                          onSelect={setEventEndDate}
                          initialFocus
                          className="border-none shadow-md"
                        />
                      </PopoverContent>
                    </Popover>
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="start-time">Start Time</Label>
                    <Input
                      id="start-time"
                      type="time"
                      value={eventStartTime}
                      onChange={(e) => setEventStartTime(e.target.value)}
                      disabled={isGeneratingCodes}
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="end-time">End Time</Label>
                    <Input
                      id="end-time"
                      type="time"
                      value={eventEndTime}
                      onChange={(e) => setEventEndTime(e.target.value)}
                      disabled={isGeneratingCodes}
                    />
                  </div>
                </div>
              </div>

              {/* QR Code Configuration */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold">QR Code Configuration</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="sfluv-per-code">SFLuv per QR Code</Label>
                    <Input
                      id="sfluv-per-code"
                      type="text"
                      placeholder="50"
                      value={sfluvPerCode}
                      onChange={(e) => {
                        const formatted = formatAmount(e.target.value)
                        setSfluvPerCode(formatted)
                      }}
                      disabled={isGeneratingCodes}
                    />
                    <p className="text-xs text-muted-foreground">Amount of SFLuv each volunteer will receive</p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="number-of-codes">Number of QR Codes</Label>
                    <Input
                      id="number-of-codes"
                      type="number"
                      placeholder="100"
                      min="1"
                      max="1000"
                      value={numberOfCodes}
                      onChange={(e) => setNumberOfCodes(e.target.value)}
                      disabled={isGeneratingCodes}
                    />
                    <p className="text-xs text-muted-foreground">Maximum 1000 codes per event</p>
                  </div>
                </div>

                <div className="p-4 bg-blue-50 dark:bg-blue-950 rounded-lg">
                  <div className="flex items-start gap-2">
                    <QrCode className="h-4 w-4 text-blue-600 mt-0.5 flex-shrink-0" />
                    <div className="text-xs text-blue-700 dark:text-blue-300">
                      <p className="font-medium mb-1">QR Code Usage</p>
                      <p>
                        Each QR code can only be redeemed once during the event period. Volunteers scan the code to
                        receive their SFLuv directly to their wallet.
                      </p>
                    </div>
                  </div>
                </div>
              </div>

              {/* Generation Actions */}
              <div className="flex flex-col gap-3 sm:flex-row">
                <Button
                  onClick={handleGenerateQRCodes}
                  disabled={isGeneratingCodes || !eventStartDate || !eventEndDate}
                  className="w-full sm:flex-1"
                >
                  {isGeneratingCodes ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Generating QR Codes...
                    </>
                  ) : (
                    <>
                      <QrCode className="mr-2 h-4 w-4" />
                      Generate QR Codes
                    </>
                  )}
                </Button>

                {generatedCodes.length > 0 && (
                  <Button className="w-full sm:w-auto" onClick={handleDownloadQRCodes} variant="outline" disabled={isGeneratingCodes}>
                    <Download className="mr-2 h-4 w-4" />
                    Download CSV
                  </Button>
                )}
              </div>

              {/* Generated Codes Preview */}
              {generatedCodes.length > 0 && (
                <div className="space-y-4">
                  <h3 className="text-lg font-semibold">Generated QR Codes</h3>

                  <div className="p-4 bg-green-50 dark:bg-green-950 rounded-lg">
                    <div className="flex items-center gap-2 mb-2">
                      <Check className="h-4 w-4 text-green-600" />
                      <span className="font-medium text-green-700 dark:text-green-300">Successfully Generated</span>
                    </div>
                    <div className="text-sm text-green-700 dark:text-green-300 space-y-1">
                      <p>
                        <strong>QR Codes:</strong> {generatedCodes.length}
                      </p>
                      <p>
                        <strong>SFLuv per Code:</strong> {sfluvPerCode}
                      </p>
                      <p>
                        <strong>Total SFLuv:</strong>{" "}
                        {(Number.parseFloat(sfluvPerCode) * generatedCodes.length).toLocaleString()}
                      </p>
                      <p>
                        <strong>Event Period:</strong> {eventStartDate?.toLocaleDateString()} {eventStartTime} -{" "}
                        {eventEndDate?.toLocaleDateString()} {eventEndTime}
                      </p>
                    </div>
                  </div>

                  <div className="border rounded-lg p-4 max-h-60 overflow-y-auto">
                    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2 text-xs">
                      {generatedCodes.slice(0, 20).map((code) => (
                        <div key={code.id} className="p-2 bg-muted rounded text-center">
                          <div className="font-mono">Code #{code.codeNumber}</div>
                          <div className="text-muted-foreground">{code.sfluvAmount} SFLuv</div>
                        </div>
                      ))}
                      {generatedCodes.length > 20 && (
                        <div className="p-2 bg-muted rounded text-center text-muted-foreground">
                          +{generatedCodes.length - 20} more codes
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
          </div>
        </div>
      </Tabs>

      {/* Location Review Modal */}
      <Dialog open={isLocationReviewModalOpen} onOpenChange={setisLocationReviewModalOpen}>
        <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto p-4 sm:max-w-[900px] sm:p-6">
          <DialogHeader>
            <DialogTitle className="text-lg font-bold sm:text-xl">
              Manage Merchant - {selectedLocationForReview?.name}
            </DialogTitle>
            <DialogDescription>Review merchant details and update approval status.</DialogDescription>
          </DialogHeader>

          {selectedLocationForReview && (
            <div className="space-y-6 py-4">
              <div className="space-y-2">
                <Label>Change Approval Status</Label>
                <Select
                  value={merchantStatusDraft}
                  onValueChange={(value) => setMerchantStatusDraft(value as ApprovalStatus)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="pending">Pending</SelectItem>
                    <SelectItem value="approved">Approved</SelectItem>
                    <SelectItem value="rejected">Rejected</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {merchantModalError && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{merchantModalError}</span>
                </div>
              )}

              {/* Business Information Section */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Business Information</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business Name</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.name}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business Type</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.type}
                    </p>
                  </div>

                  <div className="md:col-span-2">
                    <Label className="text-sm font-medium text-muted-foreground">Business Description</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.description}
                    </p>
                  </div>

                </div>
              </div>

              {/* Contact Information Section */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Contact Information</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Primary Contact Name</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">{selectedLocationForReview.contact_firstname + " " + selectedLocationForReview.contact_lastname}</p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Admin Email Address</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.admin_email}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Admin Phone Number</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.admin_phone}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Website</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                     {selectedLocationForReview.website}
                    </p>
                  </div>
                </div>
              </div>

              {/* Business Address Section */}
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Business Address</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="md:col-span-2">
                    <Label className="text-sm font-medium text-muted-foreground">Street Address</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.street}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">City</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.city}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">State</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {selectedLocationForReview.state}
                    </p>
                  </div>
                </div>
              </div>

              {/* SFLuv Integration Section */}
              {/*
              <div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">SFLuv Integration Questions</h3>

                <div className="space-y-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      Why do you want to join the SFLuv network?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-3 bg-muted rounded">
                      We believe in supporting our local community and want to be part of a network that promotes local
                      businesses. SFLuv aligns with our values of community engagement and supporting the local economy.
                      We see this as an opportunity to connect with more local customers and contribute to the growth of
                      San Francisco's small business ecosystem.
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      How do you plan to integrate SFLuv tokens into your business operations?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-3 bg-muted rounded">
                      We plan to accept SFLuv tokens as payment for all our products and services. We will also offer
                      special discounts and promotions for customers who pay with SFLuv tokens to encourage adoption.
                      Additionally, we're interested in participating in community events and offering rewards to loyal
                      customers through the SFLuv platform.
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      What percentage of transactions do you expect to process with SFLuv tokens?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">15-25% within the first year</p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">
                      Do you have experience with cryptocurrency or digital payment systems?
                    </Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      Yes, we currently accept various digital payment methods including mobile payments and have basic
                      knowledge of cryptocurrency systems.
                    </p>
                  </div>
                </div>
              </div>
              */}

              {/* Business Verification Section */}
              {/*<div className="space-y-4">
                <h3 className="text-lg font-semibold text-foreground border-b pb-2">Business Verification</h3>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business License Number</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      BL-SF-{Math.floor(Math.random() * 100000)}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Tax ID (EIN)</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      **-***{Math.floor(Math.random() * 10000)}
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Business Insurance</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      ✅ General Liability Insurance Active
                    </p>
                  </div>

                  <div>
                    <Label className="text-sm font-medium text-muted-foreground">Application Date</Label>
                    <p className="text-sm text-foreground mt-1 p-2 bg-muted rounded">
                      {new Date().toLocaleDateString()}
                    </p>
                  </div>
                </div>
              </div>
            */}
            </div>
          )}

          <DialogFooter className="gap-2">
            <Button className="w-full sm:w-auto" variant="outline" onClick={() => setisLocationReviewModalOpen(false)}>
              Close
            </Button>
            <Button
              className="w-full sm:w-auto"
              onClick={saveMerchantModal}
              disabled={merchantModalSaving}
            >
              {merchantModalSaving ? "Saving..." : "Save Changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={adminRevokeModalOpen} onOpenChange={setAdminRevokeModalOpen}>
        <DialogContent className="w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] max-h-[90vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Revoke Improver Claim</DialogTitle>
            <DialogDescription>
              Select an improver claim to revoke for this workflow series.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {adminRevokeLoading ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading claimants...
              </div>
            ) : adminRevokeClaimants.length === 0 ? (
              <p className="text-sm text-muted-foreground">No claimants found for this series.</p>
            ) : (
              <div className="space-y-2">
                <Label>Improver Email</Label>
                <Select value={adminRevokeImproverId} onValueChange={setAdminRevokeImproverId}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select an improver" />
                  </SelectTrigger>
                  <SelectContent>
                    {adminRevokeClaimants.map((claimant) => (
                      <SelectItem key={claimant.user_id} value={claimant.user_id}>
                        {claimant.email || claimant.name || claimant.user_id}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}

            {adminRevokeError && (
              <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                <span>{adminRevokeError}</span>
              </div>
            )}

            <div className="flex justify-end">
              <Button
                className="w-full sm:w-auto"
                variant="destructive"
                disabled={
                  adminRevokeSubmitting ||
                  adminRevokeLoading ||
                  adminRevokeClaimants.length === 0 ||
                  !adminRevokeImproverId
                }
                onClick={revokeAdminSeriesClaim}
              >
                {adminRevokeSubmitting ? "Revoking..." : "Revoke Claim"}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <WorkflowDetailsModal
        workflow={adminWorkflowDetail}
        open={adminWorkflowDetailOpen}
        onOpenChange={(open) => {
          setAdminWorkflowDetailOpen(open)
          if (!open) {
            setAdminDetailSeriesContext(null)
          }
        }}
        loading={adminWorkflowDetailLoading}
        renderWorkflowActions={
          adminDetailSeriesContext
            ? (workflow) => {
                const hasSeriesNavigation = workflow.recurrence !== "one_time" && adminDetailSeriesContext.workflowIds.length > 1
                const canShiftBackward = hasSeriesNavigation && adminDetailSeriesContext.index > 0
                const canShiftForward =
                  hasSeriesNavigation && adminDetailSeriesContext.index < adminDetailSeriesContext.workflowIds.length - 1
                return (
                  <div className="space-y-2 rounded-md border bg-secondary/30 p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <p className="text-xs text-muted-foreground">
                        Series workflow {adminDetailSeriesContext.index + 1} of {adminDetailSeriesContext.workflowIds.length}
                      </p>
                      {hasSeriesNavigation && (
                        <div className="flex items-center gap-2">
                          <Button
                            className="h-8 w-8 p-0"
                            size="sm"
                            variant="outline"
                            onClick={() => void shiftAdminDetailSeriesWorkflow(-1)}
                            disabled={!canShiftBackward}
                            aria-label="Show previous workflow in this series"
                          >
                            <ChevronLeft className="h-4 w-4" />
                          </Button>
                          <Button
                            className="h-8 w-8 p-0"
                            size="sm"
                            variant="outline"
                            onClick={() => void shiftAdminDetailSeriesWorkflow(1)}
                            disabled={!canShiftForward}
                            aria-label="Show next workflow in this series"
                          >
                            <ChevronRight className="h-4 w-4" />
                          </Button>
                        </div>
                      )}
                    </div>
                    <Button
                      className="w-full sm:w-auto"
                      size="sm"
                      variant="destructive"
                      onClick={() => void openAdminRevokeModal(adminDetailSeriesContext.seriesId)}
                    >
                      Revoke Improver Claim
                    </Button>
                  </div>
                )
              }
            : undefined
        }
      />
    </div>
  )
}
