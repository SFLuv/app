"use client"

import { useMemo } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { CapabilityGroup } from "@/types/organization"

/**
 * The capability tree, as a controlled input.
 *
 * Shared by the two places a permission set is decided — configuring a member
 * who is already here, and pre-permissioning somebody who has been invited —
 * because those two screens have to agree about what the boxes mean. The
 * invite one writes a set that will be applied on acceptance; the member one
 * writes a set that takes effect now. Neither saves as you tick: the caller
 * holds a draft and decides when to send it, so a half-configured member is
 * never briefly live.
 *
 * The presets are the shape of the decision rather than a shortcut. Almost
 * every real answer is "everything", "look but do not touch" or "nothing", and
 * offering those by name means an admin does not have to read nine hints to
 * express one of them.
 */

export type CapabilityPreset = "full" | "read" | "none" | "custom"

/**
 * Which preset a set of keys corresponds to, or "custom" when it is none of
 * them. Derived rather than stored so the control cannot claim "Read-only"
 * about a set somebody has since edited.
 */
export function presetForKeys(catalog: CapabilityGroup[], mode: string, keys: Set<string>): CapabilityPreset {
  if (mode === "inherit") return "full"
  const all = catalog.flatMap((group) => group.capabilities.map((c) => c.key))
  const read = catalog.flatMap((group) => group.capabilities.filter((c) => c.read).map((c) => c.key))
  if (all.length > 0 && all.every((k) => keys.has(k))) return "full"
  if (keys.size === 0) return "none"
  if (read.length > 0 && keys.size === read.length && read.every((k) => keys.has(k))) return "read"
  return "custom"
}

export function CapabilityPicker({
  catalog,
  value,
  onChange,
  disabled = false,
  /**
   * "full" is sent as inherit rather than as every key ticked, so the member
   * keeps tracking the organization's roles instead of being pinned to the set
   * that existed the day somebody configured them.
   */
  onModeChange,
  mode,
}: {
  catalog: CapabilityGroup[]
  value: Set<string>
  onChange: (next: Set<string>) => void
  disabled?: boolean
  mode: "inherit" | "custom"
  onModeChange: (next: "inherit" | "custom") => void
}) {
  const allKeys = useMemo(
    () => catalog.flatMap((group) => group.capabilities.map((c) => c.key)),
    [catalog],
  )
  const readKeys = useMemo(
    () => catalog.flatMap((group) => group.capabilities.filter((c) => c.read).map((c) => c.key)),
    [catalog],
  )

  const preset = presetForKeys(catalog, mode, value)

  const applyPreset = (next: string) => {
    if (next === "full") {
      onModeChange("inherit")
      onChange(new Set(allKeys))
      return
    }
    onModeChange("custom")
    if (next === "read") onChange(new Set(readKeys))
    else if (next === "none") onChange(new Set())
    // "custom" keeps the current set and only stops it tracking the roles.
  }

  const toggle = (key: string, on: boolean) => {
    onModeChange("custom")
    const next = new Set(value)
    if (on) next.add(key)
    else next.delete(key)
    onChange(next)
  }

  if (catalog.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        This organization has no approved roles yet, so there is nothing to grant. Permissions appear here once a role
        request is approved.
      </p>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <Label className="text-xs uppercase tracking-wide text-muted-foreground">Access</Label>
        <div className="w-44">
          <Select value={preset} onValueChange={applyPreset} disabled={disabled}>
            <SelectTrigger className="h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="full">Full access</SelectItem>
              <SelectItem value="read">Read-only</SelectItem>
              <SelectItem value="none">No access</SelectItem>
              <SelectItem value="custom">Custom</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {mode === "inherit" ? (
        <p className="rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground">
          Full access: everything the organization&apos;s approved roles carry, now and whenever a new role is
          approved. Pick another option to choose capabilities one at a time.
        </p>
      ) : (
        <div className="space-y-4">
          {catalog.map((group) => (
            <div key={group.role_type} className="space-y-2">
              <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{group.label}</p>
              <div className="space-y-2">
                {group.capabilities.map((capability) => (
                  <label
                    key={capability.key}
                    className="flex cursor-pointer items-start gap-3 rounded-lg border p-3 hover:bg-muted/40"
                  >
                    <Checkbox
                      className="mt-0.5"
                      checked={value.has(capability.key)}
                      disabled={disabled}
                      onCheckedChange={(checked) => toggle(capability.key, checked === true)}
                    />
                    <span className="min-w-0">
                      <span className="block text-sm font-medium text-black dark:text-white">{capability.label}</span>
                      <span className="block text-xs text-muted-foreground">{capability.hint}</span>
                    </span>
                  </label>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
