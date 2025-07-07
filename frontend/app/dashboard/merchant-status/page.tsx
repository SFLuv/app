"use client"

import { useApp } from "@/context/app-context"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { AlertCircle, CheckCircle, Clock, XCircle } from "lucide-react"
import { useRouter } from "next/navigation"

export default function MerchantStatusPage() {
  const { user } = useApp()
  const router = useRouter()

  const renderStatusContent = () => {
    switch (user?.merchantStatus) {
      case "pending":
        return (
          <div className="text-center">
            <Clock className="h-16 w-16 text-yellow-500 mx-auto mb-4" />
            <h2 className="text-2xl font-bold text-black dark:text-white mb-2">Application Under Review</h2>
            <p className="text-gray-600 dark:text-gray-400 mb-6">
              Your merchant application is currently being reviewed by our team. This process typically takes 1-3
              business days.
            </p>
            <div className="bg-yellow-100 dark:bg-yellow-900/30 p-4 rounded-lg text-yellow-800 dark:text-yellow-200 mb-6">
              <p>While your application is pending, you can continue to use SFLuv as a community member.</p>
            </div>
          </div>
        )

      case "approved":
        return (
          <div className="text-center">
            <CheckCircle className="h-16 w-16 text-green-500 mx-auto mb-4" />
            <h2 className="text-2xl font-bold text-black dark:text-white mb-2">Application Approved!</h2>
            <p className="text-gray-600 dark:text-gray-400 mb-6">
              Congratulations! Your merchant application has been approved. You now have access to all merchant
              features.
            </p>
            <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={() => router.push("/dashboard")}>
              Go to Merchant Dashboard
            </Button>
          </div>
        )

      case "rejected":
        return (
          <div className="text-center">
            <XCircle className="h-16 w-16 text-red-500 mx-auto mb-4" />
            <h2 className="text-2xl font-bold text-black dark:text-white mb-2">Application Not Approved</h2>
            <p className="text-gray-600 dark:text-gray-400 mb-6">
              Unfortunately, your merchant application was not approved at this time.
            </p>
            <div className="bg-red-100 dark:bg-red-900/30 p-4 rounded-lg text-red-800 dark:text-red-200 mb-6">
              <p>
                If you believe this was an error or would like to provide additional information, please contact our
                support team.
              </p>
            </div>
            <Button variant="outline" onClick={() => router.push("/dashboard/settings")}>
              Return to Settings
            </Button>
          </div>
        )

      default:
        return (
          <div className="text-center">
            <AlertCircle className="h-16 w-16 text-gray-500 mx-auto mb-4" />
            <h2 className="text-2xl font-bold text-black dark:text-white mb-2">No Application Found</h2>
            <p className="text-gray-600 dark:text-gray-400 mb-6">You haven't submitted a merchant application yet.</p>
            <Button
              className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
              onClick={() => router.push("/dashboard/merchant-approval")}
            >
              Apply to Become a Merchant
            </Button>
          </div>
        )
    }
  }

  return (
    <div className="max-w-3xl mx-auto">
      <h1 className="text-3xl font-bold text-black dark:text-white mb-6">Merchant Status</h1>

      <Card>
        <CardHeader>
          <CardTitle className="text-black dark:text-white">Application Status</CardTitle>
          <CardDescription>Check the status of your merchant application</CardDescription>
        </CardHeader>
        <CardContent>{renderStatusContent()}</CardContent>
      </Card>
    </div>
  )
}
