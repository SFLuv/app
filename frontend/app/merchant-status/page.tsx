"use client"

import { useApp } from "@/context/AppProvider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { AlertCircle, CheckCircle, Clock, XCircle } from "lucide-react"
import { useRouter } from "next/navigation"
import { useMemo } from "react"

type LocationApplicationStatus = "approved" | "pending" | "rejected"

const getLocationApplicationStatus = (approval?: boolean | null): LocationApplicationStatus => {
  if (approval === true) return "approved"
  if (approval === false) return "rejected"
  return "pending"
}

export default function MerchantStatusPage() {
  const { userLocations, status } = useApp()
  const router = useRouter()
  const sortedUserLocations = useMemo(() => {
    return [...userLocations].sort((a, b) => b.id - a.id)
  }, [userLocations])

  return (
    <div className="max-w-3xl mx-auto">
      <h1 className="text-3xl font-bold text-black dark:text-white mb-6">Merchant Status</h1>

      {status === "loading" && (
        <div className="min-h-screen flex items-center justify-center">
          <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
        </div>
      )}

      {status !== "loading" && sortedUserLocations.length === 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-black dark:text-white">Application Status</CardTitle>
            <CardDescription>Check the status of your merchant applications</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="text-center">
              <AlertCircle className="h-16 w-16 text-gray-500 mx-auto mb-4" />
              <h2 className="text-2xl font-bold text-black dark:text-white mb-2">No Application Found</h2>
              <p className="text-gray-600 dark:text-gray-400 mb-6">You haven't submitted a merchant application yet.</p>
              <Button
                className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                onClick={() => router.push("/settings/merchant-approval")}
              >
                Apply to Become a Merchant
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {status !== "loading" && sortedUserLocations.length > 0 && (
        <div className="space-y-4">
          {sortedUserLocations.map((location) => {
            const applicationStatus = getLocationApplicationStatus(location.approval)
            let borderClass = "border-yellow-300 dark:border-yellow-700"
            let headerClass = "bg-yellow-50 dark:bg-yellow-900/20 rounded-t-lg"
            let title = "Location Application Pending"
            let body = `Your application for ${location.name} is currently under review.`
            let Icon = Clock
            let iconClass = "h-5 w-5 text-yellow-500 mr-2"

            if (applicationStatus === "approved") {
              borderClass = "border-green-300 dark:border-green-700"
              headerClass = "bg-green-50 dark:bg-green-900/20 rounded-t-lg"
              title = "Location Application Approved"
              body = `Your application for ${location.name} has been approved!`
              Icon = CheckCircle
              iconClass = "h-5 w-5 text-green-500 mr-2"
            } else if (applicationStatus === "rejected") {
              borderClass = "border-red-300 dark:border-red-700"
              headerClass = "bg-red-50 dark:bg-red-900/20 rounded-t-lg"
              title = "Location Application Not Approved"
              body = `Your application for ${location.name} was not approved.`
              Icon = XCircle
              iconClass = "h-5 w-5 text-red-500 mr-2"
            }

            return (
              <Card key={location.id} className={borderClass}>
                <CardHeader className={headerClass}>
                  <CardTitle className="text-black dark:text-white">Application Status</CardTitle>
                  <CardDescription className="text-black dark:text-white/80">{location.name}</CardDescription>
                </CardHeader>
                <CardContent className="pt-4">
                  <h2 className="text-xl font-semibold text-black dark:text-white mb-2 flex items-center">
                    <Icon className={iconClass} />
                    {title}
                  </h2>
                  <p className="text-gray-600 dark:text-gray-400">{body}</p>
                </CardContent>
              </Card>
            )
          })}

          <Button
            variant="outline"
            className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
            onClick={() => router.push("/settings/merchant-approval")}
          >
            Submit Another Application
          </Button>
        </div>
      )}
    </div>
  )
}
