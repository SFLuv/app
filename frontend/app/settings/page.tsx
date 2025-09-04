"use client"

import type React from "react"

import { useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Separator } from "@/components/ui/separator"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Check, Loader2, Upload, User, Clock, XCircle } from "lucide-react"

type MerchantStatus = "approved" | "pending" | "rejected" | "none"

export default function SettingsPage() {
  const router = useRouter()
  const { user, updateUser, userLocations, status } = useApp()

  const merchantStatus: MerchantStatus = useMemo(() => {
    console.log(userLocations)
    if(userLocations.length == 0) return "none"
    if(userLocations.find((loc) => loc.approval)) return "approved"
    return "pending"
  }, [userLocations])

  // Form states
  const [activeTab, setActiveTab] = useState("account")
  const [isUpdating, setIsUpdating] = useState(false)
  const [isChangingPassword, setIsChangingPassword] = useState(false)
  const [successMessage, setSuccessMessage] = useState("")

  // Account form
  const [name, setName] = useState(user?.name || "")

  // Password form
  const [currentPassword, setCurrentPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [passwordError, setPasswordError] = useState("")

  // Merchant form (only for merchants)
  const [merchantName, setMerchantName] = useState("Your Business Name")
  const [merchantDescription, setMerchantDescription] = useState(
    "A brief description of your business and what you offer to the community.",
  )
  const [merchantAddress, setMerchantAddress] = useState({
    street: "123 Main St",
    city: "San Francisco",
    state: "CA",
    zip: "94105",
  })
  const [merchantPhone, setMerchantPhone] = useState("(415) 555-1234")
  const [merchantWebsite, setMerchantWebsite] = useState("www.yourbusiness.com")
  // Notification settings
  const [emailNotifications, setEmailNotifications] = useState(true)
  const [opportunityAlerts, setOpportunityAlerts] = useState(true)
  const [transactionAlerts, setTransactionAlerts] = useState(true)

  // Handle account update

  // Handle password change
  const handlePasswordChange = (e: React.FormEvent) => {
    e.preventDefault()

    // Validate passwords
    if (newPassword.length < 8) {
      setPasswordError("Password must be at least 8 characters long")
      return
    }

    if (newPassword !== confirmPassword) {
      setPasswordError("Passwords do not match")
      return
    }

    setPasswordError("")
    setIsChangingPassword(true)

    // Simulate API call
    setTimeout(() => {
      setIsChangingPassword(false)
      setCurrentPassword("")
      setNewPassword("")
      setConfirmPassword("")
      showSuccessMessage("Password changed successfully")
    }, 1500)
  }

  // Handle merchant update
  const handleMerchantUpdate = (e: React.FormEvent) => {
    e.preventDefault()
    setIsUpdating(true)

    // Simulate API call
    setTimeout(() => {
      setIsUpdating(false)
      showSuccessMessage("Merchant details updated successfully")
    }, 1500)
  }

  // Handle notification settings update
  const handleNotificationUpdate = (e: React.FormEvent) => {
    e.preventDefault()
    setIsUpdating(true)

    // Simulate API call
    setTimeout(() => {
      setIsUpdating(false)
      showSuccessMessage("Notification settings updated successfully")
    }, 1500)
  }

  // Show success message
  const showSuccessMessage = (message: string) => {
    setSuccessMessage(message)
    setTimeout(() => {
      setSuccessMessage("")
    }, 3000)
  }


  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="space-y-6 w-full">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Account Settings</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Manage your account preferences and settings</p>
      </div>

      {/* {successMessage && (
        <div className="bg-green-100 dark:bg-green-900 border border-green-200 dark:border-green-800 text-green-800 dark:text-green-200 px-4 py-3 rounded flex items-center">
          <Check className="h-5 w-5 mr-2" />
          {successMessage}
        </div>
      )} */}

      <Tabs defaultValue="account" value={activeTab} onValueChange={setActiveTab} className="w-full">
        <TabsList className="w-full mb-6 bg-secondary">
          {user?.isMerchant ? (
            <div className={"grid grid-cols-2 w-full"}>
              <TabsTrigger value="account" className="text-black dark:text-white w-full">
                Account
              </TabsTrigger>
              <TabsTrigger value="merchant" className="text-black dark:text-white w-full">
                Merchant Profile
              </TabsTrigger>
            </div>
          ) : (
            <div className="grid grid-cols-1 w-full">
              <TabsTrigger value="account" className="text-black dark:text-white w-full">
                Account
              </TabsTrigger>
            </div>
          )}
        </TabsList>

        <TabsContent value="account">
          {/* <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-black dark:text-white">Profile Information</CardTitle>
                <CardDescription>Update your account details</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleAccountUpdate} className="space-y-4">
                  <div className="flex items-center gap-4 mb-6">
                    <Avatar className="h-20 w-20">
                      <AvatarImage src={user?.avatar || "/placeholder.svg"} alt={user?.name} />
                      <AvatarFallback className="bg-[#eb6c6c] text-white text-xl">
                        {user?.name?.charAt(0) || <User />}
                      </AvatarFallback>
                    </Avatar>
                    <div>
                      <Button variant="outline" size="sm" className="text-black dark:text-white bg-secondary">
                        <Upload className="h-4 w-4 mr-2" />
                        Change Avatar
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="name" className="text-black dark:text-white">
                      Full Name
                    </Label>
                    <Input
                      id="name"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="email" className="text-black dark:text-white">
                      Email Address
                    </Label>
                    <Input
                      id="email"
                      type="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      className="text-black dark:text-white bg-secondary rounded-md"
                    />
                  </div>

                  <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isUpdating}>
                    {isUpdating ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Updating...
                      </>
                    ) : (
                      "Update Profile"
                    )}
                  </Button>
                </form>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-black dark:text-white">Change Password</CardTitle>
                <CardDescription>Update your account password</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handlePasswordChange} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="current-password" className="text-black dark:text-white">
                      Current Password
                    </Label>
                    <Input
                      id="current-password"
                      type="password"
                      value={currentPassword}
                      onChange={(e) => setCurrentPassword(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="new-password" className="text-black dark:text-white">
                      New Password
                    </Label>
                    <Input
                      id="new-password"
                      type="password"
                      value={newPassword}
                      onChange={(e) => {
                        setNewPassword(e.target.value)
                        setPasswordError("")
                      }}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="confirm-password" className="text-black dark:text-white">
                      Confirm New Password
                    </Label>
                    <Input
                      id="confirm-password"
                      type="password"
                      value={confirmPassword}
                      onChange={(e) => {
                        setConfirmPassword(e.target.value)
                        setPasswordError("")
                      }}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  {passwordError && <p className="text-red-500 text-sm">{passwordError}</p>}

                  <Button
                    type="submit"
                    className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]"
                    disabled={isChangingPassword || !currentPassword || !newPassword || !confirmPassword}
                  >
                    {isChangingPassword ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Changing Password...
                      </>
                    ) : (
                      "Change Password"
                    )}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </div> */}
          {merchantStatus == "none" && (
            <Card className="mt-6">
              <CardHeader>
                <CardTitle className="text-black dark:text-white">Become a Merchant</CardTitle>
                <CardDescription>Apply to accept SFLuv as payment for your business</CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-gray-600 dark:text-gray-400 mb-4">
                  As a merchant, you can accept SFLuv as payment, appear on the merchant map, and access
                  merchant-specific features.
                </p>
                <Button
                  variant="outline"
                  className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                  onClick={() => router.push("/settings/merchant-approval")}
                >
                  Apply to Become a Merchant
                </Button>
              </CardContent>
            </Card>
          )}

          {(merchantStatus === "pending" || merchantStatus == "approved") && (
            <div className="space-y-4">
              <div className="space-y-2">
                {userLocations.filter((loc) => loc.approval).map((loc) => {
                  return (
                    <Card className="mt-6 border-green-300 dark:border-green-700" key={loc.id}>
                      <CardHeader className="bg-green-50 dark:bg-green-900/20 rounded-t-lg">
                        <CardTitle className="text-black dark:text-white flex items-center">
                          <Clock className="h-5 w-5 text-green-500 mr-2" />
                          Location Application Approved
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="pt-4">
                        <p className="text-gray-600 dark:text-gray-400 mb-4">
                          Your application for {loc.name} has been approved!
                        </p>
                        {/* <Button
                          variant="outline"
                          className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                          onClick={() => router.push("/merchant-status")}
                        >
                          Check Application Status
                        </Button> */}
                      </CardContent>
                    </Card>
                  )
                })}
                {userLocations.filter((loc) => loc.approval === false).map((loc) => {
                  return (
                    <Card className="mt-6 border-yellow-300 dark:border-yellow-700" key={loc.id}>
                      <CardHeader className="bg-yellow-50 dark:bg-yellow-900/20 rounded-t-lg">
                        <CardTitle className="text-black dark:text-white flex items-center">
                          <Clock className="h-5 w-5 text-yellow-500 mr-2" />
                          Location Application Pending
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="pt-4">
                        <p className="text-gray-600 dark:text-gray-400 mb-4">
                          Your application for {loc.name} is currently under review.
                        </p>
                        {/* <Button
                          variant="outline"
                          className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                          onClick={() => router.push("/merchant-status")}
                        >
                          Check Application Status
                        </Button> */}
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            <Button
              variant="outline"
              className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
              onClick={() => router.push("/settings/merchant-approval")}
            >
              Submit Another Application
            </Button>
            </div>
          )}

          {/* {user?.merchantStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Merchant Application Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400 mb-4">
                  Unfortunately, your merchant application was not approved. You can contact support for more
                  information.
                </p>
                <Button
                  variant="outline"
                  className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                  onClick={() => router.push("/merchant-status")}
                >
                  View Details
                </Button>
              </CardContent>
            </Card>
          )} */}
        </TabsContent>

        {merchantStatus == "approved" && (
          <TabsContent value="merchant">
            <Card>
              <CardHeader>
                <CardTitle className="text-black dark:text-white">Merchant Profile</CardTitle>
                <CardDescription>Update your business information</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleMerchantUpdate} className="space-y-6">
                  <div className="space-y-2">
                    <Label htmlFor="merchant-name" className="text-black dark:text-white">
                      Business Name
                    </Label>
                    <Input
                      id="merchant-name"
                      value={merchantName}
                      onChange={(e) => setMerchantName(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="merchant-description" className="text-black dark:text-white">
                      Business Description
                    </Label>
                    <Textarea
                      id="merchant-description"
                      value={merchantDescription}
                      onChange={(e) => setMerchantDescription(e.target.value)}
                      className="text-black dark:text-white bg-secondary min-h-[100px]"
                    />
                  </div>

                  <div>
                    <h3 className="text-lg font-medium text-black dark:text-white mb-4">Business Address</h3>
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label htmlFor="street" className="text-black dark:text-white">
                          Street Address
                        </Label>
                        <Input
                          id="street"
                          value={merchantAddress.street}
                          onChange={(e) => setMerchantAddress({ ...merchantAddress, street: e.target.value })}
                          className="text-black dark:text-white bg-secondary"
                        />
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="city" className="text-black dark:text-white">
                          City
                        </Label>
                        <Input
                          id="city"
                          value={merchantAddress.city}
                          onChange={(e) => setMerchantAddress({ ...merchantAddress, city: e.target.value })}
                          className="text-black dark:text-white bg-secondary"
                        />
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="state" className="text-black dark:text-white">
                          State
                        </Label>
                        <Input
                          id="state"
                          value={merchantAddress.state}
                          onChange={(e) => setMerchantAddress({ ...merchantAddress, state: e.target.value })}
                          className="text-black dark:text-white bg-secondary"
                        />
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="zip" className="text-black dark:text-white">
                          ZIP Code
                        </Label>
                        <Input
                          id="zip"
                          value={merchantAddress.zip}
                          onChange={(e) => setMerchantAddress({ ...merchantAddress, zip: e.target.value })}
                          className="text-black dark:text-white bg-secondary"
                        />
                      </div>
                    </div>
                  </div>

                  <Separator />

                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-2">
                      <Label htmlFor="merchant-phone" className="text-black dark:text-white">
                        Business Phone
                      </Label>
                      <Input
                        id="merchant-phone"
                        value={merchantPhone}
                        onChange={(e) => setMerchantPhone(e.target.value)}
                        className="text-black dark:text-white bg-secondary"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="merchant-website" className="text-black dark:text-white">
                        Business Website
                      </Label>
                      <Input
                        id="merchant-website"
                        value={merchantWebsite}
                        onChange={(e) => setMerchantWebsite(e.target.value)}
                        className="text-black dark:text-white bg-secondary"
                      />
                    </div>
                  </div>

                  <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isUpdating}>
                    {isUpdating ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Updating...
                      </>
                    ) : (
                      "Update Merchant Profile"
                    )}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </TabsContent>
        )}

        <TabsContent value="notifications">
          <Card>
            <CardHeader>
              <CardTitle className="text-black dark:text-white">Notification Preferences</CardTitle>
              <CardDescription>Manage how you receive notifications</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleNotificationUpdate} className="space-y-6">
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-base font-medium text-black dark:text-white">Email Notifications</h3>
                      <p className="text-sm text-gray-500 dark:text-gray-400">
                        Receive email notifications for important updates
                      </p>
                    </div>
                    <Switch
                      id="email-notifications"
                      checked={emailNotifications}
                      onCheckedChange={setEmailNotifications}
                    />
                  </div>

                  <Separator />

                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-base font-medium text-black dark:text-white">Opportunity Alerts</h3>
                      <p className="text-sm text-gray-500 dark:text-gray-400">
                        Get notified about new volunteer opportunities
                      </p>
                    </div>
                    <Switch
                      id="opportunity-alerts"
                      checked={opportunityAlerts}
                      onCheckedChange={setOpportunityAlerts}
                    />
                  </div>

                  <Separator />

                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-base font-medium text-black dark:text-white">Transaction Alerts</h3>
                      <p className="text-sm text-gray-500 dark:text-gray-400">
                        Receive notifications for SFLuv transactions
                      </p>
                    </div>
                    <Switch
                      id="transaction-alerts"
                      checked={transactionAlerts}
                      onCheckedChange={setTransactionAlerts}
                    />
                  </div>
                </div>

                <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isUpdating}>
                  {isUpdating ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Updating...
                    </>
                  ) : (
                    "Save Notification Preferences"
                  )}
                </Button>
              </form>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
