import { Event } from "@/types/event"
import { Card, CardContent } from "../ui/card"
import { Badge } from "../ui/badge"
import { Star, Trash } from "lucide-react"
import { Button } from "../ui/button"

interface EventCardProps {
  event: Event
  toggleEventModal: () => void
  setEventModalEvent: (c: Event) => void
  ownerLabel?: string
}

const EventCard = ({
  event,
  toggleEventModal,
  setEventModalEvent,
  ownerLabel,
}: EventCardProps) => {
  const toggleModal = () => {
    setEventModalEvent(event)
    toggleEventModal()
  }
  return(
    <Card key={event.id} className="overflow-hidden hover:shadow-md transition-shadow">
      <CardContent className="p-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div>
              <div className="flex items-center gap-2">
                <h3 className="font-medium text-black dark:text-white">
                  {event.title}
                </h3>
              </div>
              <p className="text-sm text-gray-600 dark:text-gray-400 truncate max-w-[200px] md:max-w-[300px] font-mono">
                {event.description.slice(0, 20) + (event.description.length > 20 ? "..." : "")}
              </p>
              {ownerLabel && (
                <p className="text-xs text-muted-foreground mt-1">
                  Owner: {ownerLabel}
                </p>
              )}
            </div>
          </div>

          <div className="flex items-center gap-2">
              <Button
                size="sm"
                onClick={toggleModal}
              >
                Details
              </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default EventCard
