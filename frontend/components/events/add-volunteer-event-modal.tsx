"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { AlertTriangle, ImageIcon, Loader2, Upload, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { useApp } from "@/context/AppProvider"
import { cn } from "@/lib/utils"

const MAX_PHOTOS = 6
const MAX_PHOTO_BYTES = 8 * 1024 * 1024

const TIMEZONES = [
  "America/Los_Angeles",
  "America/Denver",
  "America/Chicago",
  "America/New_York",
  "UTC",
]

export interface VolunteerEventDraft {
  title: string
  description: string
  start_at_local: string
  end_at_local: string
  timezone: string
  max_participants: number
  reward_amount_sfluv: number
  signup_mode: "none" | "external" | "internal"
  signup_url?: string
  qr_cutoff_local?: string
  recurrence?: {
    frequency: "daily" | "weekly" | "monthly"
    monthly_mode?: "day_of_month" | "day_of_week"
    week_of_month?: number
    until_local?: string
  }
  /** Cover photos already staged. The server attaches them as it inserts. */
  photo_ids?: string[]
}

interface AddVolunteerEventModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Creates the event and returns its id, or null when creation failed. */
  createEvent: (draft: VolunteerEventDraft) => Promise<string | null>
  /**
   * Uploads one cover photo ahead of the event and resolves to its staged id.
   * Rejects with a message worth showing if the upload fails.
   */
  stagePhoto: (file: File) => Promise<string>
  /** Discards a staged photo the author removed before submitting. */
  discardPhoto: (photoId: string) => Promise<void>
  unallocatedBalance: number
  /** "Create event" for admins; affiliates submit a request for approval. */
  submitLabel?: string
  /**
   * The event being edited, or undefined to create a new one.
   *
   * Editing reuses this form rather than a second one: a divergent edit form is
   * how a field ends up creatable but not editable.
   */
  editEvent?: EditableVolunteerEvent | null
}

/**
 * Creation flow for a volunteer event — the one that populates the public
 * volunteers panel, replacing the old faucet-only event form.
 *
 * Times are entered and submitted as WALL CLOCK (`datetime-local` emits exactly
 * "2026-08-06T13:00") together with the event's timezone. The server converts
 * to an instant. The browser's own zone is deliberately not used: an admin in
 * one timezone scheduling an event in another must get the event's local time,
 * and a recurring series has to re-anchor to local time across DST.
 */
/** The subset of a managed event this form can round-trip. */
export interface EditableVolunteerEvent {
  id: string
  title: string
  description?: string
  start_at: string
  end_at: string
  timezone: string
  max_participants: number
  reward_amount_sfluv: number
  signup?: { mode: string; url?: string | null }
  /**
   * The machine-readable rule, not just its summary.
   *
   * The form rebuilds the recurrence from these on save, and a draft with no
   * recurrence block means "one-off" — so reading only the summary would
   * quietly turn a repeating event into a single occurrence on the first edit.
   */
  recurrence?: {
    frequency?: string
    monthly_mode?: string
    week_of_month?: number | null
    until?: string | null
    summary?: string
  } | null
}

/**
 * An instant rendered as a `datetime-local` value in a named timezone.
 *
 * The payload carries instants, the inputs carry wall clock in the event's own
 * zone, and the two are only the same for someone sitting in that zone.
 * Formatting through Intl is what keeps an editor in another city from shifting
 * every event they open.
 */
function toLocalInputValue(instant: string, timezone: string): string {
  const date = new Date(instant)
  if (Number.isNaN(date.getTime())) return ""

  try {
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone: timezone || undefined,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).formatToParts(date)
    const get = (type: string) => parts.find((part) => part.type === type)?.value ?? ""
    const hour = get("hour") === "24" ? "00" : get("hour")
    return `${get("year")}-${get("month")}-${get("day")}T${hour}:${get("minute")}`
  } catch {
    // An unknown zone should not blank the form; fall back to the viewer's.
    return ""
  }
}

/** One chosen cover photo and how far its upload has got. */
interface StagedPhoto {
  /** Stable across re-renders; the object URL doubles as the React key. */
  previewUrl: string
  name: string
  status: "uploading" | "ready" | "failed"
  /** Set once the server has the bytes. */
  id?: string
  error?: string
}

export function AddVolunteerEventModal({
  open,
  onOpenChange,
  createEvent,
  stagePhoto,
  discardPhoto,
  unallocatedBalance,
  submitLabel = "Create event",
  editEvent,
}: AddVolunteerEventModalProps) {
  const { user } = useApp()
  // Shown read-only so the organizer can see their identity is recorded
  // alongside the event, without implying it is theirs to change.
  const createdBy = [user?.name, user?.contact_email].filter(Boolean).join(" · ")

  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [startAtLocal, setStartAtLocal] = useState("")
  const [endAtLocal, setEndAtLocal] = useState("")
  const [timezone, setTimezone] = useState(TIMEZONES[0])
  const [maxParticipants, setMaxParticipants] = useState(20)
  const [rewardAmount, setRewardAmount] = useState(10)
  const [signupMode, setSignupMode] = useState<"none" | "external" | "internal">("internal")
  const [signupUrl, setSignupUrl] = useState("")
  const [frequency, setFrequency] = useState<"none" | "daily" | "weekly" | "monthly">("none")
  const [monthlyMode, setMonthlyMode] = useState<"day_of_month" | "day_of_week">("day_of_month")
  const [weekOfMonth, setWeekOfMonth] = useState(1)
  const [untilLocal, setUntilLocal] = useState("")
  const [useCustomCutoff, setUseCustomCutoff] = useState(false)
  const [qrCutoffLocal, setQrCutoffLocal] = useState("")
  /*
   * Photos upload the moment they are chosen rather than after the event is
   * created, so by submit time they are almost always already on the server.
   * Each carries its own state: an event is only created once every one of
   * them has a staged id to hand to it.
   */
  const [photos, setPhotos] = useState<StagedPhoto[]>([])
  const [error, setError] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  /*
   * Prefill from the event being edited, once per opening.
   *
   * Keyed on the event id rather than the object: the panel behind this modal
   * polls every 30s, and re-seeding on a new object identity would overwrite
   * whatever the editor had typed.
   */
  const editEventId = editEvent?.id ?? null
  useEffect(() => {
    if (!open || !editEvent) return

    const zone = editEvent.timezone || TIMEZONES[0]
    setTitle(editEvent.title)
    setDescription(editEvent.description ?? "")
    setTimezone(zone)
    setStartAtLocal(toLocalInputValue(editEvent.start_at, zone))
    setEndAtLocal(toLocalInputValue(editEvent.end_at, zone))
    setMaxParticipants(editEvent.max_participants)
    setRewardAmount(editEvent.reward_amount_sfluv)

    const mode = editEvent.signup?.mode
    setSignupMode(mode === "none" || mode === "external" ? mode : "internal")
    setSignupUrl(editEvent.signup?.url ?? "")

    const rule = editEvent.recurrence
    const ruleFrequency = rule?.frequency
    if (ruleFrequency === "daily" || ruleFrequency === "weekly" || ruleFrequency === "monthly") {
      setFrequency(ruleFrequency)
      setMonthlyMode(rule?.monthly_mode === "day_of_week" ? "day_of_week" : "day_of_month")
      setWeekOfMonth(rule?.week_of_month ?? 1)
      setUntilLocal(rule?.until ? toLocalInputValue(rule.until, zone) : "")
    } else {
      setFrequency("none")
      setMonthlyMode("day_of_month")
      setWeekOfMonth(1)
      setUntilLocal("")
    }

    setError("")
    // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed on the id so
    // a background poll cannot reset a half-typed form.
  }, [open, editEventId])

  useEffect(() => {
    if (open) return
    // Reset only on close, so a validation failure never wipes the form.
    setTitle("")
    setDescription("")
    setStartAtLocal("")
    setEndAtLocal("")
    setMaxParticipants(20)
    setRewardAmount(10)
    setSignupMode("internal")
    setSignupUrl("")
    setFrequency("none")
    setMonthlyMode("day_of_month")
    setWeekOfMonth(1)
    setUntilLocal("")
    setUseCustomCutoff(false)
    setQrCutoffLocal("")
    // Discard anything staged but never submitted, so the bytes go now rather
    // than waiting on the server's orphan sweep. Best effort by design.
    for (const photo of photosRef.current) {
      URL.revokeObjectURL(photo.previewUrl)
      if (photo.id) void discardPhoto(photo.id).catch(() => undefined)
    }
    setPhotos([])
    setError("")
    // eslint-disable-next-line react-hooks/exhaustive-deps -- runs on close only
  }, [open])

  const totalCost = useMemo(
    () => Math.max(0, maxParticipants) * Math.max(0, rewardAmount),
    [maxParticipants, rewardAmount],
  )
  // Requesters (affiliates) do not spend against a balance, so the budget line
  // would be noise; approval is where the faucet is actually verified.
  const showsBudget = unallocatedBalance < Number.MAX_SAFE_INTEGER && !editEvent
  /*
   * Only creation is checked against the whole cost here. An edit's cost is
   * already reserved, so the server checks the DELTA — blocking an edit whose
   * total exceeds the free balance would refuse harmless changes to an event
   * that is already funded.
   */
  const overBudget = showsBudget && totalCost > unallocatedBalance

  const addPhotos = (files: FileList | null) => {
    if (!files) return
    const incoming = Array.from(files)
    const tooBig = incoming.find((file) => file.size > MAX_PHOTO_BYTES)
    if (tooBig) {
      setError(`"${tooBig.name}" is larger than 8 MB.`)
      return
    }

    const room = MAX_PHOTOS - photos.length
    const accepted = incoming.slice(0, Math.max(0, room))
    if (accepted.length === 0) return

    setError("")
    for (const file of accepted) {
      // Its own object URL is the identity for the whole upload: unique per
      // pick, so two files with the same name never collide, and it is what
      // the thumbnail renders from.
      const previewUrl = URL.createObjectURL(file)
      setPhotos((current) => [...current, { previewUrl, name: file.name, status: "uploading" }])

      void stagePhoto(file)
        .then((id) => {
          setPhotos((current) =>
            current.map((photo) =>
              photo.previewUrl === previewUrl ? { ...photo, status: "ready", id, error: undefined } : photo,
            ),
          )
        })
        .catch((uploadError: unknown) => {
          setPhotos((current) =>
            current.map((photo) =>
              photo.previewUrl === previewUrl
                ? {
                    ...photo,
                    status: "failed",
                    error: uploadError instanceof Error ? uploadError.message : "Upload failed.",
                  }
                : photo,
            ),
          )
        })
    }
  }

  const removePhoto = (previewUrl: string) => {
    const target = photos.find((photo) => photo.previewUrl === previewUrl)
    setPhotos((current) => current.filter((photo) => photo.previewUrl !== previewUrl))
    URL.revokeObjectURL(previewUrl)
    // Best effort: an orphaned staged photo is swept server-side anyway, so a
    // failed cleanup must not block the person still filling in the form.
    if (target?.id) void discardPhoto(target.id).catch(() => undefined)
  }

  /*
   * A mirror of `photos` for the async submit path.
   *
   * handleSubmit awaits, and the closure it captured would still be holding
   * the photo list as it was when the button was pressed — including the
   * "uploading" status of a photo that has since finished. The ref is always
   * current.
   */
  const photosRef = useRef<StagedPhoto[]>([])
  useEffect(() => {
    photosRef.current = photos
  }, [photos])

  /** Resolves once nothing is in flight; false if anything ended up failed. */
  const waitForUploads = async (): Promise<boolean> => {
    while (photosRef.current.some((photo) => photo.status === "uploading")) {
      await new Promise((resolve) => setTimeout(resolve, 120))
    }
    return photosRef.current.every((photo) => photo.status === "ready")
  }

  const collectPhotoIds = (): string[] =>
    photosRef.current.map((photo) => photo.id).filter((id): id is string => typeof id === "string" && id !== "")

  const uploadingCount = photos.filter((photo) => photo.status === "uploading").length
  const failedCount = photos.filter((photo) => photo.status === "failed").length
  const readyCount = photos.filter((photo) => photo.status === "ready").length
  const uploadProgress = photos.length === 0 ? 100 : Math.round((readyCount / photos.length) * 100)

  const handleSubmit = async () => {
    setError("")

    if (title.trim() === "") return setError("Give the event a title.")
    if (startAtLocal === "" || endAtLocal === "") return setError("Set a start and end time.")
    if (endAtLocal <= startAtLocal) return setError("The end time must be after the start time.")
    // Mirrors the server rule; catching it here avoids a round trip for
    // something the admin can see in the field they just filled in.
    if (new Date(startAtLocal).getTime() < Date.now() - 5 * 60 * 1000) {
      return setError("The start time must be in the future.")
    }
    if (useCustomCutoff && qrCutoffLocal === "") {
      return setError("Set a QR redemption cutoff, or untick the box to use the default.")
    }
    if (useCustomCutoff && qrCutoffLocal < endAtLocal) {
      return setError("The QR cutoff must not be before the event ends.")
    }
    if (maxParticipants < 1) return setError("Max participants must be at least 1.")
    if (signupMode === "external" && signupUrl.trim() === "") {
      return setError("An external signup needs a signup link.")
    }
    if (overBudget) {
      return setError(
        `This event needs ${totalCost} SFLUV but only ${unallocatedBalance} is unallocated in the faucet.`,
      )
    }

    const draft: VolunteerEventDraft = {
      title: title.trim(),
      description: description.trim(),
      start_at_local: startAtLocal,
      end_at_local: endAtLocal,
      timezone,
      max_participants: maxParticipants,
      reward_amount_sfluv: rewardAmount,
      signup_mode: signupMode,
      ...(signupMode === "external" ? { signup_url: signupUrl.trim() } : {}),
      ...(useCustomCutoff && qrCutoffLocal !== "" ? { qr_cutoff_local: qrCutoffLocal } : {}),
      ...(frequency !== "none"
        ? {
            recurrence: {
              frequency,
              ...(frequency === "monthly"
                ? {
                    monthly_mode: monthlyMode,
                    ...(monthlyMode === "day_of_week" ? { week_of_month: weekOfMonth } : {}),
                  }
                : {}),
              ...(untilLocal !== "" ? { until_local: untilLocal } : {}),
            },
          }
        : {}),
    }

    /*
     * Photos are a precondition, not an afterthought.
     *
     * They used to upload after the event was created, so a failed photo left
     * a published event missing its artwork and an author told to go and fix
     * it themselves. Now the event is only created once every photo has a
     * staged id, and the server attaches them inside the same transaction that
     * inserts the event — so it either exists with all of them or not at all.
     */
    if (failedCount > 0) {
      setError(
        failedCount === 1
          ? "One photo failed to upload. Remove it or retry before submitting."
          : `${failedCount} photos failed to upload. Remove them or retry before submitting.`,
      )
      return
    }

    setSubmitting(true)
    try {
      // Almost always already done — uploads started when the files were
      // chosen. The wait is only for someone who submits the instant they pick
      // a photo, and the progress bar below is what they see while it finishes.
      if (uploadingCount > 0) {
        const settled = await waitForUploads()
        if (!settled) {
          setError("A photo failed to upload. Remove it or retry before submitting.")
          return
        }
      }

      /*
       * Every failure has to land in the error line below.
       *
       * createEvent may reject with the server's own message, or resolve falsy
       * when a caller has already handled the failure its own way. The second
       * case used to `return` silently: the dialog simply sat there after a
       * press, which reads as the button being broken rather than the request
       * being refused.
       */
      try {
        const eventId = await createEvent({ ...draft, photo_ids: collectPhotoIds() })
        if (!eventId) {
          setError("Could not submit the event. Please try again.")
          return
        }
      } catch (createError) {
        setError(
          createError instanceof Error && createError.message
            ? createError.message
            : "Could not submit the event. Please try again.",
        )
        return
      }

      onOpenChange(false)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>New Volunteer Event</DialogTitle>
          <DialogDescription>
            Published to the volunteers panel on the app and sfluv.org. QR codes are minted now and become
            redeemable 24 hours before the event starts.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          {createdBy !== "" && (
            <div className="space-y-1">
              <Label htmlFor="ve-created-by">Created by</Label>
              <Input id="ve-created-by" value={createdBy} disabled readOnly />
              <p className="text-xs text-muted-foreground">
                Recorded with the event and shown to admins. Not visible to volunteers.
              </p>
            </div>
          )}

          <div className="space-y-1">
            <Label htmlFor="ve-title">Title *</Label>
            <Input id="ve-title" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Ocean Beach Cleanup" />
          </div>

          <div className="space-y-1">
            <Label htmlFor="ve-description">Description</Label>
            <Textarea
              id="ve-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What volunteers will be doing, what to bring…"
              rows={4}
            />
          </div>

          <div className="space-y-2">
            <Label>Cover photos</Label>
            <div className="flex flex-wrap items-center gap-2">
              {photos.map((photo) => (
                <div key={photo.previewUrl} className="relative">
                  {/* eslint-disable-next-line @next/next/no-img-element -- local object URL preview */}
                  <img
                    src={photo.previewUrl}
                    alt={photo.name}
                    className={cn(
                      "h-16 w-24 rounded border object-cover transition-opacity",
                      photo.status !== "ready" && "opacity-60",
                      photo.status === "failed" && "border-destructive",
                    )}
                  />
                  {/* Each photo says where it has got to, so a failure is
                      attributable to a file rather than to "something". */}
                  {photo.status === "uploading" ? (
                    <span className="absolute inset-0 flex items-center justify-center rounded bg-background/40">
                      <Loader2 className="h-4 w-4 animate-spin text-foreground" />
                    </span>
                  ) : null}
                  {photo.status === "failed" ? (
                    <span
                      className="absolute inset-0 flex items-center justify-center rounded bg-destructive/15"
                      title={photo.error}
                    >
                      <AlertTriangle className="h-4 w-4 text-destructive" />
                    </span>
                  ) : null}
                  <button
                    type="button"
                    className="absolute -right-2 -top-2 rounded-full bg-destructive p-0.5 text-white"
                    onClick={() => removePhoto(photo.previewUrl)}
                    aria-label={`Remove ${photo.name}`}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              ))}
              {photos.length < MAX_PHOTOS && (
                <Button variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
                  <Upload className="mr-2 h-3.5 w-3.5" />
                  Add photo
                </Button>
              )}
              <input
                ref={fileInputRef}
                type="file"
                multiple
                accept="image/png,image/jpeg,image/gif,image/webp"
                className="hidden"
                onChange={(e) => {
                  addPhotos(e.target.files)
                  e.target.value = ""
                }}
              />
            </div>
            {photos.length === 0 && (
              <p className="flex items-center gap-1 text-xs text-muted-foreground">
                <ImageIcon className="h-3 w-3" /> Events without a photo still publish, but look sparse in the list.
              </p>
            )}

            {/* Only while something is in flight. Uploads start on selection,
                so most of the time this never appears at all. */}
            {uploadingCount > 0 && (
              <div className="space-y-1">
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
                  <div
                    className="h-full rounded-full bg-[#eb6c6c] transition-[width] duration-300 ease-out"
                    style={{ width: `${uploadProgress}%` }}
                  />
                </div>
                <p className="text-xs text-muted-foreground">
                  Uploading photos… {readyCount} of {photos.length} done
                </p>
              </div>
            )}
            {failedCount > 0 && (
              <p className="text-xs text-destructive">
                {failedCount === 1 ? "A photo failed to upload." : `${failedCount} photos failed to upload.`} Remove
                it and try again — the event cannot be submitted without it.
              </p>
            )}
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="ve-start">Starts *</Label>
              <Input id="ve-start" type="datetime-local" value={startAtLocal} onChange={(e) => setStartAtLocal(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label htmlFor="ve-end">Ends *</Label>
              <Input id="ve-end" type="datetime-local" value={endAtLocal} onChange={(e) => setEndAtLocal(e.target.value)} />
            </div>
          </div>

          <div className="space-y-1">
            <Label>Timezone</Label>
            <Select value={timezone} onValueChange={setTimezone}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {TIMEZONES.map((zone) => (
                  <SelectItem key={zone} value={zone}>{zone.replace("_", " ")}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Times above are the event&apos;s local time in this zone, not your browser&apos;s.
            </p>
          </div>

          <div className="space-y-2">
            <Label>Repeats</Label>
            <Select value={frequency} onValueChange={(value) => setFrequency(value as typeof frequency)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="none">Does not repeat</SelectItem>
                <SelectItem value="daily">Daily</SelectItem>
                <SelectItem value="weekly">Weekly</SelectItem>
                <SelectItem value="monthly">Monthly</SelectItem>
              </SelectContent>
            </Select>

            {frequency === "monthly" && (
              <div className="grid gap-3 sm:grid-cols-2">
                <Select value={monthlyMode} onValueChange={(value) => setMonthlyMode(value as typeof monthlyMode)}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="day_of_month">On the same date each month</SelectItem>
                    <SelectItem value="day_of_week">On the same weekday each month</SelectItem>
                  </SelectContent>
                </Select>
                {monthlyMode === "day_of_week" && (
                  <Select value={String(weekOfMonth)} onValueChange={(value) => setWeekOfMonth(Number(value))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="1">First</SelectItem>
                      <SelectItem value="2">Second</SelectItem>
                      <SelectItem value="3">Third</SelectItem>
                      <SelectItem value="4">Fourth</SelectItem>
                      <SelectItem value="-1">Last</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              </div>
            )}

            {frequency !== "none" && (
              <div className="space-y-1">
                <Label htmlFor="ve-until">Repeat until (optional)</Label>
                <Input id="ve-until" type="datetime-local" value={untilLocal} onChange={(e) => setUntilLocal(e.target.value)} />
              </div>
            )}
          </div>

          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Checkbox
                id="ve-custom-cutoff"
                checked={useCustomCutoff}
                onCheckedChange={(checked) => setUseCustomCutoff(checked === true)}
              />
              <Label htmlFor="ve-custom-cutoff" className="cursor-pointer font-normal">
                Set an exact QR redemption cutoff
              </Label>
            </div>
            {useCustomCutoff ? (
              <Input
                type="datetime-local"
                value={qrCutoffLocal}
                onChange={(event) => setQrCutoffLocal(event.target.value)}
                aria-label="QR redemption cutoff"
              />
            ) : (
              <p className="text-xs text-muted-foreground">
                QR codes stay redeemable until 24 hours after the event ends, so anyone still in the queue
                when it wraps up can claim their reward.
              </p>
            )}
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="ve-max">Max participants *</Label>
              <Input
                id="ve-max"
                type="number"
                min={1}
                value={maxParticipants}
                onChange={(e) => setMaxParticipants(Number(e.target.value))}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="ve-reward">Reward per participant (SFLUV) *</Label>
              <Input
                id="ve-reward"
                type="number"
                min={0}
                value={rewardAmount}
                onChange={(e) => setRewardAmount(Number(e.target.value))}
              />
            </div>
          </div>

          <div className={`rounded-md border p-3 text-sm ${overBudget ? "border-destructive text-destructive" : "text-muted-foreground"}`}>
            {showsBudget ? (
              <>
                Reserves <strong>{totalCost} SFLUV</strong> from the faucet ({maxParticipants} × {rewardAmount}).
                {" "}Unallocated available: {unallocatedBalance}.
                {overBudget && <div className="mt-1 font-medium">Not enough unallocated balance for this event.</div>}
              </>
            ) : (
              <>
                Requests <strong>{totalCost} SFLUV</strong> ({maxParticipants} × {rewardAmount}). The faucet is
                checked by an admin at approval.
              </>
            )}
          </div>

          <div className="space-y-2">
            <Label>Sign-up</Label>
            <Select value={signupMode} onValueChange={(value) => setSignupMode(value as typeof signupMode)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="internal">Managed in SFLuv (collect sign-ups here)</SelectItem>
                <SelectItem value="external">External link</SelectItem>
                <SelectItem value="none">No sign-up needed (drop in)</SelectItem>
              </SelectContent>
            </Select>
            {signupMode === "external" && (
              <Input
                value={signupUrl}
                onChange={(e) => setSignupUrl(e.target.value)}
                placeholder="https://partner.org/signup"
              />
            )}
          </div>

          {error !== "" && <p className="text-sm text-destructive">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={submitting || overBudget}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
