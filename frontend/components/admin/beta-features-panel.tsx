"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { AlertTriangle, Loader2, Plus, RefreshCw, Search, UserMinus, UserPlus } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"

/**
 * Beta features: who may use a surface that is not finished yet.
 *
 * Two things are shown together on purpose, because either alone misleads. A
 * grant does nothing while the feature's server switch is off, so the switch is
 * stated next to the list rather than left to be discovered; and revoked
 * members stay on the list, greyed, because taking somebody out and putting
 * them back is a thing that happens and a list that forgot it would make the
 * second decision look like the first.
 */

type BetaFeature = {
  id: string
  label: string
  description: string
  env_enabled: boolean
  env_var: string
  members: number
}

type BetaMember = {
  user_id: string
  email?: string
  name?: string
  granted_at: string
  granted_by?: string
  revoked_at?: string
  revoked_by?: string
  active: boolean
}

type AdminUser = {
  id: string
  contact_email?: string | null
  contact_name?: string | null
}

const personLabel = (member: BetaMember) =>
  member.name?.trim() || member.email?.trim() || member.user_id

export function BetaFeaturesPanel() {
  const { authFetch } = useApp()
  const { toast } = useToast()

  const [features, setFeatures] = useState<BetaFeature[]>([])
  const [selected, setSelected] = useState<string>("")
  const [members, setMembers] = useState<BetaMember[]>([])
  const [loading, setLoading] = useState(true)
  const [membersLoading, setMembersLoading] = useState(false)
  const [unavailable, setUnavailable] = useState("")
  const [busyUser, setBusyUser] = useState<string>("")

  const [search, setSearch] = useState("")
  const [results, setResults] = useState<AdminUser[]>([])
  const [searching, setSearching] = useState(false)

  const feature = useMemo(() => features.find((f) => f.id === selected), [features, selected])

  const loadFeatures = useCallback(async () => {
    setLoading(true)
    try {
      const res = await authFetch("/admin/beta/features")
      if (!res.ok) {
        setUnavailable((await res.text()).trim() || "Beta features are unavailable.")
        return
      }
      setUnavailable("")
      const body = (await res.json()) as { features?: BetaFeature[] }
      const list = body.features ?? []
      setFeatures(list)
      setSelected((current) => current || list[0]?.id || "")
    } catch {
      setUnavailable("Could not reach the server.")
    } finally {
      setLoading(false)
    }
  }, [authFetch])

  const loadMembers = useCallback(
    async (featureId: string) => {
      if (!featureId) return
      setMembersLoading(true)
      try {
        const res = await authFetch(`/admin/beta/members?feature=${encodeURIComponent(featureId)}`)
        if (!res.ok) {
          setMembers([])
          return
        }
        const body = (await res.json()) as { members?: BetaMember[] }
        setMembers(body.members ?? [])
      } finally {
        setMembersLoading(false)
      }
    },
    [authFetch],
  )

  useEffect(() => {
    void loadFeatures()
  }, [loadFeatures])

  useEffect(() => {
    void loadMembers(selected)
  }, [loadMembers, selected])

  const runSearch = useCallback(async () => {
    const term = search.trim()
    if (!term) {
      setResults([])
      return
    }
    setSearching(true)
    try {
      const res = await authFetch(`/admin/users?page=0&count=10&search=${encodeURIComponent(term)}`)
      if (!res.ok) {
        setResults([])
        return
      }
      const body = await res.json()
      setResults(Array.isArray(body) ? body : (body?.users ?? []))
    } catch {
      setResults([])
    } finally {
      setSearching(false)
    }
  }, [authFetch, search])

  const setAccess = async (userId: string, grant: boolean) => {
    if (!selected) return
    setBusyUser(userId)
    try {
      const res = await authFetch(`/admin/beta/${grant ? "grant" : "revoke"}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ feature: selected, user_id: userId }),
      })
      if (!res.ok) {
        toast({ title: grant ? "Could not grant access" : "Could not revoke access", variant: "destructive" })
        return
      }
      toast({ title: grant ? "Access granted" : "Access revoked" })
      await Promise.all([loadMembers(selected), loadFeatures()])
    } finally {
      setBusyUser("")
    }
  }

  if (loading && features.length === 0) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Reading beta features…
      </div>
    )
  }

  if (unavailable) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base text-black dark:text-white">Beta features</CardTitle>
          <CardDescription>{unavailable}</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const active = members.filter((m) => m.active)
  const removed = members.filter((m) => !m.active)

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-black dark:text-white">Beta features</h2>
          <p className="text-sm text-muted-foreground">
            Features that exist in the app but are shown only to the people added here.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void loadFeatures()} disabled={loading}>
          <RefreshCw className={`mr-2 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {/* One card per feature, so the switch and the headcount are read together. */}
      <div className="grid gap-3 sm:grid-cols-2">
        {features.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => setSelected(item.id)}
            className={`rounded-lg border p-4 text-left transition ${
              item.id === selected ? "border-primary bg-primary/5" : "hover:bg-muted/50"
            }`}
          >
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium text-black dark:text-white">{item.label}</span>
              {item.env_enabled ? (
                <Badge variant="outline" className="border-emerald-500 text-emerald-600">
                  Switch on
                </Badge>
              ) : (
                <Badge variant="destructive" className="gap-1">
                  <AlertTriangle className="h-3 w-3" /> Switch off
                </Badge>
              )}
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{item.description}</p>
            <p className="mt-2 text-xs tabular-nums text-muted-foreground">
              {item.members} {item.members === 1 ? "person" : "people"}
            </p>
          </button>
        ))}
      </div>

      {feature ? (
        <>
          {/* Stated rather than left to be discovered: with the switch off, a
              grant is recorded and does nothing. */}
          {!feature.env_enabled ? (
            <Card className="border-destructive">
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base text-black dark:text-white">
                  <AlertTriangle className="h-4 w-4 text-destructive" />
                  {feature.label} is switched off for everyone
                </CardTitle>
                <CardDescription>
                  Nobody can use it whatever is listed below, because{" "}
                  <span className="font-mono text-xs">{feature.env_var}</span> is not{" "}
                  <span className="font-mono text-xs">true</span> on the backend. Grants made now are kept and take
                  effect when it is switched on.
                </CardDescription>
              </CardHeader>
            </Card>
          ) : null}

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base text-black dark:text-white">Add someone to {feature.label}</CardTitle>
              <CardDescription>Search by name or email, then add them.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <form
                className="flex gap-2"
                onSubmit={(event) => {
                  event.preventDefault()
                  void runSearch()
                }}
              >
                <Input
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder="Name or email"
                  className="max-w-sm"
                />
                <Button type="submit" variant="outline" size="sm" disabled={searching}>
                  {searching ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <Search className="mr-2 h-3.5 w-3.5" />}
                  Search
                </Button>
              </form>

              {results.length > 0 ? (
                <div className="space-y-2">
                  {results.map((user) => {
                    const already = active.some((m) => m.user_id === user.id)
                    return (
                      <div key={user.id} className="flex flex-wrap items-center gap-3 rounded-lg border p-3">
                        <div className="min-w-0 flex-1">
                          <div className="font-medium text-black dark:text-white">
                            {user.contact_name?.trim() || user.contact_email?.trim() || user.id}
                          </div>
                          <div className="font-mono text-xs text-muted-foreground">{user.id}</div>
                        </div>
                        {already ? (
                          <Badge variant="outline" className="border-emerald-500 text-emerald-600">
                            Already has it
                          </Badge>
                        ) : (
                          <Button
                            size="sm"
                            onClick={() => void setAccess(user.id, true)}
                            disabled={busyUser === user.id}
                          >
                            {busyUser === user.id ? (
                              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                            ) : (
                              <Plus className="mr-2 h-3.5 w-3.5" />
                            )}
                            Add
                          </Button>
                        )}
                      </div>
                    )
                  })}
                </div>
              ) : null}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base text-black dark:text-white">
                {feature.label} — {active.length} {active.length === 1 ? "person" : "people"}
              </CardTitle>
              <CardDescription>
                {feature.id === "chat"
                  ? "Both people need access for a chat to start. Removing someone leaves their conversations intact and greys them out for whoever they were talking to; adding them back restores everything."
                  : "Removing someone leaves their posts intact; adding them back restores access to them."}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {membersLoading ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" /> Loading…
                </div>
              ) : active.length === 0 ? (
                <p className="text-sm text-muted-foreground">Nobody has this yet.</p>
              ) : (
                active.map((member) => (
                  <div key={member.user_id} className="flex flex-wrap items-center gap-3 rounded-lg border p-3">
                    <div className="min-w-0 flex-1">
                      <div className="font-medium text-black dark:text-white">{personLabel(member)}</div>
                      <div className="font-mono text-xs text-muted-foreground">{member.user_id}</div>
                    </div>
                    <span className="text-xs text-muted-foreground">
                      since {new Date(member.granted_at).toLocaleDateString()}
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => void setAccess(member.user_id, false)}
                      disabled={busyUser === member.user_id}
                    >
                      {busyUser === member.user_id ? (
                        <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <UserMinus className="mr-2 h-3.5 w-3.5" />
                      )}
                      Remove
                    </Button>
                  </div>
                ))
              )}

              {/* Kept rather than dropped: a second decision about the same
                  person should not look like a first one. */}
              {removed.length > 0 ? (
                <div className="space-y-2 pt-2">
                  <p className="text-xs uppercase tracking-wide text-muted-foreground">Previously had access</p>
                  {removed.map((member) => (
                    <div
                      key={member.user_id}
                      className="flex flex-wrap items-center gap-3 rounded-lg border border-dashed p-3 opacity-70"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="text-muted-foreground">{personLabel(member)}</div>
                        <div className="font-mono text-xs text-muted-foreground">{member.user_id}</div>
                      </div>
                      <span className="text-xs text-muted-foreground">
                        removed {member.revoked_at ? new Date(member.revoked_at).toLocaleDateString() : ""}
                      </span>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void setAccess(member.user_id, true)}
                        disabled={busyUser === member.user_id}
                      >
                        {busyUser === member.user_id ? (
                          <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <UserPlus className="mr-2 h-3.5 w-3.5" />
                        )}
                        Add back
                      </Button>
                    </div>
                  ))}
                </div>
              ) : null}
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  )
}
