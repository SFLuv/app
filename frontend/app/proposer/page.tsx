"use client"

import { DragEvent, MouseEvent, PointerEvent, WheelEvent, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Checkbox } from "@/components/ui/checkbox"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { usePathname, useSearchParams } from "next/navigation"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { formatWorkflowDisplayStatus } from "@/lib/workflow-status"
import { cn } from "@/lib/utils"
import { useToast } from "@/hooks/use-toast"
import { AlertTriangle, CheckCircle2, ChevronDown, ChevronsUpDown, Clock, GripVertical, Loader2, Plus, Search, Trash2, X } from "lucide-react"
import {
  GlobalCredentialType,
  ProposerWorkflowListResponse,
  Workflow,
  WorkflowCreateRequest,
  WorkflowDeletionProposal,
  WorkflowEditProposal,
  WorkflowEditProposalCreateRequest,
  WorkflowDeletionTargetType,
  WorkflowDropdownOptionCreateInput,
  WorkflowPhotoAspectRatio,
  WorkflowRecurrence,
  WorkflowTemplate,
  WorkflowTemplateCreateRequest,
  WorkflowWorkItemCreateInput,
  WorkflowSupervisorDataField,
} from "@/types/workflow"
import { Supervisor } from "@/types/supervisor"
import { Proposer } from "@/types/proposer"
import { WorkflowDetailsModal } from "@/components/workflows/workflow-details-modal"

interface DraftRole {
  client_id: string
  title: string
  required_credentials: string[]
}

interface DraftDropdownOption extends WorkflowDropdownOptionCreateInput {
  notify_email_input: string
}

interface DraftWorkItem extends WorkflowWorkItemCreateInput {
  id: string
  dropdown_options: DraftDropdownOption[]
}

interface DraftStep {
  id: string
  title: string
  description: string
  bounty: string
  role_client_id: string
  allow_step_not_possible: boolean
  work_items: DraftWorkItem[]
}

interface DraftWorkflowSupervisor {
  enabled: boolean
  user_id: string
  bounty: string
}

interface DraftSupervisorDataField extends WorkflowSupervisorDataField {
  id: string
}

interface WorkflowSeriesGroup {
  key: string
  series_id: string
  workflows: Workflow[]
}

interface PendingWorkflowSubmissionPreview {
  payload: WorkflowCreateRequest
  workflow: Workflow
}

type DraftReorderDragState =
  | { type: "step"; stepId: string }
  | { type: "work-item"; stepId: string; itemId: string }
  | { type: "dropdown-option"; stepId: string; itemId: string; optionIndex: number }

type DraftReorderDropPosition = "before" | "after"

type DraftReorderDropIndicator =
  | { type: "step"; targetStepId: string; position: DraftReorderDropPosition }
  | { type: "work-item"; stepId: string; targetItemId: string; position: DraftReorderDropPosition }
  | { type: "dropdown-option"; stepId: string; itemId: string; targetOptionIndex: number; position: DraftReorderDropPosition }

const draftReorderMimeType = "text/plain"

const createDraftRole = (): DraftRole => ({
  client_id: crypto.randomUUID(),
  title: "",
  required_credentials: [],
})

const createDraftWorkItem = (): DraftWorkItem => ({
  id: crypto.randomUUID(),
  title: "",
  description: "",
  optional: false,
  requires_photo: false,
  camera_capture_only: false,
  photo_required_count: 1,
  photo_allow_any_count: false,
  photo_aspect_ratio: "square",
  requires_written_response: true,
  requires_dropdown: false,
  dropdown_options: [],
})

const createDraftStep = (): DraftStep => ({
  id: crypto.randomUUID(),
  title: "",
  description: "",
  bounty: "",
  role_client_id: "",
  allow_step_not_possible: false,
  work_items: [createDraftWorkItem()],
})

const createDraftWorkflowSupervisor = (): DraftWorkflowSupervisor => ({
  enabled: false,
  user_id: "",
  bounty: "",
})

const createDraftSupervisorDataField = (): DraftSupervisorDataField => ({
  id: crypto.randomUUID(),
  key: "",
  value: "",
})

const WORKFLOW_STATUS_FILTER_OPTIONS: Array<{
  value: "all" | Workflow["status"]
  label: string
}> = [
  { value: "all", label: "All Statuses" },
  { value: "pending", label: "Pending" },
  { value: "approved", label: "Approved" },
  { value: "rejected", label: "Rejected" },
  { value: "expired", label: "Expired" },
  { value: "failed", label: "Failed" },
  { value: "skipped", label: "Skipped" },
  { value: "blocked", label: "Blocked" },
  { value: "in_progress", label: "In Progress" },
  { value: "completed", label: "Completed" },
  { value: "paid_out", label: "Finalized" },
	{ value: "deleted", label: "Archived" },
]

const workflowNotificationEmailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const normalizeWorkflowPhotoAspectRatio = (value: string): WorkflowPhotoAspectRatio => {
  const normalized = value.trim().toLowerCase()
  if (normalized === "vertical" || normalized === "horizontal" || normalized === "square") {
    return normalized
  }
  return "square"
}

const isWellFormedWorkflowNotificationEmail = (value: string) => {
  if (!value) return false
  return workflowNotificationEmailPattern.test(value)
}

const moveArrayItem = <T,>(items: T[], fromIndex: number, toIndex: number): T[] => {
  if (fromIndex === toIndex) return items
  if (fromIndex < 0 || toIndex < 0 || fromIndex >= items.length || toIndex >= items.length) return items
  const next = [...items]
  const [moved] = next.splice(fromIndex, 1)
  if (moved === undefined) return items
  next.splice(toIndex, 0, moved)
  return next
}

const reorderTargetIndex = (
  sourceIndex: number,
  targetIndex: number,
  position: DraftReorderDropPosition,
  length: number,
) => {
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex >= length || targetIndex >= length) {
    return sourceIndex
  }

  let insertionIndex = position === "before" ? targetIndex : targetIndex + 1
  if (sourceIndex < insertionIndex) {
    insertionIndex -= 1
  }
  if (insertionIndex < 0) return 0
  if (insertionIndex >= length) return length - 1
  return insertionIndex
}

const toUTCISOStringFromDatetimeLocal = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    throw new Error("Workflow start date/time is invalid.")
  }
  return date.toISOString()
}

const toDatetimeLocalValueFromUnix = (value?: number | null) => {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) return ""
  const date = new Date(value * 1000)
  if (Number.isNaN(date.getTime())) return ""
  const offsetMs = date.getTimezoneOffset() * 60 * 1000
  return new Date(date.getTime() - offsetMs).toISOString().slice(0, 16)
}

const getDatePartFromDatetimeLocal = (value: string) => {
  const trimmed = value.trim()
  return trimmed.length >= 10 ? trimmed.slice(0, 10) : ""
}

const getTimePartFromDatetimeLocal = (value: string) => {
  const trimmed = value.trim()
  return trimmed.length >= 16 ? trimmed.slice(11, 16) : ""
}

const replaceTimePartInDatetimeLocal = (value: string, nextTime: string) => {
  const datePart = getDatePartFromDatetimeLocal(value)
  if (!datePart) return value
  const trimmedTime = nextTime.trim()
  if (!trimmedTime) return `${datePart}T00:00`
  return `${datePart}T${trimmedTime}`
}

const toTimeValueFromTemplateStartAt = (value?: number | null) => {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) return ""
  const totalSeconds = Math.max(0, Math.floor(value) - 1)
  const hours = Math.floor(totalSeconds / 3600) % 24
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}`
}

const getTodayDatePart = () => {
  const now = new Date()
  const offsetMs = now.getTimezoneOffset() * 60 * 1000
  return new Date(now.getTime() - offsetMs).toISOString().slice(0, 10)
}

const slugifyWorkflowPreviewOptionValue = (label: string, optionIndex: number) => {
  const base = label
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
  return base || `option_${optionIndex + 1}`
}

const applyTemplateStartTimeToDatetimeLocal = (currentValue: string, templateStartAt?: number | null) => {
  const templateTime = toTimeValueFromTemplateStartAt(templateStartAt)
  if (!templateTime) {
    return currentValue
  }

  const datePart = getDatePartFromDatetimeLocal(currentValue) || getTodayDatePart()
  return `${datePart}T${templateTime}`
}

const preventNumberInputScrollChange = (event: WheelEvent<HTMLInputElement>) => {
  event.currentTarget.blur()
}

const formatRecurrenceLabel = (value: WorkflowRecurrence) => {
  if (value === "one_time") return "One Time"
  return `${value.slice(0, 1).toUpperCase()}${value.slice(1)}`
}

const formatStepBountyIndicator = (value: string) => {
  const parsed = Number(value)
  const normalized = Number.isFinite(parsed) && parsed >= 0 ? parsed : 0
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(normalized)} SFLuv`
}

const sectionCardOpenByDefault = (state: Record<string, boolean>, key: string) => state[key] === true
const nestedCardOpenByDefault = (state: Record<string, boolean>, key: string) => state[key] !== false

const didClickInteractiveElement = (event: MouseEvent<HTMLElement>) => {
  const target = event.target as HTMLElement | null
  if (!target) return false
  return Boolean(target.closest("button, a, input, textarea, select, [role='button']"))
}

export default function ProposerPage() {
  const { user, status, authFetch } = useApp()
  const { toast } = useToast()
  const searchParams = useSearchParams()
  const pathname = usePathname()
  const tabFromQuery = searchParams.get("tab")
  const workflowStatusFromQuery = searchParams.get("workflow_status")
  const templateSearchFromQuery = searchParams.get("template_search")
  const workflowSearchFromQuery = searchParams.get("workflow_search")
  const workflowProposerFromQuery = searchParams.get("workflow_proposer")
  const workflowPageFromQueryRaw = Number(searchParams.get("workflow_page") || "0")
  const workflowPageFromQuery = Number.isFinite(workflowPageFromQueryRaw) && workflowPageFromQueryRaw >= 0 ? workflowPageFromQueryRaw : 0

  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([])
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [supervisors, setSupervisors] = useState<Supervisor[]>([])
  const [proposerOptions, setProposerOptions] = useState<Proposer[]>([])
  const [deletionProposals, setDeletionProposals] = useState<WorkflowDeletionProposal[]>([])
  const [workflowTotal, setWorkflowTotal] = useState(0)
  const [createDataLoading, setCreateDataLoading] = useState(false)
  const [createDataLoaded, setCreateDataLoaded] = useState(false)
  const [workflowListLoading, setWorkflowListLoading] = useState(false)
  const [workflowListLoaded, setWorkflowListLoaded] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [templateSaving, setTemplateSaving] = useState(false)
  const [deletionSubmitting, setDeletionSubmitting] = useState("")
  const [activeTab, setActiveTab] = useState<"create-workflow" | "your-workflows">(
    tabFromQuery === "your-workflows" ? "your-workflows" : "create-workflow"
  )
  const [workflowStatusFilter, setWorkflowStatusFilter] = useState<"all" | Workflow["status"]>(
    workflowStatusFromQuery &&
    WORKFLOW_STATUS_FILTER_OPTIONS.some((option) => option.value === workflowStatusFromQuery)
      ? (workflowStatusFromQuery as "all" | Workflow["status"])
      : "all"
  )
  const [error, setError] = useState("")
  const [successMessage, setSuccessMessage] = useState("")
  const [selectedTemplateId, setSelectedTemplateId] = useState("")
  const [templateComboOpen, setTemplateComboOpen] = useState(false)
  const [templateSearch, setTemplateSearch] = useState(templateSearchFromQuery || "")
  const [workflowSearch, setWorkflowSearch] = useState(workflowSearchFromQuery || "")
  const [workflowPage, setWorkflowPage] = useState(workflowPageFromQuery)
  const [workflowProposerFilter, setWorkflowProposerFilter] = useState(workflowProposerFromQuery || "all")
  const [deleteTemplateId, setDeleteTemplateId] = useState("")
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const [deleteTemplateLoading, setDeleteTemplateLoading] = useState(false)
  const [templateTitle, setTemplateTitle] = useState("")
  const [templateDescription, setTemplateDescription] = useState("")
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailWorkflow, setDetailWorkflow] = useState<Workflow | null>(null)
  const [submissionPreviewOpen, setSubmissionPreviewOpen] = useState(false)
  const [pendingWorkflowSubmission, setPendingWorkflowSubmission] = useState<PendingWorkflowSubmissionPreview | null>(null)
  const [submissionPreviewError, setSubmissionPreviewError] = useState("")

  const [saveFromWorkflowOpen, setSaveFromWorkflowOpen] = useState(false)
  const [saveFromWorkflowTitle, setSaveFromWorkflowTitle] = useState("")
  const [saveFromWorkflowDescription, setSaveFromWorkflowDescription] = useState("")
  const [saveFromWorkflowError, setSaveFromWorkflowError] = useState("")
  const [saveFromWorkflowSubmitting, setSaveFromWorkflowSubmitting] = useState(false)

  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [recurrence, setRecurrence] = useState<WorkflowRecurrence>("one_time")
  const [startAt, setStartAt] = useState("")
  const [hasRecurrenceEndDate, setHasRecurrenceEndDate] = useState(false)
  const [recurrenceEndAt, setRecurrenceEndAt] = useState("")
  const [editProposalWorkflowId, setEditProposalWorkflowId] = useState("")
  const [editProposalReason, setEditProposalReason] = useState("")
  const [roles, setRoles] = useState<DraftRole[]>([createDraftRole()])
  const [workflowSupervisor, setWorkflowSupervisor] = useState<DraftWorkflowSupervisor>(createDraftWorkflowSupervisor())
  const [workflowSupervisorDataFields, setWorkflowSupervisorDataFields] = useState<DraftSupervisorDataField[]>([])
  const [steps, setSteps] = useState<DraftStep[]>([createDraftStep()])
  const [dragState, setDragState] = useState<DraftReorderDragState | null>(null)
  const [dropIndicator, setDropIndicator] = useState<DraftReorderDropIndicator | null>(null)
  const [templateLibraryOpen, setTemplateLibraryOpen] = useState(false)
  const [workflowSupervisorOpen, setWorkflowSupervisorOpen] = useState(false)
  const [workflowRolesOpen, setWorkflowRolesOpen] = useState(false)
  const [roleCardOpenState, setRoleCardOpenState] = useState<Record<string, boolean>>({})
  const [stepCardOpenState, setStepCardOpenState] = useState<Record<string, boolean>>({})
  const [workItemCardOpenState, setWorkItemCardOpenState] = useState<Record<string, boolean>>({})

  const isApproved = Boolean(user?.isProposer || user?.isAdmin)
  const isAdminUser = Boolean(user?.isAdmin)
  const workflowPageSize = isAdminUser ? 10 : 200
  const createDataLoadedRef = useRef(false)
  const workflowListLoadedRef = useRef(false)
  const createDataRequestRef = useRef<Promise<void> | null>(null)
  const workflowListRequestIdRef = useRef(0)
  const canProposeDeletion = Boolean(user?.isProposer)
  const isEditProposalMode = editProposalWorkflowId.trim().length > 0
  const touchDragRef = useRef<{
    pointerId: number
    payload: DraftReorderDragState
  } | null>(null)
  const touchScrollRestoreRef = useRef<{
    overflow: string
    touchAction: string
  } | null>(null)

  const totalDraftBounty = useMemo(() => {
    const stepTotal = steps.reduce((sum, step) => sum + (Number(step.bounty) || 0), 0)
    const supervisorBounty = workflowSupervisor.enabled ? Number(workflowSupervisor.bounty) || 0 : 0
    return stepTotal + supervisorBounty
  }, [steps, workflowSupervisor.bounty, workflowSupervisor.enabled])

  const workflowSeriesGroups = useMemo<WorkflowSeriesGroup[]>(() => {
    const bySeries = new Map<string, Workflow[]>()
    for (const workflow of workflows) {
      const key = workflow.series_id?.trim() || workflow.id
      const existing = bySeries.get(key)
      if (existing) {
        existing.push(workflow)
      } else {
        bySeries.set(key, [workflow])
      }
    }

    let groups = Array.from(bySeries.entries()).map(([seriesId, items]) => {
      const sorted = [...items].sort((a, b) => {
        if (a.start_at !== b.start_at) return a.start_at - b.start_at
        if (a.created_at !== b.created_at) return a.created_at - b.created_at
        return a.id.localeCompare(b.id)
      })
      return {
        key: seriesId,
        series_id: seriesId,
        workflows: sorted,
      }
    })

    if (workflowStatusFilter !== "all") {
      groups = groups.filter((group) =>
        group.workflows.some((workflow) => workflow.status === workflowStatusFilter),
      )
    }

    groups.sort((a, b) => {
      const latestA = a.workflows[a.workflows.length - 1]
      const latestB = b.workflows[b.workflows.length - 1]
      if (latestA.created_at !== latestB.created_at) return latestB.created_at - latestA.created_at
      return latestB.start_at - latestA.start_at
    })

    return groups
  }, [workflows, workflowStatusFilter])

  const filteredTemplates = useMemo(() => {
    const s = templateSearch.trim().toLowerCase()
    if (!s) return templates
    return templates.filter((t) => t.template_title.toLowerCase().includes(s))
  }, [templates, templateSearch])

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === selectedTemplateId) ?? null,
    [templates, selectedTemplateId]
  )

  const proposerLabelById = useMemo(() => {
    const entries = proposerOptions.map((proposer) => {
      const label = (proposer.nickname || "").trim() || proposer.organization.trim() || proposer.email.trim() || proposer.user_id
      return [proposer.user_id, label] as const
    })
    return new Map(entries)
  }, [proposerOptions])

  const workflowPageCount = Math.max(1, Math.ceil(workflowTotal / workflowPageSize))

  const loadCreateData = useCallback(
    async (mode: "blocking" | "background" = "background") => {
      if (!isApproved) {
        createDataLoadedRef.current = true
        setCreateDataLoaded(true)
        return
      }
      if (createDataRequestRef.current) {
        return createDataRequestRef.current
      }

      const shouldSurfaceError = mode === "blocking" || !createDataLoadedRef.current
      const request = (async () => {
        setCreateDataLoading(true)
        try {
          const [templatesRes, credentialTypesRes, supervisorsRes] = await Promise.all([
            authFetch("/proposers/workflow-templates"),
            authFetch("/credentials/types"),
            authFetch("/supervisors/approved"),
          ])

          if (templatesRes.ok) {
            const templatesJson = await templatesRes.json()
            setTemplates(templatesJson || [])
          } else {
            setTemplates([])
          }

          if (credentialTypesRes.ok) {
            const credentialTypesJson = await credentialTypesRes.json()
            setCredentialTypes(credentialTypesJson || [])
          } else {
            setCredentialTypes([])
          }

          if (supervisorsRes.ok) {
            const supervisorsJson = await supervisorsRes.json()
            setSupervisors(supervisorsJson || [])
          } else {
            setSupervisors([])
          }

          setError((prev) => (prev === "Unable to load proposer form data right now." ? "" : prev))
        } catch {
          if (shouldSurfaceError) {
            setError("Unable to load proposer form data right now.")
          }
        } finally {
          createDataLoadedRef.current = true
          createDataRequestRef.current = null
          setCreateDataLoaded(true)
          setCreateDataLoading(false)
        }
      })()

      createDataRequestRef.current = request
      return request
    },
    [authFetch, isApproved],
  )

  const loadWorkflowListData = useCallback(
    async (mode: "blocking" | "background" = "background") => {
      if (!isApproved) {
        workflowListLoadedRef.current = true
        setWorkflowListLoaded(true)
        return
      }

      const shouldSurfaceError = mode === "blocking" || !workflowListLoadedRef.current
      const requestId = workflowListRequestIdRef.current + 1
      workflowListRequestIdRef.current = requestId

      setWorkflowListLoading(true)
      try {
        const params = new URLSearchParams()
        const effectiveWorkflowPage = isAdminUser ? workflowPage : 0
        params.set("page", String(effectiveWorkflowPage))
        params.set("count", String(workflowPageSize))
        if (workflowStatusFilter !== "all") {
          params.set("status", workflowStatusFilter)
        }
        const trimmedWorkflowSearch = workflowSearch.trim()
        if (trimmedWorkflowSearch) {
          params.set("search", trimmedWorkflowSearch)
        }
        if (isAdminUser && workflowProposerFilter !== "all") {
          params.set("proposer_id", workflowProposerFilter)
        }

        const workflowUrl = `/proposers/workflows${params.toString() ? `?${params.toString()}` : ""}`
        const requests: Promise<Response>[] = [
          authFetch(workflowUrl),
          authFetch("/proposers/workflow-deletion-proposals"),
        ]

        if (isAdminUser) {
          requests.push(authFetch("/admin/proposers?page=0&count=500"))
        }

        const [workflowsRes, deletionProposalsRes, proposersRes] = await Promise.all(requests)
        if (requestId !== workflowListRequestIdRef.current) {
          return
        }

        if (workflowsRes.ok) {
          const workflowsJson = (await workflowsRes.json()) as ProposerWorkflowListResponse
          const nextItems = workflowsJson?.items || []
          const nextTotal = workflowsJson?.total || 0
          const maxPage = nextTotal > 0 ? Math.max(0, Math.ceil(nextTotal / workflowPageSize) - 1) : 0
          if (effectiveWorkflowPage > maxPage) {
            setWorkflowPage(maxPage)
            return
          }
          setWorkflows(nextItems)
          setWorkflowTotal(nextTotal)
        } else {
          setWorkflows([])
          setWorkflowTotal(0)
        }

        if (deletionProposalsRes.ok) {
          const deletionProposalsJson = await deletionProposalsRes.json()
          setDeletionProposals(deletionProposalsJson || [])
        } else {
          setDeletionProposals([])
        }

        if (isAdminUser) {
          if (proposersRes && proposersRes.ok) {
            const proposersJson = (await proposersRes.json()) as Proposer[]
            setProposerOptions(proposersJson || [])
          } else {
            setProposerOptions([])
          }
        } else {
          setProposerOptions([])
        }

        setError((prev) => (prev === "Unable to load your workflows right now." ? "" : prev))
      } catch {
        if (requestId !== workflowListRequestIdRef.current) {
          return
        }
        if (shouldSurfaceError) {
          setError("Unable to load your workflows right now.")
        }
      } finally {
        if (requestId !== workflowListRequestIdRef.current) {
          return
        }
        workflowListLoadedRef.current = true
        setWorkflowListLoaded(true)
        setWorkflowListLoading(false)
      }
    },
    [authFetch, isAdminUser, isApproved, workflowPage, workflowPageSize, workflowProposerFilter, workflowSearch, workflowStatusFilter],
  )

  useEffect(() => {
    if (typeof window === "undefined") return
    const params = new URLSearchParams(window.location.search)
    params.set("tab", activeTab)
    if (workflowStatusFilter === "all") {
      params.delete("workflow_status")
    } else {
      params.set("workflow_status", workflowStatusFilter)
    }
    if (templateSearch) {
      params.set("template_search", templateSearch)
    } else {
      params.delete("template_search")
    }
    if (workflowSearch) {
      params.set("workflow_search", workflowSearch)
    } else {
      params.delete("workflow_search")
    }
    if (isAdminUser && workflowProposerFilter !== "all") {
      params.set("workflow_proposer", workflowProposerFilter)
    } else {
      params.delete("workflow_proposer")
    }
    if (isAdminUser && workflowPage > 0) {
      params.set("workflow_page", String(workflowPage))
    } else {
      params.delete("workflow_page")
    }
    const nextQuery = params.toString()
    const currentQuery = window.location.search.replace(/^\?/, "")
    if (nextQuery !== currentQuery) {
      window.history.replaceState(window.history.state, "", nextQuery ? `${pathname}?${nextQuery}` : pathname)
    }
  }, [activeTab, isAdminUser, pathname, templateSearch, workflowPage, workflowProposerFilter, workflowSearch, workflowStatusFilter])

  useEffect(() => {
    if (status !== "authenticated" || !isApproved) return

    if (activeTab === "your-workflows") {
      void loadWorkflowListData(workflowListLoadedRef.current ? "background" : "blocking")
      if (!createDataLoadedRef.current) {
        void loadCreateData("background")
      }
      return
    }

    void loadCreateData(createDataLoadedRef.current ? "background" : "blocking")
    if (!workflowListLoadedRef.current) {
      void loadWorkflowListData("background")
    }
  }, [activeTab, isApproved, loadCreateData, loadWorkflowListData, status])

  useEffect(() => {
    if (recurrence !== "one_time") return
    setHasRecurrenceEndDate(false)
    setRecurrenceEndAt("")
  }, [recurrence])

  useEffect(() => {
    return () => {
      touchDragRef.current = null
      if (typeof document !== "undefined" && touchScrollRestoreRef.current) {
        document.body.style.overflow = touchScrollRestoreRef.current.overflow
        document.body.style.touchAction = touchScrollRestoreRef.current.touchAction
        touchScrollRestoreRef.current = null
      }
    }
  }, [])

  useEffect(() => {
    setError("")
  }, [activeTab])

  useEffect(() => {
    if (activeTab !== "create-workflow") return
    setError("")
  }, [
    activeTab,
    title,
    description,
    recurrence,
    startAt,
    hasRecurrenceEndDate,
    recurrenceEndAt,
    editProposalReason,
    templateTitle,
    templateDescription,
    selectedTemplateId,
    workflowSupervisor,
    roles,
    steps,
  ])

  const updateRole = (roleId: string, update: Partial<DraftRole>) => {
    setRoles((prev) => prev.map((role) => (role.client_id === roleId ? { ...role, ...update } : role)))
  }

  const toggleRoleCredential = (roleId: string, credential: string, checked: boolean) => {
    setRoles((prev) =>
      prev.map((role) => {
        if (role.client_id !== roleId) return role
        const hasCredential = role.required_credentials.includes(credential)
        if (checked && !hasCredential) {
          return { ...role, required_credentials: [...role.required_credentials, credential] }
        }
        if (!checked && hasCredential) {
          return { ...role, required_credentials: role.required_credentials.filter((c) => c !== credential) }
        }
        return role
      }),
    )
  }

  const updateStep = (stepId: string, update: Partial<DraftStep>) => {
    setSteps((prev) => prev.map((step) => (step.id === stepId ? { ...step, ...update } : step)))
  }

  const updateWorkItem = (stepId: string, itemId: string, update: Partial<DraftWorkItem>) => {
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        return {
          ...step,
          work_items: step.work_items.map((item) => (item.id === itemId ? { ...item, ...update } : item)),
        }
      }),
    )
  }

  const dragPreviewFromElement = (event: DragEvent<HTMLElement>) => {
    const sourceElement = (event.currentTarget as HTMLElement).closest<HTMLElement>("[data-reorder-preview]")
    if (!sourceElement) return
    const sourceRect = sourceElement.getBoundingClientRect()
    if (!sourceRect.width || !sourceRect.height) return

    const clone = sourceElement.cloneNode(true) as HTMLElement
    clone.style.position = "fixed"
    clone.style.top = "-9999px"
    clone.style.left = "-9999px"
    clone.style.width = `${sourceRect.width}px`
    clone.style.pointerEvents = "none"
    clone.style.opacity = "0.95"
    clone.style.transform = "scale(0.98)"
    clone.style.boxShadow = "0 14px 28px rgba(0,0,0,0.2)"
    document.body.appendChild(clone)

    event.dataTransfer.setDragImage(clone, Math.min(40, sourceRect.width / 2), 20)
    window.setTimeout(() => {
      clone.remove()
    }, 0)
  }

  const dropPositionFromCoordinates = (clientY: number, element: HTMLElement): DraftReorderDropPosition => {
    const rect = element.getBoundingClientRect()
    const midpoint = rect.top + rect.height / 2
    return clientY < midpoint ? "before" : "after"
  }

  const dropPositionFromPointer = (event: DragEvent<HTMLElement>): DraftReorderDropPosition => {
    return dropPositionFromCoordinates(event.clientY, event.currentTarget)
  }

  const isTouchLikePointer = (event: PointerEvent<HTMLElement>) =>
    event.pointerType === "touch" || event.pointerType === "pen"

  const lockTouchScroll = () => {
    if (typeof document === "undefined") return
    if (touchScrollRestoreRef.current) return

    touchScrollRestoreRef.current = {
      overflow: document.body.style.overflow,
      touchAction: document.body.style.touchAction,
    }
    document.body.style.overflow = "hidden"
    document.body.style.touchAction = "none"
  }

  const unlockTouchScroll = () => {
    if (typeof document === "undefined") return
    if (!touchScrollRestoreRef.current) return

    document.body.style.overflow = touchScrollRestoreRef.current.overflow
    document.body.style.touchAction = touchScrollRestoreRef.current.touchAction
    touchScrollRestoreRef.current = null
  }

  const resolveTouchDropIndicator = (
    payload: DraftReorderDragState,
    clientX: number,
    clientY: number,
  ): DraftReorderDropIndicator | null => {
    if (typeof document === "undefined") return null
    const target = document.elementFromPoint(clientX, clientY) as HTMLElement | null
    if (!target) return null

    if (payload.type === "step") {
      const dropTarget = target.closest<HTMLElement>("[data-drop-step-id]")
      const targetStepId = dropTarget?.dataset.dropStepId?.trim()
      if (!dropTarget || !targetStepId) return null
      return {
        type: "step",
        targetStepId,
        position: dropPositionFromCoordinates(clientY, dropTarget),
      }
    }

    if (payload.type === "work-item") {
      const dropTarget = target.closest<HTMLElement>("[data-drop-work-item-id]")
      const stepId = dropTarget?.dataset.dropWorkItemStepId?.trim()
      const targetItemId = dropTarget?.dataset.dropWorkItemId?.trim()
      if (!dropTarget || !stepId || !targetItemId) return null
      if (stepId !== payload.stepId) return null
      return {
        type: "work-item",
        stepId,
        targetItemId,
        position: dropPositionFromCoordinates(clientY, dropTarget),
      }
    }

    const dropTarget = target.closest<HTMLElement>("[data-drop-option-index]")
    const stepId = dropTarget?.dataset.dropOptionStepId?.trim()
    const itemId = dropTarget?.dataset.dropOptionItemId?.trim()
    const targetOptionIndexRaw = dropTarget?.dataset.dropOptionIndex
    const targetOptionIndex = Number.parseInt(targetOptionIndexRaw || "", 10)
    if (!dropTarget || !stepId || !itemId || Number.isNaN(targetOptionIndex)) return null
    if (stepId !== payload.stepId || itemId !== payload.itemId) return null
    return {
      type: "dropdown-option",
      stepId,
      itemId,
      targetOptionIndex,
      position: dropPositionFromCoordinates(clientY, dropTarget),
    }
  }

  const beginTouchReorderDrag = (event: PointerEvent<HTMLElement>, payload: DraftReorderDragState) => {
    if (!isTouchLikePointer(event)) return
    event.preventDefault()
    event.stopPropagation()

    touchDragRef.current = {
      pointerId: event.pointerId,
      payload,
    }
    lockTouchScroll()
    setDragState(payload)
    setDropIndicator(null)
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  const updateTouchReorderDrag = (event: PointerEvent<HTMLElement>) => {
    const touchDrag = touchDragRef.current
    if (!touchDrag || touchDrag.pointerId !== event.pointerId) return
    event.preventDefault()
    event.stopPropagation()

    const nextDropIndicator = resolveTouchDropIndicator(touchDrag.payload, event.clientX, event.clientY)
    setDropIndicator(nextDropIndicator)
  }

  const completeTouchReorderDrag = (event: PointerEvent<HTMLElement>) => {
    const touchDrag = touchDragRef.current
    if (!touchDrag || touchDrag.pointerId !== event.pointerId) return
    event.preventDefault()
    event.stopPropagation()

    const target = resolveTouchDropIndicator(touchDrag.payload, event.clientX, event.clientY)
    if (target) {
      switch (touchDrag.payload.type) {
        case "step":
          if (target.type === "step" && touchDrag.payload.stepId !== target.targetStepId) {
            reorderSteps(touchDrag.payload.stepId, target.targetStepId, target.position)
          }
          break
        case "work-item":
          if (
            target.type === "work-item" &&
            touchDrag.payload.stepId === target.stepId &&
            touchDrag.payload.itemId !== target.targetItemId
          ) {
            reorderWorkItems(target.stepId, touchDrag.payload.itemId, target.targetItemId, target.position)
          }
          break
        case "dropdown-option":
          if (
            target.type === "dropdown-option" &&
            touchDrag.payload.stepId === target.stepId &&
            touchDrag.payload.itemId === target.itemId &&
            touchDrag.payload.optionIndex !== target.targetOptionIndex
          ) {
            reorderDropdownOptions(
              target.stepId,
              target.itemId,
              touchDrag.payload.optionIndex,
              target.targetOptionIndex,
              target.position,
            )
          }
          break
      }
    }

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    touchDragRef.current = null
    unlockTouchScroll()
    endReorderDrag()
  }

  const cancelTouchReorderDrag = (event: PointerEvent<HTMLElement>) => {
    const touchDrag = touchDragRef.current
    if (!touchDrag || touchDrag.pointerId !== event.pointerId) return

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    touchDragRef.current = null
    unlockTouchScroll()
    endReorderDrag()
  }

  const endReorderDrag = () => {
    setDragState(null)
    setDropIndicator(null)
  }

  const beginReorderDrag = (event: DragEvent<HTMLElement>, payload: DraftReorderDragState) => {
    event.dataTransfer.effectAllowed = "move"
    event.dataTransfer.setData(draftReorderMimeType, JSON.stringify(payload))
    dragPreviewFromElement(event)
    setDragState(payload)
    setDropIndicator(null)
  }

  const reorderSteps = (sourceStepId: string, targetStepId: string, position: DraftReorderDropPosition) => {
    if (sourceStepId === targetStepId) return
    setSteps((prev) => {
      const sourceIndex = prev.findIndex((step) => step.id === sourceStepId)
      const targetIndex = prev.findIndex((step) => step.id === targetStepId)
      const nextIndex = reorderTargetIndex(sourceIndex, targetIndex, position, prev.length)
      return moveArrayItem(prev, sourceIndex, nextIndex)
    })
  }

  const reorderWorkItems = (
    stepId: string,
    sourceItemId: string,
    targetItemId: string,
    position: DraftReorderDropPosition,
  ) => {
    if (sourceItemId === targetItemId) return
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        const sourceIndex = step.work_items.findIndex((item) => item.id === sourceItemId)
        const targetIndex = step.work_items.findIndex((item) => item.id === targetItemId)
        const nextIndex = reorderTargetIndex(sourceIndex, targetIndex, position, step.work_items.length)
        return {
          ...step,
          work_items: moveArrayItem(step.work_items, sourceIndex, nextIndex),
        }
      }),
    )
  }

  const reorderDropdownOptions = (
    stepId: string,
    itemId: string,
    sourceIndex: number,
    targetIndex: number,
    position: DraftReorderDropPosition,
  ) => {
    if (sourceIndex === targetIndex) return
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        return {
          ...step,
          work_items: step.work_items.map((item) => {
            if (item.id !== itemId) return item
            const nextIndex = reorderTargetIndex(sourceIndex, targetIndex, position, item.dropdown_options.length)
            return {
              ...item,
              dropdown_options: moveArrayItem(item.dropdown_options, sourceIndex, nextIndex),
            }
          }),
        }
      }),
    )
  }

  const addWorkItem = (stepId: string) => {
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        return { ...step, work_items: [...step.work_items, createDraftWorkItem()] }
      }),
    )
  }

  const removeWorkItem = (stepId: string, itemId: string) => {
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        const nextItems = step.work_items.filter((item) => item.id !== itemId)
        return { ...step, work_items: nextItems.length ? nextItems : [createDraftWorkItem()] }
      }),
    )
  }

  const addDropdownOption = (stepId: string, itemId: string) => {
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        return {
          ...step,
          work_items: step.work_items.map((item) => {
            if (item.id !== itemId) return item
            return {
              ...item,
                dropdown_options: [
                ...item.dropdown_options,
                {
                  label: "",
                  requires_written_response: false,
                  requires_photo_attachment: false,
                  camera_capture_only: false,
                  photo_instructions: "",
                  notify_emails: [],
                  notify_email_input: "",
                  send_pictures_with_email: false,
                },
              ],
            }
          }),
        }
      }),
    )
  }

  const updateDropdownOption = (
    stepId: string,
    itemId: string,
    optionIndex: number,
    update: Partial<DraftDropdownOption>,
  ) => {
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        return {
          ...step,
          work_items: step.work_items.map((item) => {
            if (item.id !== itemId) return item
            return {
              ...item,
              dropdown_options: item.dropdown_options.map((option, index) =>
                index === optionIndex ? { ...option, ...update } : option,
              ),
            }
          }),
        }
      }),
    )
  }

  const addDropdownOptionEmail = (stepId: string, itemId: string, optionIndex: number) => {
    const draftStep = steps.find((step) => step.id === stepId)
    const draftItem = draftStep?.work_items.find((item) => item.id === itemId)
    const draftOption = draftItem?.dropdown_options[optionIndex]
    if (!draftOption) return

    const email = draftOption.notify_email_input.trim().toLowerCase()
    if (!email) return
    const stepNumber = steps.findIndex((step) => step.id === stepId) + 1
    const itemNumber = draftStep?.work_items.findIndex((item) => item.id === itemId) ?? -1
    if (!isWellFormedWorkflowNotificationEmail(email)) {
      setSuccessMessage("")
      setError(`Malformed notification email on step ${stepNumber} item ${itemNumber + 1} option ${optionIndex + 1}.`)
      return
    }
    if (draftOption.notify_emails.includes(email)) {
      updateDropdownOption(stepId, itemId, optionIndex, { notify_email_input: "" })
      return
    }

    setError("")
    updateDropdownOption(stepId, itemId, optionIndex, {
      notify_emails: [...draftOption.notify_emails, email],
      notify_email_input: "",
    })
  }

  const removeDropdownOptionEmail = (stepId: string, itemId: string, optionIndex: number, email: string) => {
    const draftStep = steps.find((step) => step.id === stepId)
    const draftItem = draftStep?.work_items.find((item) => item.id === itemId)
    const draftOption = draftItem?.dropdown_options[optionIndex]
    if (!draftOption) return

    updateDropdownOption(stepId, itemId, optionIndex, {
      notify_emails: draftOption.notify_emails.filter((value) => value !== email),
    })
  }

  const removeDropdownOption = (stepId: string, itemId: string, optionIndex: number) => {
    setSteps((prev) =>
      prev.map((step) => {
        if (step.id !== stepId) return step
        return {
          ...step,
          work_items: step.work_items.map((item) => {
            if (item.id !== itemId) return item
            return {
              ...item,
              dropdown_options: item.dropdown_options.filter((_, index) => index !== optionIndex),
            }
          }),
        }
      }),
    )
  }

  const normalizeOptionNotificationEmails = (
    stepNumber: number,
    itemNumber: number,
    optionNumber: number,
    option: DraftDropdownOption,
  ) => {
    const normalized = new Set<string>()
    option.notify_emails.forEach((email) => {
      const value = email.trim().toLowerCase()
      if (!value) return
      if (!isWellFormedWorkflowNotificationEmail(value)) {
        throw new Error(`Malformed notification email on step ${stepNumber} item ${itemNumber} option ${optionNumber}.`)
      }
      normalized.add(value)
    })

    const pendingInput = option.notify_email_input.trim().toLowerCase()
    if (pendingInput) {
      if (!isWellFormedWorkflowNotificationEmail(pendingInput)) {
        throw new Error(`Malformed notification email on step ${stepNumber} item ${itemNumber} option ${optionNumber}.`)
      }
      normalized.add(pendingInput)
    }

    return Array.from(normalized)
  }

  const addWorkflowSupervisorDataField = () => {
    setWorkflowSupervisorDataFields((prev) => [...prev, createDraftSupervisorDataField()])
  }

  const updateWorkflowSupervisorDataField = (
    fieldId: string,
    patch: Partial<Pick<DraftSupervisorDataField, "key" | "value">>,
  ) => {
    setWorkflowSupervisorDataFields((prev) =>
      prev.map((field) => (field.id === fieldId ? { ...field, ...patch } : field)),
    )
  }

  const removeWorkflowSupervisorDataField = (fieldId: string) => {
    setWorkflowSupervisorDataFields((prev) => prev.filter((field) => field.id !== fieldId))
  }

  const workItemCollapseKey = (stepId: string, itemId: string) => `${stepId}:${itemId}`

  const resetForm = () => {
    setTitle("")
    setDescription("")
    setRecurrence("one_time")
    setStartAt("")
    setHasRecurrenceEndDate(false)
    setRecurrenceEndAt("")
    setSelectedTemplateId("")
    setEditProposalWorkflowId("")
    setEditProposalReason("")
    setRoles([createDraftRole()])
    setWorkflowSupervisor(createDraftWorkflowSupervisor())
    setWorkflowSupervisorDataFields([])
    setSteps([createDraftStep()])
    setDragState(null)
    setDropIndicator(null)
    setTemplateLibraryOpen(false)
    setWorkflowSupervisorOpen(false)
    setWorkflowRolesOpen(false)
    setRoleCardOpenState({})
    setStepCardOpenState({})
    setWorkItemCardOpenState({})
  }

  const normalizeDraftWorkflowFields = () => {
    const normalizedRoles = roles.map((role) => ({
      client_id: role.client_id,
      title: role.title.trim(),
      required_credentials: role.required_credentials,
    }))

    if (normalizedRoles.some((role) => !role.title || role.required_credentials.length === 0)) {
      throw new Error("Every role needs a title and at least one required credential.")
    }

    const normalizedSteps = steps.map((step, stepIndex) => ({
      title: step.title.trim(),
      description: step.description.trim(),
      bounty: Number(step.bounty),
      role_client_id: step.role_client_id,
      allow_step_not_possible: step.allow_step_not_possible,
      work_items: step.work_items.map((item, itemIndex) => {
        const requiresPhoto = Boolean(item.requires_photo)
        const photoRequiredCount = Number.isFinite(item.photo_required_count)
          ? Math.max(1, Math.floor(Number(item.photo_required_count)))
          : 1
        const photoAllowAnyCount = requiresPhoto ? Boolean(item.photo_allow_any_count) : false
        const photoAspectRatio = requiresPhoto
          ? normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio)
          : "square"

        return {
          title: item.title.trim(),
          description: item.description.trim(),
          optional: item.optional,
          requires_photo: requiresPhoto,
          camera_capture_only: requiresPhoto ? item.camera_capture_only : false,
          photo_required_count: photoRequiredCount,
          photo_allow_any_count: photoAllowAnyCount,
          photo_aspect_ratio: photoAspectRatio,
          requires_written_response: item.requires_written_response,
          requires_dropdown: item.requires_dropdown,
          dropdown_options: item.dropdown_options.map((option, optionIndex) => ({
            label: option.label.trim(),
            requires_written_response: option.requires_written_response,
            requires_photo_attachment: Boolean(option.requires_photo_attachment),
            camera_capture_only: Boolean(option.requires_photo_attachment) && Boolean(option.camera_capture_only),
            photo_instructions: Boolean(option.requires_photo_attachment) ? (option.photo_instructions || "").trim() : "",
            notify_emails: normalizeOptionNotificationEmails(stepIndex + 1, itemIndex + 1, optionIndex + 1, option),
            send_pictures_with_email: Boolean(option.send_pictures_with_email),
          })),
        }
      }),
    }))

    if (normalizedSteps.some((step) => !step.title || Number.isNaN(step.bounty) || step.bounty < 0 || !step.role_client_id)) {
      throw new Error("Every step needs a title, role assignment, and bounty zero or greater.")
    }

    for (const step of normalizedSteps) {
      for (const item of step.work_items) {
        if (!item.title) {
          throw new Error("Every work item needs a title.")
        }
        if (!item.requires_photo && !item.requires_written_response && !item.requires_dropdown) {
          throw new Error("Each work item must require photo, written response, or dropdown.")
        }
        if (item.requires_photo && !item.photo_allow_any_count && item.photo_required_count < 1) {
          throw new Error("Photo-required work items need a required photo count of at least 1.")
        }
        if (item.requires_dropdown && item.dropdown_options.length === 0) {
          throw new Error("Dropdown work items need at least one dropdown option.")
        }
        if (item.requires_dropdown && item.dropdown_options.some((option) => !option.label)) {
          throw new Error("Each dropdown option needs a label.")
        }
      }
    }

    let normalizedSupervisor: WorkflowCreateRequest["supervisor"] | undefined
    let normalizedSupervisorDataFields: WorkflowSupervisorDataField[] = []
    if (workflowSupervisor.enabled) {
      const supervisorUserID = workflowSupervisor.user_id.trim()
      if (!supervisorUserID) {
        throw new Error("Select a workflow supervisor.")
      }
      const supervisorBounty = workflowSupervisor.bounty.trim() === "" ? 0 : Number(workflowSupervisor.bounty)
      if (Number.isNaN(supervisorBounty) || supervisorBounty < 0) {
        throw new Error("Workflow supervisor bounty must be zero or greater.")
      }
      normalizedSupervisor = {
        user_id: supervisorUserID,
        bounty: supervisorBounty,
      }

      const seenSupervisorFieldKeys = new Set<string>()
      normalizedSupervisorDataFields = workflowSupervisorDataFields
        .map((field) => ({
          key: field.key.trim(),
          value: field.value.trim(),
        }))
        .filter((field) => field.key !== "" || field.value !== "")

      for (const field of normalizedSupervisorDataFields) {
        if (!field.key || !field.value) {
          throw new Error("Supervisor data fields require both key and value.")
        }
        const keyLookup = field.key.toLowerCase()
        if (seenSupervisorFieldKeys.has(keyLookup)) {
          throw new Error(`Duplicate supervisor data field key: ${field.key}`)
        }
        seenSupervisorFieldKeys.add(keyLookup)
      }
    }

    return {
      normalizedRoles,
      normalizedSteps,
      normalizedSupervisor,
      normalizedSupervisorDataFields,
    }
  }

  const buildTemplatePayload = (): WorkflowTemplateCreateRequest => {
    const { normalizedRoles, normalizedSteps, normalizedSupervisor, normalizedSupervisorDataFields } = normalizeDraftWorkflowFields()
    const payload: WorkflowTemplateCreateRequest = {
      template_title: templateTitle.trim(),
      template_description: templateDescription.trim(),
      recurrence,
      roles: normalizedRoles,
      steps: normalizedSteps,
    }
    const startTime = getTimePartFromDatetimeLocal(startAt)
    if (startTime) {
      payload.start_at = startTime
    }
    if (normalizedSupervisor) {
      payload.supervisor_user_id = normalizedSupervisor.user_id
      payload.supervisor_bounty = normalizedSupervisor.bounty
      if (normalizedSupervisorDataFields.length > 0) {
        payload.supervisor_data_fields = normalizedSupervisorDataFields
      }
    }

    return payload
  }

  const buildWorkflowSubmissionPreview = (
    payload: WorkflowCreateRequest,
    normalizedSupervisorDataFields: WorkflowSupervisorDataField[],
  ): Workflow => {
    const nowUnix = Math.floor(Date.now() / 1000)
    const workflowStartUnix = Math.floor(new Date(payload.start_at).getTime() / 1000)
    const recurrenceEndUnix =
      payload.recurrence_end_at && payload.recurrence_end_at.trim()
        ? Math.floor(new Date(payload.recurrence_end_at).getTime() / 1000)
        : null

    const rolesByClientId = new Map<string, string>()
    const previewRoles = payload.roles.map((role, roleIndex) => {
      const roleId = `preview-role-${roleIndex + 1}`
      rolesByClientId.set(role.client_id, roleId)
      return {
        id: roleId,
        workflow_id: "preview-workflow",
        title: role.title,
        required_credentials: role.required_credentials,
      }
    })

    const previewSteps = payload.steps.map((step, stepIndex) => ({
      id: `preview-step-${stepIndex + 1}`,
      workflow_id: "preview-workflow",
      step_order: stepIndex + 1,
      title: step.title,
      description: step.description,
      bounty: step.bounty,
      allow_step_not_possible: Boolean(step.allow_step_not_possible),
      role_id: rolesByClientId.get(step.role_client_id) || null,
      assigned_improver_id: null,
      assigned_improver_name: null,
      status: "locked" as const,
      started_at: null,
      completed_at: null,
      payout_error: null,
      payout_last_try_at: null,
      retry_requested_at: null,
      retry_requested_by: null,
      submission: null,
      work_items: step.work_items.map((item, itemIndex) => ({
        id: `preview-step-${stepIndex + 1}-item-${itemIndex + 1}`,
        step_id: `preview-step-${stepIndex + 1}`,
        item_order: itemIndex + 1,
        title: item.title,
        description: item.description,
        optional: item.optional,
        requires_photo: item.requires_photo,
        camera_capture_only: Boolean(item.camera_capture_only),
        photo_required_count: Math.max(1, item.photo_required_count || 1),
        photo_allow_any_count: Boolean(item.photo_allow_any_count),
        photo_aspect_ratio: normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio || "square"),
        requires_written_response: item.requires_written_response,
        requires_dropdown: item.requires_dropdown,
        dropdown_options: item.dropdown_options.map((option, optionIndex) => ({
          value: slugifyWorkflowPreviewOptionValue(option.label, optionIndex),
          label: option.label,
          requires_written_response: Boolean(option.requires_written_response),
          requires_photo_attachment: Boolean(option.requires_photo_attachment),
          camera_capture_only: Boolean(option.requires_photo_attachment) && Boolean(option.camera_capture_only),
          photo_instructions: option.photo_instructions || "",
          notify_emails: option.notify_emails || [],
          notify_email_count: option.notify_emails?.length || 0,
          send_pictures_with_email: Boolean(option.send_pictures_with_email),
        })),
        dropdown_requires_written_response: Object.fromEntries(
          item.dropdown_options.map((option, optionIndex) => [
            slugifyWorkflowPreviewOptionValue(option.label, optionIndex),
            Boolean(option.requires_written_response),
          ]),
        ),
      })),
    }))

    const supervisorId = payload.supervisor?.user_id?.trim() || null
    const selectedSupervisor = supervisorId
      ? supervisors.find((candidate) => candidate.user_id === supervisorId) || null
      : null

    return {
      id: "preview-workflow",
      series_id: "preview-series",
      workflow_state_id: null,
      proposer_id: user?.id || "preview-proposer",
      title: payload.title,
      description: payload.description,
      recurrence: payload.recurrence as WorkflowRecurrence,
      recurrence_end_at: recurrenceEndUnix,
      start_at: workflowStartUnix,
      status: "pending",
      is_start_blocked: false,
      blocked_by_workflow_id: null,
      total_bounty: previewSteps.reduce((sum, step) => sum + step.bounty, 0) + (payload.supervisor?.bounty || 0),
      weekly_bounty_requirement: 0,
      budget_weekly_deducted: 0,
      budget_one_time_deducted: 0,
      vote_quorum_reached_at: null,
      vote_finalize_at: null,
      vote_finalized_at: null,
      vote_finalized_by_user_id: null,
      vote_decision: null,
      supervisor_required: Boolean(payload.supervisor),
      supervisor_user_id: supervisorId,
      supervisor_bounty: payload.supervisor?.bounty || 0,
      supervisor_data_fields: normalizedSupervisorDataFields,
      supervisor_paid_out_at: null,
      supervisor_payout_error: null,
      supervisor_payout_last_try_at: null,
      supervisor_retry_requested_at: null,
      supervisor_retry_requested_by: null,
      supervisor_title: selectedSupervisor?.nickname || null,
      supervisor_organization: selectedSupervisor?.organization || null,
      created_at: nowUnix,
      updated_at: nowUnix,
      roles: previewRoles,
      steps: previewSteps,
      votes: {
        approve: 0,
        deny: 0,
        votes_cast: 0,
        total_voters: 0,
        quorum_reached: false,
        quorum_threshold: 0,
        quorum_reached_at: null,
        finalize_at: null,
        finalized_at: null,
        decision: null,
        my_decision: null,
      },
    }
  }

  const saveTemplate = async (asDefault: boolean) => {
    setError("")
    setSuccessMessage("")
    const templateTitleValue = templateTitle.trim()
    const templateDescriptionValue = templateDescription.trim()
    if (!templateTitleValue) {
      setError("Template title is required.")
      return
    }

    let payload: WorkflowTemplateCreateRequest
    try {
      payload = buildTemplatePayload()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save template.")
      return
    }

    payload.template_title = templateTitleValue
    payload.template_description = templateDescriptionValue

    setTemplateSaving(true)
    try {
      const res = await authFetch(asDefault ? "/admin/workflow-templates/default" : "/proposers/workflow-templates", {
        method: "POST",
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to save workflow template.")
      }
      const created = (await res.json()) as WorkflowTemplate
      setTemplates((prev) => [created, ...prev.filter((template) => template.id !== created.id)])
      setSelectedTemplateId(created.id)
      setTemplateTitle("")
      setTemplateDescription("")
      setSuccessMessage(asDefault ? "Default template saved successfully." : "Template saved successfully.")
      toast({
        title: asDefault ? "Default template saved" : "Template saved",
        description: created.template_title,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save workflow template.")
    } finally {
      setTemplateSaving(false)
    }
  }

  const applyTemplate = (templateId: string) => {
    const template = templates.find((value) => value.id === templateId)
    if (!template) return
    const preserveEditMode = isEditProposalMode

    const roleIdMap = new Map<string, string>()
    const mappedRoles: DraftRole[] = template.roles.map((role, index) => {
      const fallbackClientId = `role-${index + 1}`
      const sourceClientId = role.client_id || fallbackClientId
      const newClientId = crypto.randomUUID()
      roleIdMap.set(sourceClientId, newClientId)
      return {
        client_id: newClientId,
        title: role.title,
        required_credentials: role.required_credentials,
      }
    })

    const mappedSteps: DraftStep[] = template.steps.map((step) => ({
      id: crypto.randomUUID(),
      title: step.title,
      description: step.description,
      bounty: String(step.bounty),
      role_client_id: roleIdMap.get(step.role_client_id) || "",
      allow_step_not_possible: Boolean(step.allow_step_not_possible),
      work_items: step.work_items.map((item) => ({
        id: crypto.randomUUID(),
        title: item.title,
        description: item.description,
        optional: item.optional,
        requires_photo: item.requires_photo,
        camera_capture_only: Boolean(item.camera_capture_only),
        photo_required_count:
          typeof item.photo_required_count === "number" && Number.isFinite(item.photo_required_count)
            ? Math.max(1, Math.floor(item.photo_required_count))
            : 1,
        photo_allow_any_count: Boolean(item.photo_allow_any_count),
        photo_aspect_ratio: normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio || "square"),
        requires_written_response: item.requires_written_response,
        requires_dropdown: item.requires_dropdown,
        dropdown_options: item.dropdown_options.map((option) => ({
          label: option.label,
          requires_written_response: option.requires_written_response,
          requires_photo_attachment: Boolean(option.requires_photo_attachment),
          camera_capture_only: Boolean(option.requires_photo_attachment) && Boolean(option.camera_capture_only),
          photo_instructions: option.photo_instructions || "",
          notify_emails: option.notify_emails || [],
          notify_email_input: "",
          send_pictures_with_email: Boolean(option.send_pictures_with_email),
        })),
      })),
    }))

    const templateSupervisorUserId = (template.supervisor_user_id || "").trim()
    const templateSupervisorDataFields = (template.supervisor_data_fields || [])
      .map((field) => ({
        id: crypto.randomUUID(),
        key: (field.key || "").trim(),
        value: (field.value || "").trim(),
      }))
      .filter((field) => field.key || field.value)
    const templateHasSupervisor =
      templateSupervisorUserId.length > 0 ||
      (template.supervisor_bounty !== undefined && template.supervisor_bounty !== null) ||
      templateSupervisorDataFields.length > 0
    setWorkflowSupervisor({
      enabled: templateHasSupervisor,
      user_id: templateSupervisorUserId,
      bounty:
        template.supervisor_bounty !== undefined && template.supervisor_bounty !== null
          ? String(template.supervisor_bounty)
          : "",
    })
    setWorkflowSupervisorDataFields(templateSupervisorDataFields)
    const nextStartAtValue = applyTemplateStartTimeToDatetimeLocal(startAt, template.start_at)
    if (!preserveEditMode) {
      setRecurrence(template.recurrence)
      setStartAt(nextStartAtValue)
      setHasRecurrenceEndDate(false)
      setRecurrenceEndAt("")
      setEditProposalWorkflowId("")
      setEditProposalReason("")
    } else if (nextStartAtValue !== startAt) {
      setStartAt(nextStartAtValue)
    }
    setRoles(mappedRoles.length ? mappedRoles : [createDraftRole()])
    setSteps(mappedSteps.length ? mappedSteps : [createDraftStep()])
    setRoleCardOpenState({})
    setStepCardOpenState({})
    setWorkItemCardOpenState({})
    setWorkflowRolesOpen(false)
    setError("")
    setSuccessMessage(
      preserveEditMode
        ? `Applied template to workflow edit: ${template.template_title}`
        : `Applied template: ${template.template_title}`,
    )
  }

  const beginWorkflowEditProposal = async (workflow: Workflow) => {
    if (!workflow) return
    if (!canProposeWorkflowEditFromWorkflow(workflow)) {
      setError("This workflow has ended and can no longer be edited.")
      setSuccessMessage("")
      return
    }

    let sourceWorkflow = workflow
    setError("")
    setDetailLoading(true)
    try {
      const res = await authFetch(`/proposers/workflows/${workflow.id}?include_notify_emails=true`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load workflow details for editing.")
      }
      sourceWorkflow = (await res.json()) as Workflow
      setDetailWorkflow(sourceWorkflow)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load workflow details for editing.")
      setSuccessMessage("")
      return
    } finally {
      setDetailLoading(false)
    }

    const roleIdMap = new Map<string, string>()
    const mappedRoles: DraftRole[] = sourceWorkflow.roles.map((role, index) => {
      const fallbackClientId = `role-${index + 1}`
      const sourceClientId = (role.id || "").trim() || fallbackClientId
      const newClientId = crypto.randomUUID()
      roleIdMap.set(sourceClientId, newClientId)
      return {
        client_id: newClientId,
        title: role.title,
        required_credentials: role.required_credentials || [],
      }
    })

    const mappedSteps: DraftStep[] = [...sourceWorkflow.steps]
      .sort((a, b) => a.step_order - b.step_order)
      .map((step) => ({
        id: crypto.randomUUID(),
        title: step.title,
        description: step.description,
        bounty: String(step.bounty),
        role_client_id: step.role_id ? roleIdMap.get(step.role_id) || "" : "",
        allow_step_not_possible: Boolean(step.allow_step_not_possible),
        work_items: [...step.work_items]
          .sort((a, b) => a.item_order - b.item_order)
          .map((item) => ({
            id: crypto.randomUUID(),
            title: item.title,
            description: item.description,
            optional: item.optional,
            requires_photo: item.requires_photo,
            camera_capture_only: Boolean(item.camera_capture_only),
            photo_required_count:
              typeof item.photo_required_count === "number" && Number.isFinite(item.photo_required_count)
                ? Math.max(1, Math.floor(item.photo_required_count))
                : 1,
            photo_allow_any_count: Boolean(item.photo_allow_any_count),
            photo_aspect_ratio: normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio || "square"),
            requires_written_response: item.requires_written_response,
            requires_dropdown: item.requires_dropdown,
            dropdown_options: (item.dropdown_options || []).map((option) => ({
              label: option.label,
              requires_written_response: option.requires_written_response,
              requires_photo_attachment: Boolean(option.requires_photo_attachment),
              camera_capture_only: Boolean(option.requires_photo_attachment) && Boolean(option.camera_capture_only),
              photo_instructions: option.photo_instructions || "",
              notify_emails: option.notify_emails || [],
              notify_email_input: "",
              send_pictures_with_email: Boolean(option.send_pictures_with_email),
            })),
          })),
      }))

    const supervisorUserId = (sourceWorkflow.supervisor_user_id || "").trim()
    const supervisorDataFields = (sourceWorkflow.supervisor_data_fields || [])
      .map((field) => ({
        id: crypto.randomUUID(),
        key: (field.key || "").trim(),
        value: (field.value || "").trim(),
      }))
      .filter((field) => field.key || field.value)
    const hasSupervisor =
      supervisorUserId.length > 0 ||
      (sourceWorkflow.supervisor_bounty !== undefined && sourceWorkflow.supervisor_bounty !== null) ||
      supervisorDataFields.length > 0

    const recurrenceEndAtValue = toDatetimeLocalValueFromUnix(sourceWorkflow.recurrence_end_at)

    setTitle(sourceWorkflow.title || "")
    setDescription(sourceWorkflow.description || "")
    setRecurrence(sourceWorkflow.recurrence)
    setStartAt(toDatetimeLocalValueFromUnix(sourceWorkflow.start_at))
    setHasRecurrenceEndDate(recurrenceEndAtValue.length > 0)
    setRecurrenceEndAt(recurrenceEndAtValue)
    setWorkflowSupervisor({
      enabled: hasSupervisor,
      user_id: supervisorUserId,
      bounty:
        sourceWorkflow.supervisor_bounty !== undefined && sourceWorkflow.supervisor_bounty !== null
          ? String(sourceWorkflow.supervisor_bounty)
          : "",
    })
    setWorkflowSupervisorDataFields(supervisorDataFields)
    setRoles(mappedRoles.length ? mappedRoles : [createDraftRole()])
    setSteps(mappedSteps.length ? mappedSteps : [createDraftStep()])
    setRoleCardOpenState({})
    setStepCardOpenState({})
    setWorkItemCardOpenState({})
    setWorkflowRolesOpen(false)
    setEditProposalWorkflowId(sourceWorkflow.id)
    setEditProposalReason("")
    setSelectedTemplateId("")
    setError("")
    setSuccessMessage(`Editing workflow: ${sourceWorkflow.title}`)
    setDetailOpen(false)
    setDetailWorkflow(null)
    setActiveTab("create-workflow")
  }

  const cancelWorkflowEditProposalDraft = () => {
    resetForm()
    setError("")
    setSuccessMessage("Exited workflow edit mode.")
  }

  const showWorkflowSubmitSuccessInline =
    successMessage === "Workflow proposal created successfully." ||
    successMessage === "Workflow edit proposal submitted successfully." ||
    successMessage === "Workflow edit was auto-approved and applied."

  const confirmWorkflowSubmission = async () => {
    if (!pendingWorkflowSubmission) {
      setError("Unable to submit workflow right now.")
      return
    }

    setSubmitting(true)
    setError("")
    setSuccessMessage("")
    setSubmissionPreviewError("")
    try {
      const res = await authFetch("/proposers/workflows", {
        method: "POST",
        body: JSON.stringify(pendingWorkflowSubmission.payload),
      })

      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to create workflow right now.")
      }

      const created = (await res.json()) as Workflow
      workflowListLoadedRef.current = true
      setWorkflowListLoaded(true)
      setWorkflows((prev) => [created, ...prev])
      setSubmissionPreviewOpen(false)
      setPendingWorkflowSubmission(null)
      setSubmissionPreviewError("")
      resetForm()
      setSuccessMessage("Workflow proposal created successfully.")
      toast({
        title: "Workflow proposal created",
        description: created.title,
      })
      await loadWorkflowListData("background")
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unable to create workflow right now."
      setSubmissionPreviewError(message)
      setError(message)
    } finally {
      setSubmitting(false)
    }
  }

  const submitWorkflow = async () => {
    setError("")
    setSuccessMessage("")

    if (!title.trim()) {
      setError("Workflow title is required.")
      return
    }

    let normalizedRoles: WorkflowCreateRequest["roles"] = []
    let normalizedSteps: WorkflowCreateRequest["steps"] = []
    let normalizedSupervisor: WorkflowCreateRequest["supervisor"] | undefined
    let normalizedSupervisorDataFields: WorkflowSupervisorDataField[] = []
    try {
      const normalized = normalizeDraftWorkflowFields()
      normalizedRoles = normalized.normalizedRoles
      normalizedSteps = normalized.normalizedSteps
      normalizedSupervisor = normalized.normalizedSupervisor
      normalizedSupervisorDataFields = normalized.normalizedSupervisorDataFields
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to validate workflow.")
      return
    }

    let recurrenceEndAtISO: string | undefined
    if (recurrence !== "one_time" && hasRecurrenceEndDate) {
      if (!recurrenceEndAt.trim()) {
        setError("Workflow recurrence end date/time is required when enabled.")
        return
      }
      try {
        recurrenceEndAtISO = toUTCISOStringFromDatetimeLocal(recurrenceEndAt)
      } catch (err) {
        setError(err instanceof Error ? err.message : "Workflow recurrence end date/time is invalid.")
        return
      }
    }
    if (recurrenceEndAtISO) {
      try {
        const startAtISOForValidation = toUTCISOStringFromDatetimeLocal(startAt)
        const startUnix = Math.floor(new Date(startAtISOForValidation).getTime() / 1000)
        const endUnix = Math.floor(new Date(recurrenceEndAtISO).getTime() / 1000)
        if (endUnix < startUnix) {
          setError("Workflow recurrence end date/time must be on or after start date/time.")
          return
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "Workflow start date/time is invalid.")
        return
      }
    }

    setSubmitting(true)
    try {
      if (editProposalWorkflowId) {
        const payload: WorkflowEditProposalCreateRequest = {
          title: title.trim(),
          description: description.trim(),
          start_at: toUTCISOStringFromDatetimeLocal(startAt),
          timezone_offset_minutes: new Date().getTimezoneOffset(),
          roles: normalizedRoles,
          steps: normalizedSteps,
        }
        if (recurrence !== "one_time") {
          payload.recurrence_end_at = recurrenceEndAtISO || ""
        }
        if (normalizedSupervisor) {
          payload.supervisor = normalizedSupervisor
          if (normalizedSupervisorDataFields.length > 0) {
            payload.supervisor_data_fields = normalizedSupervisorDataFields
          }
        }
        if (editProposalReason.trim()) {
          payload.reason = editProposalReason.trim()
        }

        const res = await authFetch(`/proposers/workflows/${editProposalWorkflowId}/edit-proposals`, {
          method: "POST",
          body: JSON.stringify(payload),
        })

        if (!res.ok) {
          const text = await res.text()
          throw new Error(text || "Unable to submit workflow edit proposal right now.")
        }

        const created = (await res.json()) as WorkflowEditProposal
        resetForm()
        setSuccessMessage(created.status === "approved" ? "Workflow edit was auto-approved and applied." : "Workflow edit proposal submitted successfully.")
        toast({
          title: created.status === "approved" ? "Workflow edit applied" : "Workflow edit proposal submitted",
          description: created.workflow_title,
        })
        await loadWorkflowListData("background")
        return
      }

      let startAtISO = ""
      try {
        startAtISO = toUTCISOStringFromDatetimeLocal(startAt)
      } catch (err) {
        setError(err instanceof Error ? err.message : "Workflow start date/time is invalid.")
        return
      }
      const payload: WorkflowCreateRequest = {
        title: title.trim(),
        description: description.trim(),
        recurrence,
        start_at: startAtISO,
        roles: normalizedRoles,
        steps: normalizedSteps,
      }
      if (recurrenceEndAtISO) {
        payload.recurrence_end_at = recurrenceEndAtISO
      }
      if (normalizedSupervisor) {
        payload.supervisor = normalizedSupervisor
        if (normalizedSupervisorDataFields.length > 0) {
          payload.supervisor_data_fields = normalizedSupervisorDataFields
        }
      }
      const previewWorkflow = buildWorkflowSubmissionPreview(payload, normalizedSupervisorDataFields)
      setSubmissionPreviewError("")
      setPendingWorkflowSubmission({
        payload,
        workflow: previewWorkflow,
      })
      setSubmissionPreviewOpen(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to create workflow right now.")
    } finally {
      setSubmitting(false)
    }
  }

  const deleteWorkflow = async (workflowId: string) => {
    setError("")
    try {
      const res = await authFetch(`/proposers/workflows/${workflowId}`, {
        method: "DELETE",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to archive workflow.")
      }
      setWorkflows((prev) => prev.filter((workflow) => workflow.id !== workflowId))
      workflowListLoadedRef.current = true
      setWorkflowListLoaded(true)
      await loadWorkflowListData("background")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to archive workflow.")
    }
  }

  const proposeDeletion = async (workflowId: string, targetType: WorkflowDeletionTargetType) => {
    setError("")
    setDeletionSubmitting(`${workflowId}:${targetType}`)
    try {
      const res = await authFetch("/proposers/workflow-deletion-proposals", {
        method: "POST",
        body: JSON.stringify({
          workflow_id: workflowId,
          target_type: targetType,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to create deletion proposal.")
      }
      workflowListLoadedRef.current = true
      setWorkflowListLoaded(true)
      await loadWorkflowListData("background")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to create deletion proposal.")
    } finally {
      setDeletionSubmitting("")
    }
  }

  const handleConfirmDelete = async () => {
    if (!deleteTemplateId) return
    setDeleteTemplateLoading(true)
    setSuccessMessage("")
    try {
      const res = await authFetch(`/proposers/workflow-templates/${deleteTemplateId}`, { method: "DELETE" })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to delete template.")
      }
      setTemplates((prev) => prev.filter((t) => t.id !== deleteTemplateId))
      if (selectedTemplateId === deleteTemplateId) setSelectedTemplateId("")
      setDeleteTemplateId("")
      setDeleteConfirmOpen(false)
      setSuccessMessage("Template deleted successfully.")
      toast({
        title: "Template deleted",
        description: "The template was deleted successfully.",
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to delete template.")
    } finally {
      setDeleteTemplateLoading(false)
    }
  }

  const isSeriesWorkflow = (workflow: Workflow) =>
    workflow.recurrence !== "one_time" ||
    workflows.some((candidate) => candidate.id !== workflow.id && candidate.series_id === workflow.series_id)
  const isWorkflowOwner = (workflow: Workflow) => workflow.proposer_id === user?.id
  const canProposeDeletionForStatus = (workflow: Workflow) =>
    workflow.status === "approved" ||
    workflow.status === "blocked" ||
    workflow.status === "in_progress" ||
    workflow.status === "completed"
  const getPendingDeletionProposal = (workflow: Workflow) =>
    deletionProposals.find(
      (proposal) =>
        proposal.status === "pending" &&
        ((proposal.target_type === "series" && proposal.target_series_id === workflow.series_id) ||
          (proposal.target_type === "workflow" && proposal.target_workflow_id === workflow.id)),
    ) ?? null

  const canSaveTemplateFromWorkflow = (workflow: Workflow) =>
    (isAdminUser || isWorkflowOwner(workflow)) &&
    (workflow.status === "approved" ||
      workflow.status === "blocked" ||
      workflow.status === "in_progress" ||
      workflow.status === "completed" ||
      workflow.status === "paid_out")

  const hasWorkflowEndedForEdit = (workflow: Workflow) => {
    const nowUnix = Math.floor(Date.now() / 1000)
    if (workflow.recurrence !== "one_time") {
      return typeof workflow.recurrence_end_at === "number" && workflow.recurrence_end_at > 0 && workflow.recurrence_end_at < nowUnix
    }
    return (
      workflow.status === "completed" ||
      workflow.status === "paid_out" ||
      workflow.status === "failed" ||
      workflow.status === "skipped"
    )
  }

  const canProposeWorkflowEditFromWorkflow = (workflow: Workflow) =>
    (isAdminUser || isWorkflowOwner(workflow)) &&
    !hasWorkflowEndedForEdit(workflow) &&
    (workflow.status === "approved" ||
      workflow.status === "blocked" ||
      workflow.status === "in_progress" ||
      workflow.status === "completed" ||
      workflow.status === "paid_out" ||
      workflow.status === "failed" ||
      workflow.status === "skipped")

  const openWorkflowDetails = async (workflowId: string, fallback?: Workflow) => {
    setDetailOpen(true)
    setDetailLoading(true)
    if (fallback) {
      setDetailWorkflow(fallback)
    }
    try {
      const res = await authFetch(`/proposers/workflows/${workflowId}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load workflow details.")
      }
      const workflow = (await res.json()) as Workflow
      setDetailWorkflow(workflow)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load workflow details.")
      setDetailOpen(false)
      setDetailWorkflow(null)
    } finally {
      setDetailLoading(false)
    }
  }

  const openSaveFromWorkflowModal = (workflow: Workflow) => {
    if (!canSaveTemplateFromWorkflow(workflow)) {
      setError("Only approved or active workflows can be saved as templates.")
      return
    }
    setError("")
    setSaveFromWorkflowError("")
    setSaveFromWorkflowTitle(`${workflow.title} Template`)
    setSaveFromWorkflowDescription(workflow.description || "")
    setSaveFromWorkflowOpen(true)
  }

  const buildTemplatePayloadFromWorkflow = (
    workflow: Workflow,
    templateTitleValue: string,
    templateDescriptionValue: string,
  ): WorkflowTemplateCreateRequest => {
    const roleClientIdById = new Map<string, string>()
    const roles = workflow.roles.map((role, index) => {
      const clientId = `role-${index + 1}`
      roleClientIdById.set(role.id, clientId)
      return {
        client_id: clientId,
        title: role.title,
        required_credentials: role.required_credentials,
      }
    })

    const steps = workflow.steps
      .slice()
      .sort((a, b) => a.step_order - b.step_order)
      .map((step) => {
        const roleId = step.role_id || ""
        const roleClientId = roleClientIdById.get(roleId) || ""
        if (!roleClientId) {
          throw new Error(`Workflow step "${step.title}" is missing a valid role assignment.`)
        }

        return {
          title: step.title,
          description: step.description,
          bounty: step.bounty,
          role_client_id: roleClientId,
          allow_step_not_possible: Boolean(step.allow_step_not_possible),
          work_items: step.work_items
            .slice()
            .sort((a, b) => a.item_order - b.item_order)
            .map((item) => ({
              title: item.title,
              description: item.description,
              optional: item.optional,
              requires_photo: item.requires_photo,
              camera_capture_only: Boolean(item.camera_capture_only),
              photo_required_count: Math.max(1, item.photo_required_count || 1),
              photo_allow_any_count: Boolean(item.photo_allow_any_count),
              photo_aspect_ratio: normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio || "square"),
              requires_written_response: item.requires_written_response,
              requires_dropdown: item.requires_dropdown,
              dropdown_options: item.dropdown_options.map((option) => ({
                label: option.label,
                requires_written_response: Boolean(option.requires_written_response),
                requires_photo_attachment: Boolean(option.requires_photo_attachment),
                camera_capture_only: Boolean(option.requires_photo_attachment) && Boolean(option.camera_capture_only),
                photo_instructions: option.photo_instructions || "",
                notify_emails: option.notify_emails || [],
                send_pictures_with_email: Boolean(option.send_pictures_with_email),
              })),
            })),
        }
      })

    const payload: WorkflowTemplateCreateRequest = {
      template_title: templateTitleValue,
      template_description: templateDescriptionValue,
      recurrence: workflow.recurrence,
      roles,
      steps,
    }
    const workflowStartTime = getTimePartFromDatetimeLocal(toDatetimeLocalValueFromUnix(workflow.start_at))
    if (workflowStartTime) {
      payload.start_at = workflowStartTime
    }

    const supervisorUserId = (workflow.supervisor_user_id || "").trim()
    const supervisorDataFields = (workflow.supervisor_data_fields || [])
      .map((field) => ({
        key: (field.key || "").trim(),
        value: (field.value || "").trim(),
      }))
      .filter((field) => field.key || field.value)

    if (supervisorUserId) {
      payload.supervisor_user_id = supervisorUserId
      payload.supervisor_bounty = workflow.supervisor_bounty
    }
    if (supervisorDataFields.length > 0) {
      payload.supervisor_data_fields = supervisorDataFields
    }

    return payload
  }

  const saveCurrentWorkflowAsTemplate = async () => {
    setSaveFromWorkflowError("")
    if (!detailWorkflow) {
      setSaveFromWorkflowError("No workflow selected.")
      return
    }
    if (!canSaveTemplateFromWorkflow(detailWorkflow)) {
      setSaveFromWorkflowError("Only approved or active workflows can be saved as templates.")
      return
    }

    const templateTitleValue = saveFromWorkflowTitle.trim()
    const templateDescriptionValue = saveFromWorkflowDescription.trim()
    if (!templateTitleValue) {
      setSaveFromWorkflowError("Template title is required.")
      return
    }

    setSaveFromWorkflowSubmitting(true)

    // Admins viewing another user's workflow get notify_emails redacted by the
    // backend (sanitizeWorkflowForUserWithOptions). Refetch the workflow with
    // include_notify_emails=true so the resulting template carries the emails.
    let sourceWorkflow = detailWorkflow
    try {
      const refetchRes = await authFetch(
        `/proposers/workflows/${detailWorkflow.id}?include_notify_emails=true`,
      )
      if (refetchRes.ok) {
        sourceWorkflow = (await refetchRes.json()) as Workflow
      }
    } catch {
      // Fall back to detailWorkflow on refetch failure; emails may be missing
      // for non-owned workflows but the rest of the template will still save.
    }

    let payload: WorkflowTemplateCreateRequest
    try {
      payload = buildTemplatePayloadFromWorkflow(sourceWorkflow, templateTitleValue, templateDescriptionValue)
    } catch (err) {
      setSaveFromWorkflowError(err instanceof Error ? err.message : "Unable to build template from workflow.")
      setSaveFromWorkflowSubmitting(false)
      return
    }

    try {
      const res = await authFetch("/proposers/workflow-templates", {
        method: "POST",
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to save template from workflow.")
      }
      const created = (await res.json()) as WorkflowTemplate
      setTemplates((prev) => [created, ...prev.filter((template) => template.id !== created.id)])
      setSelectedTemplateId(created.id)
      setSaveFromWorkflowOpen(false)
      setSaveFromWorkflowError("")
      setSaveFromWorkflowTitle("")
      setSaveFromWorkflowDescription("")
      setSuccessMessage("Template saved from workflow successfully.")
      toast({
        title: "Template saved",
        description: created.template_title,
      })
    } catch (err) {
      setSaveFromWorkflowError(err instanceof Error ? err.message : "Unable to save template from workflow.")
    } finally {
      setSaveFromWorkflowSubmitting(false)
    }
  }

  if (status === "loading") {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]" />
      </div>
    )
  }

  if (!isApproved) {
    return (
      <div className="container mx-auto p-4 space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Proposer Access Required</CardTitle>
            <CardDescription>
              Your account is not approved for proposer workflows yet. Request proposer access in settings.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Proposer Panel</h1>
        <p className="text-muted-foreground">Design workflow proposals with role gates, sequential steps, and step bounties.</p>
      </div>

      {error && (
        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {successMessage && (
        <div className="flex items-center gap-2 text-green-700 text-sm p-3 bg-green-50 dark:bg-green-900/20 rounded-lg">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          <span>{successMessage}</span>
        </div>
      )}

      <Tabs value={activeTab} onValueChange={(value) => setActiveTab(value as "create-workflow" | "your-workflows")} className="w-full">
        <TabsList className="w-full h-auto grid grid-cols-1 gap-1 p-1 sm:grid-cols-2">
          <TabsTrigger value="create-workflow">Create Workflow</TabsTrigger>
          <TabsTrigger value="your-workflows">Your Workflows</TabsTrigger>
        </TabsList>

        <TabsContent value="create-workflow" className="mt-4 space-y-6">
          {!createDataLoaded ? (
            <Card>
              <CardContent className="flex min-h-[320px] items-center justify-center">
                <div className="flex items-center gap-3 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  <span>Loading workflow form data...</span>
                </div>
              </CardContent>
            </Card>
          ) : (
          <Card>
            <CardHeader>
              <CardTitle>{isEditProposalMode ? "Edit Workflow Proposal" : "Create Workflow Proposal"}</CardTitle>
              <CardDescription>
                {isEditProposalMode
                  ? "Propose a new workflow state for this recurring series. Existing workflows keep their current state snapshots."
                  : "Steps unlock sequentially. Each step has one assignee role and configurable work-item evidence requirements."}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {createDataLoading && (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  <span>Refreshing workflow form data...</span>
                </div>
              )}

              <div className="flex flex-wrap items-center gap-3">
                <Badge variant="outline">Draft Total Bounty: {totalDraftBounty} SFLuv</Badge>
                <Button onClick={submitWorkflow} disabled={submitting}>
                  {submitting ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Submitting...
                    </>
                  ) : (
                    isEditProposalMode ? "Submit Workflow Edit Proposal" : "Submit Workflow Proposal"
                  )}
                </Button>
                {isEditProposalMode && (
                  <Button type="button" variant="outline" onClick={cancelWorkflowEditProposalDraft} disabled={submitting}>
                    Cancel Edit Mode
                  </Button>
                )}
              </div>

              {isEditProposalMode && (
                <div className="rounded-xl border border-border/70 bg-muted/40 px-4 py-3 text-sm">
                  <span className="font-medium text-foreground">Edit mode.</span>{" "}
                  <span className="text-muted-foreground">Approved changes apply to future workflows only.</span>
                </div>
              )}

	              <Card>
	                <Collapsible open={templateLibraryOpen} onOpenChange={setTemplateLibraryOpen}>
	                  <CardHeader
	                    className="cursor-pointer pb-3"
	                    onClick={(event) => {
	                      if (didClickInteractiveElement(event)) return
	                      setTemplateLibraryOpen((prev) => !prev)
	                    }}
	                  >
	                    <div className="flex items-center justify-between gap-3">
	                      <div>
	                        <CardTitle className="text-base">Template Library</CardTitle>
	                        <CardDescription>
	                          Apply a saved template to prefill workflow fields. Workflow title and description stay manual.
	                        </CardDescription>
	                      </div>
	                      <div className="flex items-center gap-2 shrink-0">
	                        <Badge variant="outline" className="shrink-0">
	                          {templates.length} templates
	                        </Badge>
	                        <CollapsibleTrigger asChild>
	                          <Button type="button" variant="ghost" size="icon" aria-label={templateLibraryOpen ? "Collapse template library" : "Expand template library"}>
	                            <ChevronDown className={cn("h-4 w-4 transition-transform", !templateLibraryOpen && "-rotate-90")} />
	                          </Button>
	                        </CollapsibleTrigger>
	                      </div>
	                    </div>
	                  </CardHeader>
	                  <CollapsibleContent>
	                    <CardContent className="space-y-4">
	                      <div className="grid gap-3">
	                        <Popover open={templateComboOpen} onOpenChange={setTemplateComboOpen}>
	                          <PopoverTrigger asChild>
	                            <Button variant="outline" role="combobox" className="w-full justify-between font-normal">
	                              <span className="truncate">
	                                {selectedTemplate ? `${selectedTemplate.template_title} (${selectedTemplate.is_default ? "Default" : "Personal"})` : "Select a template"}
	                              </span>
	                              <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
	                            </Button>
	                          </PopoverTrigger>
	                          <PopoverContent className="w-[--radix-popover-trigger-width] p-2" align="start">
	                            <div className="relative mb-2">
	                              <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
	                              <Input
	                                placeholder="Search templates..."
	                                value={templateSearch}
	                                onChange={(e) => setTemplateSearch(e.target.value)}
	                                className="pl-8 h-8 text-sm"
	                              />
	                            </div>
	                            <div className="max-h-60 overflow-y-auto space-y-0.5">
	                              {filteredTemplates.length === 0 ? (
	                                <p className="text-sm text-muted-foreground px-2 py-1.5">No templates found.</p>
	                              ) : (
	                                filteredTemplates.map((template) => (
	                                  <div
	                                    key={template.id}
	                                    className={cn(
	                                      "flex items-center justify-between gap-2 rounded px-2 py-1.5 text-sm group",
	                                      selectedTemplateId === template.id ? "bg-accent font-medium" : "hover:bg-accent"
	                                    )}
	                                  >
	                                    <button
	                                      type="button"
	                                      className="flex-1 text-left min-w-0"
	                                      onClick={() => {
	                                        setSelectedTemplateId(template.id)
	                                        applyTemplate(template.id)
	                                        setTemplateComboOpen(false)
	                                        setTemplateSearch("")
	                                      }}
	                                    >
	                                      <span className="block truncate">{template.template_title}</span>
	                                      <span className="text-xs text-muted-foreground">{template.is_default ? "Default" : "Personal"}</span>
	                                    </button>
	                                    {(!template.is_default || user?.isAdmin) && (
	                                      <button
	                                        type="button"
	                                        className="shrink-0 text-muted-foreground hover:text-destructive transition-colors p-0.5"
	                                        title="Delete template"
	                                        onClick={(e) => {
	                                          e.stopPropagation()
	                                          setDeleteTemplateId(template.id)
	                                          setDeleteConfirmOpen(true)
	                                          setTemplateComboOpen(false)
	                                        }}
	                                      >
	                                        <Trash2 className="h-3.5 w-3.5" />
	                                      </button>
	                                    )}
	                                  </div>
	                                ))
	                              )}
	                            </div>
	                          </PopoverContent>
	                        </Popover>
	                      </div>

	                      <div className="grid gap-3 md:grid-cols-2">
	                        <div className="space-y-2">
	                          <Label>Template Title</Label>
	                          <Input
	                            value={templateTitle}
	                            onChange={(e) => setTemplateTitle(e.target.value)}
	                            placeholder="Storefront verification baseline"
	                          />
	                        </div>
	                        <div className="space-y-2">
	                          <Label>Template Description</Label>
	                          <Input
	                            value={templateDescription}
	                            onChange={(e) => setTemplateDescription(e.target.value)}
	                            placeholder="Reusable workflow shape for recurring storefront checks"
	                          />
	                        </div>
	                      </div>

	                      <div className="flex flex-wrap gap-2">
	                        <Button type="button" variant="outline" onClick={() => saveTemplate(false)} disabled={templateSaving}>
	                          {templateSaving ? "Saving..." : `Save${user?.isAdmin ? " Personal" : ""} Template`}
	                        </Button>
	                        {user?.isAdmin && (
	                          <Button type="button" onClick={() => saveTemplate(true)} disabled={templateSaving}>
	                            {templateSaving ? "Saving..." : "Save Default Template"}
	                          </Button>
	                        )}
	                      </div>
	                    </CardContent>
	                  </CollapsibleContent>
	                </Collapsible>
	              </Card>

	          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2 md:col-span-2">
              <Label>Workflow Title</Label>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Neighborhood storefront verification" />
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label>Description</Label>
              <Textarea
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Describe the outcome, stakeholders, and acceptance criteria."
              />
            </div>
            {isEditProposalMode ? (
              <div className="space-y-4 md:col-span-2">
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Recurrence</Label>
                    <Input value={formatRecurrenceLabel(recurrence)} readOnly disabled />
                  </div>
                  <div className="space-y-2">
                    <Label>Start Date</Label>
                    <Input type="date" value={getDatePartFromDatetimeLocal(startAt)} readOnly disabled />
                  </div>
                  <div className="space-y-2">
                    <Label>Start Time</Label>
                    <Input
                      type="time"
                      value={getTimePartFromDatetimeLocal(startAt)}
                      onChange={(e) => setStartAt((prev) => replaceTimePartInDatetimeLocal(prev, e.target.value))}
                    />
                    <p className="text-xs text-muted-foreground">The date stays locked. Only the start time can be edited.</p>
                  </div>
                  {recurrence !== "one_time" && (
                    <div className="space-y-2 md:col-span-2">
                      <Label>Recurrence End Date (Optional)</Label>
                      <label className="flex items-center gap-2 text-sm">
                        <Checkbox
                          checked={hasRecurrenceEndDate}
                          onCheckedChange={(checked) => {
                            const enabled = Boolean(checked)
                            setHasRecurrenceEndDate(enabled)
                            if (!enabled) {
                              setRecurrenceEndAt("")
                            }
                          }}
                        />
                        Specify an end date
                      </label>
                      {hasRecurrenceEndDate ? (
                        <Input type="datetime-local" value={recurrenceEndAt} onChange={(e) => setRecurrenceEndAt(e.target.value)} />
                      ) : (
                        <p className="text-xs text-muted-foreground">No end date specified. Recurring generation continues indefinitely.</p>
                      )}
                    </div>
                  )}
                </div>
              </div>
            ) : (
              <>
                <div className="space-y-2">
                  <Label>Recurrence</Label>
                  <Select value={recurrence} onValueChange={(value: WorkflowRecurrence) => setRecurrence(value)}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="one_time">One Time</SelectItem>
                      <SelectItem value="daily">Daily</SelectItem>
                      <SelectItem value="weekly">Weekly</SelectItem>
                      <SelectItem value="monthly">Monthly</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label>Start Date & Time</Label>
                  <Input type="datetime-local" value={startAt} onChange={(e) => setStartAt(e.target.value)} required />
                </div>

                {recurrence !== "one_time" && (
                  <div className="space-y-2 md:col-span-2">
                    <Label>Recurrence End Date (Optional)</Label>
                    <label className="flex items-center gap-2 text-sm">
                      <Checkbox
                        checked={hasRecurrenceEndDate}
                        onCheckedChange={(checked) => {
                          const enabled = Boolean(checked)
                          setHasRecurrenceEndDate(enabled)
                          if (!enabled) {
                            setRecurrenceEndAt("")
                          }
                        }}
                      />
                      Specify an end date
                    </label>
                    {!hasRecurrenceEndDate ? (
                      <p className="text-xs text-muted-foreground">No end date specified. Recurring generation will continue indefinitely.</p>
                    ) : (
                      <Input type="datetime-local" value={recurrenceEndAt} onChange={(e) => setRecurrenceEndAt(e.target.value)} />
                    )}
                  </div>
                )}
              </>
            )}

            {isEditProposalMode && (
              <div className="space-y-2 md:col-span-2">
                <Label>Edit Proposal Reason (Optional)</Label>
                <Textarea
                  value={editProposalReason}
                  onChange={(event) => setEditProposalReason(event.target.value)}
                  placeholder="Describe why this edit is needed."
                />
              </div>
            )}
	          </div>

	          <Card>
	            <Collapsible open={workflowSupervisorOpen} onOpenChange={setWorkflowSupervisorOpen}>
	              <CardHeader
	                className="cursor-pointer pb-3"
	                onClick={(event) => {
	                  if (didClickInteractiveElement(event)) return
	                  setWorkflowSupervisorOpen((prev) => !prev)
	                }}
	              >
	                <div className="flex items-center justify-between gap-3">
	                  <div>
	                    <CardTitle className="text-base">Workflow Supervisor (Optional)</CardTitle>
	                    <CardDescription>
	                      Assign an approved supervisor to this workflow and optionally reserve a supervisor completion payout.
	                    </CardDescription>
	                  </div>
	                  <div className="flex items-center gap-2">
	                    {workflowSupervisor.enabled && <Badge variant="outline">Enabled</Badge>}
	                    <CollapsibleTrigger asChild>
	                      <Button type="button" variant="ghost" size="icon" aria-label={workflowSupervisorOpen ? "Collapse workflow supervisor" : "Expand workflow supervisor"}>
	                        <ChevronDown className={cn("h-4 w-4 transition-transform", !workflowSupervisorOpen && "-rotate-90")} />
	                      </Button>
	                    </CollapsibleTrigger>
	                  </div>
	                </div>
	              </CardHeader>
	              <CollapsibleContent>
	                <CardContent className="space-y-4">
	                  <label className="flex items-center gap-2 text-sm">
	                    <Checkbox
	                      checked={workflowSupervisor.enabled}
	                      onCheckedChange={(checked) =>
	                        setWorkflowSupervisor((prev) => ({
	                          ...prev,
	                          enabled: Boolean(checked),
	                        }))
	                      }
	                    />
	                    Enable Workflow Supervisor
	                  </label>

	                  {workflowSupervisor.enabled && (
	                    <div className="space-y-4">
	                      <div className="space-y-2">
	                        <Label>Supervisor</Label>
	                        {supervisors.length === 0 ? (
	                          <p className="text-xs text-muted-foreground">
	                            No approved supervisors available yet.
	                          </p>
	                        ) : (
	                          <Select
	                            value={workflowSupervisor.user_id}
	                            onValueChange={(value) =>
	                              setWorkflowSupervisor((prev) => ({
	                                ...prev,
	                                user_id: value,
	                              }))
	                            }
	                          >
	                            <SelectTrigger>
	                              <SelectValue placeholder="Select a supervisor..." />
	                            </SelectTrigger>
	                            <SelectContent>
	                              {supervisors.map((supervisor) => (
	                                <SelectItem key={supervisor.user_id} value={supervisor.user_id}>
	                                  {supervisor.nickname || supervisor.organization}
	                                </SelectItem>
	                              ))}
	                            </SelectContent>
	                          </Select>
	                        )}
	                      </div>
	                      <div className="space-y-2">
	                        <Label>Supervisor Completion Payout (Optional)</Label>
	                        <Input
	                          type="number"
	                          min="0"
	                          value={workflowSupervisor.bounty}
                              onWheel={preventNumberInputScrollChange}
	                          onChange={(e) =>
	                            setWorkflowSupervisor((prev) => ({
	                              ...prev,
	                              bounty: e.target.value,
	                            }))
	                          }
	                          placeholder="0"
	                        />
	                      </div>

	                      <div className="space-y-3">
	                        <div className="flex items-center justify-between gap-2">
	                          <div>
	                            <Label>Supervisor Data Fields (Optional)</Label>
	                            <p className="text-xs text-muted-foreground">
	                              Add key/value metadata tags for supervisor export records.
	                            </p>
	                          </div>
	                          <Button type="button" variant="outline" size="sm" onClick={addWorkflowSupervisorDataField}>
	                            <Plus className="mr-2 h-4 w-4" />
	                            Add Field
	                          </Button>
	                        </div>

	                        {workflowSupervisorDataFields.length === 0 ? (
	                          <p className="text-xs text-muted-foreground">No supervisor data fields added.</p>
	                        ) : (
	                          <div className="space-y-2">
	                            {workflowSupervisorDataFields.map((field) => (
	                              <div key={field.id} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
	                                <Input
	                                  value={field.key}
	                                  onChange={(e) => updateWorkflowSupervisorDataField(field.id, { key: e.target.value })}
	                                  placeholder="Key (e.g. internal_reference)"
	                                />
	                                <Input
	                                  value={field.value}
	                                  onChange={(e) => updateWorkflowSupervisorDataField(field.id, { value: e.target.value })}
	                                  placeholder="Value"
	                                />
	                                <Button
	                                  type="button"
	                                  variant="ghost"
	                                  size="icon"
	                                  onClick={() => removeWorkflowSupervisorDataField(field.id)}
	                                  aria-label="Remove supervisor data field"
	                                >
	                                  <Trash2 className="h-4 w-4" />
	                                </Button>
	                              </div>
	                            ))}
	                          </div>
	                        )}
	                      </div>
	                    </div>
	                  )}
	                </CardContent>
	              </CollapsibleContent>
	            </Collapsible>
	          </Card>

          <Card>
            <Collapsible
              open={workflowRolesOpen}
              onOpenChange={(open) => {
                setWorkflowRolesOpen(open)
                if (!open) return
                setRoleCardOpenState((prev) => {
                  const next = { ...prev }
                  roles.forEach((role) => {
                    next[role.client_id] = true
                  })
                  return next
                })
              }}
            >
              <CardHeader
                className="cursor-pointer pb-3"
                onClick={(event) => {
                  if (didClickInteractiveElement(event)) return
                  const nextOpen = !workflowRolesOpen
                  setWorkflowRolesOpen(nextOpen)
                  if (!nextOpen) return
                  setRoleCardOpenState((prev) => {
                    const next = { ...prev }
                    roles.forEach((role) => {
                      next[role.client_id] = true
                    })
                    return next
                  })
                }}
              >
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <CardTitle className="text-base">Workflow Roles</CardTitle>
                    <CardDescription>Define required roles and credentials for workflow assignments.</CardDescription>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Badge variant="outline" className="shrink-0">
                      {roles.length} {roles.length === 1 ? "Role" : "Roles"}
                    </Badge>
                    <CollapsibleTrigger asChild>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={workflowRolesOpen ? "Collapse workflow roles" : "Expand workflow roles"}
                      >
                        <ChevronDown className={cn("h-4 w-4 transition-transform", !workflowRolesOpen && "-rotate-90")} />
                      </Button>
                    </CollapsibleTrigger>
                  </div>
                </div>
              </CardHeader>
              <CollapsibleContent>
                <CardContent className="space-y-4">
                  {roles.map((role, roleIndex) => {
                    const roleCardOpen = nestedCardOpenByDefault(roleCardOpenState, role.client_id)

                    return (
                      <Card key={role.client_id}>
                        <Collapsible
                          open={roleCardOpen}
                          onOpenChange={(open) =>
                            setRoleCardOpenState((prev) => ({
                              ...prev,
                              [role.client_id]: open,
                            }))
                          }
                        >
	                          <CardHeader
	                            className="cursor-pointer pb-3"
	                            onClick={(event) => {
	                              if (didClickInteractiveElement(event)) return
	                              setRoleCardOpenState((prev) => ({
	                                ...prev,
	                                [role.client_id]: !roleCardOpen,
	                              }))
	                            }}
	                          >
                            <div className="flex items-center justify-between gap-3">
                              <div className="min-w-0">
                                <CardTitle className="truncate text-sm">
                                  Role {roleIndex + 1}: {role.title.trim() || "Untitled role"}
                                </CardTitle>
                                <CardDescription>
                                  {role.required_credentials.length}{" "}
                                  {role.required_credentials.length === 1 ? "credential" : "credentials"} required
                                </CardDescription>
                              </div>
                              <div className="flex items-center gap-1">
                                {roles.length > 1 && (
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => setRoles((prev) => prev.filter((item) => item.client_id !== role.client_id))}
                                    aria-label={`Delete role ${roleIndex + 1}`}
                                  >
                                    <Trash2 className="h-4 w-4" />
                                  </Button>
                                )}
                                <CollapsibleTrigger asChild>
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="icon"
                                    aria-label={roleCardOpen ? `Collapse role ${roleIndex + 1}` : `Expand role ${roleIndex + 1}`}
                                  >
                                    <ChevronDown className={cn("h-4 w-4 transition-transform", !roleCardOpen && "-rotate-90")} />
                                  </Button>
                                </CollapsibleTrigger>
                              </div>
                            </div>
                          </CardHeader>
                          <CollapsibleContent>
                            <CardContent className="space-y-4">
                              <div className="space-y-2">
                                <Label>Role Title</Label>
                                <Input
                                  value={role.title}
                                  onChange={(e) => updateRole(role.client_id, { title: e.target.value })}
                                  placeholder="Field verifier"
                                />
                              </div>

                              <div className="space-y-2">
                                <Label>Required Credentials</Label>
                                {credentialTypes.length === 0 ? (
                                  <p className="text-xs text-muted-foreground">
                                    No credential types defined. Add them in the Admin panel.
                                  </p>
                                ) : (
                                  <Select
                                    value=""
                                    onValueChange={(value) => toggleRoleCredential(role.client_id, value, true)}
                                  >
                                    <SelectTrigger>
                                      <SelectValue placeholder="Add a required credential..." />
                                    </SelectTrigger>
                                    <SelectContent>
                                      {credentialTypes
                                        .filter((ct) => !role.required_credentials.includes(ct.value))
                                        .map((ct) => (
                                          <SelectItem key={ct.value} value={ct.value}>
                                            {ct.label}
                                          </SelectItem>
                                        ))}
                                    </SelectContent>
                                  </Select>
                                )}
                                {role.required_credentials.length > 0 && (
                                  <div className="flex flex-wrap gap-1.5 pt-1">
                                    {role.required_credentials.map((cred) => {
                                      const label = credentialTypes.find((ct) => ct.value === cred)?.label || cred
                                      return (
                                        <Badge key={cred} variant="secondary" className="gap-1 pr-1">
                                          {label}
                                          <button
                                            type="button"
                                            className="ml-0.5 rounded-sm opacity-70 hover:opacity-100"
                                            onClick={() => toggleRoleCredential(role.client_id, cred, false)}
                                          >
                                            <X className="h-3 w-3" />
                                          </button>
                                        </Badge>
                                      )
                                    })}
                                  </div>
                                )}
                              </div>
                            </CardContent>
                          </CollapsibleContent>
                        </Collapsible>
                      </Card>
                    )
                  })}

                  <div className="flex justify-end">
                    <Button type="button" variant="outline" onClick={() => setRoles((prev) => [...prev, createDraftRole()])}>
                      <Plus className="h-4 w-4 mr-2" />
                      Add Role
                    </Button>
                  </div>
                </CardContent>
              </CollapsibleContent>
            </Collapsible>
          </Card>

          <div className="space-y-4">
            <h3 className="text-lg font-medium">Workflow Steps</h3>

            {steps.map((step, stepIndex) => {
              const draggingStep = dragState?.type === "step" && dragState.stepId === step.id
              const stepCardOpen = sectionCardOpenByDefault(stepCardOpenState, step.id)
              const stepTitle = step.title.trim() || `Step ${stepIndex + 1}`
              const stepDropBefore =
                dropIndicator?.type === "step" &&
                dropIndicator.targetStepId === step.id &&
                dropIndicator.position === "before"
              const stepDropAfter =
                dropIndicator?.type === "step" &&
                dropIndicator.targetStepId === step.id &&
                dropIndicator.position === "after"

              return (
                <div key={step.id} className="space-y-2">
                  {stepDropBefore && <div className="h-1.5 rounded-full bg-[#eb6c6c] shadow-sm" />}
                  <Card
                    data-reorder-preview
                    data-drop-step-id={step.id}
                    className={cn(draggingStep && "opacity-60 ring-2 ring-[#eb6c6c]/50")}
                    onDragOver={(event) => {
                      if (dragState?.type !== "step" || dragState.stepId === step.id) return
                      event.preventDefault()
                      const position = dropPositionFromPointer(event)
                      setDropIndicator({ type: "step", targetStepId: step.id, position })
                    }}
                    onDragLeave={(event) => {
                      const relatedTarget = event.relatedTarget as Node | null
                      if (relatedTarget && event.currentTarget.contains(relatedTarget)) return
                      setDropIndicator((prev) => {
                        if (prev?.type !== "step" || prev.targetStepId !== step.id) return prev
                        return null
                      })
                    }}
                    onDrop={(event) => {
                      if (dragState?.type !== "step" || dragState.stepId === step.id) return
                      event.preventDefault()
                      const position =
                        dropIndicator?.type === "step" && dropIndicator.targetStepId === step.id
                          ? dropIndicator.position
                          : dropPositionFromPointer(event)
                      reorderSteps(dragState.stepId, step.id, position)
                      endReorderDrag()
                    }}
                  >
                    <Collapsible
                      open={stepCardOpen}
                      onOpenChange={(open) => {
                        setStepCardOpenState((prev) => ({
                          ...prev,
                          [step.id]: open,
                        }))
                        if (!open) return
                        setWorkItemCardOpenState((prev) => {
                          const next = { ...prev }
                          step.work_items.forEach((item) => {
                            next[workItemCollapseKey(step.id, item.id)] = true
                          })
                          return next
                        })
                      }}
                    >
	                      <CardHeader
	                        className="cursor-pointer pb-3"
	                        onClick={(event) => {
	                          if (didClickInteractiveElement(event)) return
                          const nextOpen = !stepCardOpen
	                          setStepCardOpenState((prev) => ({
	                            ...prev,
	                            [step.id]: nextOpen,
	                          }))
                          if (!nextOpen) return
                          setWorkItemCardOpenState((prev) => {
                            const next = { ...prev }
                            step.work_items.forEach((item) => {
                              next[workItemCollapseKey(step.id, item.id)] = true
                            })
                            return next
                          })
	                        }}
	                      >
                        <div className="flex items-center justify-between gap-3">
                          <div className="flex min-w-0 items-center gap-2">
                            <button
                              type="button"
                              aria-label={`Drag step ${stepIndex + 1}`}
                              className="inline-flex h-8 w-8 touch-none items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted cursor-grab active:cursor-grabbing"
                              draggable
                              onDragStart={(event) => beginReorderDrag(event, { type: "step", stepId: step.id })}
                              onDragEnd={endReorderDrag}
                              onPointerDown={(event) => beginTouchReorderDrag(event, { type: "step", stepId: step.id })}
                              onPointerMove={updateTouchReorderDrag}
                              onPointerUp={completeTouchReorderDrag}
                              onPointerCancel={cancelTouchReorderDrag}
                            >
                              <GripVertical className="h-4 w-4" />
                            </button>
                            <div className="min-w-0">
                              <CardTitle className="truncate text-base">{stepTitle}</CardTitle>
                              <CardDescription>Step {stepIndex + 1}</CardDescription>
                            </div>
                          </div>
                          <div className="flex items-center gap-1">
                            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-muted/40 px-2.5 py-1 text-xs shadow-sm">
                              <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                                Bounty
                              </span>
                              <span className="font-semibold text-foreground">
                                {formatStepBountyIndicator(step.bounty)}
                              </span>
                            </div>
                            {steps.length > 1 && (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={() => setSteps((prev) => prev.filter((item) => item.id !== step.id))}
                                aria-label={`Delete step ${stepIndex + 1}`}
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            )}
                            <CollapsibleTrigger asChild>
                              <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                aria-label={stepCardOpen ? `Collapse step ${stepIndex + 1}` : `Expand step ${stepIndex + 1}`}
                              >
                                <ChevronDown className={cn("h-4 w-4 transition-transform", !stepCardOpen && "-rotate-90")} />
                              </Button>
                            </CollapsibleTrigger>
                          </div>
                        </div>
                      </CardHeader>
                      <CollapsibleContent>
                        <CardContent className="space-y-4">
                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-2 md:col-span-2">
                      <Label>Step Title</Label>
                      <Input
                        value={step.title}
                        onChange={(e) => updateStep(step.id, { title: e.target.value })}
                        placeholder="Capture storefront evidence"
                      />
                    </div>
                    <div className="space-y-2 md:col-span-2">
                      <Label>Step Description (Optional)</Label>
                      <Textarea
                        value={step.description}
                        onChange={(e) => updateStep(step.id, { description: e.target.value })}
                        placeholder="Describe expected work and completion criteria for this step."
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Bounty (SFLuv)</Label>
                      <Input
                        type="number"
                        min="0"
                        value={step.bounty}
                        onWheel={preventNumberInputScrollChange}
                        onChange={(e) => updateStep(step.id, { bounty: e.target.value })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Assigned Role</Label>
                      <Select value={step.role_client_id} onValueChange={(value) => updateStep(step.id, { role_client_id: value })}>
                        <SelectTrigger>
                          <SelectValue placeholder="Select role" />
                        </SelectTrigger>
                        <SelectContent>
                          {roles.map((role) => (
                            <SelectItem key={role.client_id} value={role.client_id}>
                              {role.title || "Untitled role"}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={step.allow_step_not_possible}
                      onCheckedChange={(value) => updateStep(step.id, { allow_step_not_possible: Boolean(value) })}
                    />
                    Enable "Step not possible" option (requires details, ends workflow with no payouts)
                  </label>

                  <div className="space-y-4">
                    <Label>Work Items</Label>

                    {step.work_items.map((item, itemIndex) => {
                      const draggingItem =
                        dragState?.type === "work-item" &&
                        dragState.stepId === step.id &&
                        dragState.itemId === item.id
                      const itemCollapseId = workItemCollapseKey(step.id, item.id)
                      const itemCardOpen = nestedCardOpenByDefault(workItemCardOpenState, itemCollapseId)
                      const itemTitle = item.title.trim() || `Item ${itemIndex + 1}`
                      const workItemDropBefore =
                        dropIndicator?.type === "work-item" &&
                        dropIndicator.stepId === step.id &&
                        dropIndicator.targetItemId === item.id &&
                        dropIndicator.position === "before"
                      const workItemDropAfter =
                        dropIndicator?.type === "work-item" &&
                        dropIndicator.stepId === step.id &&
                        dropIndicator.targetItemId === item.id &&
                        dropIndicator.position === "after"

                      return (
                        <div key={item.id} className="space-y-1.5">
                          {workItemDropBefore && <div className="h-1.5 rounded-full bg-[#eb6c6c] shadow-sm" />}
                          <Card
                            data-reorder-preview
                            data-drop-work-item-step-id={step.id}
                            data-drop-work-item-id={item.id}
                            className={cn(draggingItem && "opacity-60 ring-2 ring-[#eb6c6c]/40")}
                            onDragOver={(event) => {
                              if (dragState?.type !== "work-item" || dragState.stepId !== step.id || dragState.itemId === item.id) return
                              event.preventDefault()
                              event.stopPropagation()
                              const position = dropPositionFromPointer(event)
                              setDropIndicator({ type: "work-item", stepId: step.id, targetItemId: item.id, position })
                            }}
                            onDragLeave={(event) => {
                              const relatedTarget = event.relatedTarget as Node | null
                              if (relatedTarget && event.currentTarget.contains(relatedTarget)) return
                              setDropIndicator((prev) => {
                                if (prev?.type !== "work-item" || prev.stepId !== step.id || prev.targetItemId !== item.id) return prev
                                return null
                              })
                            }}
                            onDrop={(event) => {
                              if (dragState?.type !== "work-item" || dragState.stepId !== step.id || dragState.itemId === item.id) return
                              event.preventDefault()
                              event.stopPropagation()
                              const position =
                                dropIndicator?.type === "work-item" &&
                                dropIndicator.stepId === step.id &&
                                dropIndicator.targetItemId === item.id
                                  ? dropIndicator.position
                                  : dropPositionFromPointer(event)
                              reorderWorkItems(step.id, dragState.itemId, item.id, position)
                              endReorderDrag()
                            }}
                          >
                            <Collapsible
                              open={itemCardOpen}
                              onOpenChange={(open) =>
                                setWorkItemCardOpenState((prev) => ({
                                  ...prev,
                                  [itemCollapseId]: open,
                                }))
                              }
                            >
	                              <CardHeader
	                                className="cursor-pointer pb-3 pt-4"
	                                onClick={(event) => {
	                                  if (didClickInteractiveElement(event)) return
	                                  setWorkItemCardOpenState((prev) => ({
	                                    ...prev,
	                                    [itemCollapseId]: !itemCardOpen,
	                                  }))
	                                }}
	                              >
                                <div className="flex items-center justify-between gap-3">
                                  <div className="flex min-w-0 items-center gap-2">
                                    <button
                                      type="button"
                                      aria-label={`Drag work item ${itemIndex + 1}`}
                                      className="inline-flex h-7 w-7 touch-none items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted cursor-grab active:cursor-grabbing"
                                      draggable
                                      onDragStart={(event) =>
                                        beginReorderDrag(event, { type: "work-item", stepId: step.id, itemId: item.id })
                                      }
                                      onDragEnd={endReorderDrag}
                                      onPointerDown={(event) =>
                                        beginTouchReorderDrag(event, { type: "work-item", stepId: step.id, itemId: item.id })
                                      }
                                      onPointerMove={updateTouchReorderDrag}
                                      onPointerUp={completeTouchReorderDrag}
                                      onPointerCancel={cancelTouchReorderDrag}
                                    >
                                      <GripVertical className="h-4 w-4" />
                                    </button>
                                    <div className="min-w-0">
                                      <CardTitle className="truncate text-sm">{itemTitle}</CardTitle>
                                      <CardDescription>Item {itemIndex + 1}</CardDescription>
                                    </div>
                                  </div>
                                  <div className="flex items-center gap-1">
                                    <Button
                                      type="button"
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => removeWorkItem(step.id, item.id)}
                                      aria-label={`Delete item ${itemIndex + 1}`}
                                    >
                                      <Trash2 className="h-4 w-4" />
                                    </Button>
                                    <CollapsibleTrigger asChild>
                                      <Button
                                        type="button"
                                        variant="ghost"
                                        size="icon"
                                        aria-label={itemCardOpen ? `Collapse item ${itemIndex + 1}` : `Expand item ${itemIndex + 1}`}
                                      >
                                        <ChevronDown className={cn("h-4 w-4 transition-transform", !itemCardOpen && "-rotate-90")} />
                                      </Button>
                                    </CollapsibleTrigger>
                                  </div>
                                </div>
                              </CardHeader>
                              <CollapsibleContent>
                                <CardContent className="space-y-3">
                                  <Input
                            value={item.title}
                            onChange={(e) => updateWorkItem(step.id, item.id, { title: e.target.value })}
                            placeholder="Item title"
                          />
                          <Textarea
                            value={item.description}
                            onChange={(e) => updateWorkItem(step.id, item.id, { description: e.target.value })}
                            placeholder="Item instructions"
                          />

                          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                            <label className="flex items-center gap-2 text-sm">
                              <Checkbox
                                checked={item.optional}
                                onCheckedChange={(value) => updateWorkItem(step.id, item.id, { optional: Boolean(value) })}
                              />
                              Optional
                            </label>
                            <label className="flex items-center gap-2 text-sm">
                              <Checkbox
                                checked={item.requires_photo}
                                onCheckedChange={(value) => {
                                  const requiresPhoto = Boolean(value)
                                  updateWorkItem(step.id, item.id, {
                                    requires_photo: requiresPhoto,
                                    camera_capture_only: requiresPhoto ? item.camera_capture_only : false,
                                    photo_required_count: requiresPhoto ? Math.max(1, Number(item.photo_required_count) || 1) : 1,
                                    photo_allow_any_count: requiresPhoto ? item.photo_allow_any_count : false,
                                    photo_aspect_ratio: requiresPhoto
                                      ? normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio)
                                      : "square",
                                  })
                                }}
                              />
                              Require Photo
                            </label>
                            <label className="flex items-center gap-2 text-sm">
                              <Checkbox
                                checked={item.requires_written_response}
                                onCheckedChange={(value) =>
                                  updateWorkItem(step.id, item.id, { requires_written_response: Boolean(value) })
                                }
                              />
                              Require Written
                            </label>
                            <label className="flex items-center gap-2 text-sm">
                              <Checkbox
                                checked={item.requires_dropdown}
                                onCheckedChange={(value) => updateWorkItem(step.id, item.id, { requires_dropdown: Boolean(value) })}
                              />
                              Require Dropdown
                            </label>
                          </div>

                          {item.requires_photo && (
                            <div className="space-y-3 rounded-md border p-3 bg-secondary/40">
                              <label className="flex items-center gap-2 text-sm">
                                <Checkbox
                                  checked={item.camera_capture_only}
                                  onCheckedChange={(value) =>
                                    updateWorkItem(step.id, item.id, {
                                      camera_capture_only: Boolean(value),
                                      photo_aspect_ratio: Boolean(value)
                                        ? "square"
                                        : normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio),
                                    })
                                  }
                                />
                                Require live camera capture only (disallow camera roll)
                              </label>

                              <div className="grid gap-3 md:grid-cols-3">
                                <div className="space-y-1">
                                  <Label className="text-xs">Photo Aspect Ratio</Label>
                                  <Select
                                    value={normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio)}
                                    onValueChange={(value: WorkflowPhotoAspectRatio) =>
                                      updateWorkItem(step.id, item.id, {
                                        photo_aspect_ratio: normalizeWorkflowPhotoAspectRatio(value),
                                      })
                                    }
                                  >
                                    <SelectTrigger>
                                      <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent>
                                      <SelectItem value="vertical">Vertical</SelectItem>
                                      <SelectItem value="square">Square</SelectItem>
                                      <SelectItem value="horizontal">Horizontal</SelectItem>
                                    </SelectContent>
                                  </Select>
                                </div>

                                <div className="space-y-1 md:col-span-2">
                                  <Label className="text-xs">Photo Count Requirement</Label>
                                  <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                                    <label className="flex items-center gap-2 text-xs">
                                      <Checkbox
                                        checked={item.photo_allow_any_count}
                                        onCheckedChange={(value) =>
                                          updateWorkItem(step.id, item.id, {
                                            photo_allow_any_count: Boolean(value),
                                          })
                                        }
                                      />
                                      Allow any amount
                                    </label>
                                    {!item.photo_allow_any_count && (
                                      <Input
                                        type="number"
                                        min={1}
                                        value={item.photo_required_count}
                                        onWheel={preventNumberInputScrollChange}
                                        onChange={(e) =>
                                          updateWorkItem(step.id, item.id, {
                                            photo_required_count: Math.max(1, Number(e.target.value) || 1),
                                          })
                                        }
                                        className="w-full sm:w-32"
                                      />
                                    )}
                                  </div>
                                </div>
                              </div>
                            </div>
                          )}

                          {item.requires_dropdown && (
                            <div className="space-y-3 border rounded-md p-3 bg-secondary/50">
                              <Label className="text-sm">Dropdown Options</Label>

                              {item.dropdown_options.map((option, optionIndex) => {
                                const draggingOption =
                                  dragState?.type === "dropdown-option" &&
                                  dragState.stepId === step.id &&
                                  dragState.itemId === item.id &&
                                  dragState.optionIndex === optionIndex
                                const optionDropBefore =
                                  dropIndicator?.type === "dropdown-option" &&
                                  dropIndicator.stepId === step.id &&
                                  dropIndicator.itemId === item.id &&
                                  dropIndicator.targetOptionIndex === optionIndex &&
                                  dropIndicator.position === "before"
                                const optionDropAfter =
                                  dropIndicator?.type === "dropdown-option" &&
                                  dropIndicator.stepId === step.id &&
                                  dropIndicator.itemId === item.id &&
                                  dropIndicator.targetOptionIndex === optionIndex &&
                                  dropIndicator.position === "after"

                                return (
                                <div key={`${item.id}-${optionIndex}`} className="space-y-1.5">
                                  {optionDropBefore && <div className="h-1 rounded-full bg-[#eb6c6c] shadow-sm" />}
                                  <div
                                    data-reorder-preview
                                    data-drop-option-step-id={step.id}
                                    data-drop-option-item-id={item.id}
                                    data-drop-option-index={optionIndex}
                                    className={cn(
                                      "space-y-2 rounded-md border p-3",
                                      draggingOption && "opacity-60 ring-2 ring-[#eb6c6c]/40",
                                    )}
                                    onDragOver={(event) => {
                                      if (
                                        dragState?.type !== "dropdown-option" ||
                                        dragState.stepId !== step.id ||
                                        dragState.itemId !== item.id ||
                                        dragState.optionIndex === optionIndex
                                      ) {
                                        return
                                      }
                                      event.preventDefault()
                                      event.stopPropagation()
                                      const position = dropPositionFromPointer(event)
                                      setDropIndicator({
                                        type: "dropdown-option",
                                        stepId: step.id,
                                        itemId: item.id,
                                        targetOptionIndex: optionIndex,
                                        position,
                                      })
                                    }}
                                    onDragLeave={(event) => {
                                      const relatedTarget = event.relatedTarget as Node | null
                                      if (relatedTarget && event.currentTarget.contains(relatedTarget)) return
                                      setDropIndicator((prev) => {
                                        if (
                                          prev?.type !== "dropdown-option" ||
                                          prev.stepId !== step.id ||
                                          prev.itemId !== item.id ||
                                          prev.targetOptionIndex !== optionIndex
                                        ) {
                                          return prev
                                        }
                                        return null
                                      })
                                    }}
                                    onDrop={(event) => {
                                      if (
                                        dragState?.type !== "dropdown-option" ||
                                        dragState.stepId !== step.id ||
                                        dragState.itemId !== item.id ||
                                        dragState.optionIndex === optionIndex
                                      ) {
                                        return
                                      }
                                      event.preventDefault()
                                      event.stopPropagation()
                                      const position =
                                        dropIndicator?.type === "dropdown-option" &&
                                        dropIndicator.stepId === step.id &&
                                        dropIndicator.itemId === item.id &&
                                        dropIndicator.targetOptionIndex === optionIndex
                                          ? dropIndicator.position
                                          : dropPositionFromPointer(event)
                                      reorderDropdownOptions(step.id, item.id, dragState.optionIndex, optionIndex, position)
                                      endReorderDrag()
                                    }}
                                  >
                                  <div className="grid gap-2 md:grid-cols-[auto,1fr,auto,auto]" /* drag, label, write-up, photo */>
                                    <button
                                      type="button"
                                      aria-label={`Drag dropdown option ${optionIndex + 1}`}
                                      className="inline-flex h-9 w-9 touch-none items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted cursor-grab active:cursor-grabbing"
                                      draggable
                                      onDragStart={(event) =>
                                        beginReorderDrag(event, {
                                          type: "dropdown-option",
                                          stepId: step.id,
                                          itemId: item.id,
                                          optionIndex,
                                        })
                                      }
                                      onDragEnd={endReorderDrag}
                                      onPointerDown={(event) =>
                                        beginTouchReorderDrag(event, {
                                          type: "dropdown-option",
                                          stepId: step.id,
                                          itemId: item.id,
                                          optionIndex,
                                        })
                                      }
                                      onPointerMove={updateTouchReorderDrag}
                                      onPointerUp={completeTouchReorderDrag}
                                      onPointerCancel={cancelTouchReorderDrag}
                                    >
                                      <GripVertical className="h-4 w-4" />
                                    </button>
                                    <Input
                                      value={option.label}
                                      onChange={(e) => updateDropdownOption(step.id, item.id, optionIndex, { label: e.target.value })}
                                      placeholder="Option label"
                                    />
                                    <label className="flex items-center gap-2 text-xs">
                                      <Checkbox
                                        checked={option.requires_written_response}
                                        onCheckedChange={(value) =>
                                          updateDropdownOption(step.id, item.id, optionIndex, {
                                            requires_written_response: Boolean(value),
                                          })
                                        }
                                      />
                                      Needs write-up
                                    </label>
                                    <label className="flex items-center gap-2 text-xs">
                                      <Checkbox
                                        checked={Boolean(option.requires_photo_attachment)}
                                        onCheckedChange={(value) =>
                                          updateDropdownOption(step.id, item.id, optionIndex, {
                                            requires_photo_attachment: Boolean(value),
                                            camera_capture_only: Boolean(value) ? Boolean(option.camera_capture_only) : false,
                                            photo_instructions: Boolean(value) ? option.photo_instructions || "" : "",
                                          })
                                        }
                                      />
                                      Needs photo
                                    </label>
                                  </div>

                                    <div className="space-y-2">
                                      {option.requires_photo_attachment && (
                                        <div className="space-y-3">
                                          <label className="flex items-center gap-2 text-xs text-muted-foreground">
                                            <Checkbox
                                              checked={Boolean(option.camera_capture_only)}
                                              onCheckedChange={(value) =>
                                                updateDropdownOption(step.id, item.id, optionIndex, {
                                                  camera_capture_only: Boolean(value),
                                                })
                                              }
                                            />
                                            Require live photo
                                          </label>
                                          <div className="space-y-1">
                                            <Label className="text-xs">Photo Instructions</Label>
                                            <Textarea
                                              value={option.photo_instructions || ""}
                                              onChange={(e) =>
                                                updateDropdownOption(step.id, item.id, optionIndex, {
                                                  photo_instructions: e.target.value,
                                                })
                                              }
                                              placeholder="Explain what photo should be attached when this option is selected."
                                            />
                                          </div>
                                        </div>
                                      )}
                                      <Label className="text-xs">Notify Emails For This Option</Label>
                                      <Input
                                      value={option.notify_email_input}
                                      onChange={(e) =>
                                        updateDropdownOption(step.id, item.id, optionIndex, {
                                          notify_email_input: e.target.value,
                                        })
                                      }
                                      placeholder="name@example.com"
                                    />
                                    {option.notify_emails.length > 0 && (
                                      <div className="flex flex-wrap gap-2">
                                        {option.notify_emails.map((email) => (
                                          <Badge key={email} variant="secondary" className="gap-1">
                                            {email}
                                            <button
                                              type="button"
                                              className="ml-1"
                                              onClick={() => removeDropdownOptionEmail(step.id, item.id, optionIndex, email)}
                                            >
                                              <Trash2 className="h-3 w-3" />
                                            </button>
                                          </Badge>
                                        ))}
                                      </div>
                                    )}
                                    <div className="flex justify-end">
                                      <Button
                                        type="button"
                                        size="sm"
                                        variant="outline"
                                        onClick={() => addDropdownOptionEmail(step.id, item.id, optionIndex)}
                                      >
                                        Add Email
                                      </Button>
                                    </div>
                                    <label className="flex items-center gap-2 text-xs text-muted-foreground">
                                      <Checkbox
                                        checked={Boolean(option.send_pictures_with_email)}
                                        onCheckedChange={(value) =>
                                          updateDropdownOption(step.id, item.id, optionIndex, {
                                            send_pictures_with_email: Boolean(value),
                                          })
                                        }
                                      />
                                      Send pictures with email
                                    </label>
                                    <div className="flex justify-end pt-2">
                                      <Button
                                        type="button"
                                        size="sm"
                                        variant="ghost"
                                        onClick={() => removeDropdownOption(step.id, item.id, optionIndex)}
                                      >
                                        <Trash2 className="h-4 w-4" />
                                      </Button>
                                    </div>
                                  </div>
                                  </div>
                                  {optionDropAfter && <div className="h-1 rounded-full bg-[#eb6c6c] shadow-sm" />}
                                </div>
                                )
                              })}

                              <div className="flex justify-end">
                                <Button type="button" variant="outline" size="sm" onClick={() => addDropdownOption(step.id, item.id)}>
                                  <Plus className="h-4 w-4 mr-1" />
                                  Add Option
                                </Button>
                              </div>
                            </div>
                          )}
                                </CardContent>
                              </CollapsibleContent>
                            </Collapsible>
                          </Card>
                        {workItemDropAfter && <div className="h-1.5 rounded-full bg-[#eb6c6c] shadow-sm" />}
                      </div>
                      )
                    })}

                    <div className="flex justify-end">
                      <Button type="button" variant="outline" size="sm" onClick={() => addWorkItem(step.id)}>
                        <Plus className="h-4 w-4 mr-1" />
                        Add Work Item
                      </Button>
                    </div>
                  </div>
                        </CardContent>
                      </CollapsibleContent>
                    </Collapsible>
                  </Card>
                {stepDropAfter && <div className="h-1.5 rounded-full bg-[#eb6c6c] shadow-sm" />}
              </div>
              )
            })}

            <div className="flex justify-end">
              <Button type="button" variant="outline" onClick={() => setSteps((prev) => [...prev, createDraftStep()])}>
                <Plus className="h-4 w-4 mr-2" />
                New Step
              </Button>
            </div>
          </div>

              {error && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              {showWorkflowSubmitSuccessInline && (
                <div className="flex items-center gap-2 rounded-lg bg-green-50 p-3 text-sm text-green-700 dark:bg-green-900/20">
                  <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
                  <span>{successMessage}</span>
                </div>
              )}

		              <div className="flex flex-wrap items-center gap-3">
                <Badge variant="outline">Draft Total Bounty: {totalDraftBounty} SFLuv</Badge>
                <Button onClick={submitWorkflow} disabled={submitting}>
                  {submitting ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Submitting...
                    </>
                  ) : (
                    isEditProposalMode ? "Submit Workflow Edit Proposal" : "Submit Workflow Proposal"
                  )}
                </Button>
                {isEditProposalMode && (
                  <Button type="button" variant="outline" onClick={cancelWorkflowEditProposalDraft} disabled={submitting}>
                    Cancel Edit Mode
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>
          )}
        </TabsContent>

        <TabsContent value="your-workflows" className="mt-4 space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>{isAdminUser ? "All Workflows" : "Your Workflows"}</CardTitle>
              <CardDescription>
                {isAdminUser
                  ? "Review workflows across all proposers, search by title or description, and filter by proposer."
                  : "Review only your submitted workflows, filter by status, and propose deletions for active workflows."}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {workflowListLoading && workflowListLoaded && (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  <span>Refreshing workflows...</span>
                </div>
              )}

              {!workflowListLoaded ? (
                <div className="flex min-h-[220px] items-center justify-center">
                  <div className="flex items-center gap-3 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    <span>{isAdminUser ? "Loading workflows..." : "Loading your workflows..."}</span>
                  </div>
                </div>
              ) : (
                <>
              <div className="grid gap-4 lg:grid-cols-[minmax(0,1.35fr)_minmax(220px,0.75fr)_minmax(220px,0.85fr)]">
                <div className="space-y-2">
                  <Label>{isAdminUser ? "Search Workflows" : "Search"}</Label>
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      value={workflowSearch}
                      onChange={(event) => {
                        setWorkflowSearch(event.target.value)
                        setWorkflowPage(0)
                      }}
                      placeholder={isAdminUser ? "Search title or description" : "Search title or description"}
                      className="pl-9"
                    />
                  </div>
                </div>

                <div className="space-y-2">
                  <Label>Filter By Approval Status</Label>
                  <Select
                    value={workflowStatusFilter}
                    onValueChange={(value) => {
                      setWorkflowStatusFilter(value as "all" | Workflow["status"])
                      setWorkflowPage(0)
                    }}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {WORKFLOW_STATUS_FILTER_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {isAdminUser && (
                  <div className="space-y-2">
                    <Label>Filter By Proposer</Label>
                    <Select
                      value={workflowProposerFilter}
                      onValueChange={(value) => {
                        setWorkflowProposerFilter(value)
                        setWorkflowPage(0)
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">All Proposers</SelectItem>
                        {proposerOptions.map((proposer) => {
                          const label = proposerLabelById.get(proposer.user_id) || proposer.user_id
                          return (
                            <SelectItem key={proposer.user_id} value={proposer.user_id}>
                              {label}
                            </SelectItem>
                          )
                        })}
                      </SelectContent>
                    </Select>
                  </div>
                )}
              </div>

              {isAdminUser && (
                <div className="text-xs text-muted-foreground">
                  Showing {workflowSeriesGroups.length} series on this page, {workflowTotal} total.
                </div>
              )}

              {workflowSeriesGroups.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflows found for the selected status.</p>
              ) : (
                workflowSeriesGroups.map((group) => {
                  const workflow = group.workflows[group.workflows.length - 1]
                  const pendingDeletionProposal = getPendingDeletionProposal(workflow)
                  const deletionTargetType: WorkflowDeletionTargetType =
                    pendingDeletionProposal?.target_type ?? (isSeriesWorkflow(workflow) ? "series" : "workflow")
                  const deletionAlreadyProposed = Boolean(pendingDeletionProposal)
                  const isOwner = isWorkflowOwner(workflow)
                  const proposerLabel = proposerLabelById.get(workflow.proposer_id) || workflow.proposer_id

                  return (
                  <Card key={group.key}>
                    <CardContent
                      className="pt-4 cursor-pointer transition-colors hover:bg-muted/30"
                      onClick={() => void openWorkflowDetails(workflow.id, workflow)}
                    >
                      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                        <div className="space-y-2">
                          <div className="flex items-center gap-2">
                            <h4 className="font-semibold">{workflow.title}</h4>
                            <Badge
                              variant={
                                workflow.status === "approved"
                                  ? "default"
                                  : workflow.status === "rejected"
                                    ? "destructive"
                                    : "secondary"
                              }
                            >
                              {formatWorkflowDisplayStatus(workflow)}
                            </Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">{workflow.description}</p>
                          <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                            <span>Start: {new Date(workflow.start_at * 1000).toLocaleString()}</span>
                            <span>Recurring: {workflow.recurrence}</span>
                            <span>Total bounty: {workflow.total_bounty}</span>
                            <span>Weekly requirement: {workflow.weekly_bounty_requirement}</span>
                            {isAdminUser && <span>Proposer: {proposerLabel}</span>}
                          </div>
                          <div className="flex items-center gap-2 text-xs text-muted-foreground">
                            <Clock className="h-3 w-3" />
                            <span>
                              Votes {workflow.votes.approve} approve / {workflow.votes.deny} deny
                            </span>
                          </div>
                        </div>

                        <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:flex-wrap">
                          <Button
                            className="w-full sm:w-auto"
                            variant="outline"
                            onClick={(event) => {
                              event.stopPropagation()
                              void openWorkflowDetails(workflow.id, workflow)
                            }}
                          >
                            View Details
                          </Button>

                          {isOwner && (workflow.status === "pending" || workflow.status === "rejected" || workflow.status === "expired" || workflow.status === "failed" || workflow.status === "skipped") && (
                            <Button
                              className="w-full sm:w-auto"
                              variant="outline"
                              onClick={(event) => {
                                event.stopPropagation()
                                void deleteWorkflow(workflow.id)
                              }}
                            >
                              <Trash2 className="h-4 w-4 mr-2" />
                              Archive
                            </Button>
                          )}

                          {isOwner && canProposeDeletion && canProposeDeletionForStatus(workflow) && (
                            <Button
                              className="w-full sm:w-auto"
                              variant="outline"
                              onClick={(event) => {
                                event.stopPropagation()
                                void proposeDeletion(workflow.id, deletionTargetType)
                              }}
                              disabled={Boolean(deletionSubmitting) || deletionAlreadyProposed}
                            >
                              {deletionSubmitting === `${workflow.id}:${deletionTargetType}`
                                ? "Submitting..."
                                : deletionAlreadyProposed
                                  ? deletionTargetType === "series"
                                    ? "Series Deletion Proposed"
                                    : "Workflow Deletion Proposed"
                                  : deletionTargetType === "series"
                                    ? "Propose Series Deletion"
                                    : "Propose Workflow Deletion"}
                            </Button>
                          )}

                        </div>
                      </div>
                    </CardContent>
                  </Card>
                )})
              )}

              {isAdminUser && workflowTotal > workflowPageSize && (
                <div className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
                  <p className="text-xs text-muted-foreground">
                    Page {workflowPage + 1} of {workflowPageCount}
                  </p>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      onClick={() => setWorkflowPage((prev) => Math.max(0, prev - 1))}
                      disabled={workflowPage === 0 || workflowListLoading}
                    >
                      Previous
                    </Button>
                    <Button
                      variant="outline"
                      onClick={() => setWorkflowPage((prev) => prev + 1)}
                      disabled={workflowPage + 1 >= workflowPageCount || workflowListLoading}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
                </>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <WorkflowDetailsModal
        workflow={pendingWorkflowSubmission?.workflow || null}
        open={submissionPreviewOpen}
        onOpenChange={(open) => {
          if (submitting) return
          setSubmissionPreviewOpen(open)
          if (!open) {
            setPendingWorkflowSubmission(null)
            setSubmissionPreviewError("")
          }
        }}
        renderHeaderContent={() => (
          <div className="space-y-3">
            <div className="rounded-xl border border-border/70 bg-secondary/20 p-3 text-sm text-muted-foreground">
              Review the workflow details below. Nothing is submitted until you confirm.
            </div>
            {submissionPreviewError && (
              <div className="flex items-center gap-2 rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
                <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                <span>{submissionPreviewError}</span>
              </div>
            )}
          </div>
        )}
        renderBottomActions={() => (
          <div className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-end">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                if (submitting) return
                setSubmissionPreviewOpen(false)
                setPendingWorkflowSubmission(null)
                setSubmissionPreviewError("")
              }}
              disabled={submitting}
            >
              Back to Edit
            </Button>
            <Button type="button" onClick={confirmWorkflowSubmission} disabled={submitting}>
              {submitting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Submitting...
                </>
              ) : (
                "Confirm Submission"
              )}
            </Button>
          </div>
        )}
      />

      <WorkflowDetailsModal
        workflow={detailWorkflow}
        open={detailOpen}
        onOpenChange={(open) => {
          setDetailOpen(open)
          if (!open) {
            setDetailWorkflow(null)
          }
        }}
        loading={detailLoading}
        disableStepPagination
        hideSubmissionData
        renderWorkflowActions={(workflow) => {
          const canEditWorkflow = canProposeWorkflowEditFromWorkflow(workflow)
          const canSaveTemplate = canSaveTemplateFromWorkflow(workflow)
          if (!canEditWorkflow && !canSaveTemplate) return null

          return (
            <div className="flex w-full flex-col gap-2 sm:flex-row sm:justify-end">
              {canEditWorkflow && (
                <Button
                  className="w-full sm:w-auto"
                  onClick={() => beginWorkflowEditProposal(workflow)}
                >
                  Edit Workflow
                </Button>
              )}
              {canSaveTemplate && (
                <Button
                  className="w-full sm:w-auto"
                  variant="outline"
                  onClick={() => openSaveFromWorkflowModal(workflow)}
                  disabled={saveFromWorkflowSubmitting}
                >
                  Save as Template
                </Button>
              )}
            </div>
          )
        }}
      />

      <Dialog open={deleteConfirmOpen} onOpenChange={setDeleteConfirmOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Template?</DialogTitle>
            <DialogDescription>
              This will permanently delete this workflow template. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteConfirmOpen(false)} disabled={deleteTemplateLoading}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleConfirmDelete} disabled={deleteTemplateLoading}>
              {deleteTemplateLoading ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={saveFromWorkflowOpen}
        onOpenChange={(open) => {
          setSaveFromWorkflowOpen(open)
          if (!open) {
            setSaveFromWorkflowError("")
          }
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Save As Template</DialogTitle>
            <DialogDescription>
              Save this approved workflow structure as a reusable template.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1">
              <Label>Template Title</Label>
              <Input
                value={saveFromWorkflowTitle}
                onChange={(e) => setSaveFromWorkflowTitle(e.target.value)}
                placeholder="Template title"
              />
            </div>
            <div className="space-y-1">
              <Label>Template Description</Label>
              <Textarea
                value={saveFromWorkflowDescription}
                onChange={(e) => setSaveFromWorkflowDescription(e.target.value)}
                placeholder="Template description (optional)"
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setSaveFromWorkflowOpen(false)
                setSaveFromWorkflowError("")
              }}
              disabled={saveFromWorkflowSubmitting}
            >
              Cancel
            </Button>
            <Button onClick={saveCurrentWorkflowAsTemplate} disabled={saveFromWorkflowSubmitting}>
              {saveFromWorkflowSubmitting ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
          {saveFromWorkflowError && (
            <p className="text-sm text-red-600">{saveFromWorkflowError}</p>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
