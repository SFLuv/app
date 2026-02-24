"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { WorkflowDetailsModal } from "@/components/workflows/workflow-details-modal"
import { buildCredentialLabelMap, formatCredentialLabel } from "@/lib/credential-labels"
import { formatStatusLabel } from "@/lib/status-labels"
import { AlertTriangle, CheckCircle2, ClipboardCheck, Download, ImageDown, Wrench } from "lucide-react"
import { CredentialRequest } from "@/types/issuer"
import { CredentialType, GlobalCredentialType, ImproverAbsencePeriod, ImproverAbsencePeriodCreateResult, ImproverWorkflowFeed, Workflow, WorkflowStep } from "@/types/workflow"

type ItemFormState = {
  photos: File[]
  written: string
  dropdown: string
}

type CameraCaptureState = {
  open: boolean
  error: string
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

const maxWorkflowPhotoUploadBytes = 2 * 1024 * 1024
const maxWorkflowPhotoUploadLabel = "2MB"
const minWorkflowPhotoResizeDimension = 640
const maxWorkflowPhotoInitialDimension = 4096

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

const renderJpegBlob = (image: HTMLImageElement, width: number, height: number, quality: number) =>
  new Promise<Blob | null>((resolve) => {
    const canvas = document.createElement("canvas")
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext("2d")
    if (!ctx) {
      resolve(null)
      return
    }
    ctx.drawImage(image, 0, 0, width, height)
    canvas.toBlob((blob) => resolve(blob), "image/jpeg", quality)
  })

const toJpegFileName = (fileName: string) => {
  const trimmed = fileName.trim()
  if (!trimmed) return "workflow_photo.jpg"
  const dotIndex = trimmed.lastIndexOf(".")
  if (dotIndex <= 0) return `${trimmed}.jpg`
  return `${trimmed.slice(0, dotIndex)}.jpg`
}

export default function ImproverPage() {
  const { authFetch, status, user } = useApp()
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [managedWorkflows, setManagedWorkflows] = useState<Workflow[]>([])
  const [unpaidWorkflows, setUnpaidWorkflows] = useState<Workflow[]>([])
  const [activeCredentials, setActiveCredentials] = useState<CredentialType[]>([])
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [credentialRequests, setCredentialRequests] = useState<CredentialRequest[]>([])
  const [credentialRequestType, setCredentialRequestType] = useState<string>("")
  const [absencePeriods, setAbsencePeriods] = useState<ImproverAbsencePeriod[]>([])
  const [error, setError] = useState<string>("")
  const [notice, setNotice] = useState<string>("")
  const [loading, setLoading] = useState<boolean>(true)
  const [submitting, setSubmitting] = useState<string>("")
  const [forms, setForms] = useState<Record<string, Record<string, ItemFormState>>>({})
  const [cameraStates, setCameraStates] = useState<Record<string, CameraCaptureState>>({})
  const [absenceSelection, setAbsenceSelection] = useState<string>("")
  const [absenceFrom, setAbsenceFrom] = useState<string>("")
  const [absenceUntil, setAbsenceUntil] = useState<string>("")
  const [detailWorkflow, setDetailWorkflow] = useState<Workflow | null>(null)
  const [detailOpen, setDetailOpen] = useState<boolean>(false)
  const [detailLoading, setDetailLoading] = useState<boolean>(false)
  const [downloadingPhotoId, setDownloadingPhotoId] = useState<string | null>(null)
  const [managerSearch, setManagerSearch] = useState<string>("")
  const videoElementRefs = useRef<Record<string, HTMLVideoElement | null>>({})
  const cameraStreamRefs = useRef<Record<string, MediaStream | null>>({})
  const cameraVideoRefCallbacks = useRef<Record<string, (element: HTMLVideoElement | null) => void>>({})

  const canUsePanel = Boolean(user?.isImprover || user?.isAdmin)

  const loadFeed = useCallback(async () => {
    if (!canUsePanel) {
      setLoading(false)
      return
    }

    try {
      const [feedRes, managedRes, unpaidRes, absenceRes, credentialTypesRes, credentialRequestsRes] = await Promise.all([
        authFetch("/improvers/workflows"),
        authFetch("/improvers/managed-workflows"),
        authFetch("/improvers/unpaid-workflows"),
        authFetch("/improvers/workflows/absence-periods"),
        authFetch("/credentials/types"),
        authFetch("/improvers/credential-requests"),
      ])
      if (!feedRes.ok) {
        const text = await feedRes.text()
        throw new Error(text || "Unable to load improver workflows.")
      }
      const data = (await feedRes.json()) as ImproverWorkflowFeed
      setWorkflows(data.workflows || [])
      setActiveCredentials((data.active_credentials || []) as CredentialType[])
      if (managedRes.ok) {
        const managedData = (await managedRes.json()) as Workflow[]
        setManagedWorkflows(managedData || [])
      } else {
        setManagedWorkflows([])
      }
      if (unpaidRes.ok) {
        const unpaidData = (await unpaidRes.json()) as Workflow[]
        setUnpaidWorkflows(unpaidData || [])
      } else {
        setUnpaidWorkflows([])
      }
      if (absenceRes.ok) {
        const absenceData = (await absenceRes.json()) as ImproverAbsencePeriod[]
        setAbsencePeriods(absenceData || [])
      } else {
        setAbsencePeriods([])
      }
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
    () => credentialTypes.filter((type) => !credentialSet.has(type.value)),
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

  type RecurringClaimOption = {
    key: string
    seriesId: string
    stepOrder: number
    stepTitle: string
    workflowTitle: string
    recurrence: Workflow["recurrence"]
    claimedCount: number
    nextStartAt?: string
  }

  const recurringClaimOptions = useMemo<RecurringClaimOption[]>(() => {
    if (!user?.id) return []

    const map = new Map<string, RecurringClaimOption>()
    workflows.forEach((workflow) => {
      if (workflow.recurrence === "one_time") return

      workflow.steps.forEach((step) => {
        if (step.assigned_improver_id !== user.id) return
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
          !existing.nextStartAt || new Date(workflow.start_at).getTime() < new Date(existing.nextStartAt).getTime()
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

  const hasClaimedRoleInWorkflow = useCallback(
    (workflow: Workflow) => {
      if (!user?.id) return false
      if (workflow.manager_improver_id === user.id) return true
      return workflow.steps.some((step) => step.assigned_improver_id === user.id)
    },
    [user?.id],
  )

  const isStepCoveredByMyAbsence = (workflow: Workflow, step: WorkflowStep) => {
    if (!user?.id) return false
    if (workflow.recurrence === "one_time") return false

    const workflowStart = new Date(workflow.start_at).getTime()
    if (Number.isNaN(workflowStart)) return false

    return absencePeriods.some((period) => {
      if (period.series_id !== workflow.series_id) return false
      if (period.step_order !== step.step_order) return false

      const absentFrom = new Date(period.absent_from).getTime()
      const absentUntil = new Date(period.absent_until).getTime()
      if (Number.isNaN(absentFrom) || Number.isNaN(absentUntil)) return false

      return workflowStart >= absentFrom && workflowStart < absentUntil
    })
  }

  const canClaimStep = (workflow: Workflow, step: WorkflowStep) => {
    if (!user?.id) return false
    if (step.assigned_improver_id) return false
    if (step.status !== "available" && step.status !== "locked") return false
    if (workflow.manager_improver_id === user.id) return false
    if (alreadyAssignedInWorkflow(workflow)) return false
    if (isStepCoveredByMyAbsence(workflow, step)) return false
    if (!step.role_id) return false

    const roleMap = roleMapForWorkflow(workflow)
    const role = roleMap.get(step.role_id)
    if (!role) return false
    return role.required_credentials.every((credential) => credentialSet.has(credential))
  }

  const canClaimManager = (workflow: Workflow) => {
    if (!user?.id) return false
    if (!workflow.manager_required) return false
    if (workflow.manager_improver_id) return false
    if (!workflow.manager_role_id) return false
    if (alreadyAssignedInWorkflow(workflow)) return false

    const roleMap = roleMapForWorkflow(workflow)
    const managerRole = roleMap.get(workflow.manager_role_id)
    if (!managerRole) return false
    return managerRole.required_credentials.every((credential) => credentialSet.has(credential))
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

  const claimWorkflowManager = async (workflowId: string) => {
    setSubmitting(`manager:${workflowId}`)
    try {
      const res = await authFetch(`/improvers/workflows/${workflowId}/manager/claim`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to claim workflow manager role.")
      }
      await loadFeed()
      await refreshDetailWorkflow(workflowId)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to claim workflow manager role.")
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

  const toISOFromLocalInput = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return ""
    return date.toISOString()
  }

  const createAbsencePeriod = async () => {
    if (!absenceSelection) {
      setError("Choose a recurring claim before setting an absent period.")
      setNotice("")
      return
    }
    if (!absenceFrom || !absenceUntil) {
      setError("Absent period start and end are required.")
      setNotice("")
      return
    }

    const separatorIndex = absenceSelection.lastIndexOf(":")
    if (separatorIndex <= 0) {
      setError("Invalid recurring claim selection.")
      setNotice("")
      return
    }

    const seriesId = absenceSelection.slice(0, separatorIndex)
    const stepOrder = Number.parseInt(absenceSelection.slice(separatorIndex + 1), 10)
    if (!seriesId || Number.isNaN(stepOrder) || stepOrder <= 0) {
      setError("Invalid recurring claim selection.")
      setNotice("")
      return
    }

    const absentFromISO = toISOFromLocalInput(absenceFrom)
    const absentUntilISO = toISOFromLocalInput(absenceUntil)
    if (!absentFromISO || !absentUntilISO) {
      setError("Invalid absent period dates.")
      setNotice("")
      return
    }

    setSubmitting("absence")
    try {
      const res = await authFetch("/improvers/workflows/absence-periods", {
        method: "POST",
        body: JSON.stringify({
          series_id: seriesId,
          step_order: stepOrder,
          absent_from: absentFromISO,
          absent_until: absentUntilISO,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to create absence period.")
      }

      const data = (await res.json()) as ImproverAbsencePeriodCreateResult
      setNotice(
        data.skipped_count > 0
          ? `Absent period created. Released ${data.released_count} assignments. ${data.skipped_count} assigned steps were already in progress or completed and were not released.`
          : `Absent period created. Released ${data.released_count} assignments for coverage.`
      )
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

  const removeItemPhoto = (stepId: string, itemId: string, photoIndex: number) => {
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

  const shrinkPhotoToUploadLimit = useCallback(async (file: File) => {
    if (!file.type.startsWith("image/")) {
      throw new Error(`Only image uploads are allowed: ${file.name}`)
    }
    if (file.size <= maxWorkflowPhotoUploadBytes) {
      return file
    }

    const image = await loadImageFromFile(file)
    const imageWidth = image.naturalWidth || image.width
    const imageHeight = image.naturalHeight || image.height
    if (!imageWidth || !imageHeight) {
      throw new Error(`Unable to process image dimensions for ${file.name}`)
    }

    let targetWidth = imageWidth
    let targetHeight = imageHeight
    const largestDimension = Math.max(targetWidth, targetHeight)
    if (largestDimension > maxWorkflowPhotoInitialDimension) {
      const initialScale = maxWorkflowPhotoInitialDimension / largestDimension
      targetWidth = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetWidth * initialScale))
      targetHeight = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetHeight * initialScale))
    }

    const qualitySteps = [0.9, 0.82, 0.74, 0.66, 0.58, 0.5, 0.42]
    for (let scaleAttempt = 0; scaleAttempt < 6; scaleAttempt += 1) {
      for (const quality of qualitySteps) {
        const blob = await renderJpegBlob(image, targetWidth, targetHeight, quality)
        if (!blob) continue
        if (blob.size > maxWorkflowPhotoUploadBytes) continue
        return new File([blob], toJpegFileName(file.name), {
          type: "image/jpeg",
          lastModified: Date.now(),
        })
      }

      if (targetWidth <= minWorkflowPhotoResizeDimension && targetHeight <= minWorkflowPhotoResizeDimension) {
        break
      }
      targetWidth = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetWidth * 0.8))
      targetHeight = Math.max(minWorkflowPhotoResizeDimension, Math.round(targetHeight * 0.8))
    }

    throw new Error(`Unable to downsize ${file.name} below ${maxWorkflowPhotoUploadLabel}.`)
  }, [])

  const prepareSelectedPhotos = useCallback(
    async (files: File[]) => {
      const processed: File[] = []
      for (const file of files) {
        processed.push(await shrinkPhotoToUploadLimit(file))
      }
      return processed
    },
    [shrinkPhotoToUploadLimit],
  )

  const replaceItemPhotosFromSelection = useCallback(
    async (stepId: string, itemId: string, selectedFiles: FileList | null) => {
      const files = Array.from(selectedFiles || [])
      if (files.length === 0) {
        updateItemForm(stepId, itemId, { photos: [] })
        return
      }

      try {
        const prepared = await prepareSelectedPhotos(files)
        updateItemForm(stepId, itemId, { photos: prepared })
        setError("")
      } catch (err) {
        updateItemForm(stepId, itemId, { photos: [] })
        setError(err instanceof Error ? err.message : `Unable to process uploaded photos to fit ${maxWorkflowPhotoUploadLabel}.`)
      }
    },
    [prepareSelectedPhotos],
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

  const startCameraCapture = async (stepId: string, itemId: string) => {
    const cameraKey = cameraKeyForItem(stepId, itemId)

    if (typeof navigator === "undefined" || !navigator.mediaDevices?.getUserMedia) {
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: false,
          error: "Camera capture is not supported in this browser.",
        },
      }))
      return
    }

    stopCameraStreamByKey(cameraKey)

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: {
            ideal: "environment",
          },
        },
        audio: false,
      })
      cameraStreamRefs.current[cameraKey] = stream
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: true,
          error: "",
        },
      }))

      const videoElement = videoElementRefs.current[cameraKey]
      if (videoElement) {
        videoElement.srcObject = stream
        void videoElement.play().catch(() => undefined)
      }
    } catch {
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: false,
          error: "Camera access is required for this work item.",
        },
      }))
    }
  }

  const captureCameraPhoto = async (stepId: string, itemId: string, itemTitle: string) => {
    const cameraKey = cameraKeyForItem(stepId, itemId)
    const stream = cameraStreamRefs.current[cameraKey]
    const videoElement = videoElementRefs.current[cameraKey]
    if (!stream || !videoElement) {
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: false,
          error: "Open your camera before capturing a photo.",
        },
      }))
      return
    }

    const width = videoElement.videoWidth
    const height = videoElement.videoHeight
    if (!width || !height) {
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: true,
          error: "Camera is still initializing. Try capture again.",
        },
      }))
      return
    }

    const canvas = document.createElement("canvas")
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext("2d")
    if (!ctx) {
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: true,
          error: "Unable to capture photo from the camera stream.",
        },
      }))
      return
    }

    ctx.drawImage(videoElement, 0, 0, width, height)
    const blob = await new Promise<Blob | null>((resolve) => {
      canvas.toBlob((value) => resolve(value), "image/jpeg", 0.92)
    })
    if (!blob) {
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: true,
          error: "Unable to capture photo from the camera stream.",
        },
      }))
      return
    }

    const slug = itemTitle
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^_+|_+$/g, "")
    const filenamePrefix = slug || "workflow_photo"
    const photo = new File([blob], `${filenamePrefix}_${Date.now()}.jpg`, {
      type: blob.type || "image/jpeg",
      lastModified: Date.now(),
    })
    let preparedPhoto: File
    try {
      preparedPhoto = await shrinkPhotoToUploadLimit(photo)
    } catch (err) {
      const message = err instanceof Error ? err.message : `Unable to process captured photo for ${maxWorkflowPhotoUploadLabel} limit.`
      setCameraStates((prev) => ({
        ...prev,
        [cameraKey]: {
          open: true,
          error: message,
        },
      }))
      setError(message)
      return
    }

    addItemPhoto(stepId, itemId, preparedPhoto)
    setCameraStates((prev) => ({
      ...prev,
      [cameraKey]: {
        open: true,
        error: "",
      },
    }))
    setError("")
  }

  useEffect(() => {
    return () => {
      stopAllCameraCaptures()
    }
  }, [stopAllCameraCaptures])

  useEffect(() => {
    if (!detailOpen) {
      stopAllCameraCaptures()
    }
  }, [detailOpen, stopAllCameraCaptures])

  const fileToBase64 = (file: File) =>
    new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result
        if (typeof result !== "string") {
          reject(new Error("Unable to read uploaded photo."))
          return
        }
        const commaIndex = result.indexOf(",")
        resolve(commaIndex >= 0 ? result.slice(commaIndex + 1) : result)
      }
      reader.onerror = () => reject(new Error("Unable to read uploaded photo."))
      reader.readAsDataURL(file)
    })

  const buildCompletionPayload = async (step: WorkflowStep) => {
    const stepForms = forms[step.id] || {}
    const items: Array<{
      item_id: string
      photo_uploads?: Array<{
        file_name: string
        content_type: string
        data_base64: string
      }>
      written_response?: string
      dropdown_value?: string
    }> = []

    for (const item of step.work_items) {
      const form = stepForms[item.id] || defaultItemFormState
      const photoUploads = await Promise.all(
        form.photos.map(async (file) => {
          if (file.size > maxWorkflowPhotoUploadBytes) {
            throw new Error(`Photo ${file.name || "upload"} exceeds ${maxWorkflowPhotoUploadLabel}.`)
          }
          return {
            file_name: file.name || "photo",
            content_type: file.type || "image/jpeg",
            data_base64: await fileToBase64(file),
          }
        })
      )

      const dropdownValue = form.dropdown.trim()
      const writtenResponse = form.written.trim()
      const dropdownRequiresWritten = dropdownValue ? Boolean(item.dropdown_requires_written_response?.[dropdownValue]) : false
      const requiredWritten = item.requires_written_response || dropdownRequiresWritten

      const hasAnyInput = photoUploads.length > 0 || dropdownValue.length > 0 || writtenResponse.length > 0
      if (!item.optional && !hasAnyInput) {
        throw new Error(`Missing response for required work item: ${item.title}`)
      }

      if (item.requires_photo && photoUploads.length === 0) {
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
        photo_uploads?: Array<{
          file_name: string
          content_type: string
          data_base64: string
        }>
        written_response?: string
        dropdown_value?: string
      } = {
        item_id: item.id,
      }
      if (photoUploads.length > 0) payloadItem.photo_uploads = photoUploads
      if (writtenResponse.length > 0) payloadItem.written_response = writtenResponse
      if (dropdownValue.length > 0) payloadItem.dropdown_value = dropdownValue
      items.push(payloadItem)
    }

    return { items }
  }

  const completeStep = async (workflowId: string, step: WorkflowStep) => {
    setSubmitting(`complete:${step.id}`)
    try {
      const payload = await buildCompletionPayload(step)
      const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${step.id}/complete`, {
        method: "POST",
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to complete this step.")
      }
      await loadFeed()
      await refreshDetailWorkflow(workflowId)
      stopCameraCapturesForStep(step.id)
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

  const getFirstClaimableStep = (workflow: Workflow): WorkflowStep | null => {
    for (const step of workflow.steps) {
      if (canClaimStep(workflow, step)) {
        return step
      }
    }
    return null
  }

  const workflowBoardWorkflows = useMemo(() => {
    return workflows.filter((workflow) => {
      if (hasClaimedRoleInWorkflow(workflow)) return false
      return Boolean(getFirstClaimableStep(workflow)) || canClaimManager(workflow)
    })
  }, [workflows, hasClaimedRoleInWorkflow, credentialSet, absencePeriods, user?.id])

  const myClaimedWorkflows = useMemo(() => {
    return workflows.filter((workflow) => hasClaimedRoleInWorkflow(workflow))
  }, [workflows, hasClaimedRoleInWorkflow])

  const hasActiveManagerRole = useMemo(() => {
    if (!user?.id) return false
    return managedWorkflows.some(
      (workflow) =>
        workflow.manager_improver_id === user.id &&
        (workflow.status === "approved" ||
          workflow.status === "blocked" ||
          workflow.status === "in_progress" ||
          workflow.status === "completed"),
    )
  }, [managedWorkflows, user?.id])

  const openWorkflowDetails = async (workflowId: string, workflow?: Workflow) => {
    setError("")

    if (workflow) {
      setDetailWorkflow(workflow)
      setDetailLoading(false)
      setDetailOpen(true)
      return
    }

    const existing = [...workflows, ...managedWorkflows, ...unpaidWorkflows].find((item) => item.id === workflowId)
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

  const parseAttachmentFilename = (value: string | null) => {
    if (!value) return ""
    const quotedMatch = value.match(/filename=\"([^\"]+)\"/i)
    if (quotedMatch?.[1]) return quotedMatch[1]
    const plainMatch = value.match(/filename=([^;]+)/i)
    if (plainMatch?.[1]) return plainMatch[1].trim()
    return ""
  }

  const downloadManagedWorkflowExport = async (workflowId: string, format: "csv" | "photos") => {
    const key = `${format}:${workflowId}`
    setSubmitting(key)
    try {
      const endpoint =
        format === "csv"
          ? `/improvers/managed-workflows/${workflowId}/export.csv`
          : `/improvers/managed-workflows/${workflowId}/photos.zip`
      const res = await authFetch(endpoint)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to download workflow export.")
      }

      const blob = await res.blob()
      const disposition = res.headers.get("Content-Disposition")
      const fallback = format === "csv" ? `workflow_${workflowId}_export.csv` : `workflow_${workflowId}_photos.zip`
      const filename = parseAttachmentFilename(disposition) || fallback
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = filename
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to download workflow export.")
    } finally {
      setSubmitting("")
    }
  }

  const filteredManagedWorkflows = useMemo(() => {
    const search = managerSearch.trim().toLowerCase()
    if (!search) return managedWorkflows
    return managedWorkflows.filter((workflow) => workflow.title.toLowerCase().includes(search))
  }, [managedWorkflows, managerSearch])

  const requestStepPayout = async (workflowId: string, stepId: string) => {
    const key = `retry-step:${stepId}`
    setSubmitting(key)
    try {
      const res = await authFetch(`/improvers/workflows/${workflowId}/steps/${stepId}/payout-request`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
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

  const requestManagerPayout = async (workflowId: string) => {
    const key = `retry-manager:${workflowId}`
    setSubmitting(key)
    try {
      const res = await authFetch(`/improvers/workflows/${workflowId}/manager/payout-request`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to request manager payout retry.")
      }
      setNotice("Manager payout retry requested.")
      setError("")
      await loadFeed()
      await refreshDetailWorkflow(workflowId)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to request manager payout retry.")
      setNotice("")
    } finally {
      setSubmitting("")
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
    const claimableManager = canClaimManager(workflow)
    const isManager = workflow.manager_improver_id === user?.id

    if (!workflow.manager_required) return null
    if (!claimableManager && !isManager) return null

    return (
      <div className="flex flex-wrap items-center gap-2 rounded-md border bg-secondary/30 p-3">
        <p className="text-xs text-muted-foreground">
          Manager bounty: {workflow.manager_bounty}
        </p>
        {claimableManager && (
          <Button
            className="w-full sm:w-auto"
            size="sm"
            variant="secondary"
            onClick={() => claimWorkflowManager(workflow.id)}
            disabled={Boolean(submitting)}
          >
            {submitting === `manager:${workflow.id}` ? "Claiming..." : "Claim Manager Role"}
          </Button>
        )}
        {isManager && <Badge variant="default">You are manager</Badge>}
      </div>
    )
  }

  const renderWorkflowStepActions = (workflow: Workflow, step: WorkflowStep) => {
    const mine = step.assigned_improver_id === user?.id
    const claimable = canClaimStep(workflow, step)

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

        {mine && step.status === "available" && (
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
            <div className="flex items-center gap-2 text-sm font-medium">
              <ClipboardCheck className="h-4 w-4" />
              Work Item Responses
            </div>

            {step.work_items.map((item) => {
              const form = forms[step.id]?.[item.id] || defaultItemFormState
              const cameraKey = cameraKeyForItem(step.id, item.id)
              const cameraState = cameraStates[cameraKey] || defaultCameraCaptureState

              return (
                <Card key={item.id}>
                  <CardContent className="p-3 space-y-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-medium text-sm">{item.title}</p>
                      {item.optional && <Badge variant="outline">Optional</Badge>}
                    </div>
                    <p className="text-xs text-muted-foreground">{item.description}</p>

                    {item.requires_photo &&
                      (item.camera_capture_only ? (
                        <div className="space-y-2">
                          <div>
                            <Label className="text-xs">Camera Capture</Label>
                            <p className="text-xs text-muted-foreground">
                              Camera roll uploads are disabled for this work item. Each photo must be under {maxWorkflowPhotoUploadLabel}.
                            </p>
                          </div>

                          <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
                            <Button
                              className="w-full sm:w-auto"
                              type="button"
                              size="sm"
                              variant="outline"
                              onClick={() => startCameraCapture(step.id, item.id)}
                              disabled={Boolean(submitting)}
                            >
                              {cameraState.open ? "Restart Camera" : "Open Camera"}
                            </Button>
                            {cameraState.open && (
                              <Button
                                className="w-full sm:w-auto"
                                type="button"
                                size="sm"
                                variant="outline"
                                onClick={() => stopCameraCapture(step.id, item.id)}
                                disabled={Boolean(submitting)}
                              >
                                Stop Camera
                              </Button>
                            )}
                            {cameraState.open && (
                              <Button
                                className="w-full sm:w-auto"
                                type="button"
                                size="sm"
                                onClick={() => captureCameraPhoto(step.id, item.id, item.title)}
                                disabled={Boolean(submitting)}
                              >
                                Capture Photo
                              </Button>
                            )}
                          </div>

                          {cameraState.error && <p className="text-xs text-red-600">{cameraState.error}</p>}

                          {cameraState.open && (
                            <div className="overflow-hidden rounded border bg-black/80">
                              <video
                                ref={getCameraVideoRef(cameraKey)}
                                className="h-48 w-full object-cover"
                                playsInline
                                muted
                                autoPlay
                              />
                            </div>
                          )}

                          {form.photos.length > 0 ? (
                            <div className="space-y-1">
                              <p className="text-xs text-muted-foreground">
                                {form.photos.length} captured photo{form.photos.length === 1 ? "" : "s"}
                              </p>
                              <div className="space-y-1">
                                {form.photos.map((photo, photoIndex) => (
                                  <div
                                    key={`${photo.name}-${photo.lastModified}-${photoIndex}`}
                                    className="flex items-center justify-between gap-2 rounded border p-2 text-xs"
                                  >
                                    <span className="truncate">{photo.name}</span>
                                    <Button
                                      type="button"
                                      size="sm"
                                      variant="ghost"
                                      onClick={() => removeItemPhoto(step.id, item.id, photoIndex)}
                                      disabled={Boolean(submitting)}
                                    >
                                      Remove
                                    </Button>
                                  </div>
                                ))}
                              </div>
                            </div>
                          ) : (
                            <p className="text-xs text-muted-foreground">No captured photos yet.</p>
                          )}
                        </div>
                      ) : (
                        <div className="space-y-1">
                          <Label className="text-xs">Upload Photos</Label>
                          <p className="text-xs text-muted-foreground">
                            Each photo must be under {maxWorkflowPhotoUploadLabel}. Oversized images are resized automatically when possible.
                          </p>
                          <Input
                            type="file"
                            accept="image/*"
                            multiple
                            onChange={(e) => void replaceItemPhotosFromSelection(step.id, item.id, e.target.files)}
                          />
                          {form.photos.length > 0 && (
                            <p className="text-xs text-muted-foreground">
                              {form.photos.length} file{form.photos.length === 1 ? "" : "s"} selected
                            </p>
                          )}
                        </div>
                      ))}

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
                  Complete Step
                </>
              )}
            </Button>
          </div>
        )}
      </div>
    )
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

      {notice && (
        <div className="flex items-center gap-2 text-green-700 text-sm p-3 bg-green-50 dark:bg-green-900/20 rounded-lg">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          <span>{notice}</span>
        </div>
      )}

	      <Tabs defaultValue="workflow-board" className="space-y-4">
	        <TabsList className="grid h-auto w-full grid-cols-1 gap-2 p-1 sm:grid-cols-2 lg:grid-cols-3">
	          <TabsTrigger value="workflow-board">Workflow Board</TabsTrigger>
	          <TabsTrigger value="my-workflows">My Workflows</TabsTrigger>
	          {hasActiveManagerRole && (
	            <TabsTrigger value="managed-workflows">Manager View</TabsTrigger>
	          )}
	          <TabsTrigger value="unpaid-workflows">Unpaid Workflows</TabsTrigger>
	          <TabsTrigger value="credentials">Credentials</TabsTrigger>
	          <TabsTrigger value="absence">Absence Coverage</TabsTrigger>
	        </TabsList>

	        <TabsContent value="workflow-board" className="space-y-3">
          {workflowBoardWorkflows.length === 0 ? (
            <Card>
              <CardHeader>
                <CardTitle>No Eligible Workflows</CardTitle>
                <CardDescription>No workflows are currently available for you to claim.</CardDescription>
              </CardHeader>
            </Card>
          ) : (
	            workflowBoardWorkflows.map((workflow) => {
	              const claimableStep = getFirstClaimableStep(workflow)
	              const claimableManager = canClaimManager(workflow)

	              return (
                <Card
                  key={workflow.id}
                  className="cursor-pointer transition-colors hover:bg-muted/30"
                  onClick={() => openWorkflowDetails(workflow.id, workflow)}
                >
                  <CardContent className="pt-4 space-y-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <h4 className="font-semibold">{workflow.title}</h4>
                      </div>
                      <Badge variant={workflow.status === "in_progress" ? "default" : "secondary"}>
                        {formatStatusLabel(workflow.status)}
                      </Badge>
                    </div>

                    <p className="text-sm text-muted-foreground line-clamp-2">{workflow.description}</p>

	                    <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
	                      <span>Start: {new Date(workflow.start_at).toLocaleString()}</span>
	                      <span>Claims available in workflow details modal</span>
	                    </div>

	                    {workflow.manager_required && (
	                      <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
	                        <span>Manager bounty: {workflow.manager_bounty}</span>
	                        <span>
	                          Manager:{" "}
	                          {workflow.manager_improver_id
	                            ? workflow.manager_improver_id
	                            : "Unclaimed"}
	                        </span>
	                      </div>
	                    )}

	                    <div className="flex w-full flex-col gap-2 pt-1 sm:w-auto sm:flex-row sm:flex-wrap">
                      <Button
                        className="w-full sm:w-auto"
                        size="sm"
                        variant="outline"
                        onClick={(e) => {
                          e.stopPropagation()
                          void openWorkflowDetails(workflow.id, workflow)
                        }}
                      >
                        View Details
                      </Button>
                      {(claimableStep || claimableManager) && (
                        <Badge variant="secondary">
                          Claim available in modal
                        </Badge>
                      )}
	                    </div>
	                  </CardContent>
	                </Card>
              )
            })
          )}
	        </TabsContent>

	        <TabsContent value="my-workflows" className="space-y-3">
          {myClaimedWorkflows.length === 0 ? (
            <Card>
              <CardHeader>
                <CardTitle>No Claimed Workflows</CardTitle>
                <CardDescription>Workflows you claim will appear here.</CardDescription>
              </CardHeader>
            </Card>
          ) : (
            myClaimedWorkflows.map((workflow) => {
              const assignedSteps = workflow.steps.filter((step) => step.assigned_improver_id === user?.id)
              const actionableSteps = assignedSteps.filter((step) => step.status === "available" || step.status === "in_progress")
              const isManager = workflow.manager_improver_id === user?.id

              return (
                <Card
                  key={`mine-${workflow.id}`}
                  className="cursor-pointer transition-colors hover:bg-muted/30"
                  onClick={() => openWorkflowDetails(workflow.id, workflow)}
                >
                  <CardContent className="pt-4 space-y-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <h4 className="font-semibold">{workflow.title}</h4>
                      </div>
                      <Badge variant={workflow.status === "in_progress" ? "default" : "secondary"}>
                        {formatStatusLabel(workflow.status)}
                      </Badge>
                    </div>

                    <p className="text-sm text-muted-foreground line-clamp-2">{workflow.description}</p>

                    <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                      <span>Start: {new Date(workflow.start_at).toLocaleString()}</span>
                      <span>
                        Assigned to you: {assignedSteps.length} step{assignedSteps.length === 1 ? "" : "s"}
                        {actionableSteps.length > 0 ? ` (${actionableSteps.length} actionable)` : ""}
                      </span>
                    </div>

                    {workflow.manager_required && (
                      <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                        <span>Manager bounty: {workflow.manager_bounty}</span>
                        <span>
                          Manager:{" "}
                          {workflow.manager_improver_id
                            ? workflow.manager_improver_id === user?.id
                              ? "You"
                              : workflow.manager_improver_id
                            : "Unclaimed"}
                        </span>
                      </div>
                    )}

                    <div className="flex w-full flex-col gap-2 pt-1 sm:w-auto sm:flex-row sm:flex-wrap">
                      <Button
                        className="w-full sm:w-auto"
                        size="sm"
                        variant="outline"
                        onClick={(e) => {
                          e.stopPropagation()
                          void openWorkflowDetails(workflow.id, workflow)
                        }}
                      >
                        View Details
                      </Button>
                      {isManager && (
                        <Badge variant="default" className="px-2 py-1">
                          You are manager
                        </Badge>
                      )}
                    </div>
                  </CardContent>
                </Card>
              )
            })
          )}
	        </TabsContent>

          {hasActiveManagerRole && (
	        <TabsContent value="managed-workflows" className="space-y-4">
	          <Card>
	            <CardHeader>
	              <CardTitle>Managed Workflows</CardTitle>
	              <CardDescription>
	                Review workflows where you are the claimed workflow manager, inspect details, and export submission data.
	              </CardDescription>
	            </CardHeader>
	            <CardContent className="space-y-4">
	              <div className="space-y-1">
	                <Label>Search by Workflow Title</Label>
	                <Input
	                  value={managerSearch}
	                  onChange={(e) => setManagerSearch(e.target.value)}
	                  placeholder="Search managed workflows..."
	                />
	              </div>

	              {filteredManagedWorkflows.length === 0 ? (
	                <p className="text-sm text-muted-foreground">
	                  {managedWorkflows.length === 0
	                    ? "You are not currently assigned as a workflow manager."
	                    : "No managed workflows match your search."}
	                </p>
	              ) : (
	                <div className="space-y-3">
	                  {filteredManagedWorkflows.map((workflow) => (
	                    <Card key={`managed-${workflow.id}`}>
	                      <CardContent className="pt-4 space-y-3">
	                        <div className="flex flex-wrap items-center justify-between gap-2">
	                          <div>
	                            <h4 className="font-semibold">{workflow.title}</h4>
	                          </div>
	                          <Badge>{formatStatusLabel(workflow.status)}</Badge>
	                        </div>
	                        <p className="text-sm text-muted-foreground line-clamp-2">{workflow.description}</p>
	                        <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
	                          <span>Manager bounty: {workflow.manager_bounty}</span>
	                          <span>Start: {new Date(workflow.start_at).toLocaleString()}</span>
	                        </div>
	                        <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:flex-wrap">
	                          <Button
	                            className="w-full sm:w-auto"
	                            size="sm"
	                            variant="outline"
	                            onClick={() => openWorkflowDetails(workflow.id, workflow)}
	                          >
	                            View Details
	                          </Button>
	                          <Button
	                            className="w-full sm:w-auto"
	                            size="sm"
	                            variant="secondary"
	                            onClick={() => downloadManagedWorkflowExport(workflow.id, "csv")}
	                            disabled={Boolean(submitting)}
	                          >
	                            <Download className="mr-2 h-4 w-4" />
	                            {submitting === `csv:${workflow.id}` ? "Downloading..." : "Download CSV"}
	                          </Button>
	                          <Button
	                            className="w-full sm:w-auto"
	                            size="sm"
	                            variant="secondary"
	                            onClick={() => downloadManagedWorkflowExport(workflow.id, "photos")}
	                            disabled={Boolean(submitting)}
	                          >
	                            <ImageDown className="mr-2 h-4 w-4" />
	                            {submitting === `photos:${workflow.id}` ? "Downloading..." : "Download Pictures"}
	                          </Button>
	                        </div>
	                      </CardContent>
	                    </Card>
	                  ))}
	                </div>
	              )}
	            </CardContent>
	          </Card>
	        </TabsContent>
          )}

	        <TabsContent value="unpaid-workflows" className="space-y-4">
	          <Card>
	            <CardHeader>
	              <CardTitle>Unpaid Workflows</CardTitle>
	              <CardDescription>
	                Completed work awaiting payout finalization. If a payout failed, request a retry here.
	              </CardDescription>
	            </CardHeader>
	            <CardContent className="space-y-3">
	              {unpaidWorkflows.length === 0 ? (
	                <p className="text-sm text-muted-foreground">No unpaid workflow payouts are pending for you.</p>
	              ) : (
                unpaidWorkflows.map((workflow) => {
                  const unpaidSteps = workflow.steps.filter(
                    (step) => step.assigned_improver_id === user?.id && step.status === "completed" && step.bounty > 0
                  )
                  const isManagerUnpaid =
                    workflow.manager_improver_id === user?.id &&
                    workflow.manager_bounty > 0 &&
                    !workflow.manager_paid_out_at
                  if (unpaidSteps.length === 0 && !isManagerUnpaid) {
                    return null
                  }

                  return (
	                    <Card key={`unpaid-${workflow.id}`}>
	                      <CardContent className="pt-4 space-y-3">
	                        <div className="flex flex-wrap items-center justify-between gap-2">
	                          <div>
	                            <h4 className="font-semibold">{workflow.title}</h4>
	                          </div>
	                          <Badge>{formatStatusLabel(workflow.status)}</Badge>
	                        </div>
	                        <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
	                          <span>Start: {new Date(workflow.start_at).toLocaleString()}</span>
	                          <span>
	                            Pending payouts: {unpaidSteps.length + (isManagerUnpaid ? 1 : 0)}
	                          </span>
	                        </div>

	                        {unpaidSteps.map((step) => {
	                          const hasError = Boolean(step.payout_error?.trim())
	                          return (
	                            <div key={`unpaid-step-${step.id}`} className="rounded border p-3 space-y-2">
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

	                        {isManagerUnpaid && (
	                          <div className="rounded border p-3 space-y-2">
	                            <div className="flex flex-wrap items-center justify-between gap-2">
	                              <p className="text-sm font-medium">Workflow Manager Payout</p>
	                              <Badge variant={workflow.manager_payout_error ? "destructive" : "outline"}>
	                                {workflow.manager_payout_error ? "Payout Error" : "Pending"}
	                              </Badge>
	                            </div>
	                            <p className="text-xs text-muted-foreground">Bounty: {workflow.manager_bounty} SFLuv</p>
	                            {workflow.manager_payout_error ? (
	                              <p className="text-xs text-red-600 whitespace-pre-wrap">{workflow.manager_payout_error}</p>
	                            ) : (
	                              <p className="text-xs text-muted-foreground">
	                                Payout is waiting for earlier workflows in this series to finish and settle.
	                              </p>
	                            )}
	                            {workflow.manager_payout_error && (
	                              <Button
	                                className="w-full sm:w-auto"
	                                size="sm"
	                                variant="secondary"
	                                onClick={() => requestManagerPayout(workflow.id)}
	                                disabled={Boolean(submitting)}
	                              >
	                                {submitting === `retry-manager:${workflow.id}` ? "Requesting..." : "Re-request Payout"}
	                              </Button>
	                            )}
	                          </div>
	                        )}

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
	        </TabsContent>

	        <TabsContent value="credentials" className="space-y-4">
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
                  <Select value={credentialRequestType} onValueChange={setCredentialRequestType}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select a credential type" />
                    </SelectTrigger>
                    <SelectContent>
                      {requestableCredentialTypes.map((type) => (
                        <SelectItem key={type.value} value={type.value}>
                          {type.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
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
        </TabsContent>

        <TabsContent value="absence" className="space-y-4">
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
                    <div className="space-y-1">
                      <Label>Absent Start</Label>
                      <Input
                        type="datetime-local"
                        value={absenceFrom}
                        onChange={(e) => setAbsenceFrom(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Absent End</Label>
                      <Input
                        type="datetime-local"
                        value={absenceUntil}
                        onChange={(e) => setAbsenceUntil(e.target.value)}
                      />
                    </div>
                  </div>
                  <Button className="w-full sm:w-auto" onClick={createAbsencePeriod} disabled={submitting === "absence"}>
                    {submitting === "absence" ? "Saving..." : "Save Absent Period"}
                  </Button>
                </>
              )}

              {absencePeriods.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">Your Absence Periods</p>
                  {absencePeriods.map((period) => (
                    <div key={period.id} className="rounded border bg-secondary/30 p-3 text-xs space-y-1">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className="font-medium">
                          Step {period.step_order}
                        </p>
                        <Badge variant="outline">
                          {new Date(period.absent_until).getTime() < Date.now() ? "Ended" : "Scheduled"}
                        </Badge>
                      </div>
                      <p>From: {new Date(period.absent_from).toLocaleString()}</p>
                      <p>Until: {new Date(period.absent_until).toLocaleString()}</p>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

      </Tabs>

      <WorkflowDetailsModal
        workflow={detailWorkflow}
        open={detailOpen}
        onOpenChange={(open) => setDetailOpen(open)}
        loading={detailLoading}
        renderWorkflowActions={renderWorkflowHeaderActions}
        renderStepActions={renderWorkflowStepActions}
        onDownloadPhoto={downloadWorkflowPhoto}
        downloadingPhotoId={downloadingPhotoId}
      />
    </div>
  )
}
