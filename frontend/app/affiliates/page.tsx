"use client"

import { useEffect, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { AddEventModal } from "@/components/events/add-event-modal"
import { EventModal } from "@/components/events/event-modal"
import EventCard from "@/components/events/event-card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { AffiliateBalance } from "@/types/affiliate"
import { Event } from "@/types/event"
import { AlertTriangle, CalendarIcon, Leaf } from "lucide-react"

export default function AffiliatesPage() {
  const { authFetch, status, user } = useApp()

  const [events, setEvents] = useState<Event[]>([])
  const [eventsError, setEventsError] = useState<string>("")
  const [eventsModalOpen, setEventsModalOpen] = useState<boolean>(false)
  const [eventDetailModalOpen, setEventDetailModalOpen] = useState<boolean>(false)
  const [eventDetailsEvent, setEventDetailsEvent] = useState<Event | undefined>(undefined)
  const [deleteEventError] = useState<string | undefined>(undefined)
  const [balance, setBalance] = useState<AffiliateBalance | null>(null)

  const toggleNewEventModal = () => setEventsModalOpen(!eventsModalOpen)
  const toggleEventDetailModal = () => setEventDetailModalOpen(!eventDetailModalOpen)

  const getEvents = async () => {
    try {
      const res = await authFetch("/affiliates/events")
      const data = await res.json()
      setEvents(data || [])
    } catch {
      setEventsError("Error fetching events. Please try again later.")
    }
  }

  const getBalance = async () => {
    try {
      const res = await authFetch("/affiliates/balance")
      const data = await res.json()
      setBalance(data)
    } catch {
      setEventsError("Error fetching affiliate balance.")
    }
  }

  const handleAddEvent = async (ev: Event) => {
    const url = "/affiliates/events"
    try {
      const res = await authFetch(url, {
        method: "POST",
        body: JSON.stringify(ev),
      })
      if (!res.ok) {
        const message = await res.text()
        throw new Error(message || "Error adding event. Please try again later.")
      }
      setEventsError("")
    } catch (error) {
      const message = error instanceof Error ? error.message : null
      setEventsError(message || "Error adding event. Please try again later.")
    }

    await getEvents()
    await getBalance()
    toggleNewEventModal()
  }

  const handleDeleteEvent = async (id: string) => {
    const url = "/affiliates/events/" + id
    try {
      const res = await authFetch(url, {
        method: "DELETE",
      })
      if (!res.ok) throw new Error()
    } catch {
      setEventsError("Error deleting event. Please try again later.")
    }

    await getEvents()
    await getBalance()
    toggleEventDetailModal()
  }

  useEffect(() => {
    if (status !== "authenticated") return
    getEvents()
    getBalance()
  }, [status])

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
              Your account is not yet approved for affiliate events. Submit a request in settings.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Affiliates Panel</h1>
        <p className="text-muted-foreground">Create and manage affiliate-funded events</p>
      </div>

      {eventsError && (
        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
          <span>{eventsError}</span>
        </div>
      )}

      <AddEventModal
        open={eventsModalOpen}
        onOpenChange={toggleNewEventModal}
        handleAddEvent={handleAddEvent}
        addEventError={eventsError}
        currentBalance={balance?.available || 0}
      />
      <EventModal
        event={eventDetailsEvent}
        open={eventDetailModalOpen}
        onOpenChange={toggleEventDetailModal}
        handleDeleteEvent={handleDeleteEvent}
        deleteEventError={deleteEventError}
        eventsBasePath="/affiliates/events"
      />

      <Card>
        <CardHeader className="pb-6 grid grid-cols-[2fr,1fr]">
          <div>
            <CardTitle className="flex items-center gap-2 text-xl">
              <CalendarIcon className="h-6 w-6" />
              Affiliate Events
            </CardTitle>
            <CardDescription className="text-base mt-2">Allocate your affiliate balance to new events</CardDescription>
            <div className="flex items-center gap-2 mt-3">
              <Badge className="text-sm px-3 py-1">
                {balance
                  ? `${balance.available}/${balance.weekly_allocation} SFLuv`
                  : "Balance loading"}
              </Badge>
              <span className="text-sm text-muted-foreground">remaining / weekly allocation</span>
            </div>
          </div>
          <div className="text-right">
            <Button onClick={toggleNewEventModal}>
              + New Event
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {events.length === 0 ? (
            <div className="text-center py-8">
              <Leaf className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
              <h3 className="text-lg font-medium">No Active Events</h3>
              <p className="text-muted-foreground">Create a new event to see it here.</p>
            </div>
          ) : (
            <div className="space-y-4">
              {events.map((event: Event) => (
                <EventCard
                  key={event.id}
                  event={event}
                  toggleEventModal={toggleEventDetailModal}
                  setEventModalEvent={setEventDetailsEvent}
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
