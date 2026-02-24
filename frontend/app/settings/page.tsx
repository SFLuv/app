"use client"

import type React from "react"

import { useEffect, useMemo, useState } from "react"
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Check, Loader2, Upload, User, Clock, XCircle, AlertTriangle } from "lucide-react"
import { VerifiedEmailResponse } from "@/types/server"

type MerchantStatus = "approved" | "pending" | "rejected" | "none"
type LocationApplicationStatus = "approved" | "pending" | "rejected"

const getLocationApplicationStatus = (approval?: boolean | null): LocationApplicationStatus => {
  if (approval === true) return "approved"
  if (approval === false) return "rejected"
  return "pending"
}

export default function SettingsPage() {
  const router = useRouter()
  const { user, userLocations, status, affiliate, setAffiliate, proposer, setProposer, improver, setImprover, issuer, setIssuer, authFetch } = useApp()
  const userRole = useMemo(() => user?.isAdmin ? "admin" : user?.isMerchant ? "merchant" : "user", [user])

  const merchantStatus: MerchantStatus = useMemo(() => {
    if (userLocations.length === 0) return "none"
    if (userLocations.some((loc) => getLocationApplicationStatus(loc.approval) === "approved")) return "approved"
    if (userLocations.some((loc) => getLocationApplicationStatus(loc.approval) === "pending")) return "pending"
    return "rejected"
  }, [userLocations])

  const sortedUserLocations = useMemo(() => {
    return [...userLocations].sort((a, b) => b.id - a.id)
  }, [userLocations])

  const affiliateStatus = useMemo(() => {
    if (user?.isAffiliate) return "approved"
    if (affiliate?.status) return affiliate.status
    return "none"
  }, [affiliate, user])

  const proposerStatus = useMemo(() => {
    if (user?.isProposer) return "approved"
    if (proposer?.status) return proposer.status
    return "none"
  }, [proposer, user])

  const improverStatus = useMemo(() => {
    if (user?.isImprover) return "approved"
    if (improver?.status) return improver.status
    return "none"
  }, [improver, user])

  const issuerStatus = useMemo(() => {
    if (user?.isIssuer) return "approved"
    if (issuer?.status) return issuer.status
    return "none"
  }, [issuer, user])

  useEffect(() => {
    if (affiliate?.affiliate_logo) {
      setAffiliateLogoPreview(affiliate.affiliate_logo)
    } else {
      setAffiliateLogoPreview("")
    }
  }, [affiliate?.affiliate_logo])


  // Form states
  const [activeTab, setActiveTab] = useState("account")
  const [isUpdating, setIsUpdating] = useState(false)
  const [isChangingPassword, setIsChangingPassword] = useState(false)
  const [successMessage, setSuccessMessage] = useState("")
  type RoleRequestType = "merchant" | "affiliate" | "proposer" | "improver" | "issuer" | ""
  const [roleRequestType, setRoleRequestType] = useState<RoleRequestType>("")
  const [roleOrg, setRoleOrg] = useState("")
  const [roleEmail, setRoleEmail] = useState("")
  const [roleFirstName, setRoleFirstName] = useState("")
  const [roleLastName, setRoleLastName] = useState("")
  const [roleSubmitting, setRoleSubmitting] = useState(false)
  const [roleError, setRoleError] = useState("")
  const [roleSuccess, setRoleSuccess] = useState("")
  const [verifiedEmails, setVerifiedEmails] = useState<VerifiedEmailResponse[]>([])
  const [verifiedEmailsLoading, setVerifiedEmailsLoading] = useState(false)
  const [verifiedEmailFormOpen, setVerifiedEmailFormOpen] = useState(false)
  const [newVerifiedEmail, setNewVerifiedEmail] = useState("")
  const [verifiedEmailSubmitting, setVerifiedEmailSubmitting] = useState(false)
  const [verifiedEmailError, setVerifiedEmailError] = useState("")
  const [verifiedEmailSuccess, setVerifiedEmailSuccess] = useState("")
  const [affiliateLogoPreview, setAffiliateLogoPreview] = useState<string>("")
  const [affiliateLogoSaving, setAffiliateLogoSaving] = useState(false)
  const [affiliateLogoError, setAffiliateLogoError] = useState("")

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

  const verifiedEmailOptions = useMemo(
    () => verifiedEmails.filter((email) => email.status === "verified"),
    [verifiedEmails],
  )

  const pendingOrExpiredEmailOptions = useMemo(
    () => verifiedEmails.filter((email) => email.status !== "verified"),
    [verifiedEmails],
  )

  useEffect(() => {
    if (verifiedEmailOptions.length === 0) {
      if (roleEmail !== "") setRoleEmail("")
      return
    }

    const roleEmailExists = verifiedEmailOptions.some((option) => option.email === roleEmail)
    if (!roleEmailExists) {
      setRoleEmail(verifiedEmailOptions[0].email)
    }
  }, [verifiedEmailOptions, roleEmail])

  const loadVerifiedEmails = async () => {
    if (status !== "authenticated") return

    setVerifiedEmailsLoading(true)
    setVerifiedEmailError("")
    try {
      const res = await authFetch("/users/verified-emails")
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load verified emails.")
      }
      const data = (await res.json()) as VerifiedEmailResponse[]
      setVerifiedEmails(data || [])
    } catch (err) {
      setVerifiedEmailError(err instanceof Error ? err.message : "Unable to load verified emails.")
    } finally {
      setVerifiedEmailsLoading(false)
    }
  }

  useEffect(() => {
    if (status === "authenticated") {
      void loadVerifiedEmails()
    }
  }, [status])

  const handleAddVerifiedEmail = async (e: React.FormEvent) => {
    e.preventDefault()

    const email = newVerifiedEmail.trim()
    if (!email) {
      setVerifiedEmailError("Email is required.")
      setVerifiedEmailSuccess("")
      return
    }

    setVerifiedEmailSubmitting(true)
    setVerifiedEmailError("")
    setVerifiedEmailSuccess("")
    try {
      const res = await authFetch("/users/verified-emails", {
        method: "POST",
        body: JSON.stringify({ email }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to send verification email.")
      }

      setNewVerifiedEmail("")
      setVerifiedEmailFormOpen(false)
      setVerifiedEmailSuccess("Verification email sent. It expires in 30 minutes.")
      await loadVerifiedEmails()
    } catch (err) {
      setVerifiedEmailError(err instanceof Error ? err.message : "Unable to send verification email.")
    } finally {
      setVerifiedEmailSubmitting(false)
    }
  }

  const handleResendVerification = async (emailId: string) => {
    setVerifiedEmailSubmitting(true)
    setVerifiedEmailError("")
    setVerifiedEmailSuccess("")
    try {
      const res = await authFetch(`/users/verified-emails/${emailId}/resend`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to resend verification email.")
      }

      setVerifiedEmailSuccess("Verification email resent. It expires in 30 minutes.")
      await loadVerifiedEmails()
    } catch (err) {
      setVerifiedEmailError(err instanceof Error ? err.message : "Unable to resend verification email.")
    } finally {
      setVerifiedEmailSubmitting(false)
    }
  }

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

  const handleRoleRequest = async (e: React.FormEvent) => {
    e.preventDefault()
    setRoleError("")
    setRoleSuccess("")

    if (!roleRequestType) {
      setRoleError("Please select a role type.")
      return
    }

    if (roleRequestType === "merchant") {
      router.push("/settings/merchant-approval")
      return
    }

    if (roleRequestType === "affiliate" || roleRequestType === "proposer" || roleRequestType === "issuer") {
      if (!roleOrg.trim()) { setRoleError("Organization name is required."); return }
    }
    if ((roleRequestType === "proposer" || roleRequestType === "issuer") && !roleEmail.trim()) {
      setRoleError("Notification email is required.")
      return
    }
    if (roleRequestType === "improver") {
      if (!roleFirstName.trim() || !roleLastName.trim() || !roleEmail.trim()) {
        setRoleError("First name, last name, and email are required.")
        return
      }
    }

    setRoleSubmitting(true)
    try {
      let res: Response
      if (roleRequestType === "affiliate") {
        res = await authFetch("/affiliates/request", { method: "POST", body: JSON.stringify({ organization: roleOrg.trim() }) })
        if (res.ok) { const data = await res.json(); setAffiliate(data) }
      } else if (roleRequestType === "proposer") {
        res = await authFetch("/proposers/request", { method: "POST", body: JSON.stringify({ organization: roleOrg.trim(), email: roleEmail.trim() }) })
        if (res.ok) { const data = await res.json(); setProposer(data) }
      } else if (roleRequestType === "issuer") {
        res = await authFetch("/issuers/request", { method: "POST", body: JSON.stringify({ organization: roleOrg.trim(), email: roleEmail.trim() }) })
        if (res.ok) { const data = await res.json(); setIssuer(data) }
      } else {
        res = await authFetch("/improvers/request", { method: "POST", body: JSON.stringify({ first_name: roleFirstName.trim(), last_name: roleLastName.trim(), email: roleEmail.trim() }) })
        if (res.ok) { const data = await res.json(); setImprover(data) }
      }
      if (!res!.ok) {
        if (res!.status === 409) {
          setRoleError("Your status for that role is already approved.")
        } else {
          setRoleError("Unable to submit your request right now. Please try again.")
        }
        return
      }
      setRoleSuccess("Your request has been submitted and is pending review.")
      setRoleRequestType("")
      setRoleOrg("")
      setRoleEmail(verifiedEmailOptions[0]?.email || "")
      setRoleFirstName("")
      setRoleLastName("")
    } catch {
      setRoleError("Unable to submit your request. Please try again.")
    } finally {
      setRoleSubmitting(false)
    }
  }


  const handleAffiliateLogoChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setAffiliateLogoError("")
    const file = e.target.files?.[0]
    if (!file) return

    if (!file.type.startsWith("image/")) {
      setAffiliateLogoError("Please upload a valid image file.")
      return
    }

    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result
      if (typeof result === "string") {
        setAffiliateLogoPreview(result)
      }
    }
    reader.readAsDataURL(file)
  }

  const handleAffiliateLogoSave = async () => {
    if (!affiliateLogoPreview) return
    setAffiliateLogoSaving(true)
    setAffiliateLogoError("")

    try {
      const res = await authFetch("/affiliates/logo", {
        method: "PUT",
        body: JSON.stringify({ logo: affiliateLogoPreview }),
      })

      if (!res.ok) {
        throw new Error("Unable to update affiliate logo right now.")
      }

      const updated = await res.json()
      setAffiliate(updated)
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unable to update affiliate logo right now."
      setAffiliateLogoError(message)
    } finally {
      setAffiliateLogoSaving(false)
    }
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
        <TabsList className="w-full mb-6 bg-secondary flex flex-wrap h-auto gap-1 p-1">
          <TabsTrigger value="account" className="text-black dark:text-white flex-1">
            Account
          </TabsTrigger>
          {merchantStatus !== "none" && (
            <TabsTrigger value="merchant" className="text-black dark:text-white flex-1">
              Merchant
            </TabsTrigger>
          )}
          {(affiliateStatus === "pending" || affiliateStatus === "approved") && (
            <TabsTrigger value="affiliate" className="text-black dark:text-white flex-1">
              Affiliate
            </TabsTrigger>
          )}
          {(proposerStatus === "pending" || proposerStatus === "approved") && (
            <TabsTrigger value="proposer" className="text-black dark:text-white flex-1">
              Proposer
            </TabsTrigger>
          )}
          {(improverStatus === "pending" || improverStatus === "approved") && (
            <TabsTrigger value="improver" className="text-black dark:text-white flex-1">
              Improver
            </TabsTrigger>
          )}
          {(issuerStatus === "pending" || issuerStatus === "approved") && (
            <TabsTrigger value="issuer" className="text-black dark:text-white flex-1">
              Issuer
            </TabsTrigger>
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

          <Card className="mt-6">
            <CardHeader className="flex flex-row items-start justify-between gap-4">
              <div>
                <CardTitle className="text-black dark:text-white">Verified Emails</CardTitle>
                <CardDescription>
                  Only verified emails can be used for role requests and wallet notification subscriptions.
                </CardDescription>
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setVerifiedEmailFormOpen((prev) => !prev)
                  setVerifiedEmailError("")
                  setVerifiedEmailSuccess("")
                }}
                className="whitespace-nowrap"
              >
                Add Email
              </Button>
            </CardHeader>
            <CardContent className="space-y-4">
              {verifiedEmailsLoading ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading verified emails...
                </div>
              ) : (
                <>
                  <div className="space-y-2">
                    <p className="text-sm font-medium text-black dark:text-white">Verified</p>
                    {verifiedEmailOptions.length === 0 ? (
                      <p className="text-sm text-muted-foreground">No verified emails yet.</p>
                    ) : (
                      verifiedEmailOptions.map((entry) => (
                        <div key={entry.id} className="rounded border bg-secondary/30 px-3 py-2 text-sm flex items-center justify-between gap-2">
                          <span className="break-all">{entry.email}</span>
                          <span className="text-xs text-green-700 dark:text-green-300">Verified</span>
                        </div>
                      ))
                    )}
                  </div>

                  {pendingOrExpiredEmailOptions.length > 0 && (
                    <div className="space-y-2">
                      <p className="text-sm font-medium text-black dark:text-white">Pending Verification</p>
                      {pendingOrExpiredEmailOptions.map((entry) => (
                        <div key={entry.id} className="rounded border bg-secondary/30 px-3 py-2 text-sm space-y-2">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <span className="break-all">{entry.email}</span>
                            <span className={`text-xs ${entry.status === "expired" ? "text-red-600 dark:text-red-300" : "text-yellow-700 dark:text-yellow-300"}`}>
                              {entry.status === "expired" ? "Expired" : "Pending"}
                            </span>
                          </div>
                          {entry.verification_token_expires_at && (
                            <p className="text-xs text-muted-foreground">
                              Expires: {new Date(entry.verification_token_expires_at).toLocaleString()}
                            </p>
                          )}
                          {entry.status === "expired" && (
                            <Button
                              type="button"
                              size="sm"
                              variant="secondary"
                              onClick={() => void handleResendVerification(entry.id)}
                              disabled={verifiedEmailSubmitting}
                            >
                              {verifiedEmailSubmitting ? "Sending..." : "Resend Verification"}
                            </Button>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}

              {verifiedEmailFormOpen && (
                <form onSubmit={handleAddVerifiedEmail} className="space-y-3 rounded border p-3">
                  <div className="space-y-1">
                    <Label htmlFor="verified-email-input">Email</Label>
                    <Input
                      id="verified-email-input"
                      type="email"
                      value={newVerifiedEmail}
                      onChange={(e) => setNewVerifiedEmail(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                      placeholder="name@example.com"
                    />
                  </div>
                  <div className="flex flex-wrap justify-end gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => {
                        setVerifiedEmailFormOpen(false)
                        setNewVerifiedEmail("")
                      }}
                    >
                      Cancel
                    </Button>
                    <Button type="submit" disabled={verifiedEmailSubmitting}>
                      {verifiedEmailSubmitting ? (
                        <>
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          Sending...
                        </>
                      ) : (
                        "Send Verification"
                      )}
                    </Button>
                  </div>
                </form>
              )}

              {verifiedEmailError && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{verifiedEmailError}</span>
                </div>
              )}
              {verifiedEmailSuccess && (
                <p className="text-sm text-green-600">{verifiedEmailSuccess}</p>
              )}
            </CardContent>
          </Card>

          {affiliateStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Affiliate Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">
                  Your affiliate request was not approved. Use the form below to submit another request.
                </p>
              </CardContent>
            </Card>
          )}

          {proposerStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Proposer Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">Your proposer request was not approved. Use the form below to submit another request.</p>
              </CardContent>
            </Card>
          )}

          {improverStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Improver Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">Your improver request was not approved. Use the form below to submit another request.</p>
              </CardContent>
            </Card>
          )}

          {issuerStatus === "rejected" && (
            <Card className="mt-6 border-red-300 dark:border-red-700">
              <CardHeader className="bg-red-50 dark:bg-red-900/20 rounded-t-lg">
                <CardTitle className="text-black dark:text-white flex items-center">
                  <XCircle className="h-5 w-5 text-red-500 mr-2" />
                  Issuer Request Not Approved
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <p className="text-gray-600 dark:text-gray-400">Your issuer request was not approved. Use the form below to submit another request.</p>
              </CardContent>
            </Card>
          )}

          {(merchantStatus === "none" || merchantStatus === "rejected" || affiliateStatus === "none" || affiliateStatus === "rejected" || proposerStatus === "none" || proposerStatus === "rejected" || improverStatus === "none" || improverStatus === "rejected" || issuerStatus === "none" || issuerStatus === "rejected") && (
            <Card className="mt-6">
              <CardHeader>
                <CardTitle className="text-black dark:text-white">Request Role Access</CardTitle>
                <CardDescription>Apply for merchant, affiliate, proposer, improver, or issuer status</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleRoleRequest} className="space-y-4">
                  <div className="space-y-2">
                    <Label className="text-black dark:text-white">Role Type</Label>
                    <Select value={roleRequestType} onValueChange={(v) => { setRoleRequestType(v as RoleRequestType); setRoleError(""); setRoleSuccess("") }}>
                      <SelectTrigger className="text-black dark:text-white bg-secondary">
                        <SelectValue placeholder="Select a role to request..." />
                      </SelectTrigger>
                      <SelectContent>
                        {(merchantStatus === "none" || merchantStatus === "rejected") && <SelectItem value="merchant">Merchant — accept SFLuv as payment at your business</SelectItem>}
                        {(affiliateStatus === "none" || affiliateStatus === "rejected") && <SelectItem value="affiliate">Affiliate — create funded community events</SelectItem>}
                        {(proposerStatus === "none" || proposerStatus === "rejected") && <SelectItem value="proposer">Proposer — build and submit community workflows</SelectItem>}
                        {(improverStatus === "none" || improverStatus === "rejected") && <SelectItem value="improver">Improver — claim and complete workflow steps</SelectItem>}
                        {(issuerStatus === "none" || issuerStatus === "rejected") && <SelectItem value="issuer">Issuer — issue credentials to community members</SelectItem>}
                      </SelectContent>
                    </Select>
                  </div>

                  {(roleRequestType === "affiliate" || roleRequestType === "proposer" || roleRequestType === "issuer") && (
                    <div className="space-y-2">
                      <Label htmlFor="role-org" className="text-black dark:text-white">Organization Name</Label>
                      <Input id="role-org" value={roleOrg} onChange={(e) => setRoleOrg(e.target.value)} className="text-black dark:text-white bg-secondary" placeholder="Organization or group name" />
                    </div>
                  )}

                  {(roleRequestType === "proposer" || roleRequestType === "issuer") && (
                    <div className="space-y-2">
                      <Label htmlFor="role-email" className="text-black dark:text-white">Notification Email</Label>
                      {verifiedEmailOptions.length > 0 ? (
                        <Select value={roleEmail} onValueChange={setRoleEmail}>
                          <SelectTrigger id="role-email" className="text-black dark:text-white bg-secondary">
                            <SelectValue placeholder="Select a verified email" />
                          </SelectTrigger>
                          <SelectContent>
                            {verifiedEmailOptions.map((entry) => (
                              <SelectItem key={entry.id} value={entry.email}>
                                {entry.email}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <div className="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300 space-y-2">
                          <p>No verified emails available.</p>
                          <Button type="button" size="sm" variant="outline" onClick={() => setActiveTab("account")}>
                            Go to Account Emails
                          </Button>
                        </div>
                      )}
                    </div>
                  )}

                  {roleRequestType === "improver" && (
                    <>
                      <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-2">
                          <Label htmlFor="role-first-name" className="text-black dark:text-white">First Name</Label>
                          <Input id="role-first-name" value={roleFirstName} onChange={(e) => setRoleFirstName(e.target.value)} className="text-black dark:text-white bg-secondary" />
                        </div>
                        <div className="space-y-2">
                          <Label htmlFor="role-last-name" className="text-black dark:text-white">Last Name</Label>
                          <Input id="role-last-name" value={roleLastName} onChange={(e) => setRoleLastName(e.target.value)} className="text-black dark:text-white bg-secondary" />
                        </div>
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="role-email-improver" className="text-black dark:text-white">Email</Label>
                        {verifiedEmailOptions.length > 0 ? (
                          <Select value={roleEmail} onValueChange={setRoleEmail}>
                            <SelectTrigger id="role-email-improver" className="text-black dark:text-white bg-secondary">
                              <SelectValue placeholder="Select a verified email" />
                            </SelectTrigger>
                            <SelectContent>
                              {verifiedEmailOptions.map((entry) => (
                                <SelectItem key={entry.id} value={entry.email}>
                                  {entry.email}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        ) : (
                          <div className="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300 space-y-2">
                            <p>No verified emails available.</p>
                            <Button type="button" size="sm" variant="outline" onClick={() => setActiveTab("account")}>
                              Go to Account Emails
                            </Button>
                          </div>
                        )}
                      </div>
                    </>
                  )}

                  {roleError && (
                    <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                      <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                      <span>{roleError}</span>
                    </div>
                  )}

                  {roleSuccess && <p className="text-sm text-green-600">{roleSuccess}</p>}

                  <div className="flex justify-end">
                    <Button type="submit" disabled={roleSubmitting || !roleRequestType}>
                      {roleSubmitting ? (
                        <><Loader2 className="mr-2 h-4 w-4 animate-spin" />Submitting...</>
                      ) : roleRequestType === "merchant" ? (
                        "Continue to Application"
                      ) : (
                        "Submit Request"
                      )}
                    </Button>
                  </div>
                </form>
              </CardContent>
            </Card>
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

        {merchantStatus !== "none" && (
          <TabsContent value="merchant" className="space-y-6">
            <div className="space-y-2">
              {sortedUserLocations.map((loc) => {
                const applicationStatus = getLocationApplicationStatus(loc.approval)
                const borderClass = applicationStatus === "approved" ? "border-green-300 dark:border-green-700" : applicationStatus === "rejected" ? "border-red-300 dark:border-red-700" : "border-yellow-300 dark:border-yellow-700"
                const headerClass = applicationStatus === "approved" ? "bg-green-50 dark:bg-green-900/20 rounded-t-lg" : applicationStatus === "rejected" ? "bg-red-50 dark:bg-red-900/20 rounded-t-lg" : "bg-yellow-50 dark:bg-yellow-900/20 rounded-t-lg"
                const Icon = applicationStatus === "approved" ? Check : applicationStatus === "rejected" ? XCircle : Clock
                const iconClass = applicationStatus === "approved" ? "h-5 w-5 text-green-500 mr-2" : applicationStatus === "rejected" ? "h-5 w-5 text-red-500 mr-2" : "h-5 w-5 text-yellow-500 mr-2"
                const statusTitle = applicationStatus === "approved" ? "Location Application Approved" : applicationStatus === "rejected" ? "Location Application Not Approved" : "Location Application Pending"
                const statusBody = applicationStatus === "approved" ? `Your application for ${loc.name} has been approved!` : applicationStatus === "rejected" ? `Your application for ${loc.name} was not approved.` : `Your application for ${loc.name} is currently under review.`
                return (
                  <Card className={borderClass} key={loc.id}>
                    <CardHeader className={headerClass}>
                      <CardTitle className="text-black dark:text-white flex items-center">
                        <Icon className={iconClass} />
                        {statusTitle}
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="pt-4">
                      <p className="text-gray-600 dark:text-gray-400">{statusBody}</p>
                    </CardContent>
                  </Card>
                )
              })}
            </div>
            {merchantStatus === "approved" && <Card>
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
            </Card>}
          </TabsContent>
        )}

        {(affiliateStatus === "pending" || affiliateStatus === "approved") && (
          <TabsContent value="affiliate">
            <Card className={affiliateStatus === "approved" ? "border-green-300 dark:border-green-700" : "border-yellow-300 dark:border-yellow-700"}>
              <CardHeader className={`rounded-t-lg ${affiliateStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}>
                <CardTitle className="text-black dark:text-white flex items-center">
                  {affiliateStatus === "approved" ? <Check className="h-5 w-5 text-green-500 mr-2" /> : <Clock className="h-5 w-5 text-yellow-500 mr-2" />}
                  Affiliate {affiliateStatus === "approved" ? "Status Approved" : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {affiliateStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">Your affiliate request for {affiliate?.organization || "your organization"} is under review.</p>
                )}
                {affiliateStatus === "approved" && (
                  <div className="space-y-4">
                    <p className="text-gray-600 dark:text-gray-400">You are approved to create affiliate events for {affiliate?.organization || "your organization"}.</p>
                    <div className="space-y-3">
                      <Label className="text-black dark:text-white">Affiliate Logo</Label>
                      <div className="flex items-center gap-4">
                        <div className="h-16 w-16 rounded-xl bg-secondary border border-muted flex items-center justify-center overflow-hidden">
                          {affiliateLogoPreview ? (
                            <img src={affiliateLogoPreview} alt="Affiliate logo" className="h-full w-full object-contain" />
                          ) : (
                            <span className="text-xs text-muted-foreground">No logo</span>
                          )}
                        </div>
                        <div className="space-y-2">
                          <Input type="file" accept="image/*" onChange={handleAffiliateLogoChange} className="text-black dark:text-white bg-secondary" />
                          <Button type="button" variant="outline" className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white" disabled={!affiliateLogoPreview || affiliateLogoSaving} onClick={handleAffiliateLogoSave}>
                            {affiliateLogoSaving ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" />Saving...</> : "Save Logo"}
                          </Button>
                        </div>
                      </div>
                      {affiliateLogoError && (
                        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                          <span>{affiliateLogoError}</span>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {(proposerStatus === "pending" || proposerStatus === "approved") && (
          <TabsContent value="proposer">
            <Card className={proposerStatus === "approved" ? "border-green-300 dark:border-green-700" : "border-yellow-300 dark:border-yellow-700"}>
              <CardHeader className={`rounded-t-lg ${proposerStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}>
                <CardTitle className="text-black dark:text-white flex items-center">
                  {proposerStatus === "approved" ? <Check className="h-5 w-5 text-green-500 mr-2" /> : <Clock className="h-5 w-5 text-yellow-500 mr-2" />}
                  Proposer {proposerStatus === "approved" ? "Status Approved" : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {proposerStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">Your proposer request for {proposer?.organization || "your organization"} is under review.</p>
                )}
                {proposerStatus === "approved" && (
                  <>
                    <p className="text-gray-600 dark:text-gray-400 mb-4">You are approved to create workflows for {proposer?.organization || "your organization"}.</p>
                    <Button variant="outline" className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white" onClick={() => router.push("/proposer")}>Open Proposer Panel</Button>
                  </>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {(improverStatus === "pending" || improverStatus === "approved") && (
          <TabsContent value="improver">
            <Card className={improverStatus === "approved" ? "border-green-300 dark:border-green-700" : "border-yellow-300 dark:border-yellow-700"}>
              <CardHeader className={`rounded-t-lg ${improverStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}>
                <CardTitle className="text-black dark:text-white flex items-center">
                  {improverStatus === "approved" ? <Check className="h-5 w-5 text-green-500 mr-2" /> : <Clock className="h-5 w-5 text-yellow-500 mr-2" />}
                  Improver {improverStatus === "approved" ? "Status Approved" : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {improverStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">Your improver request is under review.</p>
                )}
                {improverStatus === "approved" && (
                  <>
                    <p className="text-gray-600 dark:text-gray-400 mb-4">You are approved as an improver and can now claim eligible workflow steps.</p>
                    <Button variant="outline" className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white" onClick={() => router.push("/improver")}>Open Improver Panel</Button>
                  </>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {(issuerStatus === "pending" || issuerStatus === "approved") && (
          <TabsContent value="issuer">
            <Card className={issuerStatus === "approved" ? "border-green-300 dark:border-green-700" : "border-yellow-300 dark:border-yellow-700"}>
              <CardHeader className={`rounded-t-lg ${issuerStatus === "approved" ? "bg-green-50 dark:bg-green-900/20" : "bg-yellow-50 dark:bg-yellow-900/20"}`}>
                <CardTitle className="text-black dark:text-white flex items-center">
                  {issuerStatus === "approved" ? <Check className="h-5 w-5 text-green-500 mr-2" /> : <Clock className="h-5 w-5 text-yellow-500 mr-2" />}
                  Issuer {issuerStatus === "approved" ? "Status Approved" : "Request Pending"}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                {issuerStatus === "pending" && (
                  <p className="text-gray-600 dark:text-gray-400">Your issuer request for {issuer?.organization || "your organization"} is under review.</p>
                )}
                {issuerStatus === "approved" && (
                  <p className="text-gray-600 dark:text-gray-400">You are approved to issue credentials on behalf of {issuer?.organization || "your organization"}.</p>
                )}
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
