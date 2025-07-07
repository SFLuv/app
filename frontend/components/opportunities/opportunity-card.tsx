"use client"

import { CalendarClock, MapPin, Users, Coins } from "lucide-react"
import { Card, CardContent } from "@/components/ui/card"
import type { Opportunity } from "@/types/opportunity"
import Image from "next/image"

interface OpportunityCardProps {
  opportunity: Opportunity
  onClick: () => void
}

export function OpportunityCard({ opportunity, onClick }: OpportunityCardProps) {
  // Format date
  const formattedDate = new Date(opportunity.date).toLocaleDateString("en-US", {
    weekday: "long",
    year: "numeric",
    month: "long",
    day: "numeric",
  })

  // Format time
  const formattedTime = new Date(opportunity.date).toLocaleTimeString("en-US", {
    hour: "numeric",
    minute: "2-digit",
  })

  return (
    <Card className="overflow-hidden transition-all hover:shadow-md cursor-pointer" onClick={onClick}>
      <div className="relative h-48 w-full">
        <Image
          src={opportunity.imageUrl || "/placeholder.svg?height=200&width=400"}
          alt={opportunity.title}
          fill
          className="object-cover"
        />
      </div>
      <CardContent className="p-4">
        <h3 className="text-xl font-semibold mb-2 text-black dark:text-white">{opportunity.title}</h3>
        <p className="text-gray-600 dark:text-gray-300 line-clamp-2 mb-4">{opportunity.description}</p>

        <div className="grid grid-cols-2 gap-2 text-sm">
          <div className="flex items-center text-gray-600 dark:text-gray-300">
            <CalendarClock className="h-4 w-4 mr-2 text-[#eb6c6c]" />
            <span>{formattedDate}</span>
          </div>
          <div className="flex items-center text-gray-600 dark:text-gray-300">
            <MapPin className="h-4 w-4 mr-2 text-[#eb6c6c]" />
            <span>{opportunity.location.city}</span>
          </div>
          <div className="flex items-center text-gray-600 dark:text-gray-300">
            <Users className="h-4 w-4 mr-2 text-[#eb6c6c]" />
            <span>
              {opportunity.volunteersSignedUp}/{opportunity.volunteersNeeded} volunteers
            </span>
          </div>
          <div className="flex items-center text-gray-600 dark:text-gray-300">
            <Coins className="h-4 w-4 mr-2 text-[#eb6c6c]" />
            <span>{opportunity.rewardAmount} SFLuv/hr</span>
          </div>
        </div>

        <div className="mt-4">
          <span className="text-sm text-gray-600 dark:text-gray-300">{opportunity.organizer}</span>
        </div>
      </CardContent>
    </Card>
  )
}
