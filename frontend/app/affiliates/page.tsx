"use client"

import { useState } from "react"
import { Leaf } from "lucide-react"

import { AddVolunteerEventModal, type VolunteerEventDraft } from "@/components/events/add-volunteer-event-modal"
import { VolunteerEventsManager } from "@/components/events/volunteer-events-manager"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"

/**
 * Affiliate volunteer events.
 *
 * There is no separate "affiliate event" concept any more: an organization's
 * events ARE volunteer events, and they are created by request rather than
 * directly. Approval is what commits faucet funds, which is why organizations
 * no longer hold a spendable balance — the old standing-allocation model let an
 * affiliate mint codes the faucet might not be able to honour.
 */
export default function AffiliatesPage() {
  const { authFetch, status, user } = useApp()
  const { toast } = useToast()
  const [requestModalOpen, setRequestModalOpen] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)

  const requestEvent = async (draft: VolunteerEventDraft): Promise<string | null> => {
    try {
      const res = await authFetch("/affiliates/volunteer-events", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(draft),
      })
      if (!res.ok) {
        throw new Error((await res.text()).trim() || "Unable to submit the request.")
      }
      const created = await res.json()
      toast({
        title: "Request submitted",
        description: "An SFLuv admin will review it. You'll be emailed either way.",
      })
      setReloadKey((key) => key + 1)
      return created?.id ?? null
    } catch (error) {
      toast({
        title: "Could not submit request",
        description: error instanceof Error ? error.message : "Unexpected error.",
        variant: "destructive",
      })
      return null
    }
  }

  const uploadPhoto = async (eventId: string, file: File): Promise<boolean> => {
    try {
      const form = new FormData()
      form.append("photo", file)
      const res = await authFetch(`/admin/volunteer-events/${eventId}/photos`, { method: "POST", body: form })
      return res.ok
    } catch {
      return false
    }
  }

  if (status === "loading") {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (!user?.isAffiliate) {
    return (
      <div className="container mx-auto p-4 space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Affiliate Access Required</CardTitle>
            <CardDescription>
              Your account is not yet approved for volunteer events. Submit a request in settings.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold">Volunteer Events</h1>
          <p className="text-muted-foreground">
            Request events for your organization and manage the ones that have been approved.
          </p>
        </div>
        <Button onClick={() => setRequestModalOpen(true)} className="w-full sm:w-auto">
          + Request Event
        </Button>
      </div>

      <Card>
        <CardContent className="flex items-start gap-3 pt-6 text-sm text-muted-foreground">
          <Leaf className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <p>
            Requests are reviewed by an SFLuv admin. Approving an event is what reserves its rewards from the
            faucet and generates its QR codes — once approved, the codes are downloadable here straight away and
            become redeemable 24 hours before the event starts.
          </p>
        </CardContent>
      </Card>

      <AddVolunteerEventModal
        open={requestModalOpen}
        onOpenChange={setRequestModalOpen}
        createEvent={requestEvent}
        uploadPhoto={uploadPhoto}
        // Affiliates request rather than spend, so there is no balance to check
        // against here; the faucet is verified by the admin at approval.
        unallocatedBalance={Number.MAX_SAFE_INTEGER}
        submitLabel="Submit request"
      />

      <VolunteerEventsManager
        key={reloadKey}
        basePath="/affiliates/volunteer-events"
        canReview={false}
        title="Your Organization's Events"
        description="Requests awaiting review, plus approved events with their QR codes."
      />
    </div>
  )
}
