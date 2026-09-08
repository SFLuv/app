"use client"

import { useEffect, useMemo, useState } from "react"
import { AlertTriangle, Loader2, MapPin, RefreshCw } from "lucide-react"

import PlaceAutocomplete from "@/components/merchant/google_place_finder"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import {
  OpeningHoursEditor,
  toWeek,
  validateWeek,
  type LocationDayHours,
} from "@/components/locations/opening-hours-editor"
import type { AuthedLocation, GoogleSubLocation } from "@/types/location"

interface EditMerchantLocationModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  location: AuthedLocation | null
  /** Existing types across all listings, offered as suggestions. */
  knownTypes?: string[]
  /** Called after a successful save so the caller can refetch. */
  onSaved: () => void | Promise<void>
}

/**
 * Admin editor for a merchant listing.
 *
 * Two separate save paths, deliberately not merged:
 *
 * The field editor writes exactly what the admin typed. Re-pointing at a Google
 * listing instead refetches everything server-side and overwrites name, address,
 * type and hours with Google's values — so it discards edits rather than merging
 * with them. Presenting them as one button would make that destructive and
 * invisible; they are separate actions with the consequence stated.
 */
export function EditMerchantLocationModal({
  open,
  onOpenChange,
  location,
  knownTypes = [],
  onSaved,
}: EditMerchantLocationModalProps) {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [name, setName] = useState("")
  const [type, setType] = useState("")
  const [description, setDescription] = useState("")
  const [street, setStreet] = useState("")
  const [city, setCity] = useState("")
  const [state, setState] = useState("")
  const [zip, setZip] = useState("")
  const [phone, setPhone] = useState("")
  const [email, setEmail] = useState("")
  const [website, setWebsite] = useState("")
  const [hours, setHours] = useState<LocationDayHours[]>(() => toWeek(null))
  const [hoursManual, setHoursManual] = useState(false)
  const [googlePlace, setGooglePlace] = useState<GoogleSubLocation | null>(null)

  const [saving, setSaving] = useState(false)
  const [resyncing, setResyncing] = useState(false)
  const [error, setError] = useState("")

  // Reload from the record every time the dialog opens, so a previous edit
  // cannot leak into the next merchant an admin looks at.
  useEffect(() => {
    if (!open || !location) return
    setName(location.name ?? "")
    setType(location.type ?? "")
    setDescription(location.description ?? "")
    setStreet(location.street ?? "")
    setCity(location.city ?? "")
    setState(location.state ?? "")
    setZip(location.zip ?? "")
    setPhone(location.phone ?? "")
    setEmail(location.email ?? "")
    setWebsite(location.website ?? "")
    setHours(toWeek(location.hours))
    setHoursManual(!!location.hours_manual)
    setGooglePlace(null)
    setError("")
  }, [open, location])

  const typeSuggestions = useMemo(
    () => Array.from(new Set(knownTypes.map((entry) => entry?.trim()).filter(Boolean) as string[])).sort(),
    [knownTypes],
  )

  const busy = saving || resyncing

  const saveDetails = async () => {
    if (!location) return
    const invalid = validateWeek(hours)
    if (invalid) {
      setError(invalid)
      return
    }
    setSaving(true)
    setError("")
    try {
      const res = await authFetch(`/admin/locations/${location.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name.trim(),
          description: description.trim(),
          type: type.trim(),
          street: street.trim(),
          city: city.trim(),
          state: state.trim(),
          zip: zip.trim(),
          phone: phone.trim(),
          email: email.trim(),
          website: website.trim(),
          hours,
          hours_manual: hoursManual,
        }),
      })
      if (!res.ok) {
        // The server explains validation failures; surface its wording rather
        // than a generic message an admin cannot act on.
        const body = await res.json().catch(() => null)
        throw new Error(body?.error || "Unable to save these details.")
      }
      toast({ title: "Merchant updated", description: `${name.trim()} has been saved.` })
      await onSaved()
      onOpenChange(false)
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "Unable to save these details.")
    } finally {
      setSaving(false)
    }
  }

  const resyncFromGoogle = async () => {
    if (!location || !googlePlace) return
    setResyncing(true)
    setError("")
    try {
      const res = await authFetch(`/admin/locations/${location.id}/google-place`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ google_id: googlePlace.google_id }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        throw new Error(body?.error || "Unable to re-point this listing.")
      }
      toast({
        title: "Listing re-pointed",
        description: "Name, address, type and hours now come from the selected Google listing.",
      })
      await onSaved()
      onOpenChange(false)
    } catch (resyncError) {
      setError(resyncError instanceof Error ? resyncError.message : "Unable to re-point this listing.")
    } finally {
      setResyncing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] overflow-y-auto p-4 sm:max-w-[760px] sm:p-6">
        <DialogHeader>
          <DialogTitle>Edit merchant details</DialogTitle>
          <DialogDescription>
            Corrections apply to the public listing straight away. Approval status is managed separately.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-2">
          <section className="space-y-4">
            <h3 className="border-b pb-2 text-sm font-semibold">Business</h3>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="merchant-name">Business name</Label>
                <Input id="merchant-name" value={name} onChange={(event) => setName(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="merchant-type">Merchant type</Label>
                <Input
                  id="merchant-type"
                  value={type}
                  list="merchant-type-suggestions"
                  placeholder="Restaurant, Grocery Store…"
                  onChange={(event) => setType(event.target.value)}
                />
                <datalist id="merchant-type-suggestions">
                  {typeSuggestions.map((suggestion) => (
                    <option key={suggestion} value={suggestion} />
                  ))}
                </datalist>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="merchant-description">Description</Label>
              <Textarea
                id="merchant-description"
                rows={3}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
              />
            </div>
          </section>

          <section className="space-y-4">
            <h3 className="border-b pb-2 text-sm font-semibold">Address</h3>
            <div className="space-y-1.5">
              <Label htmlFor="merchant-street">Street</Label>
              <Input id="merchant-street" value={street} onChange={(event) => setStreet(event.target.value)} />
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <div className="space-y-1.5">
                <Label htmlFor="merchant-city">City</Label>
                <Input id="merchant-city" value={city} onChange={(event) => setCity(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="merchant-state">State</Label>
                <Input id="merchant-state" value={state} onChange={(event) => setState(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="merchant-zip">ZIP</Label>
                <Input id="merchant-zip" value={zip} onChange={(event) => setZip(event.target.value)} />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              Editing the address here does not move the map pin. Coordinates come from the Google listing, so
              use the re-point below if the merchant has actually moved.
            </p>
          </section>

          <section className="space-y-4">
            <h3 className="border-b pb-2 text-sm font-semibold">Contact</h3>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div className="space-y-1.5">
                <Label htmlFor="merchant-phone">Phone</Label>
                <Input id="merchant-phone" value={phone} onChange={(event) => setPhone(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="merchant-email">Email</Label>
                <Input id="merchant-email" value={email} onChange={(event) => setEmail(event.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="merchant-website">Website</Label>
                <Input id="merchant-website" value={website} onChange={(event) => setWebsite(event.target.value)} />
              </div>
            </div>
          </section>

          <section className="space-y-3">
            <h3 className="border-b pb-2 text-sm font-semibold">Opening hours</h3>
            <OpeningHoursEditor
              week={hours}
              onChange={setHours}
              manual={hoursManual}
              onManualChange={setHoursManual}
              disabled={busy}
              lastSyncedAt={location?.hours_synced_at ?? null}
            />
          </section>

          <section className="space-y-3 rounded-lg border border-amber-500/40 bg-amber-50/40 p-4 dark:bg-amber-900/10">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              <MapPin className="h-4 w-4" />
              Re-point at a Google listing
            </h3>
            <p className="text-xs text-muted-foreground">
              Pulls name, address, coordinates, type and hours from Google and{" "}
              <strong>overwrites the fields above</strong>, including anything typed here. Use it when the
              merchant has moved or was matched to the wrong listing.
            </p>
            {/* Re-pointing is Google-only by definition: it exists to replace
                this listing's place id. A manual address has none, so that
                branch of the picker is ignored here. */}
            <PlaceAutocomplete
              value={googlePlace ? { source: "google_place", place: googlePlace } : null}
              onSelect={(selection) =>
                setGooglePlace(selection?.source === "google_place" ? selection.place : null)
              }
            />
            <Button
              type="button"
              variant="outline"
              disabled={!googlePlace || busy}
              onClick={resyncFromGoogle}
              className="w-full sm:w-auto"
            >
              {resyncing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
              Re-point and overwrite from Google
            </Button>
          </section>

          {error !== "" && (
            <div className="flex items-center gap-2 rounded-lg bg-red-50 p-3 text-sm text-red-600 dark:bg-red-900/20">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
            Cancel
          </Button>
          <Button onClick={saveDetails} disabled={busy}>
            {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save details
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
