"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { AlertTriangle, CheckCircle2, ClipboardCheck, Wrench } from "lucide-react"
import { CredentialType, ImproverWorkflowFeed, Workflow, WorkflowStep } from "@/types/workflow"

type ItemFormState = {
  photos: string
  written: string
  dropdown: string
}

const defaultItemFormState: ItemFormState = {
  photos: "",
  written: "",
  dropdown: "",
}

export default function ImproverPage() {
  const { authFetch, status, user } = useApp()
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [activeCredentials, setActiveCredentials] = useState<CredentialType[]>([])
  const [error, setError] = useState<string>("")
  const [loading, setLoading] = useState<boolean>(true)
  const [submitting, setSubmitting] = useState<string>("")
  const [forms, setForms] = useState<Record<string, Record<string, ItemFormState>>>({})

  const canUsePanel = Boolean(user?.isImprover || user?.isAdmin)

  const loadFeed = useCallback(async () => {
    if (!canUsePanel) {
      setLoading(false)
      return
    }

    try {
      const res = await authFetch("/improvers/workflows")
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load improver workflows.")
      }
      const data = (await res.json()) as ImproverWorkflowFeed
      setWorkflows(data.workflows || [])
      setActiveCredentials((data.active_credentials || []) as CredentialType[])
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load improver workflows.")
    } finally {
      setLoading(false)
    }
  }, [authFetch, canUsePanel])

  useEffect(() => {
    if (status !== "authenticated") return
    loadFeed()
  }, [status, loadFeed])

  const credentialSet = useMemo(() => {
    const set = new Set<string>()
    activeCredentials.forEach((credential) => set.add(credential))
    return set
  }, [activeCredentials])

  const roleMapForWorkflow = (workflow: Workflow) => {
    const map = new Map<string, Workflow["roles"][number]>()
    workflow.roles.forEach((role) => map.set(role.id, role))
    return map
  }

  const alreadyAssignedInWorkflow = (workflow: Workflow) => {
    return workflow.steps.some((step) => step.assigned_improver_id === user?.id)
  }

  const canClaimStep = (workflow: Workflow, step: WorkflowStep) => {
    if (!user?.id) return false
    if (step.assigned_improver_id) return false
    if (step.status !== "available" && step.status !== "locked") return false
    if (alreadyAssignedInWorkflow(workflow)) return false
    if (!step.role_id) return false

    const roleMap = roleMapForWorkflow(workflow)
    const role = roleMap.get(step.role_id)
    if (!role) return false
    return role.required_credentials.every((credential) => credentialSet.has(credential))
  }

  const claimStep = async (workflowId: string, stepId: string) => {
    setSubmitting(`claim:${stepId}`)
    try {
      const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${stepId}/claim`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to claim this step.")
      }
      await loadFeed()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to claim this step.")
    } finally {
      setSubmitting("")
    }
  }

  const startStep = async (workflowId: string, stepId: string) => {
    setSubmitting(`start:${stepId}`)
    try {
      const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${stepId}/start`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to start this step.")
      }
      await loadFeed()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to start this step.")
    } finally {
      setSubmitting("")
    }
  }

  const updateItemForm = (stepId: string, itemId: string, patch: Partial<ItemFormState>) => {
    setForms((prev) => {
      const stepForms = prev[stepId] || {}
      const current = stepForms[itemId] || defaultItemFormState
      return {
        ...prev,
        [stepId]: {
          ...stepForms,
          [itemId]: {
            ...current,
            ...patch,
          },
        },
      }
    })
  }

  const buildCompletionPayload = (step: WorkflowStep) => {
    const stepForms = forms[step.id] || {}
    const items: Array<{
      item_id: string
      photo_urls: string[]
      written_response?: string
      dropdown_value?: string
    }> = []

    for (const item of step.work_items) {
      const form = stepForms[item.id] || defaultItemFormState
      const photoUrls = form.photos
        .split(",")
        .map((value) => value.trim())
        .filter(Boolean)

      const dropdownValue = form.dropdown.trim()
      const writtenResponse = form.written.trim()
      const dropdownRequiresWritten = dropdownValue ? Boolean(item.dropdown_requires_written_response?.[dropdownValue]) : false
      const requiredWritten = item.requires_written_response || dropdownRequiresWritten

      const hasAnyInput = photoUrls.length > 0 || dropdownValue.length > 0 || writtenResponse.length > 0
      if (!item.optional && !hasAnyInput) {
        throw new Error(`Missing response for required work item: ${item.title}`)
      }

      if (item.requires_photo && photoUrls.length === 0) {
        throw new Error(`Photo evidence is required for: ${item.title}`)
      }
      if (item.requires_dropdown && dropdownValue.length === 0) {
        throw new Error(`Dropdown selection is required for: ${item.title}`)
      }
      if (requiredWritten && writtenResponse.length === 0) {
        throw new Error(`Written response is required for: ${item.title}`)
      }

      if (!hasAnyInput && item.optional) {
        continue
      }

      const payloadItem: {
        item_id: string
        photo_urls: string[]
        written_response?: string
        dropdown_value?: string
      } = {
        item_id: item.id,
        photo_urls: photoUrls,
      }
      if (writtenResponse.length > 0) payloadItem.written_response = writtenResponse
      if (dropdownValue.length > 0) payloadItem.dropdown_value = dropdownValue
      items.push(payloadItem)
    }

    return { items }
  }

  const completeStep = async (workflowId: string, step: WorkflowStep) => {
    setSubmitting(`complete:${step.id}`)
    try {
      const payload = buildCompletionPayload(step)
      const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${step.id}/complete`, {
        method: "POST",
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to complete this step.")
      }
      await loadFeed()
      setForms((prev) => {
        const next = { ...prev }
        delete next[step.id]
        return next
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to complete this step.")
    } finally {
      setSubmitting("")
    }
  }

  if (status === "loading" || loading) {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (!canUsePanel) {
    return (
      <div className="container mx-auto p-4 space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Improver Access Required</CardTitle>
            <CardDescription>
              Your account is not approved for improver access yet. Request it from settings.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Improver Panel</h1>
        <p className="text-muted-foreground">Claim workflow steps, submit work evidence, and complete assigned steps.</p>
      </div>

      {error && (
        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Wrench className="h-5 w-5" />
            Active Credentials
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {activeCredentials.length === 0 ? (
            <p className="text-sm text-muted-foreground">No active credentials issued yet.</p>
          ) : (
            activeCredentials.map((credential) => (
              <Badge key={credential} variant="secondary">
                {credential}
              </Badge>
            ))
          )}
        </CardContent>
      </Card>

      {workflows.length === 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>No Active Workflows</CardTitle>
            <CardDescription>No workflows currently match your assignments or credentials.</CardDescription>
          </CardHeader>
        </Card>
      ) : (
        workflows.map((workflow) => (
          <Card key={workflow.id}>
            <CardHeader>
              <CardTitle className="flex items-center justify-between gap-3">
                <span>{workflow.title}</span>
                <Badge variant={workflow.status === "in_progress" ? "default" : "secondary"}>{workflow.status}</Badge>
              </CardTitle>
              <CardDescription>{workflow.description}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {workflow.steps.map((step) => {
                const mine = step.assigned_improver_id === user?.id
                const claimable = canClaimStep(workflow, step)

                return (
                  <Card key={step.id}>
                    <CardContent className="p-4 space-y-3">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div>
                          <h4 className="font-semibold">
                            Step {step.step_order}: {step.title}
                          </h4>
                          <p className="text-xs text-muted-foreground">{step.description}</p>
                        </div>
                        <div className="flex items-center gap-2">
                          {mine && <Badge>Assigned to you</Badge>}
                          <Badge variant="outline">{step.status}</Badge>
                          <Badge variant="secondary">Bounty: {step.bounty}</Badge>
                        </div>
                      </div>

                      {claimable && (
                        <Button
                          size="sm"
                          onClick={() => claimStep(workflow.id, step.id)}
                          disabled={Boolean(submitting)}
                        >
                          {submitting === `claim:${step.id}` ? "Claiming..." : "Claim Step"}
                        </Button>
                      )}

                      {mine && step.status === "available" && (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => startStep(workflow.id, step.id)}
                          disabled={Boolean(submitting)}
                        >
                          {submitting === `start:${step.id}` ? "Starting..." : "Start Step"}
                        </Button>
                      )}

                      {mine && (step.status === "in_progress" || step.status === "available") && (
                        <div className="space-y-4 pt-1">
                          <div className="flex items-center gap-2 text-sm font-medium">
                            <ClipboardCheck className="h-4 w-4" />
                            Work Item Responses
                          </div>

                          {step.work_items.map((item) => {
                            const form = forms[step.id]?.[item.id] || defaultItemFormState

                            return (
                              <Card key={item.id}>
                                <CardContent className="p-3 space-y-3">
                                  <div className="flex flex-wrap items-center gap-2">
                                    <p className="font-medium text-sm">{item.title}</p>
                                    {item.optional && <Badge variant="outline">Optional</Badge>}
                                  </div>
                                  <p className="text-xs text-muted-foreground">{item.description}</p>

                                  {item.requires_photo && (
                                    <div className="space-y-1">
                                      <Label className="text-xs">Photo URLs (comma-separated)</Label>
                                      <Input
                                        value={form.photos}
                                        onChange={(e) => updateItemForm(step.id, item.id, { photos: e.target.value })}
                                        placeholder="https://..."
                                      />
                                    </div>
                                  )}

                                  {item.requires_dropdown && (
                                    <div className="space-y-1">
                                      <Label className="text-xs">Dropdown Selection</Label>
                                      <Select
                                        value={form.dropdown}
                                        onValueChange={(value) => updateItemForm(step.id, item.id, { dropdown: value })}
                                      >
                                        <SelectTrigger>
                                          <SelectValue placeholder="Select an option" />
                                        </SelectTrigger>
                                        <SelectContent>
                                          {item.dropdown_options.map((option) => (
                                            <SelectItem key={option.value} value={option.value}>
                                              {option.label}
                                            </SelectItem>
                                          ))}
                                        </SelectContent>
                                      </Select>
                                    </div>
                                  )}

                                  {(item.requires_written_response ||
                                    (form.dropdown.length > 0 && Boolean(item.dropdown_requires_written_response?.[form.dropdown]))) && (
                                    <div className="space-y-1">
                                      <Label className="text-xs">Written Response</Label>
                                      <Textarea
                                        value={form.written}
                                        onChange={(e) => updateItemForm(step.id, item.id, { written: e.target.value })}
                                        placeholder="Enter your response..."
                                      />
                                    </div>
                                  )}
                                </CardContent>
                              </Card>
                            )
                          })}

                          <Button
                            size="sm"
                            onClick={() => completeStep(workflow.id, step)}
                            disabled={Boolean(submitting)}
                          >
                            {submitting === `complete:${step.id}` ? (
                              "Submitting..."
                            ) : (
                              <>
                                <CheckCircle2 className="mr-2 h-4 w-4" />
                                Complete Step
                              </>
                            )}
                          </Button>
                        </div>
                      )}

                      {step.submission && (
                        <div className="space-y-3 rounded-md border bg-secondary/30 p-3">
                          <div>
                            <p className="text-sm font-medium">Submitted Step Details</p>
                            <p className="text-xs text-muted-foreground">
                              Submitted on {new Date(step.submission.submitted_at).toLocaleString()}
                            </p>
                          </div>

                          {step.submission.item_responses.length === 0 ? (
                            <p className="text-xs text-muted-foreground">No item responses were included in this submission.</p>
                          ) : (
                            <div className="space-y-2">
                              {step.submission.item_responses.map((response, index) => {
                                const item = step.work_items.find((workItem) => workItem.id === response.item_id)
                                const dropdownLabel =
                                  response.dropdown_value && item
                                    ? item.dropdown_options.find((option) => option.value === response.dropdown_value)?.label ||
                                      response.dropdown_value
                                    : response.dropdown_value

                                return (
                                  <div key={`${response.item_id}-${index}`} className="rounded border bg-background p-2 text-xs space-y-1">
                                    <p className="font-medium">{item?.title || response.item_id}</p>
                                    {response.dropdown_value && <p>Dropdown: {dropdownLabel}</p>}
                                    {response.written_response && <p>Written: {response.written_response}</p>}
                                    {response.photo_urls.length > 0 && (
                                      <div className="space-y-1">
                                        <p>Photos:</p>
                                        {response.photo_urls.map((url) => (
                                          <a
                                            key={url}
                                            href={url}
                                            target="_blank"
                                            rel="noreferrer"
                                            className="block text-blue-600 underline break-all"
                                          >
                                            {url}
                                          </a>
                                        ))}
                                      </div>
                                    )}
                                  </div>
                                )
                              })}
                            </div>
                          )}
                        </div>
                      )}
                    </CardContent>
                  </Card>
                )
              })}
            </CardContent>
          </Card>
        ))
      )}
    </div>
  )
}
