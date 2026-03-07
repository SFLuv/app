"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
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
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
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
import { AlertTriangle, CheckCircle2, ChevronsUpDown, Clock, Loader2, Plus, Search, Trash2, X } from "lucide-react"
import {
  GlobalCredentialType,
  Workflow,
  WorkflowCreateRequest,
  WorkflowDeletionTargetType,
  WorkflowDropdownOptionCreateInput,
  WorkflowPhotoAspectRatio,
  WorkflowRecurrence,
  WorkflowTemplate,
  WorkflowTemplateCreateRequest,
  WorkflowWorkItemCreateInput,
} from "@/types/workflow"
import { Supervisor } from "@/types/supervisor"
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

interface WorkflowSeriesGroup {
  key: string
  series_id: string
  workflows: Workflow[]
}

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

const WORKFLOW_STATUS_FILTER_OPTIONS: Array<{
  value: "all" | Workflow["status"]
  label: string
}> = [
  { value: "all", label: "All Statuses" },
  { value: "pending", label: "Pending" },
  { value: "approved", label: "Approved" },
  { value: "rejected", label: "Rejected" },
  { value: "expired", label: "Expired" },
  { value: "blocked", label: "Blocked" },
  { value: "in_progress", label: "In Progress" },
  { value: "completed", label: "Completed" },
  { value: "paid_out", label: "Finalized" },
	{ value: "deleted", label: "Deleted" },
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

const nowForDatetimeLocal = () => {
  const date = new Date()
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16)
}

const toDatetimeLocalValue = (value: number) => {
  const date = new Date(value * 1000)
  if (Number.isNaN(date.getTime())) {
    return nowForDatetimeLocal()
  }
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16)
}

const toUTCISOStringFromDatetimeLocal = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    throw new Error("Workflow start date/time is invalid.")
  }
  return date.toISOString()
}

export default function ProposerPage() {
  const { user, status, authFetch } = useApp()
  const { toast } = useToast()
  const searchParams = useSearchParams()
  const pathname = usePathname()
  const router = useRouter()
  const tabFromQuery = searchParams.get("tab")
  const workflowStatusFromQuery = searchParams.get("workflow_status")
  const templateSearchFromQuery = searchParams.get("template_search")

  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([])
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [supervisors, setSupervisors] = useState<Supervisor[]>([])
  const [loading, setLoading] = useState(true)
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
  const [deleteTemplateId, setDeleteTemplateId] = useState("")
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const [deleteTemplateLoading, setDeleteTemplateLoading] = useState(false)
  const [templateTitle, setTemplateTitle] = useState("")
  const [templateDescription, setTemplateDescription] = useState("")
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailWorkflow, setDetailWorkflow] = useState<Workflow | null>(null)

  const [saveFromWorkflowOpen, setSaveFromWorkflowOpen] = useState(false)
  const [saveFromWorkflowTitle, setSaveFromWorkflowTitle] = useState("")
  const [saveFromWorkflowDescription, setSaveFromWorkflowDescription] = useState("")
  const [saveFromWorkflowError, setSaveFromWorkflowError] = useState("")
  const [saveFromWorkflowSubmitting, setSaveFromWorkflowSubmitting] = useState(false)

  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [recurrence, setRecurrence] = useState<WorkflowRecurrence>("one_time")
  const [startAt, setStartAt] = useState(nowForDatetimeLocal())
  const [roles, setRoles] = useState<DraftRole[]>([createDraftRole()])
  const [workflowSupervisor, setWorkflowSupervisor] = useState<DraftWorkflowSupervisor>(createDraftWorkflowSupervisor())
  const [steps, setSteps] = useState<DraftStep[]>([createDraftStep()])

  const isApproved = Boolean(user?.isProposer || user?.isAdmin)
  const canProposeDeletion = Boolean(user?.isProposer)

  const totalDraftBounty = useMemo(() => {
    const stepTotal = steps.reduce((sum, step) => sum + (Number(step.bounty) || 0), 0)
    const supervisorBounty = workflowSupervisor.enabled ? Number(workflowSupervisor.bounty) || 0 : 0
    return stepTotal + supervisorBounty
  }, [steps, workflowSupervisor.bounty, workflowSupervisor.enabled])

  const filteredWorkflows = useMemo(() => {
    if (workflowStatusFilter === "all") {
      return workflows
    }
    return workflows.filter((workflow) => workflow.status === workflowStatusFilter)
  }, [workflowStatusFilter, workflows])

  const workflowSeriesGroups = useMemo<WorkflowSeriesGroup[]>(() => {
    const bySeries = new Map<string, Workflow[]>()
    for (const workflow of filteredWorkflows) {
      const key = workflow.series_id?.trim() || workflow.id
      const existing = bySeries.get(key)
      if (existing) {
        existing.push(workflow)
      } else {
        bySeries.set(key, [workflow])
      }
    }

    const groups = Array.from(bySeries.entries()).map(([seriesId, items]) => {
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

    groups.sort((a, b) => {
      const latestA = a.workflows[a.workflows.length - 1]
      const latestB = b.workflows[b.workflows.length - 1]
      if (latestA.created_at !== latestB.created_at) return latestB.created_at - latestA.created_at
      return latestB.start_at - latestA.start_at
    })

    return groups
  }, [filteredWorkflows])

  const filteredTemplates = useMemo(() => {
    const s = templateSearch.trim().toLowerCase()
    if (!s) return templates
    return templates.filter((t) => t.template_title.toLowerCase().includes(s))
  }, [templates, templateSearch])

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === selectedTemplateId) ?? null,
    [templates, selectedTemplateId]
  )

  const loadData = useCallback(async () => {
    if (!isApproved) {
      setLoading(false)
      return
    }

    setLoading(true)
    try {
      const [workflowsRes, templatesRes, credentialTypesRes, supervisorsRes] = await Promise.all([
        authFetch("/proposers/workflows"),
        authFetch("/proposers/workflow-templates"),
        authFetch("/credentials/types"),
        authFetch("/supervisors/approved"),
      ])

      if (workflowsRes.ok) {
        const workflowsJson = await workflowsRes.json()
        setWorkflows(workflowsJson || [])
      }

      if (templatesRes.ok) {
        const templatesJson = await templatesRes.json()
        setTemplates(templatesJson || [])
      }

      if (credentialTypesRes.ok) {
        const credentialTypesJson = await credentialTypesRes.json()
        setCredentialTypes(credentialTypesJson || [])
      }

      if (supervisorsRes.ok) {
        const supervisorsJson = await supervisorsRes.json()
        setSupervisors(supervisorsJson || [])
      } else {
        setSupervisors([])
      }

      setError("")
    } catch {
      setError("Unable to load proposer data right now.")
    } finally {
      setLoading(false)
    }
  }, [authFetch, isApproved])

  useEffect(() => {
    loadData()
  }, [loadData])

  useEffect(() => {
    const nextTab = searchParams.get("tab")
    if (nextTab !== "create-workflow" && nextTab !== "your-workflows") return
    setActiveTab((prev) => (nextTab === prev ? prev : nextTab))
  }, [searchParams])

  useEffect(() => {
    const nextFilter = searchParams.get("workflow_status")
    if (!nextFilter) return
    if (!WORKFLOW_STATUS_FILTER_OPTIONS.some((option) => option.value === nextFilter)) return
    setWorkflowStatusFilter((prev) => (nextFilter === prev ? prev : (nextFilter as "all" | Workflow["status"])))
  }, [searchParams])

  useEffect(() => {
    const nextTemplateSearch = searchParams.get("template_search") || ""
    setTemplateSearch((prev) => (nextTemplateSearch === prev ? prev : nextTemplateSearch))
  }, [searchParams])

  useEffect(() => {
    const params = new URLSearchParams(searchParams.toString())
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
    const nextQuery = params.toString()
    if (nextQuery !== searchParams.toString()) {
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false })
    }
  }, [activeTab, templateSearch, workflowStatusFilter, pathname, router, searchParams])

  useEffect(() => {
    if (status !== "authenticated") return
    if (!isApproved) return
    void loadData()
  }, [activeTab, isApproved, loadData, status])

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
                  notify_emails: [],
                  notify_email_input: "",
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

  const resetForm = () => {
    setTitle("")
    setDescription("")
    setRecurrence("one_time")
    setStartAt(nowForDatetimeLocal())
    setRoles([createDraftRole()])
    setWorkflowSupervisor(createDraftWorkflowSupervisor())
    setSteps([createDraftStep()])
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
            notify_emails: normalizeOptionNotificationEmails(stepIndex + 1, itemIndex + 1, optionIndex + 1, option),
          })),
        }
      }),
    }))

    if (normalizedSteps.some((step) => !step.title || !step.description || Number.isNaN(step.bounty) || step.bounty < 0 || !step.role_client_id)) {
      throw new Error("Every step needs a title, description, role assignment, and bounty zero or greater.")
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
    }

    return {
      normalizedRoles,
      normalizedSteps,
      normalizedSupervisor,
    }
  }

  const buildTemplatePayload = (): WorkflowTemplateCreateRequest => {
    const { normalizedRoles, normalizedSteps, normalizedSupervisor } = normalizeDraftWorkflowFields()
    const startAtISO = toUTCISOStringFromDatetimeLocal(startAt)
    const payload: WorkflowTemplateCreateRequest = {
      template_title: templateTitle.trim(),
      template_description: templateDescription.trim(),
      recurrence,
      start_at: startAtISO,
      roles: normalizedRoles,
      steps: normalizedSteps,
    }
    if (normalizedSupervisor) {
      payload.supervisor_user_id = normalizedSupervisor.user_id
      payload.supervisor_bounty = normalizedSupervisor.bounty
    }

    return payload
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
          notify_emails: option.notify_emails || [],
          notify_email_input: "",
        })),
      })),
    }))

    const templateSupervisorUserId = (template.supervisor_user_id || "").trim()
    const templateHasSupervisor =
      templateSupervisorUserId.length > 0 ||
      (template.supervisor_bounty !== undefined && template.supervisor_bounty !== null)
    setWorkflowSupervisor({
      enabled: templateHasSupervisor,
      user_id: templateSupervisorUserId,
      bounty:
        template.supervisor_bounty !== undefined && template.supervisor_bounty !== null
          ? String(template.supervisor_bounty)
          : "",
    })
    setRecurrence(template.recurrence)
    setStartAt(toDatetimeLocalValue(template.start_at))
    setRoles(mappedRoles.length ? mappedRoles : [createDraftRole()])
    setSteps(mappedSteps.length ? mappedSteps : [createDraftStep()])
    setError("")
    setSuccessMessage(`Applied template: ${template.template_title}`)
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
    try {
      const normalized = normalizeDraftWorkflowFields()
      normalizedRoles = normalized.normalizedRoles
      normalizedSteps = normalized.normalizedSteps
      normalizedSupervisor = normalized.normalizedSupervisor
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to validate workflow.")
      return
    }

    const payload: WorkflowCreateRequest = {
      title: title.trim(),
      description: description.trim(),
      recurrence,
      start_at: toUTCISOStringFromDatetimeLocal(startAt),
      roles: normalizedRoles,
      steps: normalizedSteps,
    }
    if (normalizedSupervisor) {
      payload.supervisor = normalizedSupervisor
    }

    setSubmitting(true)
    try {
      const res = await authFetch("/proposers/workflows", {
        method: "POST",
        body: JSON.stringify(payload),
      })

      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to create workflow right now.")
      }

      const created = (await res.json()) as Workflow
      setWorkflows((prev) => [created, ...prev])
      resetForm()
      setSuccessMessage("Workflow proposal created successfully.")
      toast({
        title: "Workflow proposal created",
        description: created.title,
      })
      await loadData()
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
        throw new Error(text || "Unable to delete workflow.")
      }
      setWorkflows((prev) => prev.filter((workflow) => workflow.id !== workflowId))
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to delete workflow.")
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
      await loadData()
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
  const canProposeDeletionForStatus = (workflow: Workflow) =>
    workflow.status === "approved" ||
    workflow.status === "blocked" ||
    workflow.status === "in_progress" ||
    workflow.status === "completed"

  const canSaveTemplateFromWorkflow = (workflow: Workflow) =>
    workflow.status === "approved" ||
    workflow.status === "blocked" ||
    workflow.status === "in_progress" ||
    workflow.status === "completed" ||
    workflow.status === "paid_out"

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
      setError("Only approved workflows can be saved as templates.")
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
                notify_emails: option.notify_emails || [],
              })),
            })),
        }
      })

    const payload: WorkflowTemplateCreateRequest = {
      template_title: templateTitleValue,
      template_description: templateDescriptionValue,
      recurrence: workflow.recurrence,
      start_at: new Date(workflow.start_at * 1000).toISOString(),
      roles,
      steps,
    }

    if (workflow.supervisor_required && workflow.supervisor_user_id) {
      payload.supervisor_user_id = workflow.supervisor_user_id
      payload.supervisor_bounty = workflow.supervisor_bounty
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
      setSaveFromWorkflowError("Only approved workflows can be saved as templates.")
      return
    }

    const templateTitleValue = saveFromWorkflowTitle.trim()
    const templateDescriptionValue = saveFromWorkflowDescription.trim()
    if (!templateTitleValue) {
      setSaveFromWorkflowError("Template title is required.")
      return
    }

    let payload: WorkflowTemplateCreateRequest
    try {
      payload = buildTemplatePayloadFromWorkflow(detailWorkflow, templateTitleValue, templateDescriptionValue)
    } catch (err) {
      setSaveFromWorkflowError(err instanceof Error ? err.message : "Unable to build template from workflow.")
      return
    }

    setSaveFromWorkflowSubmitting(true)
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

  if (status === "loading" || loading) {
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
          <Card>
            <CardHeader>
              <CardTitle>Create Workflow Proposal</CardTitle>
              <CardDescription>
                Steps unlock sequentially. Each step has one assignee role and configurable work-item evidence requirements.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="flex flex-wrap items-center gap-3">
                <Badge variant="outline">Draft Total Bounty: {totalDraftBounty} SFLuv</Badge>
                <Button onClick={submitWorkflow} disabled={submitting}>
                  {submitting ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Submitting...
                    </>
                  ) : (
                    "Submit Workflow Proposal"
                  )}
                </Button>
              </div>

              <div className="space-y-4 rounded-lg border p-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-base font-medium">Template Library</h3>
                <p className="text-xs text-muted-foreground">
                  Apply a saved template to prefill workflow fields. Workflow title and description stay manual.
                </p>
              </div>
              <Badge variant="outline">{templates.length} templates</Badge>
            </div>

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
          </div>

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
              <Input type="datetime-local" value={startAt} onChange={(e) => setStartAt(e.target.value)} />
            </div>
	          </div>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Workflow Supervisor (Optional)</CardTitle>
              <CardDescription>
                Assign an approved supervisor to this workflow and optionally reserve a supervisor completion payout.
              </CardDescription>
            </CardHeader>
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
                      onChange={(e) =>
                        setWorkflowSupervisor((prev) => ({
                          ...prev,
                          bounty: e.target.value,
                        }))
                      }
                      placeholder="0"
                    />
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

		          <div className="space-y-4">
            <h3 className="text-lg font-medium">Workflow Roles</h3>

            {roles.map((role, roleIndex) => (
              <Card key={role.client_id}>
                <CardContent className="pt-5 sm:pt-6 space-y-4">
                  <div className="flex items-center justify-between">
                    <Label className="text-sm text-muted-foreground">Role {roleIndex + 1}</Label>
                    {roles.length > 1 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setRoles((prev) => prev.filter((item) => item.client_id !== role.client_id))}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </div>

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
                      <p className="text-xs text-muted-foreground">No credential types defined. Add them in the Admin panel.</p>
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
              </Card>
            ))}

            <div className="flex justify-end">
              <Button type="button" variant="outline" onClick={() => setRoles((prev) => [...prev, createDraftRole()])}>
                <Plus className="h-4 w-4 mr-2" />
                Add Role
              </Button>
            </div>
          </div>

          <div className="space-y-4">
            <h3 className="text-lg font-medium">Workflow Steps</h3>

            {steps.map((step, stepIndex) => (
              <Card key={step.id}>
                <CardHeader className="pb-3">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base">Step {stepIndex + 1}</CardTitle>
                    {steps.length > 1 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setSteps((prev) => prev.filter((item) => item.id !== step.id))}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </CardHeader>
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
                      <Label>Step Description</Label>
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

                    {step.work_items.map((item, itemIndex) => (
                      <Card key={item.id}>
                        <CardContent className="pt-4 space-y-3">
                          <div className="flex items-center justify-between">
                            <Label className="text-sm text-muted-foreground">Item {itemIndex + 1}</Label>
                            <Button type="button" variant="ghost" size="sm" onClick={() => removeWorkItem(step.id, item.id)}>
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>

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

                              {item.dropdown_options.map((option, optionIndex) => (
                                <div key={`${item.id}-${optionIndex}`} className="space-y-2 rounded-md border p-3">
                                  <div className="grid gap-2 md:grid-cols-[1fr,auto,auto]">
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
                                    <Button
                                      type="button"
                                      size="sm"
                                      variant="ghost"
                                      onClick={() => removeDropdownOption(step.id, item.id, optionIndex)}
                                    >
                                      <Trash2 className="h-4 w-4" />
                                    </Button>
                                  </div>

                                  <div className="space-y-2">
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
                                  </div>
                                </div>
                              ))}

                              <div className="flex justify-end">
                                <Button type="button" variant="outline" size="sm" onClick={() => addDropdownOption(step.id, item.id)}>
                                  <Plus className="h-4 w-4 mr-1" />
                                  Add Option
                                </Button>
                              </div>
                            </div>
                          )}
                        </CardContent>
                      </Card>
                    ))}

                    <div className="flex justify-end">
                      <Button type="button" variant="outline" size="sm" onClick={() => addWorkItem(step.id)}>
                        <Plus className="h-4 w-4 mr-1" />
                        Add Work Item
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}

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

		              <div className="flex flex-wrap items-center gap-3">
                <Badge variant="outline">Draft Total Bounty: {totalDraftBounty} SFLuv</Badge>
                <Button onClick={submitWorkflow} disabled={submitting}>
                  {submitting ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Submitting...
                    </>
                  ) : (
                    "Submit Workflow Proposal"
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="your-workflows" className="mt-4 space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Your Workflows</CardTitle>
              <CardDescription>
                Review only your submitted workflows, filter by status, and propose deletions for active workflows.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="w-full max-w-xs space-y-2">
                <Label>Filter By Approval Status</Label>
                <Select value={workflowStatusFilter} onValueChange={(value) => setWorkflowStatusFilter(value as "all" | Workflow["status"])}>
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

              {workflowSeriesGroups.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflows found for the selected status.</p>
              ) : (
                workflowSeriesGroups.map((group) => {
                  const workflow = group.workflows[group.workflows.length - 1]
                  const deletionTargetType: WorkflowDeletionTargetType = isSeriesWorkflow(workflow) ? "series" : "workflow"

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

                          {(workflow.status === "pending" || workflow.status === "rejected" || workflow.status === "expired") && (
                            <Button
                              className="w-full sm:w-auto"
                              variant="outline"
                              onClick={(event) => {
                                event.stopPropagation()
                                void deleteWorkflow(workflow.id)
                              }}
                            >
                              <Trash2 className="h-4 w-4 mr-2" />
                              Delete
                            </Button>
                          )}

                          {canProposeDeletion && canProposeDeletionForStatus(workflow) && (
                            <Button
                              className="w-full sm:w-auto"
                              variant="outline"
                              onClick={(event) => {
                                event.stopPropagation()
                                void proposeDeletion(workflow.id, deletionTargetType)
                              }}
                              disabled={Boolean(deletionSubmitting)}
                            >
                              {deletionSubmitting === `${workflow.id}:${deletionTargetType}`
                                ? "Submitting..."
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
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

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
        renderBottomActions={(workflow) =>
          canSaveTemplateFromWorkflow(workflow) ? (
            <div className="flex justify-end">
              <Button
                className="w-full sm:w-auto"
                variant="outline"
                onClick={() => openSaveFromWorkflowModal(workflow)}
                disabled={saveFromWorkflowSubmitting}
              >
                Save as Template
              </Button>
            </div>
          ) : null
        }
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
