"use client"

import type { ReactNode } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
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
  let title = "Location Application Pending"
  let body = `Your application for ${location.name} is currently under review.`
  let Icon = Clock
  let iconClass = "h-5 w-5 text-yellow-500 mr-2"

  if (applicationStatus === "approved") {
    borderClass = "border-green-300 dark:border-green-700"
    headerClass = "bg-green-50 dark:bg-green-900/20"
    title = "Location Application Approved"
    body = `Your application for ${location.name} has been approved!`
    Icon = CheckCircle
    iconClass = "h-5 w-5 text-green-500 mr-2"
  } else if (applicationStatus === "rejected") {
    borderClass = "border-red-300 dark:border-red-700"
    headerClass = "bg-red-50 dark:bg-red-900/20"
    title = "Location Application Not Approved"
    body = `Your application for ${location.name} was not approved.`
    Icon = XCircle
    iconClass = "h-5 w-5 text-red-500 mr-2"
  }

  return (
    <Card className={`overflow-hidden shadow-sm ${borderClass}`}>
      <CardHeader className={`${headerClass} px-4 pb-3 pt-5 sm:px-6`}>
        <CardTitle className="text-black dark:text-white">Application Status</CardTitle>
        <CardDescription className="text-black dark:text-white/80">{location.name}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 px-4 pb-5 pt-4 sm:px-6">
        <div>
          <h2 className="mb-2 flex items-center text-lg font-semibold leading-tight text-black dark:text-white sm:text-xl">
            <Icon className={iconClass} />
            {title}
          </h2>
          <p className="text-sm leading-relaxed text-gray-600 dark:text-gray-400 sm:text-base">{body}</p>
        </div>
        {children}
      </CardContent>
    </Card>
  )
}
