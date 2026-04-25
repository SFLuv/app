"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Checkbox } from "@/components/ui/checkbox"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { WorkflowDetailsModal } from "@/components/workflows/workflow-details-modal"
import { buildCredentialBadgeDataUrl, buildCredentialLabelMap, formatCredentialLabel } from "@/lib/credential-labels"
import { formatStatusLabel } from "@/lib/status-labels"
import { formatWorkflowDisplayStatus } from "@/lib/workflow-status"
import { cn } from "@/lib/utils"
import { AlertTriangle, Award, CheckCircle2, ChevronLeft, ChevronRight, ChevronsUpDown, ClipboardCheck, Loader2, Search, Wrench } from "lucide-react"
import { usePathname, useSearchParams } from "next/navigation"
import { CredentialRequest } from "@/types/issuer"
import {
  CredentialVisibility,
  CredentialType,
  GlobalCredentialType,
  ImproverAbsencePeriod,
  ImproverAbsencePeriodCreateResult,
  ImproverAbsencePeriodDeleteResult,
  ImproverWorkflowFeed,
  ImproverWorkflowListItem,
  ImproverWorkflowStepSummary,
  ImproverWorkflowSeriesUnclaimResult,
  WorkflowPhotoAspectRatio,
  Workflow,
  WorkflowStepPhotoUploadResult,
  WorkflowStep,
} from "@/types/workflow"

type ItemFormState = {
  photos: File[]
  written: string
  dropdown: string
}

type CameraCaptureState = {
  open: boolean
  error: string
}

type StepNotPossibleFormState = {
  selected: boolean
  details: string
}

type LocalPhotoPreviewState = {
  url: string
  label: string
}

type PreparedWorkflowPhotoUpload = {
  file: File
  aspectRatio: WorkflowPhotoAspectRatio | null
}

type PreparedWorkflowStepCompletionItem = {
  itemId: string
  dropdownValue?: string
  writtenResponse?: string
  photoUploads: PreparedWorkflowPhotoUpload[]
}

type StepUploadProgressState = {
  uploadedUnits: number
  totalUnits: number
  completedFiles: number
  totalFiles: number
  label: string
}

type StepCompletionSuccessState = {
  workflowId: string
  stepTitle: string
}

type WorkflowSeriesCardGroup = {
  key: string
  seriesId: string
  primaryStepOrder: number | null
  primaryStepTitle: string | null
  workflows: ImproverWorkflowListItem[]
}

type WorkflowStepCompletionPayload = {
  step_not_possible: boolean
  step_not_possible_details?: string
  items: Array<{
    item_id: string
    photo_ids?: string[]
    photo_uploads?: Array<{
      file_name: string
      content_type: string
      data_base64: string
    }>
    written_response?: string
    dropdown_value?: string
  }>
}

type ImproverTab = "my-workflows" | "workflow-board" | "unpaid-workflows" | "my-badges" | "credentials" | "absence"

const isImproverTab = (value: string | null): value is ImproverTab => {
  return (
    value === "my-workflows"
    || value === "workflow-board"
    || value === "unpaid-workflows"
    || value === "my-badges"
    || value === "credentials"
    || value === "absence"
  )
}

const defaultItemFormState: ItemFormState = {
  photos: [],
  written: "",
  dropdown: "",
}

const defaultCameraCaptureState: CameraCaptureState = {
  open: false,
  error: "",
}

const defaultStepNotPossibleFormState: StepNotPossibleFormState = {
  selected: false,
  details: "",
}

const maxWorkflowPhotoUploadBytes = 2 * 1024 * 1024
const maxWorkflowPhotoUploadLabel = "2MB"
const workflowStepPhotoChunkUploadBytes = 256 * 1024
const workflowStepPhotoChunkThresholdBytes = 512 * 1024
const minWorkflowPhotoResizeDimension = 640
const maxWorkflowPhotoInitialDimension = 4096
const workflowPhotoCaptureIdealWidth = 4032
const workflowPhotoCaptureIdealHeight = 3024
const workflowPhotoCaptureFallbackMaxDimensions = [3072, 2560, 2048, 1600, 1280]
const myBadgesPageSize = 5

const formatWorkflowByteLimitLabel = (bytes: number) => {
  const inMB = bytes / (1024 * 1024)
  if (Number.isInteger(inMB)) return `${inMB}MB`
  return `${inMB.toFixed(1).replace(/\.0$/, "")}MB`
}

const createWorkflowPhotoUploadId = () => {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID()
  }
  return `workflow-photo-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

const normalizeCredentialVisibility = (value?: string | null): CredentialVisibility => {
  if (value === "private" || value === "unlisted") return value
  return "public"
}

const getAssignedStepSummaries = (workflow: ImproverWorkflowListItem): ImproverWorkflowStepSummary[] => {
  return [...(workflow.assigned_steps || [])].sort((a, b) => a.step_order - b.step_order)
}

const getPrimaryAssignedStepSummary = (workflow: ImproverWorkflowListItem): ImproverWorkflowStepSummary | null => {
  return getAssignedStepSummaries(workflow)[0] || null
}

const getInitialStepIndexForWorkflowCard = (workflow: ImproverWorkflowListItem) => {
  const assignedSteps = getAssignedStepSummaries(workflow)
  const preferred =
    assignedSteps.find((step) => step.status === "available" || step.status === "in_progress" || step.status === "locked") ||
    assignedSteps[0]
  if (!preferred) return 0
  return Math.max(0, preferred.step_order - 1)
}

const workflowPhotoAspectRatios: Record<WorkflowPhotoAspectRatio, number> = {
  vertical: 3 / 4,
  square: 1,
  horizontal: 4 / 3,
}

const normalizeWorkflowPhotoAspectRatio = (value: string): WorkflowPhotoAspectRatio => {
  const normalized = value.trim().toLowerCase()
  if (normalized === "vertical" || normalized === "horizontal" || normalized === "square") {
    return normalized
  }
  return "square"
}

const computeCropForAspectRatio = (width: number, height: number, aspectRatio: number) => {
  if (!width || !height || !Number.isFinite(width) || !Number.isFinite(height) || aspectRatio <= 0) {
    return { x: 0, y: 0, width, height }
  }

  const sourceRatio = width / height
  if (Math.abs(sourceRatio - aspectRatio) < 0.001) {
    return { x: 0, y: 0, width, height }
  }

  if (sourceRatio > aspectRatio) {
    const cropWidth = Math.max(1, Math.floor(height * aspectRatio))
    const x = Math.max(0, Math.floor((width - cropWidth) / 2))
    return { x, y: 0, width: cropWidth, height }
  }

  const cropHeight = Math.max(1, Math.floor(width / aspectRatio))
  const y = Math.max(0, Math.floor((height - cropHeight) / 2))
  return { x: 0, y, width, height: cropHeight }
}

const buildWorkflowPhotoCaptureConstraintAttempts = (aspectRatio: WorkflowPhotoAspectRatio): MediaTrackConstraints[] => {
  const normalizedAspect = normalizeWorkflowPhotoAspectRatio(aspectRatio)
  const targetAspectRatio = workflowPhotoAspectRatios[normalizedAspect]
  const withAspectRatio = (constraints: MediaTrackConstraints): MediaTrackConstraints => ({
    ...constraints,
    aspectRatio: {
      ideal: targetAspectRatio,
    },
  })

  return [
    withAspectRatio({
      facingMode: {
        ideal: "environment",
      },
      width: {
        ideal: workflowPhotoCaptureIdealWidth,
      },
      height: {
        ideal: workflowPhotoCaptureIdealHeight,
      },
    }),
    withAspectRatio({
      facingMode: {
        ideal: "environment",
      },
      width: {
        ideal: 2560,
      },
      height: {
        ideal: 1920,
      },
    }),
    withAspectRatio({
      facingMode: {
        ideal: "environment",
      },
      width: {
        ideal: 1920,
      },
      height: {
        ideal: 1440,
      },
    }),
    withAspectRatio({
      facingMode: {
        ideal: "environment",
      },
      width: {
        ideal: 1280,
      },
      height: {
        ideal: 960,
      },
    }),
    withAspectRatio({
      facingMode: {
        ideal: "environment",
      },
    }),
    {
      facingMode: {
        ideal: "environment",
      },
    },
  ]
}

const isAndroidDevice = () => {
  if (typeof navigator === "undefined") return false
  return /android/i.test(navigator.userAgent || "")
}

const getFriendlyCameraErrorMessage = (error: unknown) => {
  if (error instanceof DOMException) {
    switch (error.name) {
      case "NotAllowedError":
      case "PermissionDeniedError":
      case "SecurityError":
        return "Camera access was denied. Allow camera permission in your browser settings and try again."
      case "NotFoundError":
      case "DevicesNotFoundError":
        return "No camera was found on this device."
      case "NotReadableError":
      case "TrackStartError":
      case "AbortError":
        return "The camera could not be started. Close other apps using the camera and try again."
      case "OverconstrainedError":
      case "ConstraintNotSatisfiedError":
        return "Unable to start the camera with this device configuration."
      default:
        break
    }
  }

  if (error instanceof Error && error.message.trim() !== "") {
    return error.message
  }

  return "Unable to start the camera."
}

const getCameraPermissionState = async (): Promise<PermissionState | "unsupported"> => {
  if (typeof navigator === "undefined" || !navigator.permissions?.query) {
    return "unsupported"
  }

  try {
    const result = await navigator.permissions.query({ name: "camera" as PermissionName })
    return result.state
  } catch {
    return "unsupported"
  }
}

const buildWorkflowPhotoCaptureSizeCandidates = (width: number, height: number) => {
  const candidates: Array<{ width: number; height: number }> = []
  const seen = new Set<string>()
  const pushCandidate = (candidateWidth: number, candidateHeight: number) => {
    const normalizedWidth = Math.max(1, Math.round(candidateWidth))
    const normalizedHeight = Math.max(1, Math.round(candidateHeight))
    const key = `${normalizedWidth}x${normalizedHeight}`
    if (seen.has(key)) return
    seen.add(key)
    candidates.push({ width: normalizedWidth, height: normalizedHeight })
  }

  pushCandidate(width, height)
  const largestDimension = Math.max(width, height)
  workflowPhotoCaptureFallbackMaxDimensions.forEach((maxDimension) => {
    if (largestDimension <= maxDimension) return
    const scale = maxDimension / largestDimension
    pushCandidate(width * scale, height * scale)
  })

  return candidates
}

const loadImageFromFile = (file: File) =>
  new Promise<HTMLImageElement>((resolve, reject) => {
    const objectURL = URL.createObjectURL(file)
    const image = new Image()
    image.onload = () => {
      URL.revokeObjectURL(objectURL)
      resolve(image)
    }
    image.onerror = () => {
      URL.revokeObjectURL(objectURL)
      reject(new Error(`Unable to process image file: ${file.name}`))
    }
    image.src = objectURL
  })

const renderJpegBlob = (
  image: HTMLImageElement,
  width: number,
  height: number,
  quality: number,
  sourceCrop?: { x: number; y: number; width: number; height: number },
) =>
  new Promise<Blob | null>((resolve) => {
    const canvas = document.createElement("canvas")
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext("2d")
    if (!ctx) {
      resolve(null)
      return
    }
    if (sourceCrop) {
      ctx.imageSmoothingEnabled = true
      ctx.imageSmoothingQuality = "high"
      ctx.drawImage(
        image,
        sourceCrop.x,
        sourceCrop.y,
        sourceCrop.width,
        sourceCrop.height,
        0,
        0,
        width,
        height,
      )
    } else {
      ctx.imageSmoothingEnabled = true
      ctx.imageSmoothingQuality = "high"
      ctx.drawImage(image, 0, 0, width, height)
    }
    canvas.toBlob((blob) => resolve(blob), "image/jpeg", quality)
  })

const toJpegFileName = (fileName: string) => {
  const trimmed = fileName.trim()
  if (!trimmed) return "workflow_photo.jpg"
  const dotIndex = trimmed.lastIndexOf(".")
  if (dotIndex <= 0) return `${trimmed}.jpg`
  return `${trimmed.slice(0, dotIndex)}.jpg`
}

const isPreservableWorkflowUploadType = (contentType: string) => {
  const normalized = contentType.trim().toLowerCase()
  return normalized === "image/jpeg" || normalized === "image/jpg" || normalized === "image/png" || normalized === "image/webp"
}

const cropMatchesFullImage = (
  crop: { x: number; y: number; width: number; height: number },
  imageWidth: number,
  imageHeight: number,
) => crop.x === 0 && crop.y === 0 && crop.width === imageWidth && crop.height === imageHeight

const dateInputPattern = /^\d{4}-\d{2}-\d{2}$/

const isValidDateInput = (value: string) => dateInputPattern.test(value.trim())

const toDateInputValueFromUnix = (unixSeconds: number, endExclusive = false): string => {
  const adjustedSeconds = endExclusive ? Math.max(unixSeconds - 1, 0) : unixSeconds
  const date = new Date(adjustedSeconds * 1000)
  const year = String(date.getUTCFullYear())
  const month = String(date.getUTCMonth() + 1).padStart(2, "0")
  const day = String(date.getUTCDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

const formatDateFromUnix = (unixSeconds: number, endExclusive = false): string => {
  const adjustedSeconds = endExclusive ? Math.max(unixSeconds - 1, 0) : unixSeconds
  return new Date(adjustedSeconds * 1000).toLocaleDateString(undefined, { timeZone: "UTC" })
}

function LocalPhotoThumbnail({
  file,
  onOpen,
  disabled = false,
}: {
  file: File
  onOpen: (file: File) => void
  disabled?: boolean
}) {
  const [previewURL, setPreviewURL] = useState("")

  useEffect(() => {
    const nextURL = URL.createObjectURL(file)
    setPreviewURL(nextURL)
    return () => {
      URL.revokeObjectURL(nextURL)
    }
  }, [file])

  return (
    <button
      type="button"
      className="overflow-hidden rounded border bg-secondary/20 disabled:opacity-60"
      onClick={() => onOpen(file)}
      disabled={disabled || !previewURL}
      title={file.name}
    >
      {previewURL ? (
        <img src={previewURL} alt={file.name || "Selected photo"} className="h-24 w-full object-cover" />
      ) : (
        <div className="flex h-24 items-center justify-center text-[11px] text-muted-foreground">Loading preview...</div>
      )}
    </button>
  )
}

function ImproverTabLoadingCard({ label }: { label: string }) {
  return (
    <Card>
      <CardContent className="flex min-h-[260px] items-center justify-center">
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>{label}</span>
        </div>
      </CardContent>
    </Card>
  )
}

export default function ImproverPage() {
  const searchParams = useSearchParams()
  const pathname = usePathname()
  const tabFromQuery = searchParams.get("tab")
  const { authFetch, status, user } = useApp()
  const [workflows, setWorkflows] = useState<ImproverWorkflowListItem[]>([])
  const [unpaidWorkflows, setUnpaidWorkflows] = useState<Workflow[]>([])
  const [activeCredentials, setActiveCredentials] = useState<CredentialType[]>([])
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [credentialRequests, setCredentialRequests] = useState<CredentialRequest[]>([])
  const [credentialRequestType, setCredentialRequestType] = useState<string>("")
  const [absencePeriods, setAbsencePeriods] = useState<ImproverAbsencePeriod[]>([])
  const [error, setError] = useState<string>("")
  const [notice, setNotice] = useState<string>("")
  const [initialLoading, setInitialLoading] = useState<boolean>(true)
  const [, setWorkflowDataLoading] = useState(false)
  const [workflowDataLoaded, setWorkflowDataLoaded] = useState(false)
  const [, setUnpaidDataLoading] = useState(false)
  const [unpaidDataLoaded, setUnpaidDataLoaded] = useState(false)
  const [, setAbsenceDataLoading] = useState(false)
  const [absenceDataLoaded, setAbsenceDataLoaded] = useState(false)
  const [, setCredentialDataLoading] = useState(false)
  const [credentialDataLoaded, setCredentialDataLoaded] = useState(false)
  const [submitting, setSubmitting] = useState<string>("")
  const [forms, setForms] = useState<Record<string, Record<string, ItemFormState>>>({})
  const [stepSubmitErrors, setStepSubmitErrors] = useState<Record<string, string>>({})
  const [stepUploadProgress, setStepUploadProgress] = useState<Record<string, StepUploadProgressState>>({})
  const [stepCompletionSuccess, setStepCompletionSuccess] = useState<Record<string, StepCompletionSuccessState>>({})
  const [cameraStates, setCameraStates] = useState<Record<string, CameraCaptureState>>({})
  const [stepNotPossibleForms, setStepNotPossibleForms] = useState<Record<string, StepNotPossibleFormState>>({})
  const [localPhotoPreview, setLocalPhotoPreview] = useState<LocalPhotoPreviewState | null>(null)
  const [absenceTargetMode, setAbsenceTargetMode] = useState<"single" | "all">("single")
  const [absenceSelection, setAbsenceSelection] = useState<string>("")
  const [absenceFrom, setAbsenceFrom] = useState<string>("")
  const [absenceUntil, setAbsenceUntil] = useState<string>("")
  const [editingAbsenceId, setEditingAbsenceId] = useState<string>("")
  const [editAbsenceFrom, setEditAbsenceFrom] = useState<string>("")
  const [editAbsenceUntil, setEditAbsenceUntil] = useState<string>("")
  const [detailWorkflow, setDetailWorkflow] = useState<Workflow | null>(null)
  const [detailOpen, setDetailOpen] = useState<boolean>(false)
  const [detailLoading, setDetailLoading] = useState<boolean>(false)
  const [detailInitialStepIndex, setDetailInitialStepIndex] = useState<number>(0)
  const [detailSeriesContext, setDetailSeriesContext] = useState<{
    key: string
    seriesId: string
    stepOrder: number | null
    workflowIds: string[]
    index: number
  } | null>(null)
  const [unclaimConfirmTarget, setUnclaimConfirmTarget] = useState<{
    seriesId: string
    stepOrder: number
    workflowTitle: string
  } | null>(null)
  const [seriesCardIndexByKey, setSeriesCardIndexByKey] = useState<Record<string, number>>({})
  const [downloadingPhotoId, setDownloadingPhotoId] = useState<string | null>(null)
  const [myBadgesPage, setMyBadgesPage] = useState<number>(0)
  const [badgePreview, setBadgePreview] = useState<{ label: string; imageUrl: string } | null>(null)
  const [activeTab, setActiveTab] = useState<ImproverTab>(isImproverTab(tabFromQuery) ? tabFromQuery : "my-workflows")
  const [boardSearch, setBoardSearch] = useState<string>(searchParams.get("board_search") || "")
  const [myWorkflowsSearch, setMyWorkflowsSearch] = useState<string>(searchParams.get("my_workflows_search") || "")
  const [showOnlyActiveSeries, setShowOnlyActiveSeries] = useState<boolean>(searchParams.get("my_active_only") !== "false")
  const [unpaidSearch, setUnpaidSearch] = useState<string>(searchParams.get("unpaid_search") || "")
  const [absenceSearch, setAbsenceSearch] = useState<string>(searchParams.get("absence_search") || "")
  const [credentialComboOpen, setCredentialComboOpen] = useState<boolean>(false)
  const [credentialSearch, setCredentialSearch] = useState<string>(searchParams.get("credential_search") || "")
  const videoElementRefs = useRef<Record<string, HTMLVideoElement | null>>({})
  const cameraStreamRefs = useRef<Record<string, MediaStream | null>>({})
  const cameraVideoRefCallbacks = useRef<Record<string, (element: HTMLVideoElement | null) => void>>({})
  const cameraFileInputRefs = useRef<Record<string, HTMLInputElement | null>>({})
  const workflowDataLoadedRef = useRef(false)
  const unpaidDataLoadedRef = useRef(false)
  const absenceDataLoadedRef = useRef(false)
  const credentialDataLoadedRef = useRef(false)
  const workflowDataRequestRef = useRef<Promise<void> | null>(null)
  const unpaidDataRequestRef = useRef<Promise<void> | null>(null)
  const absenceDataRequestRef = useRef<Promise<void> | null>(null)
  const credentialDataRequestRef = useRef<Promise<void> | null>(null)

  const canUsePanel = Boolean(user?.isImprover || user?.isAdmin)

  const loadWorkflowData = useCallback(
    async (mode: "blocking" | "background" = "background") => {
      if (!canUsePanel) {
        workflowDataLoadedRef.current = true
        setWorkflowDataLoaded(true)
        return
      }
      if (workflowDataRequestRef.current) {
        return workflowDataRequestRef.current
      }

      const shouldSurfaceError = mode === "blocking" || !workflowDataLoadedRef.current
      const request = (async () => {
        setWorkflowDataLoading(true)
        try {
          const feedRes = await authFetch("/improvers/workflows")
          if (!feedRes.ok) {
            const text = await feedRes.text()
            throw new Error(text || "Unable to load improver workflows.")
          }
          const data = (await feedRes.json()) as ImproverWorkflowFeed
          setWorkflows((data.workflows || []).map((workflow) => ({
            ...workflow,
            assigned_steps: workflow.assigned_steps || [],
            claimable_step: workflow.claimable_step || null,
          })))
          setActiveCredentials((data.active_credentials || []) as CredentialType[])
          setError((prev) => (prev === "Unable to load improver workflows." ? "" : prev))
        } catch (err) {
          if (shouldSurfaceError) {
            setError(err instanceof Error ? err.message : "Unable to load improver workflows.")
          }
        } finally {
          workflowDataLoadedRef.current = true
          workflowDataRequestRef.current = null
          setWorkflowDataLoaded(true)
          setWorkflowDataLoading(false)
        }
      })()

      workflowDataRequestRef.current = request
      return request
    },
    [authFetch, canUsePanel],
  )

  const loadUnpaidData = useCallback(
    async (mode: "blocking" | "background" = "background") => {
      if (!canUsePanel) {
        unpaidDataLoadedRef.current = true
        setUnpaidDataLoaded(true)
        return
      }
      if (unpaidDataRequestRef.current) {
        return unpaidDataRequestRef.current
      }

      const shouldSurfaceError = mode === "blocking" || !unpaidDataLoadedRef.current
      const request = (async () => {
        setUnpaidDataLoading(true)
        try {
          const unpaidRes = await authFetch("/improvers/unpaid-workflows")
          if (!unpaidRes.ok) {
            const text = await unpaidRes.text()
            throw new Error(text || "Unable to load unpaid workflows.")
          }
          const unpaidData = (await unpaidRes.json()) as Workflow[]
          setUnpaidWorkflows(unpaidData || [])
          setError((prev) => (prev === "Unable to load unpaid workflows." ? "" : prev))
        } catch (err) {
          if (shouldSurfaceError) {
            setError(err instanceof Error ? err.message : "Unable to load unpaid workflows.")
          }
        } finally {
          unpaidDataLoadedRef.current = true
          unpaidDataRequestRef.current = null
          setUnpaidDataLoaded(true)
          setUnpaidDataLoading(false)
        }
      })()

      unpaidDataRequestRef.current = request
      return request
    },
    [authFetch, canUsePanel],
  )

  const loadAbsenceData = useCallback(
    async (mode: "blocking" | "background" = "background") => {
      if (!canUsePanel) {
        absenceDataLoadedRef.current = true
        setAbsenceDataLoaded(true)
        return
      }
      if (absenceDataRequestRef.current) {
        return absenceDataRequestRef.current
      }

      const shouldSurfaceError = mode === "blocking" || !absenceDataLoadedRef.current
      const request = (async () => {
        setAbsenceDataLoading(true)
        try {
          const absenceRes = await authFetch("/improvers/workflows/absence-periods")
          if (!absenceRes.ok) {
            const text = await absenceRes.text()
            throw new Error(text || "Unable to load absence coverage.")
          }
          const absenceData = (await absenceRes.json()) as ImproverAbsencePeriod[]
          setAbsencePeriods(absenceData || [])
          setError((prev) => (prev === "Unable to load absence coverage." ? "" : prev))
        } catch (err) {
          if (shouldSurfaceError) {
            setError(err instanceof Error ? err.message : "Unable to load absence coverage.")
          }
        } finally {
          absenceDataLoadedRef.current = true
          absenceDataRequestRef.current = null
          setAbsenceDataLoaded(true)
          setAbsenceDataLoading(false)
        }
      })()

      absenceDataRequestRef.current = request
      return request
    },
    [authFetch, canUsePanel],
  )

  const loadCredentialData = useCallback(
    async (mode: "blocking" | "background" = "background") => {
      if (!canUsePanel) {
        credentialDataLoadedRef.current = true
        setCredentialDataLoaded(true)
        return
      }
      if (credentialDataRequestRef.current) {
        return credentialDataRequestRef.current
      }

      const shouldSurfaceError = mode === "blocking" || !credentialDataLoadedRef.current
      const request = (async () => {
        setCredentialDataLoading(true)
        try {
          const [credentialTypesRes, credentialRequestsRes] = await Promise.all([
            authFetch("/credentials/types"),
            authFetch("/improvers/credential-requests"),
          ])

          if (credentialTypesRes.ok) {
            const typeData = (await credentialTypesRes.json()) as GlobalCredentialType[]
            setCredentialTypes(typeData || [])
          } else {
            setCredentialTypes([])
          }

          if (credentialRequestsRes.ok) {
            const requestData = (await credentialRequestsRes.json()) as CredentialRequest[]
            setCredentialRequests(requestData || [])
          } else {
            setCredentialRequests([])
          }

          setError((prev) => (prev === "Unable to load credentials right now." ? "" : prev))
        } catch {
          if (shouldSurfaceError) {
            setError("Unable to load credentials right now.")
          }
        } finally {
          credentialDataLoadedRef.current = true
          credentialDataRequestRef.current = null
          setCredentialDataLoaded(true)
          setCredentialDataLoading(false)
        }
      })()

      credentialDataRequestRef.current = request
      return request
    },
    [authFetch, canUsePanel],
  )

  const loadFeed = useCallback(async () => {
    await Promise.allSettled([
      loadWorkflowData("background"),
      loadUnpaidData("background"),
      loadAbsenceData("background"),
      loadCredentialData("background"),
    ])
  }, [loadAbsenceData, loadCredentialData, loadUnpaidData, loadWorkflowData])

  useEffect(() => {
    if (typeof window === "undefined") return
    const params = new URLSearchParams(window.location.search)
    params.set("tab", activeTab)

    if (boardSearch) params.set("board_search", boardSearch)
    else params.delete("board_search")

    if (myWorkflowsSearch) params.set("my_workflows_search", myWorkflowsSearch)
    else params.delete("my_workflows_search")

    if (!showOnlyActiveSeries) params.set("my_active_only", "false")
    else params.delete("my_active_only")

    if (unpaidSearch) params.set("unpaid_search", unpaidSearch)
    else params.delete("unpaid_search")

    if (absenceSearch) params.set("absence_search", absenceSearch)
    else params.delete("absence_search")

    if (credentialSearch) params.set("credential_search", credentialSearch)
    else params.delete("credential_search")

    const nextQuery = params.toString()
    const currentQuery = window.location.search.replace(/^\?/, "")
    if (nextQuery !== currentQuery) {
      window.history.replaceState(window.history.state, "", nextQuery ? `${pathname}?${nextQuery}` : pathname)
    }
  }, [
    absenceSearch,
    activeTab,
    boardSearch,
    credentialSearch,
    myWorkflowsSearch,
    pathname,
    showOnlyActiveSeries,
    unpaidSearch,
  ])

  useEffect(() => {
    if (status === "loading") return
    if (!canUsePanel) {
      setInitialLoading(false)
      return
    }

    const blockingTasks: Promise<void>[] = []
    const backgroundTasks: Promise<void>[] = []

    const queue = (
      loader: (mode: "blocking" | "background") => Promise<void> | undefined,
      loadedRef: { current: boolean },
      required: boolean,
    ) => {
      if (required) {
        blockingTasks.push(loader(loadedRef.current ? "background" : "blocking") ?? Promise.resolve())
      } else if (!loadedRef.current) {
        backgroundTasks.push(loader("background") ?? Promise.resolve())
      }
    }

    queue(loadWorkflowData, workflowDataLoadedRef, activeTab === "workflow-board" || activeTab === "my-workflows" || activeTab === "absence" || activeTab === "my-badges" || activeTab === "credentials")
    queue(loadCredentialData, credentialDataLoadedRef, activeTab === "my-badges" || activeTab === "credentials")
    queue(loadUnpaidData, unpaidDataLoadedRef, activeTab === "unpaid-workflows")
    queue(loadAbsenceData, absenceDataLoadedRef, activeTab === "absence")

    void Promise.all(blockingTasks).finally(() => {
      setInitialLoading(false)
    })

    if (backgroundTasks.length > 0) {
      void Promise.allSettled(backgroundTasks)
    }
  }, [activeTab, canUsePanel, loadAbsenceData, loadCredentialData, loadUnpaidData, loadWorkflowData, status])

  const credentialSet = useMemo(() => {
    const set = new Set<string>()
    activeCredentials.forEach((credential) => set.add(credential))
    return set
  }, [activeCredentials])

  const pendingCredentialRequestSet = useMemo(() => {
    const set = new Set<string>()
    credentialRequests.forEach((request) => {
      if (request.status === "pending") {
        set.add(request.credential_type)
      }
    })
    return set
  }, [credentialRequests])

  const requestableCredentialTypes = useMemo(
    () =>
      credentialTypes.filter((type) => {
        if (credentialSet.has(type.value)) return false
        return normalizeCredentialVisibility(type.visibility) === "public"
      }),
    [credentialSet, credentialTypes]
  )

  const credentialLabelMap = useMemo(
    () => buildCredentialLabelMap(credentialTypes),
    [credentialTypes],
  )

  const getCredentialLabel = useCallback(
    (credential: string) => formatCredentialLabel(credential, credentialLabelMap),
    [credentialLabelMap],
  )

  const myBadgeItems = useMemo(() => {
    const credentialTypeByValue = new Map<string, GlobalCredentialType>()
    credentialTypes.forEach((credentialType) => credentialTypeByValue.set(credentialType.value, credentialType))

    return activeCredentials
      .map((credential) => {
        const credentialType = credentialTypeByValue.get(credential)
        return {
          credential,
          label: getCredentialLabel(credential),
          badgeUrl: buildCredentialBadgeDataUrl(credentialType),
        }
      })
      .sort((a, b) => a.label.localeCompare(b.label))
  }, [activeCredentials, credentialTypes, getCredentialLabel])

  const myBadgesTotalPages = useMemo(
    () => Math.max(1, Math.ceil(myBadgeItems.length / myBadgesPageSize)),
    [myBadgeItems.length],
  )

  const paginatedMyBadgeItems = useMemo(() => {
    const start = myBadgesPage * myBadgesPageSize
    return myBadgeItems.slice(start, start + myBadgesPageSize)
  }, [myBadgeItems, myBadgesPage])

  useEffect(() => {
    if (requestableCredentialTypes.length === 0) {
      if (credentialRequestType !== "") setCredentialRequestType("")
      return
    }

    const stillValid = requestableCredentialTypes.some((type) => type.value === credentialRequestType)
    if (!stillValid) {
      setCredentialRequestType(requestableCredentialTypes[0].value)
    }
  }, [credentialRequestType, requestableCredentialTypes])

  useEffect(() => {
    setMyBadgesPage((prev) => {
      const maxPage = Math.max(0, myBadgesTotalPages - 1)
      return prev > maxPage ? maxPage : prev
    })
  }, [myBadgesTotalPages])

  type RecurringClaimOption = {
    key: string
    seriesId: string
    stepOrder: number
    stepTitle: string
    workflowTitle: string
    recurrence: Workflow["recurrence"]
    claimedCount: number
    nextStartAt?: number
  }

  const recurringClaimOptions = useMemo<RecurringClaimOption[]>(() => {
    if (!user?.id) return []

    const map = new Map<string, RecurringClaimOption>()
    workflows.forEach((workflow) => {
      if (workflow.recurrence === "one_time") return

      getAssignedStepSummaries(workflow).forEach((step) => {
        if (step.status === "paid_out") return

        const key = `${workflow.series_id}:${step.step_order}`
        const existing = map.get(key)
        if (!existing) {
          map.set(key, {
            key,
            seriesId: workflow.series_id,
            stepOrder: step.step_order,
            stepTitle: step.title,
            workflowTitle: workflow.title,
            recurrence: workflow.recurrence,
            claimedCount: 1,
            nextStartAt: workflow.start_at,
          })
          return
        }

        const nextStartAt =
          !existing.nextStartAt || workflow.start_at < existing.nextStartAt
            ? workflow.start_at
            : existing.nextStartAt

        map.set(key, {
          ...existing,
          claimedCount: existing.claimedCount + 1,
          nextStartAt,
        })
      })
    })

    return Array.from(map.values()).sort((a, b) => {
      if (a.seriesId === b.seriesId) return a.stepOrder - b.stepOrder
      return a.seriesId.localeCompare(b.seriesId)
    })
  }, [workflows, user?.id])

  useEffect(() => {
    if (recurringClaimOptions.length === 0) {
      if (absenceSelection !== "") setAbsenceSelection("")
      return
    }

    const stillValid = recurringClaimOptions.some((option) => option.key === absenceSelection)
    if (!stillValid) {
      setAbsenceSelection(recurringClaimOptions[0].key)
    }
  }, [absenceSelection, recurringClaimOptions])

  const roleMapForWorkflow = (workflow: Workflow) => {
    const map = new Map<string, Workflow["roles"][number]>()
    workflow.roles.forEach((role) => map.set(role.id, role))
    return map
  }

  const alreadyAssignedInWorkflow = (workflow: Workflow) => {
    return workflow.steps.some((step) => step.assigned_improver_id === user?.id)
  }

  const hasClaimedRoleInWorkflow = useCallback((workflow: ImproverWorkflowListItem) => {
    return workflow.has_claimed_step || (workflow.assigned_steps || []).length > 0
  }, [])

  const isWorkflowActiveForUser = useCallback((workflow: ImproverWorkflowListItem) => {
    return workflow.has_active_claimed_step || (workflow.assigned_steps || []).some(
      (step) => step.status === "available" || step.status === "in_progress",
    )
  }, [])

  const isStepCoveredByMyAbsence = (workflow: Workflow, step: WorkflowStep) => {
    if (!user?.id) return false
    if (workflow.recurrence === "one_time") return false

    const workflowStart = workflow.start_at

    return absencePeriods.some((period) => {
      if (period.series_id !== workflow.series_id) return false
      if (period.step_order !== step.step_order) return false

      return workflowStart >= period.absent_from && workflowStart < period.absent_until
    })
  }

  const canClaimStep = (workflow: Workflow, step: WorkflowStep) => {
    if (!user?.id) return false
    if (step.assigned_improver_id) return false
    if (step.status !== "available" && step.status !== "locked") return false
    if (alreadyAssignedInWorkflow(workflow)) return false
    if (isStepCoveredByMyAbsence(workflow, step)) return false
    if (!step.role_id) return false

    const roleMap = roleMapForWorkflow(workflow)
    const role = roleMap.get(step.role_id)
    if (!role) return false
    return role.required_credentials.every((credential) => credentialSet.has(credential))
  }

  const refreshDetailWorkflow = async (workflowId: string) => {
    if (!detailOpen || detailWorkflow?.id !== workflowId) return
    try {
      const workflowRes = await authFetch(`/workflows/${workflowId}`)
      if (!workflowRes.ok) return
      const refreshed = (await workflowRes.json()) as Workflow
      setDetailWorkflow(refreshed)
    } catch {
      // Keep the existing in-modal data if refresh fails.
    }
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
      await refreshDetailWorkflow(workflowId)
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
      await refreshDetailWorkflow(workflowId)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to start this step.")
    } finally {
      setSubmitting("")
    }
  }

  const parseAbsenceSelection = (selection: string) => {
    const separatorIndex = selection.lastIndexOf(":")
    if (separatorIndex <= 0) return null
    const seriesId = selection.slice(0, separatorIndex)
    const stepOrder = Number.parseInt(selection.slice(separatorIndex + 1), 10)
    if (!seriesId || Number.isNaN(stepOrder) || stepOrder <= 0) return null
    return { seriesId, stepOrder }
  }

  const createAbsencePeriodForTarget = async (
    seriesId: string,
    stepOrder: number,
    absentFromDate: string,
    absentUntilDate: string,
  ) => {
    const res = await authFetch("/improvers/workflows/absence-periods", {
      method: "POST",
      body: JSON.stringify({
        series_id: seriesId,
        step_order: stepOrder,
        absent_from: absentFromDate,
        absent_until: absentUntilDate,
      }),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || "Unable to create absence period.")
    }
    return (await res.json()) as ImproverAbsencePeriodCreateResult
  }

  const createAbsencePeriod = async () => {
    if (!absenceFrom || !absenceUntil) {
      setError("Absent period start and end dates are required.")
      setNotice("")
      return
    }
    if (!isValidDateInput(absenceFrom) || !isValidDateInput(absenceUntil)) {
      setError("Invalid absent period dates.")
      setNotice("")
      return
    }
    if (absenceFrom > absenceUntil) {
      setError("Absent end date must be on or after absent start date.")
      setNotice("")
      return
    }

    const singleTarget = parseAbsenceSelection(absenceSelection)
    if (absenceTargetMode === "single" && !singleTarget) {
      setError("Choose a recurring claim before setting an absent period.")
      setNotice("")
      return
    }
    if (absenceTargetMode === "all" && recurringClaimOptions.length === 0) {
      setError("No active recurring claimed workpieces found.")
      setNotice("")
      return
    }

    setSubmitting("absence")
    try {
      if (absenceTargetMode === "single" && singleTarget) {
        const data = await createAbsencePeriodForTarget(singleTarget.seriesId, singleTarget.stepOrder, absenceFrom, absenceUntil)
        setNotice(
          data.skipped_count > 0
            ? `Absent period created. Released ${data.released_count} assignments. ${data.skipped_count} assigned steps were already in progress or completed and were not released.`
            : `Absent period created. Released ${data.released_count} assignments for coverage.`,
        )
      } else {
        let createdCount = 0
        let releasedTotal = 0
        let skippedTotal = 0
        const failures: string[] = []

        for (const option of recurringClaimOptions) {
          const target = parseAbsenceSelection(option.key)
          if (!target) {
            failures.push("Invalid recurring claim selection.")
            continue
          }
          try {
            const data = await createAbsencePeriodForTarget(target.seriesId, target.stepOrder, absenceFrom, absenceUntil)
            createdCount += 1
            releasedTotal += data.released_count
            skippedTotal += data.skipped_count
          } catch (err) {
            failures.push(err instanceof Error ? err.message : "Unable to create absence period.")
          }
        }

        if (createdCount === 0) {
          throw new Error(failures[0] || "Unable to create absence periods.")
        }

        const successNotice =
          skippedTotal > 0
            ? `Created ${createdCount} absence period(s). Released ${releasedTotal} assignments; ${skippedTotal} assigned steps were already in progress or completed and were not released.`
            : `Created ${createdCount} absence period(s). Released ${releasedTotal} assignments for coverage.`
        const failureSuffix = failures.length > 0 ? ` ${failures.length} period(s) could not be created.` : ""
        setNotice(successNotice + failureSuffix)
      }

      setError("")
      setAbsenceFrom("")
      setAbsenceUntil("")
      await loadFeed()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to create absence period.")
      setNotice("")
    } finally {
      setSubmitting("")
    }
  }

  const beginEditAbsencePeriod = (period: ImproverAbsencePeriod) => {
    setEditingAbsenceId(period.id)
    setEditAbsenceFrom(toDateInputValueFromUnix(period.absent_from))
    setEditAbsenceUntil(toDateInputValueFromUnix(period.absent_until, true))
    setError("")
    setNotice("")
  }

  const cancelEditAbsencePeriod = () => {
    setEditingAbsenceId("")
    setEditAbsenceFrom("")
    setEditAbsenceUntil("")
  }

  const updateAbsencePeriod = async (absenceId: string) => {
    if (!editAbsenceFrom || !editAbsenceUntil) {
      setError("Absent period start and end dates are required.")
      setNotice("")
      return
    }
    if (!isValidDateInput(editAbsenceFrom) || !isValidDateInput(editAbsenceUntil)) {
      setError("Invalid absent period dates.")
      setNotice("")
      return
    }
    if (editAbsenceFrom > editAbsenceUntil) {
      setError("Absent end date must be on or after absent start date.")
      setNotice("")
      return
    }

    const submitKey = `absence-update:${absenceId}`
    setSubmitting(submitKey)
    try {
      const res = await authFetch(`/improvers/workflows/absence-periods/${absenceId}`, {
        method: "PUT",
        body: JSON.stringify({
          absent_from: editAbsenceFrom,
          absent_until: editAbsenceUntil,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to update absence period.")
      }

      const data = (await res.json()) as ImproverAbsencePeriodCreateResult
      setNotice(
        data.skipped_count > 0
          ? `Absent period updated. Released ${data.released_count} assignments. ${data.skipped_count} assigned steps were already in progress or completed and were not released.`
          : `Absent period updated. Released ${data.released_count} assignments for coverage.`,
      )
      setError("")
      cancelEditAbsencePeriod()
      await loadFeed()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to update absence period.")
      setNotice("")
    } finally {
      setSubmitting("")
    }
  }

  const deleteAbsencePeriod = async (period: ImproverAbsencePeriod) => {
    if (!window.confirm("Delete this absence period?")) {
      return
    }

    const submitKey = `absence-delete:${period.id}`
    setSubmitting(submitKey)
    try {
      const res = await authFetch(`/improvers/workflows/absence-periods/${period.id}`, {
        method: "DELETE",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to delete absence period.")
      }
      await res.json() as ImproverAbsencePeriodDeleteResult
      setNotice("Absent period deleted.")
      setError("")
      if (editingAbsenceId === period.id) {
        cancelEditAbsencePeriod()
      }
      await loadFeed()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to delete absence period.")
      setNotice("")
    } finally {
      setSubmitting("")
    }
  }

  const requestCredential = async () => {
    const credential = credentialRequestType.trim()
    if (!credential) {
      setError("Select a credential type to request.")
      setNotice("")
      return
    }
    if (pendingCredentialRequestSet.has(credential)) {
      setError("A pending request already exists for this credential.")
      setNotice("")
      return
    }

    setSubmitting("credential-request")
    try {
      const res = await authFetch("/improvers/credential-requests", {
        method: "POST",
        body: JSON.stringify({ credential_type: credential }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to submit credential request.")
      }

      setNotice(`Credential request submitted for ${getCredentialLabel(credential)}.`)
      setError("")
      await loadFeed()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to submit credential request.")
      setNotice("")
    } finally {
      setSubmitting("")
    }
  }

  const cameraKeyForItem = useCallback((stepId: string, itemId: string) => `${stepId}:${itemId}`, [])

  const updateItemForm = (stepId: string, itemId: string, patch: Partial<ItemFormState>) => {
    clearStepSubmitError(stepId)
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

  const updateStepNotPossibleForm = (stepId: string, patch: Partial<StepNotPossibleFormState>) => {
    clearStepSubmitError(stepId)
    setStepNotPossibleForms((prev) => {
      const current = prev[stepId] || defaultStepNotPossibleFormState
      return {
        ...prev,
        [stepId]: {
          ...current,
          ...patch,
        },
      }
    })
  }

  const clearStepSubmitError = useCallback((stepId: string) => {
    setStepSubmitErrors((prev) => {
      if (!prev[stepId]) return prev
      const next = { ...prev }
      delete next[stepId]
      return next
    })
  }, [])

  const setStepSubmitError = useCallback((stepId: string, message: string) => {
    setStepSubmitErrors((prev) => ({
      ...prev,
      [stepId]: message,
    }))
  }, [])

  const setCameraCaptureState = useCallback((cameraKey: string, nextState: Partial<CameraCaptureState>) => {
    setCameraStates((prev) => {
      const current = prev[cameraKey] || defaultCameraCaptureState
      return {
        ...prev,
        [cameraKey]: {
          ...current,
          ...nextState,
        },
      }
    })
  }, [])

  const closeLocalPhotoPreview = useCallback(() => {
    setLocalPhotoPreview((prev) => {
      if (prev?.url) {
        URL.revokeObjectURL(prev.url)
      }
      return null
    })
  }, [])

  const openLocalPhotoPreview = useCallback(
    (file: File) => {
      setLocalPhotoPreview((prev) => {
        if (prev?.url) {
          URL.revokeObjectURL(prev.url)
        }
        return {
          url: URL.createObjectURL(file),
          label: file.name || "Photo Preview",
        }
      })
    },
    [],
  )

  const removeItemPhoto = (stepId: string, itemId: string, photoIndex: number) => {
    clearStepSubmitError(stepId)
    setForms((prev) => {
      const stepForms = prev[stepId] || {}
      const current = stepForms[itemId] || defaultItemFormState
      const nextPhotos = current.photos.filter((_, index) => index !== photoIndex)
      return {
        ...prev,
        [stepId]: {
          ...stepForms,
          [itemId]: {
            ...current,
            photos: nextPhotos,
          },
        },
      }
    })
  }

  const addItemPhoto = (stepId: string, itemId: string, photo: File) => {
    clearStepSubmitError(stepId)
    setForms((prev) => {
      const stepForms = prev[stepId] || {}
      const current = stepForms[itemId] || defaultItemFormState
      return {
        ...prev,
        [stepId]: {
          ...stepForms,
          [itemId]: {
            ...current,
            photos: [...current.photos, photo],
          },
        },
      }
    })
  }

  const shrinkPhotoToUploadLimit = useCallback(async (
    file: File,
    aspectRatio?: WorkflowPhotoAspectRatio | null,
    maxBytes: number = maxWorkflowPhotoUploadBytes,
  ) => {
    if (!file.type.startsWith("image/")) {
      throw new Error(`Only image uploads are allowed: ${file.name}`)
    }
    const effectiveMaxBytes = Math.max(64 * 1024, Math.min(maxBytes, maxWorkflowPhotoUploadBytes))

    const image = await loadImageFromFile(file)
    const imageWidth = image.naturalWidth || image.width
    const imageHeight = image.naturalHeight || image.height
    if (!imageWidth || !imageHeight) {
      throw new Error(`Unable to process image dimensions for ${file.name}`)
    }

    const normalizedAspect = aspectRatio ? normalizeWorkflowPhotoAspectRatio(aspectRatio) : null
    const crop = normalizedAspect
      ? computeCropForAspectRatio(imageWidth, imageHeight, workflowPhotoAspectRatios[normalizedAspect])
      : { x: 0, y: 0, width: imageWidth, height: imageHeight }

    if (
      file.size <= effectiveMaxBytes &&
      cropMatchesFullImage(crop, imageWidth, imageHeight) &&
      isPreservableWorkflowUploadType(file.type)
    ) {
      return file
    }

    let targetWidth = crop.width
    let targetHeight = crop.height
    const largestDimension = Math.max(targetWidth, targetHeight)
    if (largestDimension > maxWorkflowPhotoInitialDimension) {
      const initialScale = maxWorkflowPhotoInitialDimension / largestDimension
      targetWidth = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetWidth * initialScale))
      targetHeight = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetHeight * initialScale))
    }

    const encodeBestJpegBlobForCurrentSize = async () => {
      const highestQualityBlob = await renderJpegBlob(image, targetWidth, targetHeight, 0.99, crop)
      if (highestQualityBlob && highestQualityBlob.size <= effectiveMaxBytes) {
        return highestQualityBlob
      }

      const lowestQualityBlob = await renderJpegBlob(image, targetWidth, targetHeight, 0.5, crop)
      if (!lowestQualityBlob || lowestQualityBlob.size > effectiveMaxBytes) {
        return null
      }

      let bestBlob = lowestQualityBlob
      let lowQuality = 0.5
      let highQuality = 0.99
      for (let attempt = 0; attempt < 7; attempt += 1) {
        const midQuality = (lowQuality + highQuality) / 2
        const blob = await renderJpegBlob(image, targetWidth, targetHeight, midQuality, crop)
        if (!blob) {
          highQuality = midQuality
          continue
        }
        if (blob.size <= effectiveMaxBytes) {
          bestBlob = blob
          lowQuality = midQuality
        } else {
          highQuality = midQuality
        }
      }

      return bestBlob
    }

    for (let scaleAttempt = 0; scaleAttempt < 7; scaleAttempt += 1) {
      const blob = await encodeBestJpegBlobForCurrentSize()
      if (blob) {
        return new File([blob], toJpegFileName(file.name), {
          type: "image/jpeg",
          lastModified: Date.now(),
        })
      }

      if (targetWidth <= minWorkflowPhotoResizeDimension && targetHeight <= minWorkflowPhotoResizeDimension) {
        break
      }
      targetWidth = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetWidth * 0.9))
      targetHeight = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetHeight * 0.9))
    }

    throw new Error(`Unable to downsize ${file.name} below ${formatWorkflowByteLimitLabel(effectiveMaxBytes)}.`)
  }, [])

  const prepareSelectedPhotos = useCallback(
    async (files: File[], aspectRatio?: WorkflowPhotoAspectRatio | null) => {
      const processed: File[] = []
      for (const file of files) {
        processed.push(await shrinkPhotoToUploadLimit(file, aspectRatio))
      }
      return processed
    },
    [shrinkPhotoToUploadLimit],
  )

  const replaceItemPhotosFromSelection = useCallback(
    async (stepId: string, itemId: string, aspectRatio: WorkflowPhotoAspectRatio | null, selectedFiles: FileList | null) => {
      const files = Array.from(selectedFiles || [])
      if (files.length === 0) {
        return
      }

      try {
        const prepared = await prepareSelectedPhotos(files, aspectRatio)
        updateItemForm(stepId, itemId, { photos: prepared })
        clearStepSubmitError(stepId)
      } catch (err) {
        setStepSubmitError(
          stepId,
          err instanceof Error ? err.message : `Unable to process uploaded photos to fit ${maxWorkflowPhotoUploadLabel}.`,
        )
      }
    },
    [clearStepSubmitError, prepareSelectedPhotos, setStepSubmitError],
  )

  const stopCameraStreamByKey = useCallback((cameraKey: string) => {
    const stream = cameraStreamRefs.current[cameraKey]
    if (stream) {
      stream.getTracks().forEach((track) => track.stop())
      cameraStreamRefs.current[cameraKey] = null
    }
    const videoElement = videoElementRefs.current[cameraKey]
    if (videoElement) {
      videoElement.srcObject = null
    }
  }, [])

  const stopCameraCapture = useCallback(
    (stepId: string, itemId: string) => {
      const cameraKey = cameraKeyForItem(stepId, itemId)
      stopCameraStreamByKey(cameraKey)
      setCameraStates((prev) => {
        if (!prev[cameraKey]) return prev
        return {
          ...prev,
          [cameraKey]: {
            ...prev[cameraKey],
            open: false,
          },
        }
      })
    },
    [cameraKeyForItem, stopCameraStreamByKey],
  )

  const stopCameraCapturesForStep = useCallback(
    (stepId: string) => {
      const prefix = `${stepId}:`
      const keys = Object.keys(cameraStreamRefs.current).filter((key) => key.startsWith(prefix))
      if (keys.length === 0) return
      keys.forEach((key) => stopCameraStreamByKey(key))
      setCameraStates((prev) => {
        const next = { ...prev }
        keys.forEach((key) => {
          delete next[key]
          delete cameraVideoRefCallbacks.current[key]
        })
        return next
      })
    },
    [stopCameraStreamByKey],
  )

  const stopAllCameraCaptures = useCallback(() => {
    Object.keys(cameraStreamRefs.current).forEach((key) => stopCameraStreamByKey(key))
    cameraVideoRefCallbacks.current = {}
    setCameraStates({})
  }, [stopCameraStreamByKey])

  const attachCameraVideoRef = useCallback(
    (cameraKey: string, element: HTMLVideoElement | null) => {
      videoElementRefs.current[cameraKey] = element
      if (!element) {
        stopCameraStreamByKey(cameraKey)
        setCameraStates((prev) => {
          if (!prev[cameraKey]) return prev
          return {
            ...prev,
            [cameraKey]: {
              ...prev[cameraKey],
              open: false,
            },
          }
        })
        return
      }

      const stream = cameraStreamRefs.current[cameraKey]
      if (stream) {
        element.srcObject = stream
        void element.play().catch(() => undefined)
      }
    },
    [stopCameraStreamByKey],
  )

  const getCameraVideoRef = useCallback(
    (cameraKey: string) => {
      const existing = cameraVideoRefCallbacks.current[cameraKey]
      if (existing) return existing

      const callback = (element: HTMLVideoElement | null) => {
        attachCameraVideoRef(cameraKey, element)
      }
      cameraVideoRefCallbacks.current[cameraKey] = callback
      return callback
    },
    [attachCameraVideoRef],
  )

  const attachCameraFileInputRef = useCallback((cameraKey: string, element: HTMLInputElement | null) => {
    if (element) {
      cameraFileInputRefs.current[cameraKey] = element
      return
    }
    delete cameraFileInputRefs.current[cameraKey]
  }, [])

  const triggerCameraFileInput = useCallback((cameraKey: string) => {
    const input = cameraFileInputRefs.current[cameraKey]
    if (!input) return false
    input.value = ""
    input.click()
    return true
  }, [])

  const appendItemPhotosFromSelection = useCallback(
    async (
      stepId: string,
      itemId: string,
      aspectRatio: WorkflowPhotoAspectRatio | null,
      selectedFiles: FileList | null,
      onError?: (message: string) => void,
    ) => {
      const files = Array.from(selectedFiles || [])
      if (files.length === 0) return

      try {
        const prepared = await prepareSelectedPhotos(files, aspectRatio)
        clearStepSubmitError(stepId)
        setForms((prev) => {
          const stepForms = prev[stepId] || {}
          const current = stepForms[itemId] || defaultItemFormState
          return {
            ...prev,
            [stepId]: {
              ...stepForms,
              [itemId]: {
                ...current,
                photos: [...current.photos, ...prepared],
              },
            },
          }
        })
      } catch (err) {
        const message =
          err instanceof Error ? err.message : `Unable to process uploaded photos to fit ${maxWorkflowPhotoUploadLabel}.`
        if (onError) {
          onError(message)
          return
        }
        setStepSubmitError(stepId, message)
      }
    },
    [clearStepSubmitError, prepareSelectedPhotos, setStepSubmitError],
  )

  const startCameraCapture = async (stepId: string, itemId: string, aspectRatio: WorkflowPhotoAspectRatio) => {
    const cameraKey = cameraKeyForItem(stepId, itemId)
    clearStepSubmitError(stepId)

    if (isAndroidDevice()) {
      if (triggerCameraFileInput(cameraKey)) {
        setCameraCaptureState(cameraKey, {
          open: false,
          error: "",
        })
        return
      }
    }

    if (typeof navigator === "undefined" || !navigator.mediaDevices?.getUserMedia) {
      if (triggerCameraFileInput(cameraKey)) {
        setCameraCaptureState(cameraKey, {
          open: false,
          error: "",
        })
        return
      }
      setCameraCaptureState(cameraKey, {
        open: false,
        error: "Camera capture is not supported in this browser.",
      })
      return
    }

    stopCameraStreamByKey(cameraKey)

    try {
      const permissionState = await getCameraPermissionState()
      if (permissionState === "denied") {
        setCameraCaptureState(cameraKey, {
          open: false,
          error: "Camera access is blocked. Enable camera permission in your browser settings and try again.",
        })
        return
      }

      let stream: MediaStream | null = null
      let lastError: unknown = null
      for (const videoConstraints of buildWorkflowPhotoCaptureConstraintAttempts(aspectRatio)) {
        try {
          stream = await navigator.mediaDevices.getUserMedia({
            video: videoConstraints,
            audio: false,
          })
          break
        } catch (error) {
          lastError = error
        }
      }
      if (!stream) {
        throw lastError instanceof Error ? lastError : new Error("Unable to start camera.")
      }

      cameraStreamRefs.current[cameraKey] = stream
      setCameraCaptureState(cameraKey, {
        open: true,
        error: "",
      })

      const videoElement = videoElementRefs.current[cameraKey]
      if (videoElement) {
        videoElement.srcObject = stream
        void videoElement.play().catch((playError) => {
          setCameraCaptureState(cameraKey, {
            open: false,
            error: getFriendlyCameraErrorMessage(playError),
          })
          stopCameraStreamByKey(cameraKey)
        })
      }
    } catch (error) {
      setCameraCaptureState(cameraKey, {
        open: false,
        error: getFriendlyCameraErrorMessage(error),
      })
    }
  }

  const captureCameraPhoto = async (
    stepId: string,
    itemId: string,
    itemTitle: string,
    aspectRatio: WorkflowPhotoAspectRatio,
  ) => {
    const cameraKey = cameraKeyForItem(stepId, itemId)
    try {
      const stream = cameraStreamRefs.current[cameraKey]
      const videoElement = videoElementRefs.current[cameraKey]
      if (!stream || !videoElement) {
        setCameraCaptureState(cameraKey, {
          open: false,
          error: "Open your camera before capturing a photo.",
        })
        return
      }

      const width = videoElement.videoWidth
      const height = videoElement.videoHeight
      if (!width || !height) {
        setCameraCaptureState(cameraKey, {
          open: true,
          error: "Camera is still initializing. Try capture again.",
        })
        return
      }

      const normalizedAspect = normalizeWorkflowPhotoAspectRatio(aspectRatio)
      const targetAspect = workflowPhotoAspectRatios[normalizedAspect]
      const crop = computeCropForAspectRatio(width, height, targetAspect)

      const slug = itemTitle
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "_")
        .replace(/^_+|_+$/g, "")
      const filenamePrefix = slug || "workflow_photo"
      let preparedPhoto: File | null = null
      let lastProcessingError = "Unable to capture photo from the camera stream."
      for (const size of buildWorkflowPhotoCaptureSizeCandidates(crop.width, crop.height)) {
        try {
          const canvas = document.createElement("canvas")
          canvas.width = size.width
          canvas.height = size.height
          const ctx = canvas.getContext("2d")
          if (!ctx) {
            lastProcessingError = "Unable to capture photo from the camera stream."
            break
          }

          ctx.imageSmoothingEnabled = true
          ctx.imageSmoothingQuality = "high"
          ctx.drawImage(videoElement, crop.x, crop.y, crop.width, crop.height, 0, 0, size.width, size.height)
          const blob = await new Promise<Blob | null>((resolve) => {
            canvas.toBlob((value) => resolve(value), "image/jpeg", 0.98)
          })
          if (!blob) {
            lastProcessingError = "Unable to capture photo from the camera stream."
            continue
          }

          const photo = new File([blob], `${filenamePrefix}_${Date.now()}.jpg`, {
            type: blob.type || "image/jpeg",
            lastModified: Date.now(),
          })

          preparedPhoto = await shrinkPhotoToUploadLimit(photo, normalizedAspect)
          break
        } catch (err) {
          lastProcessingError =
            err instanceof Error ? err.message : `Unable to process captured photo for ${maxWorkflowPhotoUploadLabel} limit.`
        }
      }

      if (!preparedPhoto) {
        setCameraCaptureState(cameraKey, {
          open: true,
          error: lastProcessingError,
        })
        return
      }

      addItemPhoto(stepId, itemId, preparedPhoto)
      stopCameraStreamByKey(cameraKey)
      setCameraCaptureState(cameraKey, {
        open: false,
        error: "",
      })
    } catch (error) {
      setCameraCaptureState(cameraKey, {
        open: true,
        error: getFriendlyCameraErrorMessage(error),
      })
    }
  }

  useEffect(() => {
    return () => {
      stopAllCameraCaptures()
      closeLocalPhotoPreview()
    }
  }, [stopAllCameraCaptures, closeLocalPhotoPreview])

  useEffect(() => {
    if (!detailOpen) {
      stopAllCameraCaptures()
      setStepUploadProgress({})
      setStepCompletionSuccess({})
    }
  }, [detailOpen, stopAllCameraCaptures])

  useEffect(() => {
    setStepUploadProgress({})
    setStepCompletionSuccess({})
  }, [detailWorkflow?.id])

  const uploadPreparedWorkflowPhoto = useCallback(async (
    workflowId: string,
    stepId: string,
    itemId: string,
    upload: PreparedWorkflowPhotoUpload,
    onUnitUploaded?: () => void,
  ) => {
    if (upload.file.size > workflowStepPhotoChunkThresholdBytes) {
      const uploadId = createWorkflowPhotoUploadId()
      const totalChunks = Math.max(1, Math.ceil(upload.file.size / workflowStepPhotoChunkUploadBytes))
      let finalPhotoId = ""

      for (let chunkIndex = 0; chunkIndex < totalChunks; chunkIndex += 1) {
        const start = chunkIndex * workflowStepPhotoChunkUploadBytes
        const end = Math.min(upload.file.size, start + workflowStepPhotoChunkUploadBytes)
        const chunkBlob = upload.file.slice(start, end)
        const formData = new FormData()
        formData.set("item_id", itemId)
        formData.set("upload_id", uploadId)
        formData.set("chunk_index", String(chunkIndex))
        formData.set("total_chunks", String(totalChunks))
        formData.set("file_name", upload.file.name || "photo.jpg")
        formData.set("content_type", upload.file.type || "image/jpeg")
        formData.set("chunk", chunkBlob, upload.file.name || `chunk-${chunkIndex}.part`)

        const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${stepId}/photos`, {
          method: "POST",
          body: formData,
        })
        if (!res.ok) {
          const text = await res.text()
          throw new Error(text || "Unable to upload photo for this step.")
        }

        const json = await res.json() as WorkflowStepPhotoUploadResult
        onUnitUploaded?.()
        if (json.complete && json.photo?.id) {
          finalPhotoId = json.photo.id
        }
      }

      if (!finalPhotoId) {
        throw new Error("Uploaded workflow photo did not finalize correctly.")
      }
      return finalPhotoId
    }

    const formData = new FormData()
    formData.set("item_id", itemId)
    formData.set("photo", upload.file, upload.file.name || "photo.jpg")

    const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${stepId}/photos`, {
      method: "POST",
      body: formData,
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || "Unable to upload photo for this step.")
    }

    const json = await res.json() as WorkflowStepPhotoUploadResult
    onUnitUploaded?.()
    if (!json.photo?.id) {
      throw new Error("Uploaded workflow photo is missing an id.")
    }
    return json.photo.id
  }, [authFetch])

  const mergeWorkflowIntoFeed = useCallback((updatedWorkflow: Workflow) => {
    setUnpaidWorkflows((prev) => {
      const exists = prev.some((workflow) => workflow.id === updatedWorkflow.id)
      if (!exists) return prev
      return prev.map((workflow) => (workflow.id === updatedWorkflow.id ? updatedWorkflow : workflow))
    })
    if (detailOpen && detailWorkflow?.id === updatedWorkflow.id) {
      setDetailWorkflow(updatedWorkflow)
    }
  }, [detailOpen, detailWorkflow?.id])

  const buildCompletionPayload = async (workflowId: string, step: WorkflowStep): Promise<WorkflowStepCompletionPayload> => {
    const stepNotPossibleForm = stepNotPossibleForms[step.id] || defaultStepNotPossibleFormState
    const stepNotPossible = step.allow_step_not_possible && stepNotPossibleForm.selected
    const stepNotPossibleDetails = stepNotPossibleForm.details.trim()
    if (stepNotPossible) {
      if (!stepNotPossibleDetails) {
        throw new Error("Provide details for why this step is not possible.")
      }
      return {
        step_not_possible: true,
        step_not_possible_details: stepNotPossibleDetails,
        items: [],
      }
    }

    const stepForms = forms[step.id] || {}
    const preparedItems: PreparedWorkflowStepCompletionItem[] = []

    for (const item of step.work_items) {
      const form = stepForms[item.id] || defaultItemFormState
      const photoAspectRatio = item.requires_photo
        ? normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio || "square")
        : null
      const photoUploads = form.photos.map((file) => {
        if (file.size > maxWorkflowPhotoUploadBytes) {
          throw new Error(`Photo ${file.name || "upload"} exceeds ${maxWorkflowPhotoUploadLabel}.`)
        }
        return {
          file,
          aspectRatio: photoAspectRatio,
        }
      })

      const dropdownValue = form.dropdown.trim()
      const writtenResponse = form.written.trim()
      const selectedDropdownOption = dropdownValue
        ? item.dropdown_options.find((option) => option.value === dropdownValue)
        : undefined
      const dropdownRequiresWritten = dropdownValue ? Boolean(item.dropdown_requires_written_response?.[dropdownValue]) : false
      const dropdownRequiresPhoto = Boolean(selectedDropdownOption?.requires_photo_attachment)
      const requiredWritten = item.requires_written_response || dropdownRequiresWritten

      const hasAnyInput = photoUploads.length > 0 || dropdownValue.length > 0 || writtenResponse.length > 0
      if (!item.optional && !hasAnyInput) {
        throw new Error(`Missing response for required work item: ${item.title}`)
      }

      if (item.requires_photo) {
        const requiredCount = Math.max(1, item.photo_required_count || 1)
        if (item.photo_allow_any_count) {
          if (photoUploads.length === 0) {
            throw new Error(`At least one photo is required for: ${item.title}`)
          }
        } else if (photoUploads.length !== requiredCount) {
          throw new Error(`Exactly ${requiredCount} photo${requiredCount === 1 ? "" : "s"} required for: ${item.title}`)
        }
      } else if (dropdownRequiresPhoto && photoUploads.length === 0) {
        const instructions = (selectedDropdownOption?.photo_instructions || "").trim()
        throw new Error(
          instructions
            ? `Photo attachment required for "${item.title}": ${instructions}`
            : `Photo attachment is required for: ${item.title}`,
        )
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

      preparedItems.push({
        itemId: item.id,
        photoUploads,
        ...(writtenResponse.length > 0 ? { writtenResponse } : {}),
        ...(dropdownValue.length > 0 ? { dropdownValue } : {}),
      })
    }

    const items: WorkflowStepCompletionPayload["items"] = []
    const uploadPlan = preparedItems.flatMap((preparedItem) =>
      preparedItem.photoUploads.map((upload) => ({
        itemId: preparedItem.itemId,
        upload,
        unitCount:
          upload.file.size > workflowStepPhotoChunkThresholdBytes
            ? Math.max(1, Math.ceil(upload.file.size / workflowStepPhotoChunkUploadBytes))
            : 1,
      })),
    )
    const totalUploadUnits = uploadPlan.reduce((sum, entry) => sum + entry.unitCount, 0)
    const totalUploadFiles = uploadPlan.length
    let uploadedUnits = 0
    let completedFiles = 0

    if (totalUploadUnits > 0) {
      setStepUploadProgress((prev) => ({
        ...prev,
        [step.id]: {
          uploadedUnits: 0,
          totalUnits: totalUploadUnits,
          completedFiles: 0,
          totalFiles: totalUploadFiles,
          label: totalUploadFiles === 1 ? "Uploading photo..." : "Uploading photos...",
        },
      }))
    }

    for (const preparedItem of preparedItems) {
      const payloadItem: WorkflowStepCompletionPayload["items"][number] = {
        item_id: preparedItem.itemId,
      }

      if (preparedItem.photoUploads.length > 0) {
        const uploadedPhotoIds: string[] = []
        for (const upload of preparedItem.photoUploads) {
          const uploadedPhotoId = await uploadPreparedWorkflowPhoto(
            workflowId,
            step.id,
            preparedItem.itemId,
            upload,
            () => {
              uploadedUnits += 1
              setStepUploadProgress((prev) => {
                const current = prev[step.id]
                if (!current) return prev
                return {
                  ...prev,
                  [step.id]: {
                    ...current,
                    uploadedUnits,
                  },
                }
              })
            },
          )
          completedFiles += 1
          setStepUploadProgress((prev) => {
            const current = prev[step.id]
            if (!current) return prev
            return {
              ...prev,
              [step.id]: {
                ...current,
                completedFiles,
              },
            }
          })
          uploadedPhotoIds.push(uploadedPhotoId)
        }
        payloadItem.photo_ids = uploadedPhotoIds
      }
      if (preparedItem.writtenResponse) payloadItem.written_response = preparedItem.writtenResponse
      if (preparedItem.dropdownValue) payloadItem.dropdown_value = preparedItem.dropdownValue
      items.push(payloadItem)
    }

    return {
      step_not_possible: false,
      items,
    }
  }

  const completeStep = async (workflowId: string, step: WorkflowStep) => {
    clearStepSubmitError(step.id)
    setError("")
    setSubmitting(`complete:${step.id}`)
    try {
      const payload = await buildCompletionPayload(workflowId, step)
      const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${step.id}/complete`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to complete this step.")
      }
      const updatedWorkflow = (await res.json()) as Workflow
      mergeWorkflowIntoFeed(updatedWorkflow)
      setStepCompletionSuccess((prev) => ({
        ...prev,
        [step.id]: {
          workflowId,
          stepTitle: step.title,
        },
      }))
      setStepUploadProgress((prev) => {
        if (!prev[step.id]) return prev
        const next = { ...prev }
        delete next[step.id]
        return next
      })
      void loadFeed()
      void refreshDetailWorkflow(workflowId)
      stopCameraCapturesForStep(step.id)
      setForms((prev) => {
        const next = { ...prev }
        delete next[step.id]
        return next
      })
      setStepNotPossibleForms((prev) => {
        if (!prev[step.id]) return prev
        const next = { ...prev }
        delete next[step.id]
        return next
      })
      clearStepSubmitError(step.id)
    } catch (err) {
      setStepUploadProgress((prev) => {
        if (!prev[step.id]) return prev
        const next = { ...prev }
        delete next[step.id]
        return next
      })
      const message =
        err instanceof Error
          ? err.message
          : "Unable to complete this step."
      setStepSubmitErrors((prev) => ({
        ...prev,
        [step.id]: message,
      }))
    } finally {
      setSubmitting("")
    }
  }

  const workflowBoardWorkflows = useMemo(() => {
    return workflows.filter((workflow) => {
      if (hasClaimedRoleInWorkflow(workflow)) return false
      return Boolean(workflow.claimable_step)
    })
  }, [workflows, hasClaimedRoleInWorkflow])

  const myClaimedWorkflows = useMemo(() => {
    return workflows.filter((workflow) => hasClaimedRoleInWorkflow(workflow))
  }, [workflows, hasClaimedRoleInWorkflow])

  const myWorkflowSeriesGroups = useMemo<WorkflowSeriesCardGroup[]>(() => {
    if (!user?.id) return []
    const groupMap = new Map<string, WorkflowSeriesCardGroup>()

    myClaimedWorkflows.forEach((workflow) => {
      const assignedStep = getPrimaryAssignedStepSummary(workflow)
      const key = workflow.series_id
      const existing = groupMap.get(key)
      if (!existing) {
        groupMap.set(key, {
          key,
          seriesId: workflow.series_id,
          primaryStepOrder: assignedStep ? assignedStep.step_order : null,
          primaryStepTitle: assignedStep ? assignedStep.title : null,
          workflows: [workflow],
        })
        return
      }

      let nextPrimaryStepOrder = existing.primaryStepOrder
      let nextPrimaryStepTitle = existing.primaryStepTitle
      if (nextPrimaryStepOrder == null && assignedStep) {
        nextPrimaryStepOrder = assignedStep.step_order
        nextPrimaryStepTitle = assignedStep.title
      } else if (
        assignedStep &&
        nextPrimaryStepOrder != null &&
        assignedStep.step_order < nextPrimaryStepOrder
      ) {
        nextPrimaryStepOrder = assignedStep.step_order
        nextPrimaryStepTitle = assignedStep.title
      }

      groupMap.set(key, {
        ...existing,
        primaryStepOrder: nextPrimaryStepOrder,
        primaryStepTitle: nextPrimaryStepTitle,
        workflows: [...existing.workflows, workflow],
      })
    })

    return Array.from(groupMap.values())
      .map((group) => ({
        ...group,
        workflows: [...group.workflows].sort((a, b) => a.start_at - b.start_at),
      }))
      .sort((a, b) => {
        const aHasActiveWorkflow = a.workflows.some((workflow) => isWorkflowActiveForUser(workflow))
        const bHasActiveWorkflow = b.workflows.some((workflow) => isWorkflowActiveForUser(workflow))
        if (aHasActiveWorkflow !== bHasActiveWorkflow) {
          return aHasActiveWorkflow ? -1 : 1
        }
        const aLatest = a.workflows[a.workflows.length - 1]?.start_at || 0
        const bLatest = b.workflows[b.workflows.length - 1]?.start_at || 0
        return bLatest - aLatest
      })
  }, [isWorkflowActiveForUser, myClaimedWorkflows, user?.id])

  const openWorkflowDetails = async (
    workflowId: string,
    workflow?: Workflow,
    options?: {
      initialStepIndex?: number
      seriesContext?: {
        key: string
        seriesId: string
        stepOrder: number | null
        workflowIds: string[]
        index: number
      } | null
    },
  ) => {
    setError("")
    const initialStepIndex =
      typeof options?.initialStepIndex === "number" && Number.isFinite(options.initialStepIndex) && options.initialStepIndex >= 0
        ? Math.floor(options.initialStepIndex)
        : 0
    setDetailInitialStepIndex(initialStepIndex)
    setDetailSeriesContext(options?.seriesContext || null)

    if (workflow) {
      setDetailWorkflow(workflow)
      setDetailLoading(false)
      setDetailOpen(true)
      return
    }

    const existing = unpaidWorkflows.find((item) => item.id === workflowId)
    if (existing) {
      setDetailWorkflow(existing)
      setDetailLoading(false)
      setDetailOpen(true)
      return
    }

    setDetailWorkflow(null)
    setDetailLoading(true)
    setDetailOpen(true)

    try {
      const res = await authFetch(`/workflows/${workflowId}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load workflow details.")
      }
      const workflowDetails = (await res.json()) as Workflow
      setDetailWorkflow(workflowDetails)
    } catch (err) {
      setDetailOpen(false)
      setError(err instanceof Error ? err.message : "Unable to load workflow details.")
    } finally {
      setDetailLoading(false)
    }
  }

  const getSeriesCardIndex = useCallback((group: WorkflowSeriesCardGroup) => {
    if (group.workflows.length === 0) return 0
    const maxIndex = group.workflows.length - 1
    const currentIndex = seriesCardIndexByKey[group.key]
    if (currentIndex == null) {
      for (let i = maxIndex; i >= 0; i -= 1) {
        if (isWorkflowActiveForUser(group.workflows[i])) {
          return i
        }
      }
      return maxIndex
    }
    if (currentIndex < 0) return 0
    if (currentIndex > maxIndex) return maxIndex
    return currentIndex
  }, [isWorkflowActiveForUser, seriesCardIndexByKey])

  const shiftSeriesCardIndex = useCallback((group: WorkflowSeriesCardGroup, direction: number) => {
    if (group.workflows.length <= 1) return
    setSeriesCardIndexByKey((prev) => {
      const maxIndex = group.workflows.length - 1
      const currentIndex = prev[group.key] ?? maxIndex
      const nextIndex = Math.min(maxIndex, Math.max(0, currentIndex + direction))
      return {
        ...prev,
        [group.key]: nextIndex,
      }
    })
  }, [])

  const formatImproverCardStatus = useCallback((workflow: { status?: string | null; start_at?: number | null }) => {
    const label = formatWorkflowDisplayStatus(workflow)
    if (label.trim().toLowerCase() === "approved") {
      return "Available"
    }
    return label
  }, [])

  const openSeriesWorkflowDetails = useCallback(async (group: WorkflowSeriesCardGroup, index: number) => {
    if (group.workflows.length === 0) return
    const safeIndex = ((index % group.workflows.length) + group.workflows.length) % group.workflows.length
    const workflow = group.workflows[safeIndex]
    const assignedStep = getPrimaryAssignedStepSummary(workflow)
    await openWorkflowDetails(workflow.id, undefined, {
      initialStepIndex: getInitialStepIndexForWorkflowCard(workflow),
      seriesContext: {
        key: group.key,
        seriesId: group.seriesId,
        stepOrder: assignedStep ? assignedStep.step_order : group.primaryStepOrder,
        workflowIds: group.workflows.map((item) => item.id),
        index: safeIndex,
      },
    })
  }, [openWorkflowDetails])

  const shiftDetailSeriesWorkflow = useCallback(async (direction: number) => {
    if (!detailSeriesContext || detailSeriesContext.workflowIds.length <= 1) return
    const indexFromWorkflow =
      detailWorkflow
        ? detailSeriesContext.workflowIds.findIndex((id) => id === detailWorkflow.id)
        : -1
    const currentIndex = indexFromWorkflow >= 0 ? indexFromWorkflow : detailSeriesContext.index
    const nextIndex = currentIndex + direction
    if (nextIndex < 0 || nextIndex >= detailSeriesContext.workflowIds.length) return
    const nextWorkflowId = detailSeriesContext.workflowIds[nextIndex]
    const nextWorkflow = workflows.find((item) => item.id === nextWorkflowId)
    const assignedStep = nextWorkflow ? getPrimaryAssignedStepSummary(nextWorkflow) : null
    await openWorkflowDetails(nextWorkflowId, undefined, {
      initialStepIndex: nextWorkflow ? getInitialStepIndexForWorkflowCard(nextWorkflow) : 0,
      seriesContext: {
        ...detailSeriesContext,
        stepOrder: assignedStep ? assignedStep.step_order : detailSeriesContext.stepOrder,
        index: nextIndex,
      },
    })
  }, [detailSeriesContext, detailWorkflow, workflows, openWorkflowDetails])

  const parseAttachmentFilename = (value: string | null) => {
    if (!value) return ""
    const quotedMatch = value.match(/filename=\"([^\"]+)\"/i)
    if (quotedMatch?.[1]) return quotedMatch[1]
    const plainMatch = value.match(/filename=([^;]+)/i)
    if (plainMatch?.[1]) return plainMatch[1].trim()
    return ""
  }

  const filteredBoardWorkflows = useMemo(() => {
    const s = boardSearch.trim().toLowerCase()
    if (!s) return workflowBoardWorkflows
    return workflowBoardWorkflows.filter((w) => w.title.toLowerCase().includes(s))
  }, [workflowBoardWorkflows, boardSearch])

  const filteredActiveSeriesGroups = useMemo(() => {
    const s = myWorkflowsSearch.trim().toLowerCase()
    let filtered = myWorkflowSeriesGroups

    if (showOnlyActiveSeries) {
      filtered = filtered.filter((group) =>
        group.workflows.some(
          (workflow) =>
            workflow.recurrence !== "one_time" ||
            (workflow.status !== "completed" && workflow.status !== "paid_out"),
        ),
      )
    }

    if (!s) return filtered
    return filtered.filter((group) =>
      group.workflows.some((workflow) => workflow.title.toLowerCase().includes(s)),
    )
  }, [myWorkflowSeriesGroups, myWorkflowsSearch, showOnlyActiveSeries])

  const filteredUnpaidWorkflows = useMemo(() => {
    const s = unpaidSearch.trim().toLowerCase()
    if (!s) return unpaidWorkflows
    return unpaidWorkflows.filter((w) => w.title.toLowerCase().includes(s))
  }, [unpaidWorkflows, unpaidSearch])

  const filteredAbsencePeriods = useMemo(() => {
    const s = absenceSearch.trim().toLowerCase()
    if (!s) return absencePeriods
    return absencePeriods.filter((p) => p.series_id.toLowerCase().includes(s))
  }, [absencePeriods, absenceSearch])

  const filteredCredentialTypes = useMemo(() => {
    const s = credentialSearch.trim().toLowerCase()
    if (!s) return requestableCredentialTypes
    return requestableCredentialTypes.filter((t) => t.label.toLowerCase().includes(s))
  }, [requestableCredentialTypes, credentialSearch])

  const requestStepPayout = async (workflowId: string, stepId: string) => {
    const key = `retry-step:${stepId}`
    setSubmitting(key)
    try {
      const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${stepId}/payout-request`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
        if ((text || "").toLowerCase().includes("payout already complete")) {
          setNotice("Payout was already completed.")
          setError("")
          await loadFeed()
          await refreshDetailWorkflow(workflowId)
          return
        }
        throw new Error(text || "Unable to request step payout retry.")
      }
      setNotice("Step payout retry requested.")
      setError("")
      await loadFeed()
      await refreshDetailWorkflow(workflowId)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to request step payout retry.")
      setNotice("")
    } finally {
      setSubmitting("")
    }
  }

  const requestWorkflowFailedPayouts = async (workflow: Workflow) => {
    const failedSteps = workflow.steps.filter(
      (step) =>
        step.assigned_improver_id === user?.id
        && step.status === "completed"
        && step.bounty > 0
        && Boolean(step.payout_error?.trim()),
    )

    if (failedSteps.length === 0) {
      setError("No failed payouts are available to retry for this workflow.")
      setNotice("")
      return
    }

    const key = `retry-workflow:${workflow.id}`
    setSubmitting(key)
    try {
      let alreadyCompletedCount = 0
      for (const step of failedSteps) {
        const res = await authFetch(`/improvers/workflows/${workflow.id}/steps/${step.id}/payout-request`, {
          method: "POST",
        })
        if (!res.ok) {
          const text = await res.text()
          if ((text || "").toLowerCase().includes("payout already complete")) {
            alreadyCompletedCount += 1
            continue
          }
          throw new Error(text || "Unable to request payout retry.")
        }
      }

      const requestedCount = failedSteps.length - alreadyCompletedCount
      if (requestedCount <= 0 && alreadyCompletedCount > 0) {
        setNotice(alreadyCompletedCount === 1 ? "Payout was already completed." : `${alreadyCompletedCount} payouts were already completed.`)
      } else {
        setNotice(
          requestedCount === 1
            ? "Step payout retry requested."
            : `${requestedCount} payout retries requested.`,
        )
      }
      setError("")
      await loadFeed()
      await refreshDetailWorkflow(workflow.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to request payout retries.")
      setNotice("")
    } finally {
      setSubmitting("")
    }
  }

  const unclaimSeries = async (seriesId: string, stepOrder: number) => {
    const submitKey = `unclaim-series:${seriesId}:${stepOrder}`
    setSubmitting(submitKey)
    try {
      const res = await authFetch("/improvers/workflow-series/unclaim", {
        method: "POST",
        body: JSON.stringify({
          series_id: seriesId,
          step_order: stepOrder,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to unclaim this workflow series.")
      }
      const result = (await res.json()) as ImproverWorkflowSeriesUnclaimResult
      setNotice(
        result.skipped_count > 0
          ? `Series unclaimed. Released ${result.released_count} claim(s); ${result.skipped_count} started assignment(s) were not released.`
          : `Series unclaimed. Released ${result.released_count} claim(s).`,
      )
      setError("")
      await loadFeed()
      if (detailSeriesContext?.seriesId === seriesId && detailSeriesContext.stepOrder === stepOrder) {
        setDetailOpen(false)
        setDetailWorkflow(null)
        setDetailSeriesContext(null)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to unclaim this workflow series.")
      setNotice("")
    } finally {
      setSubmitting("")
    }
  }

  const confirmUnclaimSeries = async () => {
    if (!unclaimConfirmTarget) return
    const target = unclaimConfirmTarget
    try {
      await unclaimSeries(target.seriesId, target.stepOrder)
    } finally {
      setUnclaimConfirmTarget(null)
    }
  }

  const downloadWorkflowPhoto = async (photoId: string) => {
    setDownloadingPhotoId(photoId)
    try {
      const res = await authFetch(`/workflow-photos/${photoId}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to download photo.")
      }

      const blob = await res.blob()
      const disposition = res.headers.get("Content-Disposition")
      const filename = parseAttachmentFilename(disposition) || `workflow_photo_${photoId}`
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = filename
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to download photo.")
    } finally {
      setDownloadingPhotoId(null)
    }
  }

  const renderWorkflowHeaderActions = (workflow: Workflow) => {
    if (!detailSeriesContext) return null
    if (detailSeriesContext.workflowIds.length === 0) return null

    const workflowIndexFromId = detailSeriesContext.workflowIds.findIndex((id) => id === workflow.id)
    const workflowIndex =
      workflowIndexFromId >= 0
        ? workflowIndexFromId
        : Math.min(
            detailSeriesContext.workflowIds.length - 1,
            Math.max(0, detailSeriesContext.index),
          )

    const hasSeriesNavigation = detailSeriesContext.workflowIds.length > 1
    const canShiftBackward = hasSeriesNavigation && workflowIndex > 0
    const canShiftForward = hasSeriesNavigation && workflowIndex < detailSeriesContext.workflowIds.length - 1

    return (
      <div className="space-y-2 rounded-md border bg-secondary/30 p-2.5 sm:p-3">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">
            Series workflow {workflowIndex + 1} of {detailSeriesContext.workflowIds.length}
          </p>
          {hasSeriesNavigation && (
            <div className="flex items-center gap-2 sm:ml-auto">
              <Button
                className="h-8 w-8 p-0"
                variant="outline"
                size="sm"
                onClick={() => void shiftDetailSeriesWorkflow(-1)}
                disabled={!canShiftBackward || Boolean(submitting)}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                className="h-8 w-8 p-0"
                variant="outline"
                size="sm"
                onClick={() => void shiftDetailSeriesWorkflow(1)}
                disabled={!canShiftForward || Boolean(submitting)}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          )}
        </div>
      </div>
    )
  }

  const getWorkflowCompletionSuccess = useCallback((workflow: Workflow) => {
    const matchingStep = workflow.steps.find((step) => stepCompletionSuccess[step.id]?.workflowId === workflow.id)
    if (!matchingStep) return null
    return stepCompletionSuccess[matchingStep.id]
  }, [stepCompletionSuccess])

  const renderWorkflowSuccessHeader = (workflow: Workflow) => {
    const completionSuccess = getWorkflowCompletionSuccess(workflow)
    if (!completionSuccess) return null

    return (
      <Card className="overflow-hidden border-[#eb6c6c]/25 bg-[#fff4f1] shadow-sm dark:border-[#eb6c6c]/30 dark:bg-[#3a1d1d]/55">
        <CardContent className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-4">
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-emerald-600 text-white shadow-sm dark:bg-emerald-500">
              <CheckCircle2 className="h-8 w-8" />
            </div>
            <div className="space-y-1">
              <p className="text-lg font-semibold tracking-tight text-[#8c2f29] dark:text-[#ffe2dd]">Submission Complete</p>
              <p className="text-sm text-[#a34841]/85 dark:text-[#f7c5bf]/80">
                {completionSuccess.stepTitle} was submitted successfully.
              </p>
            </div>
          </div>
          <Button
            size="sm"
            variant="outline"
            className="border-[#eb6c6c]/25 bg-white/90 text-[#8c2f29] hover:bg-white dark:border-[#eb6c6c]/30 dark:bg-[#281515]/70 dark:text-[#ffe2dd] dark:hover:bg-[#331919]"
            onClick={() => setDetailOpen(false)}
          >
            Done
          </Button>
        </CardContent>
      </Card>
    )
  }

  const renderWorkflowTopRightActions = (workflow: Workflow) => {
    if (!detailSeriesContext || detailSeriesContext.stepOrder == null) return null
    if (workflow.recurrence === "one_time") return null

    const submitKey = `unclaim-series:${detailSeriesContext.seriesId}:${detailSeriesContext.stepOrder}`
    const isUnclaiming = submitting === submitKey
    return (
      <div className="w-full sm:w-auto">
        <Button
          size="sm"
          variant="ghost"
          className="h-8 px-2 text-xs font-normal text-muted-foreground hover:text-destructive justify-start sm:justify-center"
          onClick={() =>
            setUnclaimConfirmTarget({
              seriesId: detailSeriesContext.seriesId,
              stepOrder: detailSeriesContext.stepOrder!,
              workflowTitle: workflow.title,
            })
          }
          disabled={Boolean(submitting)}
        >
          {isUnclaiming ? "Unclaiming..." : "Unclaim series"}
        </Button>
      </div>
    )
  }

  const renderWorkflowStepActions = (workflow: Workflow, step: WorkflowStep) => {
    const mine = step.assigned_improver_id === user?.id
    const claimable = canClaimStep(workflow, step)
    const stepSubmitError = stepSubmitErrors[step.id] || ""
    const uploadProgress = stepUploadProgress[step.id]
    const stepNotPossibleState = stepNotPossibleForms[step.id] || defaultStepNotPossibleFormState
    const stepNotPossibleSelected = step.allow_step_not_possible && stepNotPossibleState.selected
    const nowUnix = Math.floor(Date.now() / 1000)
    const isStartEligible =
      step.step_order <= 1
        ? workflow.start_at <= nowUnix
        : workflow.steps.some(
            (candidate) =>
              candidate.step_order === step.step_order - 1 &&
              (candidate.status === "completed" || candidate.status === "paid_out"),
          )

    if (getWorkflowCompletionSuccess(workflow)) return null

    if (!mine && !claimable) return null

    return (
      <div className="space-y-4">
        {claimable && (
          <Button
            className="w-full sm:w-auto"
            size="sm"
            onClick={() => claimStep(workflow.id, step.id)}
            disabled={Boolean(submitting)}
          >
            {submitting === `claim:${step.id}` ? "Claiming..." : `Claim Step ${step.step_order}`}
          </Button>
        )}

	        {mine && step.status === "locked" && isStartEligible && (
	          <Button
	            className="w-full sm:w-auto"
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
            {step.allow_step_not_possible && (
              <Card>
                <CardContent className="p-3 space-y-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p className="text-sm font-medium">Step not possible</p>
                    <Badge variant="outline">Optional</Badge>
                  </div>
                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={stepNotPossibleSelected}
                      onCheckedChange={(checked: boolean | "indeterminate") => {
                        const selected = Boolean(checked)
                        updateStepNotPossibleForm(step.id, {
                          selected,
                        })
                        if (selected) {
                          stopCameraCapturesForStep(step.id)
                        }
                      }}
                      disabled={Boolean(submitting)}
                    />
                    Mark this step as not possible
                  </label>
                  {stepNotPossibleSelected && (
                    <div className="space-y-1">
                      <Label className="text-xs">Required Details</Label>
                      <Textarea
                        value={stepNotPossibleState.details}
                        onChange={(e) =>
                          updateStepNotPossibleForm(step.id, {
                            details: e.target.value,
                          })
                        }
                        placeholder="Explain why this step cannot be completed."
                        disabled={Boolean(submitting)}
                      />
                      <p className="text-xs text-muted-foreground">
                        Selecting this will end the full workflow with no bounties paid out.
                      </p>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}

            <div className="flex items-center gap-2 text-sm font-medium">
              <ClipboardCheck className="h-4 w-4" />
              Work Item Responses
            </div>

            {stepNotPossibleSelected && (
              <p className="text-xs text-muted-foreground">
                Work item inputs are disabled while &quot;Step not possible&quot; is selected.
              </p>
            )}

            {step.work_items.map((item) => {
              const form = forms[step.id]?.[item.id] || defaultItemFormState
              const cameraKey = cameraKeyForItem(step.id, item.id)
              const cameraState = cameraStates[cameraKey] || defaultCameraCaptureState
              const selectedDropdownOption = form.dropdown
                ? item.dropdown_options.find((option) => option.value === form.dropdown)
                : undefined
              const dropdownRequiresPhoto = Boolean(selectedDropdownOption?.requires_photo_attachment)
              const dropdownCameraCaptureOnly =
                Boolean(selectedDropdownOption?.requires_photo_attachment) && Boolean(selectedDropdownOption?.camera_capture_only)
              const effectiveRequiresPhoto = item.requires_photo || dropdownRequiresPhoto
              const effectiveCameraCaptureOnly = (item.requires_photo && item.camera_capture_only) || dropdownCameraCaptureOnly
              const effectivePhotoAllowAnyCount = item.requires_photo ? item.photo_allow_any_count : false
              const effectivePhotoRequiredCount = item.requires_photo ? Math.max(1, item.photo_required_count || 1) : 1
              const effectivePhotoAspectRatio = effectiveRequiresPhoto
                ? normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio || "square")
                : null
              const dropdownPhotoInstructions = (selectedDropdownOption?.photo_instructions || "").trim()
              const effectiveWrittenRequired =
                item.requires_written_response ||
                (form.dropdown.length > 0 && Boolean(item.dropdown_requires_written_response?.[form.dropdown]))

              return (
                <Card key={item.id} className={cn(stepNotPossibleSelected && "opacity-60")}>
                  <CardContent className="p-3 space-y-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-medium text-sm">{item.title}</p>
                      {item.optional && <Badge variant="outline">Optional</Badge>}
                    </div>
                    <p className="text-xs text-muted-foreground">{item.description}</p>

                    {effectiveRequiresPhoto &&
                      (effectiveCameraCaptureOnly ? (
                        <div className="space-y-2">
                          <div>
                            <Label className="text-xs">Camera Capture</Label>
                            <p className="text-xs text-muted-foreground">
                              {effectivePhotoAllowAnyCount
                                ? "Capture any number of photos."
                                : `Capture exactly ${effectivePhotoRequiredCount} photo${effectivePhotoRequiredCount === 1 ? "" : "s"}.`}
                            </p>
                            {dropdownCameraCaptureOnly && !item.requires_photo && (
                              <p className="text-xs text-muted-foreground">
                                The selected dropdown option requires a live photo.
                              </p>
                            )}
                            {dropdownPhotoInstructions && (
                              <p className="text-xs text-muted-foreground whitespace-pre-wrap">{dropdownPhotoInstructions}</p>
                            )}
                          </div>

                          <input
                            ref={(element) => attachCameraFileInputRef(cameraKey, element)}
                            type="file"
                            accept="image/*"
                            capture="environment"
                            className="hidden"
                            onChange={(e) => {
                              void appendItemPhotosFromSelection(
                                step.id,
                                item.id,
                                effectivePhotoAspectRatio,
                                e.currentTarget.files,
                                (message) => {
                                  setCameraCaptureState(cameraKey, {
                                    open: false,
                                    error: message,
                                  })
                                },
                              )
                              e.currentTarget.value = ""
                            }}
                            disabled={stepNotPossibleSelected || Boolean(submitting)}
                          />

                          {form.photos.length > 0 ? (
                            <div className="space-y-2">
                              <p className="text-xs text-muted-foreground">
                                {form.photos.length} captured photo{form.photos.length === 1 ? "" : "s"}
                              </p>
                              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                                {form.photos.map((photo, photoIndex) => (
                                  <div
                                    key={`${photo.name}-${photo.lastModified}-${photoIndex}`}
                                    className="space-y-2 rounded border p-2 text-xs"
                                  >
                                    <LocalPhotoThumbnail
                                      file={photo}
                                      onOpen={openLocalPhotoPreview}
                                      disabled={Boolean(submitting)}
                                    />
                                    <div className="flex items-center justify-between gap-2">
                                      <span className="truncate">{photo.name}</span>
                                      <Button
                                        type="button"
                                        size="sm"
                                        variant="ghost"
                                        onClick={() => removeItemPhoto(step.id, item.id, photoIndex)}
                                        disabled={Boolean(submitting) || stepNotPossibleSelected}
                                      >
                                        Remove
                                      </Button>
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>
                          ) : (
                            <p className="text-xs text-muted-foreground">No captured photos yet.</p>
                          )}

                          <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
                            <Button
                              className="w-full sm:w-auto"
                              type="button"
                              size="sm"
                              variant="outline"
                              onClick={() => void startCameraCapture(step.id, item.id, effectivePhotoAspectRatio || "square")}
                              disabled={Boolean(submitting) || stepNotPossibleSelected}
                            >
                              {cameraState.open ? "Restart Camera" : form.photos.length > 0 ? "Take Another Photo" : "Open Camera"}
                            </Button>
                            {cameraState.open && (
                                <Button
                                  className="w-full sm:w-auto"
                                  type="button"
                                  size="sm"
                                  variant="outline"
                                  onClick={() => stopCameraCapture(step.id, item.id)}
                                  disabled={Boolean(submitting) || stepNotPossibleSelected}
                                >
                                Stop Camera
                              </Button>
                            )}
                          </div>

                          {cameraState.error && <p className="text-xs text-red-600">{cameraState.error}</p>}

                          {cameraState.open && (
                            <div className="space-y-2">
                              <div
                                className="overflow-hidden rounded border bg-black/80"
                                style={{
                                  aspectRatio: String(
                                    workflowPhotoAspectRatios[normalizeWorkflowPhotoAspectRatio(item.photo_aspect_ratio || "square")],
                                  ),
                                }}
                              >
                                <video
                                  ref={getCameraVideoRef(cameraKey)}
                                  className="h-full w-full object-cover"
                                  playsInline
                                  muted
                                  autoPlay
                                />
                              </div>
                              <Button
                                className="w-full sm:w-auto"
                                type="button"
                                size="sm"
                                onClick={() =>
                                  captureCameraPhoto(
                                    step.id,
                                    item.id,
                                    item.title,
                                    effectivePhotoAspectRatio || "square",
                                  )
                                }
                                disabled={Boolean(submitting) || stepNotPossibleSelected}
                              >
                                Capture Photo
                              </Button>
                            </div>
                          )}
                        </div>
                      ) : (
                        <div className="space-y-1">
                          <Label className="text-xs">{item.requires_photo ? "Upload Photos" : "Photo Attachment"}</Label>
                          <p className="text-xs text-muted-foreground">
                            {item.requires_photo
                              ? effectivePhotoAllowAnyCount
                              ? "Upload any number of photos."
                              : `Upload exactly ${effectivePhotoRequiredCount} photo${effectivePhotoRequiredCount === 1 ? "" : "s"}.`
                              : "Upload one photo attachment for the selected dropdown option."}{" "}
                            Each photo must be under {maxWorkflowPhotoUploadLabel}. Oversized images are resized automatically when possible.
                          </p>
                          {dropdownPhotoInstructions && (
                            <p className="text-xs text-muted-foreground whitespace-pre-wrap">{dropdownPhotoInstructions}</p>
                          )}
                          <Input
                            type="file"
                            accept="image/*"
                            multiple={item.requires_photo ? effectivePhotoAllowAnyCount || effectivePhotoRequiredCount > 1 : false}
                            onChange={(e) =>
                              void replaceItemPhotosFromSelection(
                                step.id,
                                item.id,
                                effectivePhotoAspectRatio,
                                e.target.files,
                              )
                            }
                            disabled={stepNotPossibleSelected || Boolean(submitting)}
                          />
                          {form.photos.length > 0 ? (
                            <div className="space-y-2">
                              <p className="text-xs text-muted-foreground">
                                {form.photos.length} file{form.photos.length === 1 ? "" : "s"} selected
                              </p>
                              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                                {form.photos.map((photo, photoIndex) => (
                                  <div
                                    key={`${photo.name}-${photo.lastModified}-${photoIndex}`}
                                    className="space-y-2 rounded border p-2 text-xs"
                                  >
                                    <LocalPhotoThumbnail
                                      file={photo}
                                      onOpen={openLocalPhotoPreview}
                                      disabled={Boolean(submitting)}
                                    />
                                    <div className="flex items-center justify-between gap-2">
                                      <span className="truncate">{photo.name}</span>
                                      <Button
                                        type="button"
                                        size="sm"
                                        variant="ghost"
                                        onClick={() => removeItemPhoto(step.id, item.id, photoIndex)}
                                        disabled={Boolean(submitting) || stepNotPossibleSelected}
                                      >
                                        Remove
                                      </Button>
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>
                          ) : (
                            <p className="text-xs text-muted-foreground">No photos selected yet.</p>
                          )}
                        </div>
                      ))}

                    {item.requires_dropdown && (
                      <div className="space-y-1">
                        <Label className="text-xs">Dropdown Selection</Label>
                        <Select
                          value={form.dropdown}
                          onValueChange={(value) => {
                            const nextSelectedOption = item.dropdown_options.find((option) => option.value === value)
                            const nextRequiresLivePhoto =
                              Boolean(nextSelectedOption?.requires_photo_attachment) && Boolean(nextSelectedOption?.camera_capture_only)
                            updateItemForm(step.id, item.id, {
                              dropdown: value,
                              photos:
                                nextRequiresLivePhoto
                                  ? []
                                  : item.requires_photo || Boolean(nextSelectedOption?.requires_photo_attachment)
                                  ? form.photos
                                  : [],
                            })
                          }}
                          disabled={stepNotPossibleSelected || Boolean(submitting)}
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

                    {effectiveWrittenRequired && (
                      <div className="space-y-1">
                        <Label className="text-xs">Written Response</Label>
                        <Textarea
                          value={form.written}
                          onChange={(e) => updateItemForm(step.id, item.id, { written: e.target.value })}
                          placeholder="Enter your response..."
                          disabled={stepNotPossibleSelected || Boolean(submitting)}
                        />
                      </div>
                    )}
                  </CardContent>
                </Card>
              )
            })}

            {stepSubmitError && (
              <div className="flex items-center gap-2 rounded border border-red-200 bg-red-50 p-2 text-xs text-red-700">
                <AlertTriangle className="h-3.5 w-3.5 flex-shrink-0" />
                <span>{stepSubmitError}</span>
              </div>
            )}

            {uploadProgress && uploadProgress.totalUnits > 0 ? (
              <Card className="border-[#eb6c6c]/30 bg-[#eb6c6c]/5">
                <CardContent className="space-y-3 p-3">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-medium">{uploadProgress.label}</p>
                    <span className="text-xs text-muted-foreground">
                      {Math.min(
                        100,
                        Math.round((uploadProgress.uploadedUnits / Math.max(1, uploadProgress.totalUnits)) * 100),
                      )}
                      %
                    </span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-secondary">
                    <div
                      className="h-full rounded-full bg-[#eb6c6c] transition-all duration-200"
                      style={{
                        width: `${Math.min(
                          100,
                          uploadProgress.uploadedUnits === 0
                            ? 0
                            : Math.max(4, (uploadProgress.uploadedUnits / Math.max(1, uploadProgress.totalUnits)) * 100),
                        )}%`,
                      }}
                    />
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Uploaded {uploadProgress.uploadedUnits} of {uploadProgress.totalUnits} transfer
                    {uploadProgress.totalUnits === 1 ? "" : "s"} across {uploadProgress.completedFiles} of{" "}
                    {uploadProgress.totalFiles} photo{uploadProgress.totalFiles === 1 ? "" : "s"}.
                  </p>
                </CardContent>
              </Card>
            ) : (
              <Button
                className="w-full sm:w-auto"
                size="sm"
                onClick={() => completeStep(workflow.id, step)}
                disabled={Boolean(submitting)}
              >
                {submitting === `complete:${step.id}` ? (
                  "Submitting..."
                ) : (
                  <>
                    <CheckCircle2 className="mr-2 h-4 w-4" />
                    {stepNotPossibleSelected ? "Mark Step Not Possible" : "Complete Step"}
                  </>
                )}
              </Button>
            )}
          </div>
        )}
      </div>
    )
  }

  if (status === "loading" || initialLoading) {
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

      {notice && (
        <div className="flex items-center gap-2 text-green-700 text-sm p-3 bg-green-50 dark:bg-green-900/20 rounded-lg">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          <span>{notice}</span>
        </div>
      )}

      <Tabs
        value={activeTab}
        onValueChange={(value) => {
          if (!isImproverTab(value)) return
          setActiveTab(value)
        }}
        className="space-y-4"
      >
        <TabsList className="grid h-auto w-full grid-cols-1 gap-2 p-1 sm:grid-cols-2 lg:grid-cols-6">
          <TabsTrigger value="my-workflows">My Workflows</TabsTrigger>
          <TabsTrigger value="workflow-board">Workflow Board</TabsTrigger>
          <TabsTrigger value="unpaid-workflows">Unpaid Workflows</TabsTrigger>
          <TabsTrigger value="my-badges">My Badges</TabsTrigger>
          <TabsTrigger value="credentials">Credentials</TabsTrigger>
          <TabsTrigger value="absence">Absence Coverage</TabsTrigger>
        </TabsList>

        <TabsContent value="workflow-board" className="space-y-3">
          {!workflowDataLoaded ? (
            <ImproverTabLoadingCard label="Loading workflow board..." />
          ) : (
            <>
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search workflows..."
              value={boardSearch}
              onChange={(e) => setBoardSearch(e.target.value)}
              className="pl-9"
            />
          </div>
          {filteredBoardWorkflows.length === 0 ? (
            <Card>
              <CardHeader>
                <CardTitle>No Eligible Workflows</CardTitle>
                <CardDescription>No workflows are currently available for you to claim.</CardDescription>
              </CardHeader>
            </Card>
	          ) : (
	            filteredBoardWorkflows.map((workflow) => {
	              return (
	                <Card
	                  key={workflow.id}
	                  className="cursor-pointer transition-colors hover:bg-muted/30"
	                  onClick={() => openWorkflowDetails(workflow.id)}
	                >
                  <CardContent className="pt-4 space-y-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <h4 className="font-semibold">{workflow.title}</h4>
                      </div>
                      <Badge variant={workflow.status === "in_progress" ? "default" : "secondary"}>
                        {formatImproverCardStatus(workflow)}
                      </Badge>
                    </div>

                    <p className="text-sm text-muted-foreground line-clamp-2">{workflow.description}</p>

	                    <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
	                      <span>Start: {new Date(workflow.start_at * 1000).toLocaleString()}</span>
	                    </div>

	                    <div className="flex w-full flex-col gap-2 pt-1 sm:w-auto sm:flex-row sm:flex-wrap">
                      <Button
                        className="w-full sm:w-auto"
                        size="sm"
                        variant="outline"
	                        onClick={(e) => {
	                          e.stopPropagation()
	                          void openWorkflowDetails(workflow.id)
	                        }}
                      >
                        View Details
                      </Button>
	                    </div>
	                  </CardContent>
                </Card>
              )
            })
          )}
            </>
          )}
	        </TabsContent>

        <TabsContent value="my-workflows" className="space-y-3">
          {!workflowDataLoaded ? (
            <ImproverTabLoadingCard label="Loading your workflows..." />
          ) : (
            <>
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search my workflows..."
              value={myWorkflowsSearch}
              onChange={(e) => setMyWorkflowsSearch(e.target.value)}
              className="pl-9"
            />
          </div>
          <div className="flex items-center gap-2 rounded-md border bg-secondary/20 px-3 py-2">
            <Checkbox
              id="my-workflows-active-only"
              checked={showOnlyActiveSeries}
              onCheckedChange={(checked: boolean | "indeterminate") => setShowOnlyActiveSeries(Boolean(checked))}
            />
            <Label htmlFor="my-workflows-active-only" className="text-sm font-normal cursor-pointer">
              Hide finished workflows.
            </Label>
          </div>
          {filteredActiveSeriesGroups.length === 0 ? (
            <Card>
              <CardHeader>
                <CardTitle>No Claimed Workflows</CardTitle>
                <CardDescription>
                  {showOnlyActiveSeries
                    ? "No workflows match your filter."
                    : "Workflows you claim will appear here."}
                </CardDescription>
              </CardHeader>
            </Card>
          ) : (
            <div className="space-y-4">
              <div className="space-y-3">
                <p className="text-sm font-medium">Workflow Series</p>
                {filteredActiveSeriesGroups.map((group) => {
                  if (group.workflows.length === 0) return null
                  const cardIndex = getSeriesCardIndex(group)
                  const workflow = group.workflows[cardIndex]
                  const workflowIsActive = isWorkflowActiveForUser(workflow)
                  const hasSeriesNavigation = group.workflows.length > 1
                  const canShiftBackward = hasSeriesNavigation && cardIndex > 0
                  const canShiftForward = hasSeriesNavigation && cardIndex < group.workflows.length - 1

                  return (
                    <Card
                      key={`series-${group.key}`}
                      className={cn(
                        "cursor-pointer transition-colors",
                        workflowIsActive
                          ? "border-[#eb6c6c]/50 bg-[#eb6c6c]/10 hover:bg-[#eb6c6c]/15"
                          : "hover:bg-muted/30",
                      )}
                      onClick={() => void openSeriesWorkflowDetails(group, cardIndex)}
                    >
                      <CardContent className="pt-4 space-y-3">
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <div>
                            <h4 className="font-semibold">{workflow.title}</h4>
                            {group.primaryStepOrder != null && group.primaryStepTitle ? (
                              <p className="text-xs text-muted-foreground">
                                Assigned step {group.primaryStepOrder}: {group.primaryStepTitle}
                              </p>
                            ) : (
                              <p className="text-xs text-muted-foreground">Series assignment</p>
                            )}
                          </div>
                          <Badge variant={workflow.status === "in_progress" ? "default" : "secondary"}>
                            {formatImproverCardStatus(workflow)}
                          </Badge>
                        </div>

                        <p className="text-sm text-muted-foreground line-clamp-2">{workflow.description}</p>

                        <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                          <span>Start: {new Date(workflow.start_at * 1000).toLocaleString()}</span>
                        </div>

                        <div className="flex flex-wrap items-center gap-2 pt-1">
                          <Button
                            className="w-auto"
                            size="sm"
                            variant="outline"
                            onClick={(event) => {
                              event.stopPropagation()
                              void openSeriesWorkflowDetails(group, cardIndex)
                            }}
                          >
                            View Details
                          </Button>
                          {hasSeriesNavigation && (
                            <div className="flex items-center gap-2">
                              <Button
                                className="h-8 w-8 p-0"
                                size="sm"
                                variant="outline"
                                onClick={(event) => {
                                  event.stopPropagation()
                                  shiftSeriesCardIndex(group, -1)
                                }}
                                disabled={!canShiftBackward || Boolean(submitting)}
                              >
                                <ChevronLeft className="h-4 w-4" />
                              </Button>
                              <Button
                                className="h-8 w-8 p-0"
                                size="sm"
                                variant="outline"
                                onClick={(event) => {
                                  event.stopPropagation()
                                  shiftSeriesCardIndex(group, 1)
                                }}
                                disabled={!canShiftForward || Boolean(submitting)}
                              >
                                <ChevronRight className="h-4 w-4" />
                              </Button>
                            </div>
                          )}
                          <span className="ml-auto hidden text-[11px] tabular-nums text-muted-foreground sm:inline">
                            {cardIndex + 1}/{group.workflows.length}
                          </span>
                        </div>

                        <div className="flex justify-end pt-1 sm:hidden">
                          <span className="text-[11px] tabular-nums text-muted-foreground">
                            {cardIndex + 1}/{group.workflows.length}
                          </span>
                        </div>
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            </div>
          )}
            </>
          )}
        </TabsContent>

	        <TabsContent value="unpaid-workflows" className="space-y-4">
          {!unpaidDataLoaded ? (
            <ImproverTabLoadingCard label="Loading unpaid workflows..." />
          ) : (
	          <Card>
	            <CardHeader>
	              <CardTitle>Unpaid Workflows</CardTitle>
	              <CardDescription>
	                Completed work awaiting payout finalization. If a payout failed, request a retry here.
	              </CardDescription>
	            </CardHeader>
	            <CardContent className="space-y-3">
	              <div className="relative">
	                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
	                <Input
	                  placeholder="Search unpaid workflows..."
	                  value={unpaidSearch}
	                  onChange={(e) => setUnpaidSearch(e.target.value)}
	                  className="pl-9"
	                />
	              </div>
	              {filteredUnpaidWorkflows.length === 0 ? (
	                <p className="text-sm text-muted-foreground">No unpaid workflow payouts are pending for you.</p>
	              ) : (
                filteredUnpaidWorkflows.map((workflow) => {
                  const unpaidSteps = workflow.steps.filter(
                    (step) => step.assigned_improver_id === user?.id && step.status === "completed" && step.bounty > 0
                  )
                  const failedUnpaidSteps = unpaidSteps.filter((step) => Boolean(step.payout_error?.trim()))
                  if (unpaidSteps.length === 0) {
                    return null
                  }

                  return (
	                    <Card key={`unpaid-${workflow.id}`}>
	                      <CardContent className="pt-4 space-y-3">
	                        <div className="flex flex-wrap items-center justify-between gap-2">
	                          <div>
	                            <h4 className="font-semibold">{workflow.title}</h4>
	                          </div>
	                          <Badge>{formatImproverCardStatus(workflow)}</Badge>
	                        </div>
                        <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                          <span>Start: {new Date(workflow.start_at * 1000).toLocaleString()}</span>
                          <span>
                            Pending payouts: {unpaidSteps.length}
                          </span>
                          <span>
                            Errors: {failedUnpaidSteps.length}
                          </span>
                        </div>

                        {failedUnpaidSteps.length > 0 && (
                          <div className="flex flex-col gap-3 rounded-lg border border-red-200 bg-red-50/80 p-3 dark:border-red-900/60 dark:bg-red-950/20 sm:flex-row sm:items-center sm:justify-between">
                            <div className="space-y-1">
                              <p className="text-sm font-medium text-red-700 dark:text-red-300">
                                {failedUnpaidSteps.length === 1
                                  ? "1 payout needs attention"
                                  : `${failedUnpaidSteps.length} payouts need attention`}
                              </p>
                              <p className="text-xs text-red-600/90 dark:text-red-300/90">
                                Retry failed payouts here without opening each step individually.
                              </p>
                            </div>
                            <Button
                              className="w-full sm:w-auto"
                              size="sm"
                              onClick={() => requestWorkflowFailedPayouts(workflow)}
                              disabled={Boolean(submitting)}
                            >
                              {submitting === `retry-workflow:${workflow.id}`
                                ? "Requesting..."
                                : failedUnpaidSteps.length === 1
                                  ? "Retry Failed Payout"
                                  : "Retry Failed Payouts"}
                            </Button>
                          </div>
                        )}

	                        {unpaidSteps.map((step) => {
	                          const hasError = Boolean(step.payout_error?.trim())
	                          return (
	                            <div
                                key={`unpaid-step-${step.id}`}
                                className={cn(
                                  "rounded border p-3 space-y-2",
                                  hasError && "border-red-200 bg-red-50/60 dark:border-red-900/50 dark:bg-red-950/10",
                                )}
                              >
	                              <div className="flex flex-wrap items-center justify-between gap-2">
	                                <p className="text-sm font-medium">
	                                  Step {step.step_order}: {step.title}
	                                </p>
	                                <Badge variant={hasError ? "destructive" : "outline"}>{hasError ? "Payout Error" : "Pending"}</Badge>
	                              </div>
	                              <p className="text-xs text-muted-foreground">Bounty: {step.bounty} SFLuv</p>
	                              {hasError ? (
	                                <p className="text-xs text-red-600 whitespace-pre-wrap">{step.payout_error}</p>
	                              ) : (
	                                <p className="text-xs text-muted-foreground">
	                                  Payout is waiting for earlier workflows in this series to finish and settle.
	                                </p>
	                              )}
	                              {hasError && (
	                                <Button
	                                  className="w-full sm:w-auto"
	                                  size="sm"
	                                  variant="secondary"
	                                  onClick={() => requestStepPayout(workflow.id, step.id)}
	                                  disabled={Boolean(submitting)}
	                                >
	                                  {submitting === `retry-step:${step.id}` ? "Requesting..." : "Re-request Payout"}
	                                </Button>
	                              )}
	                            </div>
	                          )
	                        })}

	                        <Button
	                          className="w-full sm:w-auto"
	                          size="sm"
	                          variant="outline"
	                          onClick={() => openWorkflowDetails(workflow.id, workflow)}
	                        >
	                          View Details
	                        </Button>
	                      </CardContent>
	                    </Card>
	                  )
	                })
	              )}
	            </CardContent>
	          </Card>
          )}
	        </TabsContent>

	        <TabsContent value="my-badges" className="space-y-4">
          {!workflowDataLoaded || !credentialDataLoaded ? (
            <ImproverTabLoadingCard label="Loading your badges..." />
          ) : (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Award className="h-5 w-5" />
                My Badges
              </CardTitle>
              <CardDescription>Credential badges associated with your currently active credentials.</CardDescription>
            </CardHeader>
            <CardContent>
              {myBadgeItems.length === 0 ? (
                <p className="text-sm text-muted-foreground">No active credential badges yet.</p>
              ) : (
                <div className="space-y-4">
                  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                    {paginatedMyBadgeItems.map((badge) => {
                      const badgeCardClassName = "rounded-lg border bg-secondary/20 p-3 text-left"
                      if (badge.badgeUrl) {
                        const badgeUrl = badge.badgeUrl
                        return (
                          <button
                            key={badge.credential}
                            type="button"
                            className={cn(badgeCardClassName, "transition-colors hover:bg-secondary/30 active:bg-secondary/40")}
                            onClick={() => setBadgePreview({ label: badge.label, imageUrl: badgeUrl })}
                          >
                            <div className="overflow-hidden rounded-md border bg-background">
                              <img
                                src={badgeUrl}
                                alt={`${badge.label} badge`}
                                className="h-36 w-full object-cover sm:h-44"
                              />
                            </div>
                            <p className="mt-3 text-sm font-medium">{badge.label}</p>
                          </button>
                        )
                      }

                      return (
                        <div key={badge.credential} className={badgeCardClassName}>
                          <div className="flex h-36 items-center justify-center rounded-md border bg-background sm:h-44">
                            <Award className="h-8 w-8 text-muted-foreground" />
                          </div>
                          <p className="mt-3 text-sm font-medium">{badge.label}</p>
                        </div>
                      )
                    })}
                  </div>

                  <div className="flex items-center justify-between">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setMyBadgesPage((page) => Math.max(0, page - 1))}
                      disabled={myBadgesPage === 0}
                    >
                      <ChevronLeft className="h-4 w-4" />
                      Previous
                    </Button>
                    <span className="text-sm text-muted-foreground">
                      Page {Math.min(myBadgesPage + 1, myBadgesTotalPages)} of {myBadgesTotalPages}
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setMyBadgesPage((page) => page + 1)}
                      disabled={myBadgesPage >= myBadgesTotalPages - 1}
                    >
                      Next
                      <ChevronRight className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
          )}
	        </TabsContent>

	        <TabsContent value="credentials" className="space-y-4">
          {!workflowDataLoaded || !credentialDataLoaded ? (
            <ImproverTabLoadingCard label="Loading credentials..." />
          ) : (
          <>
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
                    {getCredentialLabel(credential)}
                  </Badge>
                ))
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Credential Requests</CardTitle>
              <CardDescription>Request additional credentials for future workflow claims.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {requestableCredentialTypes.length === 0 ? (
                <p className="text-sm text-muted-foreground">No additional credential types are available to request.</p>
              ) : (
                <div className="grid gap-3 md:grid-cols-[1fr_auto]">
                  <Popover open={credentialComboOpen} onOpenChange={setCredentialComboOpen}>
                    <PopoverTrigger asChild>
                      <Button variant="outline" role="combobox" className="w-full justify-between font-normal">
                        {credentialRequestType ? getCredentialLabel(credentialRequestType) : "Select a credential type"}
                        <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-[--radix-popover-trigger-width] p-2" align="start">
                      <div className="relative mb-2">
                        <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                        <Input
                          placeholder="Search credential types..."
                          value={credentialSearch}
                          onChange={(e) => setCredentialSearch(e.target.value)}
                          className="pl-8 h-8 text-sm"
                        />
                      </div>
                      <div className="max-h-48 overflow-y-auto space-y-0.5">
                        {filteredCredentialTypes.length === 0 ? (
                          <p className="text-sm text-muted-foreground px-2 py-1.5">No credential types found.</p>
                        ) : (
                          filteredCredentialTypes.map((type) => (
                            <button
                              key={type.value}
                              type="button"
                              className={cn(
                                "w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent transition-colors",
                                credentialRequestType === type.value && "bg-accent font-medium"
                              )}
                              onClick={() => {
                                setCredentialRequestType(type.value)
                                setCredentialComboOpen(false)
                                setCredentialSearch("")
                              }}
                            >
                              {type.label}
                            </button>
                          ))
                        )}
                      </div>
                    </PopoverContent>
                  </Popover>
                  <Button className="w-full md:w-auto" onClick={requestCredential} disabled={submitting === "credential-request" || !credentialRequestType}>
                    {submitting === "credential-request" ? "Submitting..." : "Request Credential"}
                  </Button>
                </div>
              )}

              {credentialRequests.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">Your Request History</p>
                  {credentialRequests.map((request) => (
                    <div key={request.id} className="rounded border bg-secondary/30 p-3 text-xs space-y-1">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className="font-medium">{getCredentialLabel(request.credential_type)}</p>
                        <Badge variant={request.status === "approved" ? "default" : request.status === "rejected" ? "destructive" : "outline"}>
                          {formatStatusLabel(request.status)}
                        </Badge>
                      </div>
                      <p>Requested: {new Date(request.requested_at).toLocaleString()}</p>
                      {request.resolved_at && <p>Resolved: {new Date(request.resolved_at).toLocaleString()}</p>}
                      <p className="break-all text-muted-foreground">Request ID: {request.id}</p>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
          </>
          )}
        </TabsContent>

        <TabsContent value="absence" className="space-y-4">
          {!workflowDataLoaded || !absenceDataLoaded ? (
            <ImproverTabLoadingCard label="Loading absence coverage..." />
          ) : (
          <Card>
            <CardHeader>
              <CardTitle>Recurring Absence Coverage</CardTitle>
            <CardDescription>
              Set an absent period for a recurring claimed workpiece so other qualified improvers can claim those occurrences while you are away.
            </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {recurringClaimOptions.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No recurring claimed workpieces found yet. Claim a recurring workflow step first to configure absence coverage.
                </p>
              ) : (
                <>
                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-1">
                      <Label>Coverage Target</Label>
                      <Select
                        value={absenceTargetMode}
                        onValueChange={(value) => setAbsenceTargetMode(value as "single" | "all")}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="single">One workflow series step</SelectItem>
                          <SelectItem value="all">All active workflow serieses</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    {absenceTargetMode === "single" && (
                      <div className="space-y-1">
                        <Label>Recurring Claimed Workpiece</Label>
                        <Select value={absenceSelection} onValueChange={setAbsenceSelection}>
                          <SelectTrigger>
                            <SelectValue placeholder="Select a recurring claimed workpiece" />
                          </SelectTrigger>
                          <SelectContent>
                            {recurringClaimOptions.map((option) => (
                              <SelectItem key={option.key} value={option.key}>
                                {option.workflowTitle} • Step {option.stepOrder} ({option.stepTitle})
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    )}
                    <div className="space-y-1">
                      <Label>Absent Start Date</Label>
                      <Input
                        type="date"
                        value={absenceFrom}
                        onChange={(e) => setAbsenceFrom(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Absent End Date</Label>
                      <Input
                        type="date"
                        value={absenceUntil}
                        onChange={(e) => setAbsenceUntil(e.target.value)}
                      />
                    </div>
                  </div>
                  <Button className="w-full sm:w-auto" onClick={createAbsencePeriod} disabled={submitting === "absence"}>
                    {submitting === "absence"
                      ? "Saving..."
                      : absenceTargetMode === "all"
                        ? "Save Absent Period For All Active Serieses"
                        : "Save Absent Period"}
                  </Button>
                </>
              )}

              {absencePeriods.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">Your Absence Periods</p>
                  <div className="relative">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                      placeholder="Filter by series ID..."
                      value={absenceSearch}
                      onChange={(e) => setAbsenceSearch(e.target.value)}
                      className="pl-9"
                    />
                  </div>
                  {filteredAbsencePeriods.map((period) => (
                    <div key={period.id} className="rounded border bg-secondary/30 p-3 text-xs space-y-1">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className="font-medium">
                          Step {period.step_order}
                        </p>
                        <Badge variant="outline">
                          {period.absent_until * 1000 < Date.now() ? "Ended" : "Scheduled"}
                        </Badge>
                      </div>
                      <p>Series: {period.series_id}</p>
                      {editingAbsenceId === period.id ? (
                        <div className="grid gap-2 pt-1 sm:grid-cols-2">
                          <div className="space-y-1">
                            <Label className="text-xs">Absent Start Date</Label>
                            <Input
                              type="date"
                              value={editAbsenceFrom}
                              onChange={(e) => setEditAbsenceFrom(e.target.value)}
                              disabled={Boolean(submitting)}
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Absent End Date</Label>
                            <Input
                              type="date"
                              value={editAbsenceUntil}
                              onChange={(e) => setEditAbsenceUntil(e.target.value)}
                              disabled={Boolean(submitting)}
                            />
                          </div>
                          <div className="flex flex-wrap gap-2 sm:col-span-2">
                            <Button
                              size="sm"
                              className="w-full sm:w-auto"
                              onClick={() => updateAbsencePeriod(period.id)}
                              disabled={Boolean(submitting)}
                            >
                              {submitting === `absence-update:${period.id}` ? "Saving..." : "Save"}
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              className="w-full sm:w-auto"
                              onClick={cancelEditAbsencePeriod}
                              disabled={Boolean(submitting)}
                            >
                              Cancel
                            </Button>
                          </div>
                        </div>
                      ) : (
                        <>
                          <p>From: {formatDateFromUnix(period.absent_from)}</p>
                          <p>Until: {formatDateFromUnix(period.absent_until, true)}</p>
                          <div className="flex flex-wrap gap-2 pt-1">
                            <Button
                              size="sm"
                              variant="outline"
                              className="w-full sm:w-auto"
                              onClick={() => beginEditAbsencePeriod(period)}
                              disabled={Boolean(submitting)}
                            >
                              Edit
                            </Button>
                            <Button
                              size="sm"
                              variant="destructive"
                              className="w-full sm:w-auto"
                              onClick={() => deleteAbsencePeriod(period)}
                              disabled={Boolean(submitting)}
                            >
                              {submitting === `absence-delete:${period.id}` ? "Deleting..." : "Delete"}
                            </Button>
                          </div>
                        </>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
          )}
        </TabsContent>

      </Tabs>

      <Dialog
        open={Boolean(localPhotoPreview)}
        onOpenChange={(open) => {
          if (!open) {
            closeLocalPhotoPreview()
          }
        }}
      >
        <DialogContent className="w-[95vw] max-w-2xl">
          <DialogHeader>
            <DialogTitle>{localPhotoPreview?.label || "Photo Preview"}</DialogTitle>
            <DialogDescription>Preview selected photo before submitting the workflow step.</DialogDescription>
          </DialogHeader>
          {localPhotoPreview ? (
            <div className="space-y-3">
              <img
                src={localPhotoPreview.url}
                alt={localPhotoPreview.label}
                className="max-h-[70vh] w-full rounded border object-contain bg-secondary/20"
              />
              <div className="flex justify-end">
                <Button type="button" variant="outline" onClick={closeLocalPhotoPreview}>
                  Close
                </Button>
              </div>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Preview unavailable.</p>
          )}
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(badgePreview)}
        onOpenChange={(open) => {
          if (!open) {
            setBadgePreview(null)
          }
        }}
      >
        <DialogContent className="w-[95vw] max-w-md sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{badgePreview?.label || "Badge Preview"}</DialogTitle>
            <DialogDescription>Tap outside or close to return to your badges.</DialogDescription>
          </DialogHeader>
          {badgePreview ? (
            <div className="space-y-3">
              <img
                src={badgePreview.imageUrl}
                alt={`${badgePreview.label} badge`}
                className="max-h-[70vh] w-full rounded border object-contain bg-secondary/20"
              />
              <div className="flex justify-end">
                <Button type="button" variant="outline" onClick={() => setBadgePreview(null)}>
                  Close
                </Button>
              </div>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Preview unavailable.</p>
          )}
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(unclaimConfirmTarget)}
        onOpenChange={(open) => {
          if (!open) setUnclaimConfirmTarget(null)
        }}
      >
        <DialogContent className="w-[95vw] max-w-md">
          <DialogHeader>
            <DialogTitle>Unclaim Series?</DialogTitle>
            <DialogDescription>
              {unclaimConfirmTarget
                ? `This will release your future claims for "${unclaimConfirmTarget.workflowTitle}" (step ${unclaimConfirmTarget.stepOrder}) in this series.`
                : "This will release your future claims in this series."}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              variant="outline"
              onClick={() => setUnclaimConfirmTarget(null)}
              disabled={Boolean(submitting)}
              className="w-full sm:w-auto"
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => void confirmUnclaimSeries()}
              disabled={
                !unclaimConfirmTarget ||
                submitting === `unclaim-series:${unclaimConfirmTarget.seriesId}:${unclaimConfirmTarget.stepOrder}`
              }
              className="w-full sm:w-auto"
            >
              {unclaimConfirmTarget &&
              submitting === `unclaim-series:${unclaimConfirmTarget.seriesId}:${unclaimConfirmTarget.stepOrder}`
                ? "Unclaiming..."
                : "Confirm Unclaim"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <WorkflowDetailsModal
        workflow={detailWorkflow}
        open={detailOpen}
        onOpenChange={(open) => {
          setDetailOpen(open)
          if (!open) {
            setDetailSeriesContext(null)
            closeLocalPhotoPreview()
          }
        }}
        loading={detailLoading}
        initialStepIndex={detailInitialStepIndex}
        renderHeaderContent={renderWorkflowSuccessHeader}
        renderTopRightActions={renderWorkflowTopRightActions}
        renderWorkflowActions={renderWorkflowHeaderActions}
        renderStepActions={renderWorkflowStepActions}
        hideDefaultStepDetails={(workflow, step) =>
          step.assigned_improver_id === user?.id &&
          (step.status === "available" || step.status === "in_progress")
        }
        onDownloadPhoto={downloadWorkflowPhoto}
        downloadingPhotoId={downloadingPhotoId}
      />
    </div>
  )
}
