"use client"

import { useEffect, useMemo, useState } from "react"
import { Loader2 } from "lucide-react"

import {
  OpeningHoursEditor,
  toWeek,
  validateWeek,
  weekSignature,
  type LocationDayHours,
} from "@/components/locations/opening-hours-editor"
import { Button } from "@/components/ui/button"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import type { AuthedLocation } from "@/types/location"

interface MerchantHoursCardProps {
  location: AuthedLocation
  /** Called after a successful save so the caller can refetch. */
  onSaved?: () => void | Promise<void>
}

/**
 * A merchant's own opening hours, with the nightly-sync switch.
 *
 * Saved through its own endpoint rather than the surrounding profile form: the
 * profile save is a whole-record write, and hours have their own rules (a
 * partial week is invalid, and the manual flag decides whether a nightly job may
 * overwrite them). Keeping them separate means a merchant fixing Tuesday cannot
 * accidentally resubmit — or fail on — the rest of their profile.
 */
export function MerchantHoursCard({ location, onSaved }: MerchantHoursCardProps) {
  const { authFetch, refreshUserRecord } = useApp()
  const { toast } = useToast()

  const [week, setWeek] = useState<LocationDayHours[]>(() => toWeek(location.hours))
  const [manual, setManual] = useState(!!location.hours_manual)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState("")

  // The app polls the user record in the background, so a nightly sync (or an
  // admin correction) can land while this card is on screen. Adopt it only when
  // it is genuinely different and the merchant has not started editing —
  // overwriting half-typed hours under someone's cursor is worse than showing
  // them a value that is a few seconds stale.
  const [dirty, setDirty] = useState(false)
  const incoming = useMemo(() => toWeek(location.hours), [location.hours])
  const incomingSignature = weekSignature(incoming)

  useEffect(() => {
    if (dirty) return
    setWeek(incoming)
    setManual(!!location.hours_manual)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed on the
    // signature so an identical payload does not reset the form.
  }, [location.id, incomingSignature, location.hours_manual, dirty])

  const save = async () => {
    const invalid = validateWeek(week)
    if (invalid) {
      setError(invalid)
      return
    }
    setSaving(true)
    setError("")
    try {
      const res = await authFetch(`/locations/${location.id}/hours`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ hours: week, hours_manual: manual }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        throw new Error(body?.error || "Unable to save your hours.")
      }
      toast({
        title: "Hours saved",
        description: manual
          ? "Your hours will stay exactly as entered."
          : "Saved. The nightly check against Google may still update them.",
      })
      // Pull the record back rather than trusting local state: the server sorts
      // stretches and derives the display text, so what it stored can differ
      // from what was typed.
      setDirty(false)
      await refreshUserRecord()
      await onSaved?.()
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "Unable to save your hours.")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-4">
      <OpeningHoursEditor
        week={week}
        onChange={(next) => {
          setDirty(true)
          setWeek(next)
        }}
        manual={manual}
        onManualChange={(next) => {
          setDirty(true)
          setManual(next)
        }}
        disabled={saving}
        lastSyncedAt={location.hours_synced_at ?? null}
      />

      {error !== "" && <p className="text-sm text-destructive">{error}</p>}

      <Button onClick={save} disabled={saving} className="w-full sm:w-auto">
        {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        Save hours
      </Button>
    </div>
  )
}
