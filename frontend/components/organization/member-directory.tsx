"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { ChevronLeft, ChevronRight, Clock, Crown, Loader2, Search, Shield, UserPlus, Users } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { useApp } from "@/context/AppProvider"
import { AddMemberModal } from "@/components/organization/add-member-modal"
import { MemberConfigModal } from "@/components/organization/member-config-modal"
import type {
  CapabilityGroup,
  MemberPermissions,
  OrgMemberEndpoints,
  OrgMemberRole,
  OrganizationInvite,
  OrganizationMember,
  OrganizationPermissionsView,
} from "@/types/organization"

/**
 * The roster, and the only member surface an admin needs.
 *
 * It replaces a page that listed everybody twice — once as members, once again
 * at the bottom as permissions — so deciding about one person meant finding
 * their two rows and holding both. Here the person is the row, and clicking
 * them opens everything about them.
 *
 * Pending invitations are listed alongside members rather than in a separate
 * block underneath, because "who is in this organization" includes the people
 * who have been asked. They carry the role they will hold, so the list answers
 * "pending as what" rather than just "pending".
 *
 * Search and paging are client-side: the whole roster already arrives in the
 * organization view, so asking the server again for a filtered copy of
 * something already in memory would add a round trip and a loading state to a
 * list that is usually under thirty people.
 */

const PAGE_SIZE = 10

type Row =
  | { kind: "member"; key: string; name: string; email: string; role: OrgMemberRole; member: OrganizationMember }
  | { kind: "invite"; key: string; name: string; email: string; role: OrgMemberRole; invite: OrganizationInvite }

/** Superadmin, then admins, then members, then pending — the order people ask about them in. */
const rowWeight = (row: Row) => {
  if (row.kind === "invite") return 4
  if (row.role === "superadmin") return 1
  if (row.role === "admin") return 2
  return 3
}

export function MemberDirectory({
  members,
  invites,
  endpoints,
  canManage,
  canEditRoles,
  currentUserId,
  onTransferSuperadmin,
  onChanged,
  administrativeEndpoint,
  title = "Members",
  description,
}: {
  members: OrganizationMember[]
  invites: OrganizationInvite[]
  endpoints: OrgMemberEndpoints
  /** Invite and remove. An ordinary member reads the list and nothing more. */
  canManage: boolean
  /** Change member/admin. Superadmin-only in an organization's own settings. */
  canEditRoles: boolean
  currentUserId?: string
  onTransferSuperadmin?: (member: OrganizationMember) => void
  onChanged: () => void
  /**
   * Where the SFLuv-only administrative grants are written. Passed only by an
   * organization's own settings: there is deliberately no cross-organization
   * equivalent, so the admin panel leaves this unset and the section does not
   * appear there at all.
   */
  administrativeEndpoint?: string
  title?: string
  description?: string
}) {
  const { authFetch } = useApp()

  const [permissions, setPermissions] = useState<OrganizationPermissionsView | null>(null)
  const [loadingPermissions, setLoadingPermissions] = useState(true)
  const [search, setSearch] = useState("")
  const [page, setPage] = useState(0)
  const [addOpen, setAddOpen] = useState(false)
  const [selected, setSelected] = useState<OrganizationMember | null>(null)

  const loadPermissions = useCallback(async () => {
    try {
      const res = await authFetch(endpoints.permissions)
      if (!res.ok) return
      setPermissions((await res.json()) as OrganizationPermissionsView)
    } catch {
      // The list still works without it; the modal says so rather than
      // offering a tree it cannot populate.
    } finally {
      setLoadingPermissions(false)
    }
  }, [authFetch, endpoints.permissions])

  useEffect(() => {
    void loadPermissions()
  }, [loadPermissions])

  const rows = useMemo<Row[]>(() => {
    const memberRows: Row[] = members.map((m) => ({
      kind: "member",
      key: `m:${m.user_id}`,
      name: m.contact_name?.trim() || m.contact_email?.trim() || m.user_id,
      email: m.contact_email?.trim() || "",
      role: m.role,
      member: m,
    }))
    const inviteRows: Row[] = invites.map((inv) => ({
      kind: "invite",
      key: `i:${inv.id}`,
      name: inv.email,
      email: inv.email,
      role: inv.role ?? "member",
      invite: inv,
    }))
    return [...memberRows, ...inviteRows].sort(
      (a, b) => rowWeight(a) - rowWeight(b) || a.name.localeCompare(b.name),
    )
  }, [members, invites])

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase()
    if (!term) return rows
    return rows.filter((row) => row.name.toLowerCase().includes(term) || row.email.toLowerCase().includes(term))
  }, [rows, search])

  // Clamped rather than reset: typing a term that shortens the list should not
  // throw somebody back to page one of a list they were reading.
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const safePage = Math.min(page, pageCount - 1)
  const visible = filtered.slice(safePage * PAGE_SIZE, safePage * PAGE_SIZE + PAGE_SIZE)

  const permissionsFor = (userId: string): MemberPermissions | null =>
    permissions?.members.find((entry) => entry.user_id === userId) ?? null

  const catalog = permissions?.catalog ?? []

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2 text-base text-black dark:text-white">
              <Users className="h-4 w-4" /> {title}
            </CardTitle>
            <CardDescription>
              {description ??
                "Everyone in the organization, and anybody invited. Click a person to set their role and what they can do."}
            </CardDescription>
          </div>
          {canManage && (
            <Button onClick={() => setAddOpen(true)}>
              <UserPlus className="mr-2 h-4 w-4" /> Add member
            </Button>
          )}
        </div>
      </CardHeader>

      <CardContent className="space-y-3">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            placeholder="Search by name or email"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value)
              setPage(0)
            }}
          />
        </div>

        <div className="divide-y rounded-lg border">
          {visible.length === 0 ? (
            <p className="p-4 text-sm text-muted-foreground">
              {search.trim() ? "Nobody matches that." : "No members yet."}
            </p>
          ) : (
            visible.map((row) => {
              const clickable = row.kind === "member" && canManage
              return (
                <div
                  key={row.key}
                  role={clickable ? "button" : undefined}
                  tabIndex={clickable ? 0 : undefined}
                  onClick={clickable ? () => setSelected(row.member) : undefined}
                  onKeyDown={
                    clickable
                      ? (event) => {
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault()
                            setSelected(row.member)
                          }
                        }
                      : undefined
                  }
                  className={`flex flex-wrap items-center justify-between gap-2 p-3 ${
                    clickable ? "cursor-pointer hover:bg-muted/40" : ""
                  } ${row.kind === "invite" ? "opacity-70" : ""}`}
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-medium text-black dark:text-white">
                      <span className="truncate">{row.name}</span>
                      {row.role === "superadmin" && <Crown className="h-3.5 w-3.5 shrink-0 text-amber-500" />}
                      {row.role === "admin" && <Shield className="h-3.5 w-3.5 shrink-0 text-primary" />}
                      {row.kind === "member" && row.member.user_id === currentUserId && (
                        <Badge variant="outline" className="text-[10px]">
                          you
                        </Badge>
                      )}
                    </div>
                    {row.email && row.email !== row.name && (
                      <p className="truncate text-xs text-muted-foreground">{row.email}</p>
                    )}
                    {row.kind === "invite" && (
                      <p className="text-xs text-muted-foreground">
                        expires {new Date(row.invite.expires_at).toLocaleDateString()}
                      </p>
                    )}
                  </div>
                  {row.kind === "invite" ? (
                    <Badge variant="outline" className="gap-1 capitalize">
                      <Clock className="h-3 w-3" /> Pending {row.role}
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="capitalize">
                      {row.role}
                    </Badge>
                  )}
                </div>
              )
            })
          )}
        </div>

        {pageCount > 1 && (
          <div className="flex items-center justify-between">
            <p className="text-xs text-muted-foreground tabular-nums">
              {filtered.length} {filtered.length === 1 ? "person" : "people"} · page {safePage + 1} of {pageCount}
            </p>
            <div className="flex gap-1">
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8"
                disabled={safePage === 0}
                onClick={() => setPage(safePage - 1)}
                aria-label="Previous page"
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8"
                disabled={safePage >= pageCount - 1}
                onClick={() => setPage(safePage + 1)}
                aria-label="Next page"
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}

        {loadingPermissions && (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" /> Loading permissions…
          </p>
        )}
      </CardContent>

      <AddMemberModal
        open={addOpen}
        onOpenChange={setAddOpen}
        endpoints={endpoints}
        catalog={catalog}
        onInvited={onChanged}
      />

      <MemberConfigModal
        open={selected != null}
        onOpenChange={(open) => !open && setSelected(null)}
        member={selected}
        memberName={
          selected
            ? selected.contact_name?.trim() || selected.contact_email?.trim() || selected.user_id
            : ""
        }
        endpoints={endpoints}
        catalog={catalog}
        permissions={selected ? permissionsFor(selected.user_id) : null}
        canEditRole={canEditRoles}
        canRemove={canManage && selected?.user_id !== currentUserId}
        administrative={permissions?.administrative}
        canEditAdministrative={permissions?.can_edit_administrative === true && Boolean(administrativeEndpoint)}
        administrativeEndpoint={administrativeEndpoint}
        onTransferSuperadmin={onTransferSuperadmin}
        onSaved={() => {
          onChanged()
          void loadPermissions()
        }}
      />
    </Card>
  )
}
