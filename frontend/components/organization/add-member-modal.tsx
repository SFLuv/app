"use client"

import { useEffect, useState } from "react"
import { Loader2, Mail } from "lucide-react"

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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"
import { CapabilityPicker } from "@/components/organization/capability-picker"
import type { CapabilityGroup, OrgMemberEndpoints, OrgMemberRole } from "@/types/organization"

/**
 * Invite somebody, and decide now what they will be able to do.
 *
 * The permissions are chosen before the invitation goes out rather than after
 * it is accepted, which closes a window that used to be unavoidable: the only
 * way to give a new member narrow access was to let them in with everything
 * and take it away once they appeared. Between accepting and being narrowed,
 * they had the lot.
 *
 * It opens with NOTHING ticked, which is the opposite of what membership used
 * to mean and is the point. Somebody is given what they need rather than
 * stripped of what they do not, so the quiet mistake is a person who has to
 * ask for one more capability instead of a person who could already do
 * everything and nobody noticed.
 */
export function AddMemberModal({
  open,
  onOpenChange,
  endpoints,
  catalog,
  onInvited,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  endpoints: OrgMemberEndpoints
  catalog: CapabilityGroup[]
  onInvited: () => void
}) {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [email, setEmail] = useState("")
  const [role, setRole] = useState<OrgMemberRole>("member")
  const [granted, setGranted] = useState<Set<string>>(new Set())
  // Starts custom-with-nothing. "Full access" in the picker switches it to
  // inherit, which is what full access should mean on an invite too: the
  // member tracks the organization's roles rather than being pinned to the set
  // that existed the day they were invited.
  const [mode, setMode] = useState<"inherit" | "custom">("custom")
  const [sending, setSending] = useState(false)

  // Reset on open so a second invitation does not inherit the first one's
  // answer — the likeliest way to hand somebody access nobody chose for them.
  useEffect(() => {
    if (!open) return
    setEmail("")
    setRole("member")
    setGranted(new Set())
    setMode("custom")
  }, [open])

  const send = async () => {
    const trimmed = email.trim()
    if (!trimmed) return
    setSending(true)
    try {
      const res = await authFetch(endpoints.invites, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          email: trimmed,
          role,
          // Custom unless the picker was explicitly set to Full access. The
          // default is never inherit: opting into "everything" is a choice
          // somebody makes, not one this form makes for them.
          permissions_mode: role === "member" ? mode : "custom",
          capabilities: role === "member" && mode === "custom" ? Array.from(granted) : [],
        }),
      })
      if (!res.ok) {
        toast({ title: "Could not send the invite", description: (await res.text()).trim(), variant: "destructive" })
        return
      }
      toast({
        title: "Invite sent",
        description: `${trimmed} will join as ${role === "admin" ? "an admin" : "a member"} when they accept.`,
      })
      onInvited()
      onOpenChange(false)
    } finally {
      setSending(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add a member</DialogTitle>
          <DialogDescription>
            They get an email with a join link. It expires in seven days and only works for the address it was sent to.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <div className="space-y-2">
            <Label className="text-xs uppercase tracking-wide text-muted-foreground">Email</Label>
            <Input
              type="email"
              placeholder="teammate@example.org"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              disabled={sending}
            />
            <p className="text-xs text-muted-foreground">
              They will need an SFLuv account with this address verified before they can accept.
            </p>
          </div>

          <div className="space-y-2">
            <Label className="text-xs uppercase tracking-wide text-muted-foreground">Join as</Label>
            <Select value={role} onValueChange={(next) => setRole(next as OrgMemberRole)} disabled={sending}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="member">Member</SelectItem>
                <SelectItem value="admin">Admin</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {role === "admin"
                ? "Admins have full access and can invite and remove members, so there is nothing to choose below."
                : "Choose what they will be able to do. Nothing is granted unless you tick it."}
            </p>
          </div>

          {role === "member" && (
            <>
              <Separator />
              <CapabilityPicker
                catalog={catalog}
                value={granted}
                onChange={setGranted}
                mode={mode}
                onModeChange={setMode}
                disabled={sending}
              />
            </>
          )}
        </div>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={sending}>
            Cancel
          </Button>
          <Button onClick={() => void send()} disabled={sending || !email.trim()}>
            {sending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Mail className="mr-2 h-4 w-4" />}
            Send invite
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
