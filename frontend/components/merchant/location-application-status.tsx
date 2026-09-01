"use client"

import type { ReactNode } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { CheckCircle, Clock, XCircle } from "lucide-react"
import { AuthedLocation } from "@/types/location"
import { getLocationApplicationStatus } from "@/lib/merchant-onboarding"

/**
 * One merchant application and where it stands. Lifted out of
 * /merchant-status so the onboarding screen shows a rejection in exactly the
 * wording and colour the merchant has already seen elsewhere.
 *
 * The follow-up action is a slot rather than a prop: what a merchant can do
 * about a rejection depends on the screen they are reading it on, and the card
 * has no business knowing.
 */
export function LocationApplicationStatusCard({
  location,
  children,
}: {
  location: AuthedLocation
  children?: ReactNode
}) {
  const applicationStatus = getLocationApplicationStatus(location.approval)

  let borderClass = "border-yellow-300 dark:border-yellow-700"
  let headerClass = "bg-yellow-50 dark:bg-yellow-900/20"
  let title = "Awaiting review"
  let Icon = Clock
  let iconClass = "h-5 w-5 text-yellow-500 mr-2"

  if (applicationStatus === "approved") {
    borderClass = "border-green-300 dark:border-green-700"
    headerClass = "bg-green-50 dark:bg-green-900/20"
    title = "Approved"
    Icon = CheckCircle
    iconClass = "h-5 w-5 text-green-500 mr-2"
  } else if (applicationStatus === "rejected") {
    borderClass = "border-red-300 dark:border-red-700"
    headerClass = "bg-red-50 dark:bg-red-900/20"
    title = "Not approved"
    Icon = XCircle
    iconClass = "h-5 w-5 text-red-500 mr-2"
  }

  return (
    <Card className={`overflow-hidden shadow-sm ${borderClass}`}>
      {/* The listing's name is the card's subject and its status is the
          answer, so the two share the header instead of one restating the other
          as a sentence underneath. */}
      <CardHeader className={`${headerClass} flex flex-row items-center justify-between gap-3 px-4 pb-3 pt-5 sm:px-6`}>
        <CardTitle className="text-black dark:text-white">{location.name}</CardTitle>
        <span className="flex shrink-0 items-center text-sm font-semibold text-black dark:text-white">
          <Icon className={iconClass} />
          {title}
        </span>
      </CardHeader>
      <CardContent className="space-y-4 px-4 pb-5 pt-4 sm:px-6">{children}</CardContent>
    </Card>
  )
}
