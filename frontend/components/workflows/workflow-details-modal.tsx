"use client"

import { ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { buildCredentialLabelMap, formatCredentialLabel } from "@/lib/credential-labels"
import { formatStatusLabel } from "@/lib/status-labels"
import { formatWorkflowDisplayStatus } from "@/lib/workflow-status"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Separator } from "@/components/ui/separator"
import { GlobalCredentialType, Workflow, WorkflowStep, WorkflowStepItemResponseInput, WorkflowWorkItem } from "@/types/workflow"
import { ChevronLeft, ChevronRight, Loader2 } from "lucide-react"

interface WorkflowDetailsModalProps {
  workflow: Workflow | null
  open: boolean
  onOpenChange: (open: boolean) => void
  loading?: boolean
  initialStepIndex?: number
  renderTopRightActions?: (workflow: Workflow) => ReactNode
  renderWorkflowActions?: (workflow: Workflow) => ReactNode
  renderBottomActions?: (workflow: Workflow) => ReactNode
  renderStepActions?: (workflow: Workflow, step: WorkflowStep) => ReactNode
  hideDefaultStepDetails?: (workflow: Workflow, step: WorkflowStep) => boolean
  disableStepPagination?: boolean
  hideSubmissionData?: boolean
  onDownloadPhoto?: (photoId: string) => void
  downloadingPhotoId?: string | null
}

function formatWorkItemRequirements(item: WorkflowWorkItem): string {
  const requirements: string[] = []
  if (item.requires_photo) {
    const countLabel = item.photo_allow_any_count
      ? "Any Count"
      : `${Math.max(1, item.photo_required_count || 1)} Photo${Math.max(1, item.photo_required_count || 1) === 1 ? "" : "s"}`
    const aspectLabel =
      item.photo_aspect_ratio === "vertical"
        ? "Vertical"
        : item.photo_aspect_ratio === "horizontal"
          ? "Horizontal"
          : "Square"
    const sourceLabel = item.camera_capture_only ? "Live Camera Only" : "Camera/Upload"
    requirements.push(`Photo (${countLabel}, ${aspectLabel}, ${sourceLabel})`)
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
  renderTopRightActions,
  renderWorkflowActions,
  renderBottomActions,
  renderStepActions,
  hideDefaultStepDetails,
  disableStepPagination = false,
  hideSubmissionData = false,
  onDownloadPhoto,
  downloadingPhotoId = null,
}: WorkflowDetailsModalProps) {
  const { authFetch, status } = useApp()
  const [stepIndex, setStepIndex] = useState(0)
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [photoPreviewURLs, setPhotoPreviewURLs] = useState<Record<string, string>>({})
  const [photoPreviewLoading, setPhotoPreviewLoading] = useState<Record<string, boolean>>({})
  const [expandedPhoto, setExpandedPhoto] = useState<{ id: string; label: string } | null>(null)
  const photoPreviewURLRef = useRef<Record<string, string>>({})

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

  const clearPhotoPreviewCache = useCallback(() => {
    const urls = Object.values(photoPreviewURLRef.current)
    urls.forEach((url) => {
      try {
        URL.revokeObjectURL(url)
      } catch {
        // Ignore URL revoke errors.
      }
    })
    photoPreviewURLRef.current = {}
    setPhotoPreviewURLs({})
    setPhotoPreviewLoading({})
    setExpandedPhoto(null)
  }, [])

  const ensurePhotoPreviewURL = useCallback(
    async (photoId: string) => {
      const trimmed = photoId.trim()
      if (!trimmed) return
      if (photoPreviewURLRef.current[trimmed] || photoPreviewLoading[trimmed]) return

      setPhotoPreviewLoading((prev) => ({ ...prev, [trimmed]: true }))
      try {
        const res = await authFetch(`/workflow-photos/${trimmed}`)
        if (!res.ok) {
          return
        }
        const blob = await res.blob()
        if (!blob.type.startsWith("image/")) {
          return
        }
        const url = URL.createObjectURL(blob)
        photoPreviewURLRef.current = {
          ...photoPreviewURLRef.current,
          [trimmed]: url,
        }
        setPhotoPreviewURLs((prev) => ({
          ...prev,
          [trimmed]: url,
        }))
      } catch {
        // Keep download path available even if preview loading fails.
      } finally {
        setPhotoPreviewLoading((prev) => {
          if (!prev[trimmed]) return prev
          const next = { ...prev }
          delete next[trimmed]
          return next
        })
      }
    },
    [authFetch, photoPreviewLoading],
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
    if (!open) {
      clearPhotoPreviewCache()
    }
  }, [open, clearPhotoPreviewCache])

  useEffect(() => {
    clearPhotoPreviewCache()
  }, [workflow?.id, clearPhotoPreviewCache])

  useEffect(() => {
    return () => {
      const urls = Object.values(photoPreviewURLRef.current)
      urls.forEach((url) => {
        try {
          URL.revokeObjectURL(url)
        } catch {
          // Ignore URL revoke errors on unmount.
        }
      })
      photoPreviewURLRef.current = {}
    }
  }, [])

  useEffect(() => {
    if (hideSubmissionData) return
    if (!open || !currentStep || !currentStep.submission || currentStep.submission.step_not_possible) return
    const pendingPhotoIDs = new Set<string>()
    currentStep.submission.item_responses.forEach((response) => {
      ;(response.photo_ids || []).forEach((photoId) => {
        if (!photoId) return
        pendingPhotoIDs.add(photoId)
      })
    })
    pendingPhotoIDs.forEach((photoId) => {
      void ensurePhotoPreviewURL(photoId)
    })
  }, [currentStep, open, ensurePhotoPreviewURL])

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

  const renderStepCard = (step: WorkflowStep, includeStepActions: boolean) => {
    const itemList = step.work_items.slice().sort((a, b) => a.item_order - b.item_order)

    return (
      <Card key={step.id}>
        <CardContent className="pt-4 space-y-4">
          {hideDefaultStepDetails?.(workflow!, step) !== true && (
            <>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="font-semibold">
                    Step {step.step_order}: {step.title}
                  </h3>
                  <p className="text-sm text-muted-foreground whitespace-pre-wrap">{step.description}</p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline">{formatStatusLabel(step.status)}</Badge>
                  <Badge variant="secondary">Bounty: {step.bounty}</Badge>
                  {step.assigned_improver_id && (
                    <Badge variant="outline">Assigned: {step.assigned_improver_name || "Assigned"}</Badge>
                  )}
                </div>
              </div>

              {!hideSubmissionData && step.submission && (
                <p className="text-xs text-muted-foreground">
                  Submitted on {new Date(step.submission.submitted_at * 1000).toLocaleString()}
                </p>
              )}

              {!hideSubmissionData && step.submission?.step_not_possible && (
                <div className="rounded border bg-secondary/30 p-2.5 text-xs space-y-1">
                  <p className="font-medium">Step marked as not possible</p>
                  {step.submission.step_not_possible_details ? (
                    <p className="whitespace-pre-wrap">{step.submission.step_not_possible_details}</p>
                  ) : (
                    <p className="text-muted-foreground">No details were recorded.</p>
                  )}
                </div>
              )}

              <div className="space-y-3">
                <p className="text-sm font-medium">Work Items</p>
                {itemList.length === 0 ? (
                  <p className="text-xs text-muted-foreground">No work items on this step.</p>
                ) : (
                  itemList.map((item) => {
                    const itemResponses: WorkflowStepItemResponseInput[] =
                      !hideSubmissionData && step.submission && !step.submission.step_not_possible
                        ? step.submission.item_responses.filter((response) => response.item_id === item.id)
                        : []

                    return (
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
                                    {option.requires_photo_attachment && (
                                      <p className="text-muted-foreground">Requires photo attachment when selected</p>
                                    )}
                                    {option.photo_instructions && (
                                      <p className="text-muted-foreground whitespace-pre-wrap">
                                        Photo instructions: {option.photo_instructions}
                                      </p>
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

                          {!hideSubmissionData && step.submission && !step.submission.step_not_possible && (
                            <div className="space-y-2 rounded-md border bg-secondary/30 p-2.5">
                              <p className="text-xs font-medium">Submitted Response</p>
                              {itemResponses.length === 0 ? (
                                <p className="text-xs text-muted-foreground">
                                  No response submitted for this item.
                                </p>
                              ) : (
                                itemResponses.map((response, responseIndex) => {
                                  const dropdownLabel =
                                    response.dropdown_value
                                      ? item.dropdown_options.find((option) => option.value === response.dropdown_value)?.label ||
                                        response.dropdown_value
                                      : response.dropdown_value
                                  const hasResponseContent =
                                    Boolean(response.dropdown_value) ||
                                    Boolean(response.written_response) ||
                                    Boolean(response.photo_ids && response.photo_ids.length > 0) ||
                                    Boolean(response.photo_urls && response.photo_urls.length > 0)

                                  return (
                                    <div
                                      key={`${item.id}-submitted-${responseIndex}`}
                                      className="rounded border bg-background p-2 text-xs space-y-1"
                                    >
                                      {itemResponses.length > 1 && <p className="font-medium">Response {responseIndex + 1}</p>}
                                      {response.dropdown_value && <p>Dropdown: {dropdownLabel}</p>}
                                      {response.written_response && (
                                        <p className="whitespace-pre-wrap">Written: {response.written_response}</p>
                                      )}
                                      {response.photo_ids && response.photo_ids.length > 0 && (
                                        <div className="space-y-1">
                                          <p>Photos:</p>
                                          {response.photo_ids.map((photoId, photoIndex) => {
                                            const photoMeta = response.photos?.find((photo) => photo.id === photoId)
                                            const previewURL = photoPreviewURLs[photoId]
                                            const isPreviewLoading = Boolean(photoPreviewLoading[photoId])
                                            return (
                                              <div
                                                key={`${photoId}-${photoIndex}`}
                                                className="space-y-2 rounded border p-2"
                                              >
                                                <span className="block break-all">
                                                  {photoMeta?.file_name || `Photo ${photoIndex + 1}`}
                                                </span>
                                                <button
                                                  type="button"
                                                  className="w-full overflow-hidden rounded border bg-secondary/20"
                                                  onClick={() => {
                                                    if (!previewURL) return
                                                    setExpandedPhoto({
                                                      id: photoId,
                                                      label: photoMeta?.file_name || `Photo ${photoIndex + 1}`,
                                                    })
                                                  }}
                                                  disabled={!previewURL}
                                                >
                                                  {previewURL ? (
                                                    <img
                                                      src={previewURL}
                                                      alt={photoMeta?.file_name || `Photo ${photoIndex + 1}`}
                                                      className="h-36 w-full object-cover"
                                                    />
                                                  ) : (
                                                    <div className="flex h-24 items-center justify-center text-[11px] text-muted-foreground">
                                                      {isPreviewLoading ? "Loading preview..." : "Preview unavailable"}
                                                    </div>
                                                  )}
                                                </button>
                                                <div className="flex flex-wrap gap-2">
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
                                      {!hasResponseContent && (
                                        <p className="text-muted-foreground">No response data recorded.</p>
                                      )}
                                    </div>
                                  )
                                })
                              )}
                            </div>
                          )}
                        </CardContent>
                      </Card>
                    )
                  })
                )}
              </div>
            </>
          )}

          {includeStepActions && renderStepActions && (
            <>
              {hideDefaultStepDetails?.(workflow!, step) !== true && <Separator />}
              {renderStepActions(workflow!, step)}
            </>
          )}
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="w-[96vw] max-w-5xl max-h-[92vh] overflow-y-auto p-0">
          <div className="p-4 sm:p-6 space-y-5">
          <DialogHeader className="text-left">
            <div className="flex flex-wrap items-center gap-2">
              <DialogTitle>{workflow?.title || "Workflow Details"}</DialogTitle>
              {!loading && workflow && renderTopRightActions ? renderTopRightActions(workflow) : null}
            </div>
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
                <span>Status: {formatWorkflowDisplayStatus(workflow)}</span>
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
              ) : disableStepPagination ? (
                <div className="space-y-4">
                  <div className="text-sm font-medium">Workflow Steps ({sortedSteps.length})</div>
                  {sortedSteps.map((step) => renderStepCard(step, false))}
                </div>
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

                  {currentStep && renderStepCard(currentStep, true)}
                </div>
              )}

              {renderBottomActions ? renderBottomActions(workflow) : null}
            </>
          )}
          </div>
        </DialogContent>
      </Dialog>
      <Dialog
        open={Boolean(expandedPhoto)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            setExpandedPhoto(null)
          }
        }}
      >
        <DialogContent className="w-[95vw] max-w-3xl">
          <DialogHeader>
            <DialogTitle>{expandedPhoto?.label || "Photo Preview"}</DialogTitle>
            <DialogDescription>Preview of submitted workflow photo evidence.</DialogDescription>
          </DialogHeader>
          {expandedPhoto && photoPreviewURLs[expandedPhoto.id] ? (
            <div className="space-y-3">
              <img
                src={photoPreviewURLs[expandedPhoto.id]}
                alt={expandedPhoto.label}
                className="max-h-[70vh] w-full rounded border object-contain bg-secondary/20"
              />
              <div className="flex justify-end">
                <Button type="button" variant="outline" onClick={() => setExpandedPhoto(null)}>
                  Close
                </Button>
              </div>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Preview unavailable.</p>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
