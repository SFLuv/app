"use client"

import { ReactNode, useCallback, useEffect, useMemo, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { buildCredentialLabelMap, formatCredentialLabel } from "@/lib/credential-labels"
import { formatStatusLabel } from "@/lib/status-labels"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Separator } from "@/components/ui/separator"
import { GlobalCredentialType, Workflow, WorkflowStep, WorkflowWorkItem } from "@/types/workflow"
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react"

interface WorkflowDetailsModalProps {
  workflow: Workflow | null
  open: boolean
  onOpenChange: (open: boolean) => void
  loading?: boolean
  initialStepIndex?: number
  renderWorkflowActions?: (workflow: Workflow) => ReactNode
  renderStepActions?: (workflow: Workflow, step: WorkflowStep) => ReactNode
  hideDefaultStepDetails?: (workflow: Workflow, step: WorkflowStep) => boolean
  onDownloadPhoto?: (photoId: string) => void
  downloadingPhotoId?: string | null
}

function formatWorkItemRequirements(item: WorkflowWorkItem): string {
  const requirements: string[] = []
  if (item.requires_photo) {
    requirements.push(item.camera_capture_only ? "Photo (Live Camera Only)" : "Photo")
  }
  if (item.requires_written_response) requirements.push("Written Response")
  if (item.requires_dropdown) requirements.push("Dropdown")
  if (requirements.length === 0) return "No requirement"
  return requirements.join(" + ")
}

export function WorkflowDetailsModal({
  workflow,
  open,
  onOpenChange,
  loading = false,
  initialStepIndex = 0,
  renderWorkflowActions,
  renderStepActions,
  hideDefaultStepDetails,
  onDownloadPhoto,
  downloadingPhotoId = null,
}: WorkflowDetailsModalProps) {
  const { authFetch, status } = useApp()
  const [stepIndex, setStepIndex] = useState(0)
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])

  useEffect(() => {
    if (status !== "authenticated") return

    let isMounted = true
    const loadCredentialTypes = async () => {
      try {
        const res = await authFetch("/credentials/types")
        if (!res.ok) return
        const data = (await res.json()) as GlobalCredentialType[]
        if (isMounted) {
          setCredentialTypes(data || [])
        }
      } catch {
        // Fall back to default credential labels when type loading fails.
      }
    }

    void loadCredentialTypes()
    return () => {
      isMounted = false
    }
  }, [authFetch, status])

  const credentialLabelMap = useMemo(
    () => buildCredentialLabelMap(credentialTypes),
    [credentialTypes],
  )

  const getCredentialLabel = useCallback(
    (credential: string) => formatCredentialLabel(credential, credentialLabelMap),
    [credentialLabelMap],
  )

  const sortedSteps = useMemo(() => {
    if (!workflow) return []
    return [...workflow.steps].sort((a, b) => a.step_order - b.step_order)
  }, [workflow])

  const safeStepIndex = useMemo(() => {
    if (sortedSteps.length === 0) return 0
    return Math.min(stepIndex, sortedSteps.length - 1)
  }, [sortedSteps.length, stepIndex])

  const currentStep = sortedSteps[safeStepIndex]

  useEffect(() => {
    const stepsCount = sortedSteps.length
    if (stepsCount === 0) {
      setStepIndex(0)
      return
    }
    const normalizedInitialStepIndex = Number.isFinite(initialStepIndex)
      ? Math.max(0, Math.min(Math.floor(initialStepIndex), stepsCount - 1))
      : 0
    setStepIndex(normalizedInitialStepIndex)
  }, [workflow?.id, open, initialStepIndex, sortedSteps.length])

  const canGoPrev = safeStepIndex > 0
  const canGoNext = safeStepIndex < sortedSteps.length - 1

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[95vw] max-w-5xl max-h-[90vh] overflow-y-auto p-0">
        <div className="p-6 space-y-5">
          <DialogHeader>
            <DialogTitle>{workflow?.title || "Workflow Details"}</DialogTitle>
            <DialogDescription>
              {workflow
                ? "Review workflow details and each step page."
                : "Loading workflow details."}
            </DialogDescription>
          </DialogHeader>

          {loading && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading workflow details...
            </div>
          )}

          {!loading && workflow && (
            <>
              <div className="grid gap-2 text-xs text-muted-foreground sm:grid-cols-2 lg:grid-cols-2">
                <span>Status: {formatStatusLabel(workflow.status)}</span>
                <span>Start: {new Date(workflow.start_at * 1000).toLocaleString()}</span>
                {workflow.supervisor_required && (
                  <span>
                    Supervisor: {workflow.supervisor_title || workflow.supervisor_organization || "Assigned"}
                  </span>
                )}
              </div>

              <p className="text-sm text-muted-foreground whitespace-pre-wrap">
                {workflow.description || "No description provided."}
              </p>

              {renderWorkflowActions ? renderWorkflowActions(workflow) : null}

              <div className="space-y-2">
                <p className="text-sm font-medium">Roles</p>
                {workflow.roles.length === 0 ? (
                  <p className="text-xs text-muted-foreground">No roles configured for this workflow.</p>
                ) : (
                  <div className="space-y-2">
                    {workflow.roles.map((role) => (
                      <Card key={role.id}>
                        <CardContent className="pt-3 space-y-2">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <p className="text-sm font-medium">{role.title}</p>
                          </div>
                          <div className="flex flex-wrap gap-2">
                            {role.required_credentials.length === 0 ? (
                              <Badge variant="outline">No required credentials</Badge>
                            ) : (
                              role.required_credentials.map((credential) => (
                                <Badge key={`${role.id}-${credential}`} variant="secondary">
                                  {getCredentialLabel(credential)}
                                </Badge>
                              ))
                            )}
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                )}
              </div>

              <Separator />

              {sortedSteps.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflow steps were configured.</p>
              ) : (
                <div className="space-y-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="text-sm font-medium">
                      Step {safeStepIndex + 1} of {sortedSteps.length}
                    </div>
                    <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:items-center">
                      <Button
                        className="w-full sm:w-auto"
                        variant="outline"
                        size="sm"
                        onClick={() => setStepIndex((prev) => Math.max(prev - 1, 0))}
                        disabled={!canGoPrev}
                      >
                        <ChevronLeft className="h-4 w-4" />
                        Previous
                      </Button>
                      <Button
                        className="w-full sm:w-auto"
                        variant="outline"
                        size="sm"
                        onClick={() => setStepIndex((prev) => Math.min(prev + 1, sortedSteps.length - 1))}
                        disabled={!canGoNext}
                      >
                        Next
                        <ChevronRight className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>

                  {currentStep && (
                    <Card>
                      <CardContent className="pt-4 space-y-4">
                        {hideDefaultStepDetails?.(workflow, currentStep) !== true && (
                          <>
                            <div className="flex flex-wrap items-start justify-between gap-3">
                              <div>
                                <h3 className="font-semibold">
                                  Step {currentStep.step_order}: {currentStep.title}
                                </h3>
                                <p className="text-sm text-muted-foreground whitespace-pre-wrap">{currentStep.description}</p>
                              </div>
                              <div className="flex flex-wrap items-center gap-2">
                                <Badge variant="outline">{formatStatusLabel(currentStep.status)}</Badge>
                                <Badge variant="secondary">Bounty: {currentStep.bounty}</Badge>
                                {currentStep.assigned_improver_id && (
                                  <Badge variant="outline">Assigned: {currentStep.assigned_improver_name || "Assigned"}</Badge>
                                )}
                              </div>
                            </div>

                            <div className="space-y-3">
                              <p className="text-sm font-medium">Work Items</p>
                              {currentStep.work_items.length === 0 ? (
                                <p className="text-xs text-muted-foreground">No work items on this step.</p>
                              ) : (
                                currentStep.work_items
                                  .slice()
                                  .sort((a, b) => a.item_order - b.item_order)
                                  .map((item) => (
                                    <Card key={item.id}>
                                      <CardContent className="pt-3 space-y-2">
                                        <div className="flex flex-wrap items-center gap-2">
                                          <p className="text-sm font-medium">
                                            Item {item.item_order}: {item.title}
                                          </p>
                                          {item.optional && <Badge variant="outline">Optional</Badge>}
                                        </div>
                                        <p className="text-xs text-muted-foreground whitespace-pre-wrap">{item.description}</p>
                                        <p className="text-xs text-muted-foreground">
                                          Requirements: {formatWorkItemRequirements(item)}
                                        </p>

                                        {item.requires_dropdown && (
                                          <div className="space-y-1">
                                            <p className="text-xs font-medium">Dropdown Options</p>
                                            {item.dropdown_options.length === 0 ? (
                                              <p className="text-xs text-muted-foreground">No dropdown options configured.</p>
                                            ) : (
                                              item.dropdown_options.map((option) => (
                                                <div key={`${item.id}-${option.value}`} className="rounded border p-2 text-xs space-y-1">
                                                  <p>{option.label}</p>
                                                  {option.requires_written_response && (
                                                    <p className="text-muted-foreground">Requires written response when selected</p>
                                                  )}
                                                  {option.notify_emails && option.notify_emails.length > 0 && (
                                                    <p className="text-muted-foreground break-all">
                                                      Notify: {option.notify_emails.join(", ")}
                                                    </p>
                                                  )}
                                                  {(!option.notify_emails || option.notify_emails.length === 0) &&
                                                    (option.notify_email_count ?? 0) > 0 && (
                                                      <p className="text-muted-foreground">Notification email configured</p>
                                                  )}
                                                </div>
                                              ))
                                            )}
                                          </div>
                                        )}
                                      </CardContent>
                                    </Card>
                                  ))
                              )}
                            </div>

                            {currentStep.submission && (
                              <div className="space-y-3 rounded-md border bg-secondary/30 p-3">
                                <div>
                                  <p className="text-sm font-medium">Submitted Step Details</p>
                                  <p className="text-xs text-muted-foreground">
                                    Submitted on {new Date(currentStep.submission.submitted_at * 1000).toLocaleString()}
                                  </p>
                                </div>

                                {currentStep.submission.step_not_possible ? (
                                  <div className="rounded border bg-background p-2 text-xs space-y-1">
                                    <p className="font-medium">Step marked as not possible</p>
                                    {currentStep.submission.step_not_possible_details ? (
                                      <p className="whitespace-pre-wrap">{currentStep.submission.step_not_possible_details}</p>
                                    ) : (
                                      <p className="text-muted-foreground">No details were recorded.</p>
                                    )}
                                  </div>
                                ) : currentStep.submission.item_responses.length === 0 ? (
                                  <p className="text-xs text-muted-foreground">No item responses were included in this submission.</p>
                                ) : (
                                  <div className="space-y-2">
                                    {currentStep.submission.item_responses.map((response, index) => {
                                      const item = currentStep.work_items.find((workItem) => workItem.id === response.item_id)
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
                                          {response.photo_ids && response.photo_ids.length > 0 && (
                                            <div className="space-y-1">
                                              <p>Photos:</p>
                                              {response.photo_ids.map((photoId, photoIndex) => {
                                                const photoMeta = response.photos?.find((photo) => photo.id === photoId)
                                                return (
                                                  <div key={`${photoId}-${photoIndex}`} className="flex items-center justify-between gap-2 rounded border p-2">
                                                    <span className="break-all">
                                                      {photoMeta?.file_name || `Photo ${photoIndex + 1}`}
                                                    </span>
                                                    {onDownloadPhoto ? (
                                                      <Button
                                                        className="w-full sm:w-auto"
                                                        size="sm"
                                                        variant="outline"
                                                        onClick={() => onDownloadPhoto(photoId)}
                                                        disabled={downloadingPhotoId === photoId}
                                                      >
                                                        {downloadingPhotoId === photoId ? "Downloading..." : "Download"}
                                                      </Button>
                                                    ) : (
                                                      <Badge variant="outline">Photo ID</Badge>
                                                    )}
                                                  </div>
                                                )
                                              })}
                                            </div>
                                          )}
                                          {response.photo_urls && response.photo_urls.length > 0 && (
                                            <div className="space-y-1">
                                              <p>Legacy Photos:</p>
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
                          </>
                        )}

                        {renderStepActions && (
                          <>
                            {hideDefaultStepDetails?.(workflow, currentStep) !== true && <Separator />}
                            {renderStepActions(workflow, currentStep)}
                          </>
                        )}
                      </CardContent>
                    </Card>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
