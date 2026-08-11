"use client"

import { useState } from "react"
import { Info } from "lucide-react"

import { AddVolunteerEventModal, type VolunteerEventDraft } from "@/components/events/add-volunteer-event-modal"
import { VolunteerEventsManager, type ManagedVolunteerEvent } from "@/components/events/volunteer-events-manager"
import { Button } from "@/components/ui/button"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
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
  const [editingEvent, setEditingEvent] = useState<ManagedVolunteerEvent | null>(null)
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
      const created = await res.json().catch(() => null)
      const id = typeof created?.id === "string" ? created.id.trim() : ""
      if (id === "") {
        // A 2xx with no id means the request went through but we cannot attach
        // photos to it. Saying "failed" would be a lie, and staying silent
        // would leave the affiliate submitting it a second time.
        setReloadKey((key) => key + 1)
        throw new Error(
          "The request was submitted, but the server did not return its id, so photos could not be attached. Close this and check the list below.",
        )
      }
      toast({
        title: "Request submitted",
        description: "An SFLuv admin will review it. You'll be emailed either way.",
      })
      setReloadKey((key) => key + 1)
      return id
    } catch (error) {
      /*
       * Rethrown rather than swallowed into a toast. The modal is open and
       * covering the page, so a toast behind it is the one place the person
       * who just pressed Submit is not looking — the modal shows this message
       * under the form instead.
       */
      throw error instanceof Error ? error : new Error("Unable to submit the request.")
    }
  }

  /**
   * Save an edit to an existing event.
   *
   * Same form, same shape as creation — the server decides what happens next:
   * an admin's edit applies immediately, an affiliate's is parked for approval.
   */
  const saveEventEdit = async (eventId: string, draft: VolunteerEventDraft): Promise<string | null> => {
    const res = await authFetch(`/affiliates/volunteer-events/${eventId}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(draft),
    })
    if (!res.ok) {
      throw new Error((await res.text()).trim() || "Unable to save the changes.")
    }
    toast({
      title: "Changes submitted",
      description: "An SFLuv admin will review them before they go live.",
    })
    setReloadKey((key) => key + 1)
    return eventId
  }

  /**
   * Uploads a cover photo before its event exists and returns the staged id.
   *
   * Throws rather than returning a flag: the message is shown against the
   * specific thumbnail that failed, and a silent false would leave the author
   * guessing which file the modal was unhappy about.
   */
  const stagePhoto = async (file: File): Promise<string> => {
    const form = new FormData()
    form.append("photo", file)
    const res = await authFetch("/volunteer-events/staged-photos", { method: "POST", body: form })
    if (!res.ok) {
      throw new Error((await res.text()).trim() || "Could not upload that photo.")
    }
    const staged = await res.json().catch(() => null)
    const id = typeof staged?.id === "string" ? staged.id.trim() : ""
    if (id === "") {
      throw new Error("Could not upload that photo.")
    }
    return id
  }

  /** Drops a staged photo the author removed. Failure is not worth surfacing:
   *  the server sweeps anything never attached. */
  const discardPhoto = async (photoId: string): Promise<void> => {
    await authFetch(`/volunteer-events/staged-photos/${photoId}`, { method: "DELETE" }).catch(() => undefined)
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
          <div className="flex items-center gap-2">
            <h1 className="text-3xl font-bold">Volunteer Events</h1>
            {/*
              The approval rules used to sit in a card of their own under the
              header. They are worth reading once and never again, so they now
              live behind the heading rather than taking a block of the page
              every visit.
            */}
            <TooltipProvider delayDuration={150}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    aria-label="How event requests are reviewed"
                    className="inline-flex h-6 w-6 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <Info className="h-4 w-4" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom" align="start" className="max-w-xs text-left">
                  Requests are reviewed by an SFLuv admin. Approving an event is what reserves its rewards
                  from the faucet and generates its QR codes — once approved, the codes are downloadable here
                  straight away and become redeemable 24 hours before the event starts.
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <p className="text-muted-foreground">
            Request events for your organization and manage the ones that have been approved.
          </p>
        </div>
        <Button onClick={() => setRequestModalOpen(true)} className="w-full sm:w-auto">
          + Request Event
        </Button>
      </div>

      {/* Editing reuses the request form. A separate edit form is how a field
          ends up creatable but not editable. */}
      <AddVolunteerEventModal
        key={editingEvent?.id ?? "new"}
        open={editingEvent !== null}
        onOpenChange={(next) => !next && setEditingEvent(null)}
        editEvent={editingEvent}
        createEvent={(draft) => saveEventEdit(editingEvent?.id ?? "", draft)}
        stagePhoto={stagePhoto}
        discardPhoto={discardPhoto}
        unallocatedBalance={Number.MAX_SAFE_INTEGER}
        submitLabel="Submit changes"
      />

      <AddVolunteerEventModal
        open={requestModalOpen}
        onOpenChange={setRequestModalOpen}
        createEvent={requestEvent}
        stagePhoto={stagePhoto}
        discardPhoto={discardPhoto}
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
        onEditEvent={(event) => setEditingEvent(event)}
      />
    </div>
  )
}
