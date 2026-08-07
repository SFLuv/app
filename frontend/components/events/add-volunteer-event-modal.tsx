"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { ImageIcon, Loader2, Upload, X } from "lucide-react"

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
}

interface AddVolunteerEventModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Creates the event and returns its id, or null when creation failed. */
  createEvent: (draft: VolunteerEventDraft) => Promise<string | null>
  /** Attaches one cover photo to an already-created event. */
  uploadPhoto: (eventId: string, file: File) => Promise<boolean>
  unallocatedBalance: number
  /** "Create event" for admins; affiliates submit a request for approval. */
  submitLabel?: string
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
export function AddVolunteerEventModal({
  open,
  onOpenChange,
  createEvent,
  uploadPhoto,
  unallocatedBalance,
  submitLabel = "Create event",
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
  const [photos, setPhotos] = useState<File[]>([])
  const [error, setError] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

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
    setPhotos([])
    setError("")
  }, [open])

  const totalCost = useMemo(
    () => Math.max(0, maxParticipants) * Math.max(0, rewardAmount),
    [maxParticipants, rewardAmount],
  )
  // Requesters (affiliates) do not spend against a balance, so the budget line
  // would be noise; approval is where the faucet is actually verified.
  const showsBudget = unallocatedBalance < Number.MAX_SAFE_INTEGER
  const overBudget = showsBudget && totalCost > unallocatedBalance

  const addPhotos = (files: FileList | null) => {
    if (!files) return
    const incoming = Array.from(files)
    const tooBig = incoming.find((file) => file.size > MAX_PHOTO_BYTES)
    if (tooBig) {
      setError(`"${tooBig.name}" is larger than 8 MB.`)
      return
    }
    setPhotos((current) => [...current, ...incoming].slice(0, MAX_PHOTOS))
  }

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

    setSubmitting(true)
    try {
      const eventId = await createEvent(draft)
      if (!eventId) return

      // Photos attach to an existing event, so they upload after creation. A
      // failed photo does not undo the event — the admin can retry the upload.
      let failed = 0
      for (const photo of photos) {
        if (!(await uploadPhoto(eventId, photo))) failed++
      }
      if (failed > 0) {
        setError(`Event created, but ${failed} photo(s) failed to upload. You can add them from the event.`)
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
              {photos.map((photo, index) => (
                <div key={`${photo.name}-${index}`} className="relative">
                  {/* eslint-disable-next-line @next/next/no-img-element -- local object URL preview */}
                  <img
                    src={URL.createObjectURL(photo)}
                    alt={photo.name}
                    className="h-16 w-24 rounded border object-cover"
                  />
                  <button
                    type="button"
                    className="absolute -right-2 -top-2 rounded-full bg-destructive p-0.5 text-white"
                    onClick={() => setPhotos((current) => current.filter((_, i) => i !== index))}
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
