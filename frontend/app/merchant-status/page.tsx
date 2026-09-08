"use client"

import { useApp } from "@/context/AppProvider"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { AlertCircle } from "lucide-react"
import { useRouter } from "next/navigation"
import { useMemo } from "react"
import { LocationApplicationStatusCard } from "@/components/merchant/location-application-status"
import { CancelLocationApplication } from "@/components/merchant/cancel-location-application"
import { MERCHANT_ONBOARDING_PATH } from "@/lib/merchant-onboarding"

export default function MerchantStatusPage() {
  const { user, userLocations, status } = useApp()
  const router = useRouter()
  const sortedUserLocations = useMemo(() => {
    return [...userLocations].sort((a, b) => b.id - a.id)
  }, [userLocations])
  const isMerchantAccount = user?.accountType === "merchant"

  return (
    <div className="mx-auto w-full max-w-3xl space-y-5 px-3 pb-6 pt-2 sm:space-y-6 sm:px-0">
      <div className="space-y-1">
        <h1 className="text-2xl font-bold text-black dark:text-white sm:text-3xl">Merchant Status</h1>
      </div>

      {status === "loading" && (
        <div className="flex min-h-[260px] items-center justify-center rounded-lg border bg-card/40">
          <div className="h-10 w-10 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]"></div>
        </div>
      )}

      {status !== "loading" && sortedUserLocations.length === 0 && (
        <Card className="overflow-hidden border-border/80 bg-card/85 shadow-sm">
          <CardHeader className="px-4 pb-3 pt-5 sm:px-6">
            <CardTitle className="text-black dark:text-white">Application Status</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-6 pt-2 sm:px-6">
            {/* Merchant is chosen at signup now, so there is no application to
                offer a regular account — only a merchant who has not listed a
                shop yet has anywhere to be sent. */}
            <div className="rounded-lg border bg-muted/20 px-4 py-6 text-center sm:px-6">
              <AlertCircle className="mx-auto mb-3 h-12 w-12 text-gray-500" />
              <h2 className="mb-6 text-xl font-semibold text-black dark:text-white sm:text-2xl">
                {isMerchantAccount ? "No applications yet" : "Personal account"}
              </h2>
              {isMerchantAccount && (
                <Button
                  className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                  onClick={() => router.push(MERCHANT_ONBOARDING_PATH)}
                >
                  Set up your merchant account
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Applying for a further location belongs on Locations, next to the till
          it will be paid into, rather than being a second door here. */}
      {status !== "loading" && sortedUserLocations.length > 0 && (
        <div className="space-y-4 sm:space-y-5">
          {sortedUserLocations.map((location) => (
            <LocationApplicationStatusCard key={location.id} location={location}>
              {/* Renders itself only while the application is still pending, so
                  this screen — which is a read of where things stand — carries
                  the one action that belongs to that state and nothing else. */}
              <CancelLocationApplication location={location} />
            </LocationApplicationStatusCard>
          ))}
        </div>
      )}
    </div>
  )
}
