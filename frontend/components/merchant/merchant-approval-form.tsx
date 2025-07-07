"use client"

import type React from "react"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/app-context"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Loader2 } from "lucide-react"

const businessTypes = [
  "restaurant",
  "cafe",
  "retail",
  "grocery",
  "service",
  "entertainment",
  "health",
  "beauty",
  "other",
]

const posOptions = ["Square", "Shopify", "Toast", "Other"]

const soleProprietorshipOptions = ["Yes", "No", "Not sure"]

const tippingOptions = [
  "Tips are included automatically (service charge or gratuity added to bill)",
  "Customers leave tips at their discretion",
  "Both (depends on party size or situation)",
  "N/A - our employees do not receive tips",
  "Other",
]

const tableCoverageOptions = [
  "Servers are assigned to specific sections",
  "Table coverage is managed differently (e.g. rotating, team service, etc.)",
  "Both (depends on shift, staffing, or other factors)",
  "Other",
]

const tabletOptions = ["iPad", "Android tablet", "We do not have a tablet accessible near register", "Other"]

const messagingServiceOptions = [
  "Zapier",
  "Google messaging",
  "We do not currently use a messaging service",
  "I'm not sure",
  "Other",
]

export function MerchantApprovalForm() {
  const router = useRouter()
  const { requestMerchantStatus } = useApp()
  const [isSubmitting, setIsSubmitting] = useState(false)

  // Existing form state
  const [businessName, setBusinessName] = useState("")
  const [description, setDescription] = useState("")
  const [businessType, setBusinessType] = useState("")
  const [businessTypeOther, setBusinessTypeOther] = useState("")
  const [street, setStreet] = useState("")
  const [city, setCity] = useState("")
  const [state, setState] = useState("")
  const [zip, setZip] = useState("")
  const [phone, setPhone] = useState("")
  const [website, setWebsite] = useState("")

  // New form state
  const [primaryContactFirstName, setPrimaryContactFirstName] = useState("")
  const [primaryContactLastName, setPrimaryContactLastName] = useState("")
  const [primaryContactPhone, setPrimaryContactPhone] = useState("")
  const [posSystem, setPosSystem] = useState("")
  const [posSystemOther, setPosSystemOther] = useState("")
  const [soleProprietorship, setSoleProprietorship] = useState("")
  const [tippingPolicy, setTippingPolicy] = useState("")
  const [tippingPolicyOther, setTippingPolicyOther] = useState("")
  const [tableCoverage, setTableCoverage] = useState("")
  const [tableCoverageOther, setTableCoverageOther] = useState("")
  const [tablesOrStations, setTablesOrStations] = useState("")
  const [tabletModel, setTabletModel] = useState("")
  const [tabletModelOther, setTabletModelOther] = useState("")
  const [messagingService, setMessagingService] = useState("")
  const [messagingServiceOther, setMessagingServiceOther] = useState("")

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setIsSubmitting(true)

    // Create merchant profile with all fields
    const merchantProfile = {
      businessName,
      description,
      businessType: businessType === "other" ? businessTypeOther : businessType,
      address: {
        street,
        city,
        state,
        zip,
      },
      contactInfo: {
        phone,
        website: website || undefined,
      },
      primaryContact: {
        firstName: primaryContactFirstName,
        lastName: primaryContactLastName,
        phone: primaryContactPhone,
      },
      posSystem: posSystem === "Other" ? posSystemOther : posSystem,
      soleProprietorship,
      tippingPolicy: tippingPolicy === "Other" ? tippingPolicyOther : tippingPolicy,
      tableCoverage: tableCoverage === "Other" ? tableCoverageOther : tableCoverage,
      tablesOrStations,
      tabletModel: tabletModel === "Other" ? tabletModelOther : tabletModel,
      messagingService: messagingService === "Other" ? messagingServiceOther : messagingService,
    }

    // Submit merchant approval request
    setTimeout(() => {
      requestMerchantStatus(merchantProfile)
      setIsSubmitting(false)
      router.push("/dashboard/merchant-status")
    }, 1500)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-black dark:text-white">Merchant Application</CardTitle>
        <CardDescription>Please provide your business details to apply for merchant status</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-6">
          <div className="space-y-4">
            {/* Existing Business Information */}
            <div className="space-y-2">
              <Label htmlFor="business-name" className="text-black dark:text-white">
                Business Name
              </Label>
              <Input
                id="business-name"
                value={businessName}
                onChange={(e) => setBusinessName(e.target.value)}
                className="text-black dark:text-white bg-secondary"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="business-type" className="text-black dark:text-white">
                Business Type
              </Label>
              <Select value={businessType} onValueChange={setBusinessType} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select business type" />
                </SelectTrigger>
                <SelectContent>
                  {businessTypes.map((type) => (
                    <SelectItem key={type} value={type}>
                      {type.charAt(0).toUpperCase() + type.slice(1)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {businessType === "other" && (
                <div className="space-y-2 mt-2">
                  <Label htmlFor="business-type-other" className="text-black dark:text-white">
                    Business Type
                  </Label>
                  <Input
                    id="business-type-other"
                    value={businessTypeOther}
                    onChange={(e) => setBusinessTypeOther(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Specify your business type"
                  />
                </div>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="description" className="text-black dark:text-white">
                Business Description
              </Label>
              <Textarea
                id="description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                className="text-black dark:text-white bg-secondary min-h-[100px]"
                required
              />
            </div>

            {/* Business Address */}
            <div>
              <h3 className="text-lg font-medium text-black dark:text-white mb-4">Business Address</h3>
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="street" className="text-black dark:text-white">
                    Street Address
                  </Label>
                  <Input
                    id="street"
                    value={street}
                    onChange={(e) => setStreet(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="city" className="text-black dark:text-white">
                    City
                  </Label>
                  <Input
                    id="city"
                    value={city}
                    onChange={(e) => setCity(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="state" className="text-black dark:text-white">
                    State
                  </Label>
                  <Input
                    id="state"
                    value={state}
                    onChange={(e) => setState(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="zip" className="text-black dark:text-white">
                    ZIP Code
                  </Label>
                  <Input
                    id="zip"
                    value={zip}
                    onChange={(e) => setZip(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    required
                  />
                </div>
              </div>
            </div>

            {/* Business Contact Information */}
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="phone" className="text-black dark:text-white">
                  Business Phone
                </Label>
                <Input
                  id="phone"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  className="text-black dark:text-white bg-secondary"
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="website" className="text-black dark:text-white">
                  Business Website (Optional)
                </Label>
                <Input
                  id="website"
                  value={website}
                  onChange={(e) => setWebsite(e.target.value)}
                  className="text-black dark:text-white bg-secondary"
                />
              </div>
            </div>

            {/* Primary Contact Information */}
            <div>
              <h3 className="text-lg font-medium text-black dark:text-white mb-4">Primary Contact for Business</h3>
              <div className="grid gap-4 md:grid-cols-3">
                <div className="space-y-2">
                  <Label htmlFor="primary-contact-first-name" className="text-black dark:text-white">
                    First Name
                  </Label>
                  <Input
                    id="primary-contact-first-name"
                    value={primaryContactFirstName}
                    onChange={(e) => setPrimaryContactFirstName(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="primary-contact-last-name" className="text-black dark:text-white">
                    Last Name
                  </Label>
                  <Input
                    id="primary-contact-last-name"
                    value={primaryContactLastName}
                    onChange={(e) => setPrimaryContactLastName(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    required
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="primary-contact-phone" className="text-black dark:text-white">
                    Phone Number
                  </Label>
                  <Input
                    id="primary-contact-phone"
                    value={primaryContactPhone}
                    onChange={(e) => setPrimaryContactPhone(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    required
                  />
                </div>
              </div>
            </div>

            {/* Point of Sale System */}
            <div className="space-y-2">
              <Label htmlFor="pos-system" className="text-black dark:text-white">
                What Point of Sale System do you use?
              </Label>
              <Select value={posSystem} onValueChange={setPosSystem} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select POS system" />
                </SelectTrigger>
                <SelectContent>
                  {posOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {posSystem === "Other" && (
                <div className="space-y-2 mt-2">
                  <Label htmlFor="pos-system-other" className="text-black dark:text-white">
                    Point of Sale System
                  </Label>
                  <Input
                    id="pos-system-other"
                    value={posSystemOther}
                    onChange={(e) => setPosSystemOther(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Specify your POS system"
                  />
                </div>
              )}
            </div>

            {/* Sole Proprietorship */}
            <div className="space-y-2">
              <Label htmlFor="sole-proprietorship" className="text-black dark:text-white">
                Is your business a sole proprietorship?
              </Label>
              <Select value={soleProprietorship} onValueChange={setSoleProprietorship} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select option" />
                </SelectTrigger>
                <SelectContent>
                  {soleProprietorshipOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Tipping Policy */}
            <div className="space-y-2">
              <Label htmlFor="tipping-policy" className="text-black dark:text-white">
                Do you add tips to the bill automatically, or do customers tip at their discretion?
              </Label>
              <Select value={tippingPolicy} onValueChange={setTippingPolicy} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select tipping policy" />
                </SelectTrigger>
                <SelectContent>
                  {tippingOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {tippingPolicy === "Other" && (
                <div className="space-y-2 mt-2">
                  <Label htmlFor="tipping-policy-other" className="text-black dark:text-white">
                    Tipping Policy
                  </Label>
                  <Input
                    id="tipping-policy-other"
                    value={tippingPolicyOther}
                    onChange={(e) => setTippingPolicyOther(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Specify your tipping policy"
                  />
                </div>
              )}
            </div>

            {/* Table Coverage */}
            <div className="space-y-2">
              <Label htmlFor="table-coverage" className="text-black dark:text-white">
                Are servers assigned to specific sections, or is table coverage managed differently?
              </Label>
              <Select value={tableCoverage} onValueChange={setTableCoverage} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select table coverage method" />
                </SelectTrigger>
                <SelectContent>
                  {tableCoverageOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {tableCoverage === "Other" && (
                <div className="space-y-2 mt-2">
                  <Label htmlFor="table-coverage-other" className="text-black dark:text-white">
                    Table Coverage Method
                  </Label>
                  <Input
                    id="table-coverage-other"
                    value={tableCoverageOther}
                    onChange={(e) => setTableCoverageOther(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Specify your table coverage method"
                  />
                </div>
              )}
            </div>

            {/* Tables or Service Stations */}
            <div className="space-y-2">
              <Label htmlFor="tables-or-stations" className="text-black dark:text-white">
                How many tables or service stations do you have?
              </Label>
              <Input
                id="tables-or-stations"
                value={tablesOrStations}
                onChange={(e) => setTablesOrStations(e.target.value)}
                className="text-black dark:text-white bg-secondary"
                placeholder="Enter number of tables or service stations"
                required
              />
            </div>

            {/* Tablet Model */}
            <div className="space-y-2">
              <Label htmlFor="tablet-model" className="text-black dark:text-white">
                If you currently have a tablet or similar device available for servers near the register, please specify
                which model they use:
              </Label>
              <Select value={tabletModel} onValueChange={setTabletModel} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select tablet model" />
                </SelectTrigger>
                <SelectContent>
                  {tabletOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {tabletModel === "Other" && (
                <div className="space-y-2 mt-2">
                  <Label htmlFor="tablet-model-other" className="text-black dark:text-white">
                    Tablet Model
                  </Label>
                  <Input
                    id="tablet-model-other"
                    value={tabletModelOther}
                    onChange={(e) => setTabletModelOther(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Specify your tablet model"
                  />
                </div>
              )}
            </div>

            {/* Messaging Service */}
            <div className="space-y-2">
              <Label htmlFor="messaging-service" className="text-black dark:text-white">
                If your business regularly receives notifications from a system like Zapier, what service do you use:
              </Label>
              <Select value={messagingService} onValueChange={setMessagingService} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select messaging service" />
                </SelectTrigger>
                <SelectContent>
                  {messagingServiceOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {messagingService === "Other" && (
                <div className="space-y-2 mt-2">
                  <Label htmlFor="messaging-service-other" className="text-black dark:text-white">
                    Messaging Service
                  </Label>
                  <Input
                    id="messaging-service-other"
                    value={messagingServiceOther}
                    onChange={(e) => setMessagingServiceOther(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Specify your messaging service"
                  />
                </div>
              )}
            </div>
          </div>

          <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isSubmitting}>
            {isSubmitting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Submitting Application...
              </>
            ) : (
              "Submit Merchant Application"
            )}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
