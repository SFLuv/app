"use client"

import type React from "react"

import { useState, useEffect } from "react"
import { useParams, useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Calendar, Clock, MapPin, Users, Mail, ArrowLeft, Save, Trash } from "lucide-react"
import { format } from "date-fns"
import { useApp } from "@/context/app-context"
import { redirect } from "next/navigation"
import { useOpportunities } from "@/hooks/api/use-opportunities"
import { useRegisteredVolunteers } from "@/hooks/api/use-registered-volunteers"

export default function OpportunityDetailPage() {
  const params = useParams()
  const router = useRouter()
  const { user } = useApp()
  const opportunityId = params.id as string
  const [activeTab, setActiveTab] = useState("details")
  const [isEditing, setIsEditing] = useState(false)
  const [formData, setFormData] = useState({
    title: "",
    description: "",
    date: "",
    time: "",
    location: "",
    rewardAmount: 0,
    volunteersNeeded: 0,
  })

  // Use our custom hooks
  const {
    getOpportunityById,
    updateOpportunity,
    deleteOpportunity,
    isLoading: isLoadingOpportunity,
    error: opportunityError,
  } = useOpportunities()

  const { volunteers, isLoading: isLoadingVolunteers, error: volunteersError } = useRegisteredVolunteers(opportunityId)

  // Get opportunity data
  const opportunity = getOpportunityById(opportunityId)

  // Redirect if not an organizer
  if (!user?.isOrganizer) {
    redirect("/dashboard")
  }

  // Redirect if opportunity not found
  useEffect(() => {
    if (!isLoadingOpportunity && !opportunity) {
      router.push("/dashboard/your-opportunities")
    }
  }, [isLoadingOpportunity, opportunity, router])

  // Initialize form data when opportunity is loaded
  useEffect(() => {
    if (opportunity) {
      const date = new Date(opportunity.date)
      setFormData({
        title: opportunity.title,
        description: opportunity.description,
        date: format(date, "yyyy-MM-dd"),
        time: format(date, "HH:mm"),
        location: `${opportunity.location.address}, ${opportunity.location.city}, ${opportunity.location.state} ${opportunity.location.zip}`,
        rewardAmount: opportunity.rewardAmount,
        volunteersNeeded: opportunity.volunteersNeeded,
      })
    }
  }, [opportunity])

  // Handle form input changes
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target
    setFormData((prev) => ({
      ...prev,
      [name]: name === "rewardAmount" || name === "volunteersNeeded" ? Number.parseInt(value) || 0 : value,
    }))
  }

  // Handle save changes
  const handleSaveChanges = async () => {
    if (!opportunity) return

    try {
      // Combine date and time
      const dateTime = new Date(`${formData.date}T${formData.time}`)

      // Update opportunity
      await updateOpportunity(opportunityId, {
        title: formData.title,
        description: formData.description,
        date: dateTime.toISOString(),
        rewardAmount: formData.rewardAmount,
        volunteersNeeded: formData.volunteersNeeded,
      })

      setIsEditing(false)
    } catch (err) {
      console.error("Failed to update opportunity:", err)
    }
  }

  // Handle delete opportunity
  const handleDeleteOpportunity = async () => {
    if (!opportunity) return

    if (window.confirm("Are you sure you want to delete this opportunity? This action cannot be undone.")) {
      try {
        await deleteOpportunity(opportunityId)
        router.push("/dashboard/your-opportunities")
      } catch (err) {
        console.error("Failed to delete opportunity:", err)
      }
    }
  }

  // Handle send email blast
  const handleSendEmailBlast = () => {
    alert("Email blast feature will be implemented soon!")
  }

  if (isLoadingOpportunity) {
    return (
      <div className="space-y-6">
        <div className="h-10 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        <div className="h-64 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
      </div>
    )
  }

  if (opportunityError) {
    return (
      <div className="p-8 text-center text-red-500">
        <p>Error loading opportunity: {opportunityError.message}</p>
        <Button className="mt-4" onClick={() => window.location.reload()}>
          Retry
        </Button>
      </div>
    )
  }

  if (!opportunity) {
    return null // Will redirect in useEffect
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="outline" onClick={() => router.push("/dashboard/your-opportunities")}>
          <ArrowLeft className="mr-2 h-4 w-4" /> Back to Opportunities
        </Button>
        <h1 className="text-3xl font-bold text-black dark:text-white">{opportunity.title}</h1>
      </div>

      <Tabs defaultValue="details" value={activeTab} onValueChange={setActiveTab}>
        <TabsList className="grid w-full grid-cols-2 mb-6">
          <TabsTrigger value="details">Opportunity Details</TabsTrigger>
          <TabsTrigger value="volunteers">Registered Volunteers</TabsTrigger>
        </TabsList>

        <TabsContent value="details">
          <Card className="bg-white dark:bg-[#2a2a2a]">
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-black dark:text-white">Opportunity Details</CardTitle>
                <CardDescription>View and edit opportunity information</CardDescription>
              </div>
              <div className="flex gap-2">
                {isEditing ? (
                  <Button onClick={handleSaveChanges}>
                    <Save className="mr-2 h-4 w-4" /> Save Changes
                  </Button>
                ) : (
                  <Button onClick={() => setIsEditing(true)}>Edit Details</Button>
                )}
                <Button variant="destructive" onClick={handleDeleteOpportunity}>
                  <Trash className="mr-2 h-4 w-4" /> Delete
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {isEditing ? (
                <div className="space-y-4">
                  <div>
                    <Label htmlFor="title">Title</Label>
                    <Input
                      id="title"
                      name="title"
                      value={formData.title}
                      onChange={handleInputChange}
                      className="bg-secondary"
                    />
                  </div>
                  <div>
                    <Label htmlFor="description">Description</Label>
                    <Textarea
                      id="description"
                      name="description"
                      value={formData.description}
                      onChange={handleInputChange}
                      rows={5}
                      className="bg-secondary"
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <Label htmlFor="date">Date</Label>
                      <Input
                        id="date"
                        name="date"
                        type="date"
                        value={formData.date}
                        onChange={handleInputChange}
                        className="bg-secondary"
                      />
                    </div>
                    <div>
                      <Label htmlFor="time">Time</Label>
                      <Input
                        id="time"
                        name="time"
                        type="time"
                        value={formData.time}
                        onChange={handleInputChange}
                        className="bg-secondary"
                      />
                    </div>
                  </div>
                  <div>
                    <Label htmlFor="location">Location</Label>
                    <Input
                      id="location"
                      name="location"
                      value={formData.location}
                      onChange={handleInputChange}
                      className="bg-secondary"
                      disabled
                    />
                    <p className="text-xs text-gray-500 mt-1">Location editing is not available in this version</p>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <Label htmlFor="rewardAmount">Reward Amount (SFLuv)</Label>
                      <Input
                        id="rewardAmount"
                        name="rewardAmount"
                        type="number"
                        value={formData.rewardAmount}
                        onChange={handleInputChange}
                        className="bg-secondary"
                      />
                    </div>
                    <div>
                      <Label htmlFor="volunteersNeeded">Volunteers Needed</Label>
                      <Input
                        id="volunteersNeeded"
                        name="volunteersNeeded"
                        type="number"
                        value={formData.volunteersNeeded}
                        onChange={handleInputChange}
                        className="bg-secondary"
                      />
                    </div>
                  </div>
                </div>
              ) : (
                <div className="space-y-6">
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
                    <div className="flex flex-col gap-2">
                      <div className="text-sm font-medium text-gray-500 dark:text-gray-400">Date & Time</div>
                      <div className="flex items-center text-black dark:text-white">
                        <Calendar className="h-4 w-4 mr-2 text-gray-500" />
                        {format(new Date(opportunity.date), "MMMM d, yyyy")}
                      </div>
                      <div className="flex items-center text-black dark:text-white">
                        <Clock className="h-4 w-4 mr-2 text-gray-500" />
                        {format(new Date(opportunity.date), "h:mm a")}
                      </div>
                    </div>
                    <div className="flex flex-col gap-2">
                      <div className="text-sm font-medium text-gray-500 dark:text-gray-400">Location</div>
                      <div className="flex items-center text-black dark:text-white">
                        <MapPin className="h-4 w-4 mr-2 text-gray-500" />
                        {opportunity.location.address}, {opportunity.location.city}, {opportunity.location.state}{" "}
                        {opportunity.location.zip}
                      </div>
                    </div>
                    <div className="flex flex-col gap-2">
                      <div className="text-sm font-medium text-gray-500 dark:text-gray-400">Volunteers</div>
                      <div className="flex items-center text-black dark:text-white">
                        <Users className="h-4 w-4 mr-2 text-gray-500" />
                        <Badge
                          variant={
                            opportunity.volunteersSignedUp >= opportunity.volunteersNeeded
                              ? "success"
                              : opportunity.volunteersSignedUp >= opportunity.volunteersNeeded / 2
                                ? "default"
                                : "warning"
                          }
                        >
                          {opportunity.volunteersSignedUp} / {opportunity.volunteersNeeded}
                        </Badge>
                      </div>
                      <div className="flex items-center text-black dark:text-white">
                        <div className="text-sm font-medium text-gray-500 dark:text-gray-400 mr-2">Reward:</div>
                        {opportunity.rewardAmount} SFLuv
                      </div>
                    </div>
                  </div>
                  <div>
                    <div className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">Description</div>
                    <p className="text-black dark:text-white whitespace-pre-line">{opportunity.description}</p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="volunteers">
          <Card className="bg-white dark:bg-[#2a2a2a]">
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-black dark:text-white">Registered Volunteers</CardTitle>
                <CardDescription>
                  {volunteers.length} of {opportunity.volunteersNeeded} spots filled
                </CardDescription>
              </div>
              <Button onClick={handleSendEmailBlast} disabled={volunteers.length === 0}>
                <Mail className="mr-2 h-4 w-4" /> Send Email Blast
              </Button>
            </CardHeader>
            <CardContent>
              {isLoadingVolunteers ? (
                <div className="space-y-4">
                  {[...Array(3)].map((_, i) => (
                    <div key={i} className="h-16 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
                  ))}
                </div>
              ) : volunteersError ? (
                <div className="p-8 text-center text-red-500">
                  <p>Error loading volunteers: {volunteersError.message}</p>
                  <Button className="mt-4" onClick={() => window.location.reload()}>
                    Retry
                  </Button>
                </div>
              ) : volunteers.length === 0 ? (
                <div className="p-8 text-center text-gray-500 dark:text-gray-400">
                  No volunteers have registered for this opportunity yet.
                </div>
              ) : (
                <div className="space-y-4">
                  {volunteers.map((volunteer) => (
                    <div
                      key={volunteer.userId}
                      className="flex items-center justify-between p-4 border rounded-md hover:bg-gray-50 dark:hover:bg-gray-800"
                    >
                      <div className="flex items-center">
                        <Avatar className="h-10 w-10 mr-4">
                          <AvatarImage
                            src={`/abstract-geometric-shapes.png?key=vol${volunteer.userId}&height=40&width=40&query=${volunteer.name}`}
                          />
                          <AvatarFallback>{volunteer.name.substring(0, 2).toUpperCase()}</AvatarFallback>
                        </Avatar>
                        <div>
                          <div className="font-medium text-black dark:text-white">{volunteer.name}</div>
                          <div className="text-sm text-gray-500 dark:text-gray-400">{volunteer.email}</div>
                        </div>
                      </div>
                      <div className="text-sm text-gray-500 dark:text-gray-400">
                        Registered on {format(new Date(volunteer.registrationDate), "MMM d, yyyy")}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
