"use client"

import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { CalendarClock, MapPin, Users, Coins, Building, CheckCircle, XCircle } from "lucide-react"
import type { Opportunity } from "@/types/opportunity"
import Image from "next/image"

interface OpportunityModalProps {
  opportunity: Opportunity | null
  isOpen: boolean
  onClose: () => void
  isRegistered: boolean
  onRegister: () => void
  onCancelRegistration: () => void
}

export function OpportunityModal({
  opportunity,
  isOpen,
  onClose,
  isRegistered,
  onRegister,
  onCancelRegistration,
}: OpportunityModalProps) {
  const [isSubmitting, setIsSubmitting] = useState(false)

  if (!opportunity) return null

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

  const handleRegister = () => {
    setIsSubmitting(true)
    // Simulate API call
    setTimeout(() => {
      onRegister()
      setIsSubmitting(false)
    }, 1000)
  }

  const handleCancelRegistration = () => {
    setIsSubmitting(true)
    // Simulate API call
    setTimeout(() => {
      onCancelRegistration()
      setIsSubmitting(false)
    }, 1000)
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="text-2xl text-black dark:text-white">{opportunity.title}</DialogTitle>
          <DialogDescription className="text-gray-600 dark:text-gray-300">
            Organized by {opportunity.organizer}
          </DialogDescription>
        </DialogHeader>

        <div className="relative h-64 w-full my-4">
          <Image
            src={opportunity.imageUrl || "/placeholder.svg?height=300&width=600"}
            alt={opportunity.title}
            fill
            className="object-cover rounded-md"
          />
        </div>

        <div className="grid grid-cols-2 gap-4 mb-4">
          <div className="flex items-center text-gray-700 dark:text-gray-300">
            <CalendarClock className="h-5 w-5 mr-2 text-[#eb6c6c]" />
            <div>
              <div className="font-medium">Date & Time</div>
              <div>
                {formattedDate} at {formattedTime}
              </div>
            </div>
          </div>

          <div className="flex items-center text-gray-700 dark:text-gray-300">
            <Building className="h-5 w-5 mr-2 text-[#eb6c6c]" />
            <div>
              <div className="font-medium">Organizer</div>
              <div>{opportunity.organizer}</div>
            </div>
          </div>

          <div className="flex items-center text-gray-700 dark:text-gray-300">
            <MapPin className="h-5 w-5 mr-2 text-[#eb6c6c]" />
            <div>
              <div className="font-medium">Location</div>
              <div>{opportunity.location.address}</div>
              <div>
                {opportunity.location.city}, {opportunity.location.state} {opportunity.location.zip}
              </div>
            </div>
          </div>

          <div className="flex items-center text-gray-700 dark:text-gray-300">
            <Users className="h-5 w-5 mr-2 text-[#eb6c6c]" />
            <div>
              <div className="font-medium">Volunteers</div>
              <div>
                {opportunity.volunteersSignedUp} of {opportunity.volunteersNeeded} spots filled
              </div>
            </div>
          </div>

          <div className="flex items-center text-gray-700 dark:text-gray-300 col-span-2">
            <Coins className="h-5 w-5 mr-2 text-[#eb6c6c]" />
            <div>
              <div className="font-medium">Reward</div>
              <div>{opportunity.rewardAmount} SFLuv per hour</div>
            </div>
          </div>
        </div>

        <div className="mb-6">
          <h3 className="text-lg font-medium mb-2 text-black dark:text-white">Description</h3>
          <p className="text-gray-700 dark:text-gray-300 whitespace-pre-line">{opportunity.description}</p>
        </div>

        <DialogFooter>
          {isRegistered ? (
            <div className="w-full flex flex-col sm:flex-row gap-4 items-center">
              <div className="flex items-center text-green-600 dark:text-green-400 mr-auto">
                <CheckCircle className="h-5 w-5 mr-2" />
                <span className="font-medium">You're registered for this opportunity</span>
              </div>
              <Button
                variant="outline"
                className="border-red-500 text-red-500 hover:bg-red-500 hover:text-white"
                onClick={handleCancelRegistration}
                disabled={isSubmitting}
              >
                {isSubmitting ? (
                  <>
                    <div className="animate-spin mr-2 h-4 w-4 border-2 border-white border-t-transparent rounded-full" />
                    Cancelling...
                  </>
                ) : (
                  <>
                    <XCircle className="h-4 w-4 mr-2" />
                    Cancel Registration
                  </>
                )}
              </Button>
            </div>
          ) : (
            <Button
              className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]"
              onClick={handleRegister}
              disabled={isSubmitting || opportunity.volunteersSignedUp >= opportunity.volunteersNeeded}
            >
              {isSubmitting ? (
                <>
                  <div className="animate-spin mr-2 h-4 w-4 border-2 border-white border-t-transparent rounded-full" />
                  Registering...
                </>
              ) : opportunity.volunteersSignedUp >= opportunity.volunteersNeeded ? (
                "This opportunity is full"
              ) : (
                "Register for this opportunity"
              )}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
