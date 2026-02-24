"use client"

import { useEffect, useMemo, useState } from "react"
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
import { formatStatusLabel } from "@/lib/status-labels"
import { AlertTriangle, Clock, Loader2, Plus, Trash2 } from "lucide-react"
import {
  CredentialType,
  Workflow,
  WorkflowCreateRequest,
  WorkflowDeletionTargetType,
  WorkflowDropdownOptionCreateInput,
  WorkflowRecurrence,
  WorkflowTemplate,
  WorkflowTemplateCreateRequest,
  WorkflowWorkItemCreateInput,
} from "@/types/workflow"

interface DraftRole {
  client_id: string
  title: string
  required_credentials: CredentialType[]
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
  work_items: DraftWorkItem[]
}

interface DraftWorkflowManager {
  enabled: boolean
  required_credentials: CredentialType[]
  bounty: string
}

const CREDENTIAL_OPTIONS: { value: CredentialType; label: string }[] = [
  { value: "dpw_certified", label: "DPW Certified" },
  { value: "sfluv_verifier", label: "SFLuv Verifier" },
]

const createDraftRole = (): DraftRole => ({
  client_id: crypto.randomUUID(),
  title: "",
  required_credentials: ["dpw_certified"],
})

const createDraftWorkItem = (): DraftWorkItem => ({
  id: crypto.randomUUID(),
  title: "",
  description: "",
  optional: false,
  requires_photo: false,
  camera_capture_only: false,
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
  work_items: [createDraftWorkItem()],
})

const createDraftWorkflowManager = (): DraftWorkflowManager => ({
  enabled: false,
  required_credentials: ["dpw_certified"],
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
  { value: "paid_out", label: "Paid Out" },
  { value: "deleted", label: "Deleted" },
]

const nowForDatetimeLocal = () => {
  const date = new Date()
  date.setMinutes(date.getMinutes() + 60)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16)
}

const toDatetimeLocalValue = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return nowForDatetimeLocal()
  }
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16)
}

export default function ProposerPage() {
  const { user, status, authFetch } = useApp()

  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [templateSaving, setTemplateSaving] = useState(false)
  const [deletionSubmitting, setDeletionSubmitting] = useState("")
  const [activeTab, setActiveTab] = useState<"create-workflow" | "your-workflows">("create-workflow")
  const [workflowStatusFilter, setWorkflowStatusFilter] = useState<"all" | Workflow["status"]>("all")
  const [error, setError] = useState("")
  const [selectedTemplateId, setSelectedTemplateId] = useState("")
  const [templateTitle, setTemplateTitle] = useState("")
  const [templateDescription, setTemplateDescription] = useState("")

  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [recurrence, setRecurrence] = useState<WorkflowRecurrence>("one_time")
  const [startAt, setStartAt] = useState(nowForDatetimeLocal())
  const [roles, setRoles] = useState<DraftRole[]>([createDraftRole()])
  const [workflowManager, setWorkflowManager] = useState<DraftWorkflowManager>(createDraftWorkflowManager())
  const [steps, setSteps] = useState<DraftStep[]>([createDraftStep()])

  const isApproved = Boolean(user?.isProposer || user?.isAdmin)
  const canProposeDeletion = Boolean(user?.isProposer)

  const totalDraftBounty = useMemo(() => {
    const stepTotal = steps.reduce((sum, step) => sum + (Number(step.bounty) || 0), 0)
    const managerBounty = workflowManager.enabled ? Number(workflowManager.bounty) || 0 : 0
    return stepTotal + managerBounty
  }, [steps, workflowManager.bounty, workflowManager.enabled])

  const filteredWorkflows = useMemo(() => {
    if (workflowStatusFilter === "all") {
      return workflows
    }
    return workflows.filter((workflow) => workflow.status === workflowStatusFilter)
  }, [workflowStatusFilter, workflows])

  const loadData = async () => {
    if (!isApproved) {
      setLoading(false)
      return
    }

    setLoading(true)
    try {
      const [workflowsRes, templatesRes] = await Promise.all([
        authFetch("/proposers/workflows"),
        authFetch("/proposers/workflow-templates"),
      ])

      if (workflowsRes.ok) {
        const workflowsJson = await workflowsRes.json()
        setWorkflows(workflowsJson || [])
      }

      if (templatesRes.ok) {
        const templatesJson = await templatesRes.json()
        setTemplates(templatesJson || [])
      }

      setError("")
    } catch {
      setError("Unable to load proposer data right now.")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [isApproved])

  const updateRole = (roleId: string, update: Partial<DraftRole>) => {
    setRoles((prev) => prev.map((role) => (role.client_id === roleId ? { ...role, ...update } : role)))
  }

  const toggleWorkflowManagerCredential = (credential: CredentialType, checked: boolean) => {
    setWorkflowManager((prev) => {
      const hasCredential = prev.required_credentials.includes(credential)
      if (checked && !hasCredential) {
        return { ...prev, required_credentials: [...prev.required_credentials, credential] }
      }
      if (!checked && hasCredential) {
        const next = prev.required_credentials.filter((value) => value !== credential)
        return { ...prev, required_credentials: next.length ? next : [credential] }
      }
      return prev
    })
  }

  const toggleRoleCredential = (roleId: string, credential: CredentialType, checked: boolean) => {
    setRoles((prev) =>
      prev.map((role) => {
        if (role.client_id !== roleId) return role

        const hasCredential = role.required_credentials.includes(credential)
        if (checked && !hasCredential) {
          return { ...role, required_credentials: [...role.required_credentials, credential] }
        }

        if (!checked && hasCredential) {
          const next = role.required_credentials.filter((value) => value !== credential)
          return { ...role, required_credentials: next.length ? next : [credential] }
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
    if (draftOption.notify_emails.includes(email)) {
      updateDropdownOption(stepId, itemId, optionIndex, { notify_email_input: "" })
      return
    }

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

  const resetForm = () => {
    setTitle("")
    setDescription("")
    setRecurrence("one_time")
    setStartAt(nowForDatetimeLocal())
    setRoles([createDraftRole()])
    setWorkflowManager(createDraftWorkflowManager())
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

    const normalizedSteps = steps.map((step) => ({
      title: step.title.trim(),
      description: step.description.trim(),
      bounty: Number(step.bounty),
      role_client_id: step.role_client_id,
      work_items: step.work_items.map((item) => ({
        title: item.title.trim(),
        description: item.description.trim(),
        optional: item.optional,
        requires_photo: item.requires_photo,
        camera_capture_only: item.requires_photo ? item.camera_capture_only : false,
        requires_written_response: item.requires_written_response,
        requires_dropdown: item.requires_dropdown,
        dropdown_options: item.dropdown_options.map((option) => ({
          label: option.label.trim(),
          requires_written_response: option.requires_written_response,
          notify_emails: option.notify_emails
            .map((email) => email.trim().toLowerCase())
            .filter(Boolean),
        })),
      })),
    }))

    if (normalizedSteps.some((step) => !step.title || !step.description || Number.isNaN(step.bounty) || step.bounty <= 0 || !step.role_client_id)) {
      throw new Error("Every step needs a title, description, role assignment, and bounty greater than zero.")
    }

    for (const step of normalizedSteps) {
      for (const item of step.work_items) {
        if (!item.title) {
          throw new Error("Every work item needs a title.")
        }
        if (!item.requires_photo && !item.requires_written_response && !item.requires_dropdown) {
          throw new Error("Each work item must require photo, written response, or dropdown.")
        }
        if (item.requires_dropdown && item.dropdown_options.length === 0) {
          throw new Error("Dropdown work items need at least one dropdown option.")
        }
        if (item.requires_dropdown && item.dropdown_options.some((option) => !option.label)) {
          throw new Error("Each dropdown option needs a label.")
        }
      }
    }

    let normalizedManager: WorkflowCreateRequest["manager"] | undefined
    if (workflowManager.enabled) {
      const requiredCredentials = workflowManager.required_credentials
        .map((credential) => credential.trim())
        .filter(Boolean)
      if (requiredCredentials.length === 0) {
        throw new Error("Workflow manager must require at least one credential.")
      }
      const managerBounty = workflowManager.bounty.trim() === "" ? 0 : Number(workflowManager.bounty)
      if (Number.isNaN(managerBounty) || managerBounty < 0) {
        throw new Error("Workflow manager bounty must be zero or greater.")
      }
      normalizedManager = {
        required_credentials: Array.from(new Set(requiredCredentials)),
        bounty: managerBounty,
      }
    }

    return {
      normalizedRoles,
      normalizedSteps,
      normalizedManager,
    }
  }

  const buildTemplatePayload = (): WorkflowTemplateCreateRequest => {
    const { normalizedRoles, normalizedSteps, normalizedManager } = normalizeDraftWorkflowFields()
    const payload: WorkflowTemplateCreateRequest = {
      template_title: templateTitle.trim(),
      template_description: templateDescription.trim(),
      recurrence,
      start_at: startAt,
      roles: normalizedRoles,
      steps: normalizedSteps,
    }
    if (normalizedManager) {
      payload.manager = normalizedManager
    }

    return payload
  }

  const saveTemplate = async (asDefault: boolean) => {
    setError("")
    const templateTitleValue = templateTitle.trim()
    const templateDescriptionValue = templateDescription.trim()
    if (!templateTitleValue || !templateDescriptionValue) {
      setError("Template title and description are required.")
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
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save workflow template.")
    } finally {
      setTemplateSaving(false)
    }
  }

  const applyTemplate = () => {
    const template = templates.find((value) => value.id === selectedTemplateId)
    if (!template) {
      setError("Select a template to apply.")
      return
    }

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
      work_items: step.work_items.map((item) => ({
        id: crypto.randomUUID(),
        title: item.title,
        description: item.description,
        optional: item.optional,
        requires_photo: item.requires_photo,
        camera_capture_only: Boolean(item.camera_capture_only),
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

    const templateManager = template.manager
    setWorkflowManager({
      enabled: Boolean(templateManager),
      required_credentials: templateManager?.required_credentials?.length
        ? [...templateManager.required_credentials]
        : ["dpw_certified"],
      bounty: templateManager ? String(templateManager.bounty) : "",
    })
    setRecurrence(template.recurrence)
    setStartAt(toDatetimeLocalValue(template.start_at))
    setRoles(mappedRoles.length ? mappedRoles : [createDraftRole()])
    setSteps(mappedSteps.length ? mappedSteps : [createDraftStep()])
    setError("")
  }

  const submitWorkflow = async () => {
    setError("")

    if (!title.trim() || !description.trim()) {
      setError("Workflow title and description are required.")
      return
    }

    let normalizedRoles: WorkflowCreateRequest["roles"] = []
    let normalizedSteps: WorkflowCreateRequest["steps"] = []
    let normalizedManager: WorkflowCreateRequest["manager"] | undefined
    try {
      const normalized = normalizeDraftWorkflowFields()
      normalizedRoles = normalized.normalizedRoles
      normalizedSteps = normalized.normalizedSteps
      normalizedManager = normalized.normalizedManager
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to validate workflow.")
      return
    }

    const payload: WorkflowCreateRequest = {
      title: title.trim(),
      description: description.trim(),
      recurrence,
      start_at: startAt,
      roles: normalizedRoles,
      steps: normalizedSteps,
    }
    if (normalizedManager) {
      payload.manager = normalizedManager
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

  const isSeriesWorkflow = (workflow: Workflow) =>
    workflow.recurrence !== "one_time" ||
    workflows.some((candidate) => candidate.id !== workflow.id && candidate.series_id === workflow.series_id)
  const canProposeDeletionForStatus = (workflow: Workflow) =>
    workflow.status === "approved" ||
    workflow.status === "blocked" ||
    workflow.status === "in_progress" ||
    workflow.status === "completed"

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

            <div className="grid gap-3 md:grid-cols-[minmax(0,1fr),auto]">
              <Select value={selectedTemplateId} onValueChange={setSelectedTemplateId}>
                <SelectTrigger>
                  <SelectValue placeholder="Select a template" />
                </SelectTrigger>
                <SelectContent>
                  {templates.map((template) => (
                    <SelectItem key={template.id} value={template.id}>
                      {template.template_title} {template.is_default ? "(Default)" : "(Personal)"}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button type="button" variant="secondary" onClick={applyTemplate} disabled={!selectedTemplateId}>
                Apply Template
              </Button>
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
                {templateSaving ? "Saving..." : "Save Personal Template"}
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
	              <CardTitle className="text-base">Workflow Manager (Optional)</CardTitle>
	              <CardDescription>
	                Add a manager claim role with credential requirements and an optional payout when the workflow is completed.
	              </CardDescription>
	            </CardHeader>
	            <CardContent className="space-y-4">
	              <label className="flex items-center gap-2 text-sm">
	                <Checkbox
	                  checked={workflowManager.enabled}
	                  onCheckedChange={(checked) =>
	                    setWorkflowManager((prev) => ({
	                      ...prev,
	                      enabled: Boolean(checked),
	                    }))
	                  }
	                />
	                Enable Workflow Manager
	              </label>

	              {workflowManager.enabled && (
	                <div className="space-y-4">
	                  <div className="space-y-2">
	                    <Label>Required Credentials</Label>
	                    <div className="grid gap-2 sm:grid-cols-2">
	                      {CREDENTIAL_OPTIONS.map((credential) => {
	                        const checked = workflowManager.required_credentials.includes(credential.value)
	                        return (
	                          <label key={`manager-${credential.value}`} className="flex items-center gap-2 text-sm">
	                            <Checkbox
	                              checked={checked}
	                              onCheckedChange={(value) => toggleWorkflowManagerCredential(credential.value, Boolean(value))}
	                            />
	                            <span>{credential.label}</span>
	                          </label>
	                        )
	                      })}
	                    </div>
	                  </div>
	                  <div className="space-y-2">
	                    <Label>Manager Completion Payout (Optional)</Label>
	                    <Input
	                      type="number"
	                      min="0"
	                      value={workflowManager.bounty}
	                      onChange={(e) =>
	                        setWorkflowManager((prev) => ({
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
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-medium">Workflow Roles</h3>
              <Button type="button" variant="outline" onClick={() => setRoles((prev) => [...prev, createDraftRole()])}>
                <Plus className="h-4 w-4 mr-2" />
                Add Role
              </Button>
            </div>

            {roles.map((role, roleIndex) => (
              <Card key={role.client_id}>
                <CardContent className="pt-4 space-y-4">
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
                    <div className="grid gap-2 sm:grid-cols-2">
                      {CREDENTIAL_OPTIONS.map((credential) => {
                        const checked = role.required_credentials.includes(credential.value)
                        return (
                          <label key={credential.value} className="flex items-center gap-2 text-sm">
                            <Checkbox
                              checked={checked}
                              onCheckedChange={(value) => toggleRoleCredential(role.client_id, credential.value, Boolean(value))}
                            />
                            <span>{credential.label}</span>
                          </label>
                        )
                      })}
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-medium">Workflow Steps</h3>
              <Button type="button" variant="outline" onClick={() => setSteps((prev) => [...prev, createDraftStep()])}>
                <Plus className="h-4 w-4 mr-2" />
                New Step
              </Button>
            </div>

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
                        min="1"
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

                  <div className="space-y-4">
                    <div className="flex items-center justify-between">
                      <Label>Work Items</Label>
                      <Button type="button" variant="outline" size="sm" onClick={() => addWorkItem(step.id)}>
                        <Plus className="h-4 w-4 mr-1" />
                        Add Work Item
                      </Button>
                    </div>

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
                            <label className="flex items-center gap-2 text-sm">
                              <Checkbox
                                checked={item.camera_capture_only}
                                onCheckedChange={(value) =>
                                  updateWorkItem(step.id, item.id, { camera_capture_only: Boolean(value) })
                                }
                              />
                              Require live camera capture only (disallow camera roll)
                            </label>
                          )}

                          {item.requires_dropdown && (
                            <div className="space-y-3 border rounded-md p-3 bg-secondary/50">
                              <div className="flex items-center justify-between">
                                <Label className="text-sm">Dropdown Options</Label>
                                <Button type="button" variant="outline" size="sm" onClick={() => addDropdownOption(step.id, item.id)}>
                                  <Plus className="h-4 w-4 mr-1" />
                                  Add Option
                                </Button>
                              </div>

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
                                    <div className="flex gap-2">
                                      <Input
                                        value={option.notify_email_input}
                                        onChange={(e) =>
                                          updateDropdownOption(step.id, item.id, optionIndex, {
                                            notify_email_input: e.target.value,
                                          })
                                        }
                                        placeholder="name@example.com"
                                      />
                                      <Button
                                        type="button"
                                        size="sm"
                                        variant="outline"
                                        onClick={() => addDropdownOptionEmail(step.id, item.id, optionIndex)}
                                      >
                                        Add Email
                                      </Button>
                                    </div>
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
                                  </div>
                                </div>
                              ))}
                            </div>
                          )}
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

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

              {filteredWorkflows.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflows found for the selected status.</p>
              ) : (
                filteredWorkflows.map((workflow) => (
                  <Card key={workflow.id}>
                    <CardContent className="pt-4">
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
                              {formatStatusLabel(workflow.status)}
                            </Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">{workflow.description}</p>
                          <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                            <span>Start: {new Date(workflow.start_at).toLocaleString()}</span>
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
                          {(workflow.status === "pending" || workflow.status === "rejected" || workflow.status === "expired") && (
                            <Button className="w-full sm:w-auto" variant="outline" onClick={() => deleteWorkflow(workflow.id)}>
                              <Trash2 className="h-4 w-4 mr-2" />
                              Delete
                            </Button>
                          )}

                          {canProposeDeletion && canProposeDeletionForStatus(workflow) && (
                            <Button
                              className="w-full sm:w-auto"
                              variant="outline"
                              onClick={() => proposeDeletion(workflow.id, isSeriesWorkflow(workflow) ? "series" : "workflow")}
                              disabled={Boolean(deletionSubmitting)}
                            >
                              {deletionSubmitting === `${workflow.id}:${isSeriesWorkflow(workflow) ? "series" : "workflow"}`
                                ? "Submitting..."
                                : isSeriesWorkflow(workflow)
                                  ? "Propose Series Deletion"
                                  : "Propose Workflow Deletion"}
                            </Button>
                          )}
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                ))
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
