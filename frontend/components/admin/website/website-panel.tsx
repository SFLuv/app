"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { Loader2 } from "lucide-react"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useApp } from "@/context/AppProvider"
import { useAuthFetch } from "./use-auth-fetch"
import { formatWhen, siteFetch } from "@/lib/site-admin"
import type { SiteActivity, SiteCapabilities } from "@/types/site"
import { BannerEditor } from "./banner-editor"
import { FinancialsEditor } from "./financials-editor"
import { FormsEditor } from "./forms-editor"
import { PastEventsEditor } from "./past-events-editor"
import { NoticeProvider } from "./notice"

type Section = "banner" | "financials" | "forms" | "past_events"

const SECTIONS: { id: Section; label: string }[] = [
  { id: "banner", label: "Banner items" },
  { id: "financials", label: "Financials & reports" },
  { id: "forms", label: "Forms & waivers" },
  { id: "past_events", label: "Past events" },
]

/**
 * Tools for editing sfluv.org without a code change.
 *
 * Each editor is shown only if the signed-in person may use it: admins can use
 * all of them, anyone else needs the matching private credential. The server
 * enforces this too — what is hidden here is a courtesy, not the lock.
 */
export function WebsitePanel() {
  return (
    <NoticeProvider>
      <WebsitePanelInner />
    </NoticeProvider>
  )
}

function WebsitePanelInner() {
  const { status } = useApp()
  const authFetch = useAuthFetch()
  const [caps, setCaps] = useState<SiteCapabilities | null>(null)
  const [error, setError] = useState("")
  const [section, setSection] = useState<Section | "">("")
  const [activity, setActivity] = useState<SiteActivity[]>([])

  useEffect(() => {
    if (status !== "authenticated") return
    siteFetch<SiteCapabilities>(authFetch, "/admin/site/me")
      .then(setCaps)
      .catch((err) => setError(err instanceof Error ? err.message : "Unable to load."))
  }, [authFetch, status])

  const allowed = useMemo(() => SECTIONS.filter((s) => caps?.[s.id]), [caps])

  useEffect(() => {
    if (!section && allowed.length > 0) setSection(allowed[0].id)
  }, [allowed, section])

  const loadActivity = useCallback(async () => {
    try {
      const data = await siteFetch<{ activity: SiteActivity[] }>(authFetch, "/admin/site/activity")
      setActivity(data.activity)
    } catch {
      /* the log is a convenience; its failure should not get in the way */
    }
  }, [authFetch])

  // Refresh the log whenever the person moves between tools — which is when they have just finished one.
  useEffect(() => {
    if (caps && allowed.length > 0) void loadActivity()
  }, [caps, allowed.length, section, loadActivity])

  if (error) return <p className="text-sm text-destructive">{error}</p>
  if (!caps) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    )
  }
  if (allowed.length === 0) {
    return <p className="text-sm text-muted-foreground">You don’t have access to any website tools. Ask an admin to grant them.</p>
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Website</h2>
        <p className="text-sm text-muted-foreground">
          Change what appears on sfluv.org. Changes go live within about 30 seconds — no one needs to deploy anything.
        </p>
      </div>

      <Tabs value={section || allowed[0].id} onValueChange={(v) => setSection(v as Section)} className="space-y-6">
        <TabsList className="h-auto flex-wrap">
          {allowed.map((s) => (
            <TabsTrigger key={s.id} value={s.id}>
              {s.label}
            </TabsTrigger>
          ))}
        </TabsList>

        {caps.banner && (
          <TabsContent value="banner" className="mt-0">
            <BannerEditor />
          </TabsContent>
        )}
        {caps.financials && (
          <TabsContent value="financials" className="mt-0">
            <FinancialsEditor />
          </TabsContent>
        )}
        {caps.forms && (
          <TabsContent value="forms" className="mt-0">
            <FormsEditor />
          </TabsContent>
        )}
        {caps.past_events && (
          <TabsContent value="past_events" className="mt-0">
            <PastEventsEditor />
          </TabsContent>
        )}
      </Tabs>

      {activity.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Recent changes</CardTitle>
            <CardDescription>Who changed what on the website.</CardDescription>
          </CardHeader>
          <CardContent>
            <ul className="divide-y rounded-lg border">
              {activity.slice(0, 12).map((a) => (
                <li key={a.id} className="flex flex-wrap items-baseline justify-between gap-2 p-3 text-sm">
                  <span>{a.summary}</span>
                  <span className="text-xs text-muted-foreground">
                    {a.actor || "Someone"} · {formatWhen(a.at)}
                  </span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
