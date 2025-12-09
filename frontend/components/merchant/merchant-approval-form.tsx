"use client"

import type React from "react"
import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { AlertTriangle, Loader2 } from "lucide-react"
import PlaceAutocomplete from "./google_place_finder"
import { Location, GoogleSubLocation, AuthedLocation } from "@/types/location"
import { useLocation } from "@/context/LocationProvider"


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

const tippingDivisionOptions = [
  "Each member receives their own tips",
  "All tips are pooled and divided between the team",
  "Other"
]

const serviceStationOptions = [
  "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10"
]

const messagingServiceOptions = [
  "Zapier",
  "Google messaging",
  "We do not currently use a messaging service",
  "I'm not sure",
  "Other",
]


export function MerchantApprovalForm() {
  const { addLocation } = useLocation();
  const router = useRouter()
  // Internal Form State
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null);

  // User-inputted state
  const [description, setDescription] = useState("")
  const [primaryContactEmail, setPrimaryContactEmail] = useState("")
  const [primaryContactFirstName, setPrimaryContactFirstName] = useState("")
  const [primaryContactLastName, setPrimaryContactLastName] = useState("")
  const [primaryContactPhone, setPrimaryContactPhone] = useState("")
  const [posSystem, setPosSystem] = useState("")
  const [posSystemOther, setPosSystemOther] = useState("")
  const [soleProprietorship, setSoleProprietorship] = useState("")
  const [tippingPolicy, setTippingPolicy] = useState("")
  const [tippingPolicyOther, setTippingPolicyOther] = useState("")
  const [tippingDivision, setTippingDivision] = useState("")
  const [tippingDivisionOther, setTippingDivisionOther] = useState("")
  const [tableCoverage, setTableCoverage] = useState("")
  const [tableCoverageOther, setTableCoverageOther] = useState("")
  const [serviceStations, setServiceStations] = useState("")
  const [tabletModel, setTabletModel] = useState("")
  const [tabletModelOther, setTabletModelOther] = useState("")
  const [messagingService, setMessagingService] = useState("")
  const [messagingServiceOther, setMessagingServiceOther] = useState("")
  const [googleSubLocation, setGoogleSubLocation] = useState<GoogleSubLocation | null>(null);
  const [reference, setReference] = useState("")
  const [searchKey, setSearchKey] = useState(0);


  // State pulled from Google
  const [googleID, setGoogleID] = useState("")
  const [businessName, setBusinessName] = useState("")
  const [businessType, setBusinessType] = useState("")
  const [street, setStreet] = useState("")
  const [city, setCity] = useState("")
  const [state, setState] = useState("")
  const [lat, setLat] = useState(0)
  const [lng, setLng] = useState(0)
  const [zip, setZip] = useState("")
  const [businessPhone, setBusinessPhone] = useState("")
  const [businessEmail, setBusinessEmail] = useState("")
  const [imageURL, setImageURL] = useState("")
  const [rating, setRating] = useState(0)
  const [googleMapsURL, setGoogleMapsURL] = useState("")
  const [openingHours, setOpeningHours] = useState([])

  useEffect(() => {
    if(googleSubLocation) setError(null)
  }, [googleSubLocation])

  const resetForm = () => {
    setError(null);
    setDescription("");
    setStreet("");
    setPrimaryContactEmail("");
    setPrimaryContactFirstName("");
    setPrimaryContactLastName("");
    setPrimaryContactPhone("");
    setBusinessEmail("")
    setBusinessPhone("")
    setPosSystem("");
    setPosSystemOther("");
    setSoleProprietorship("");
    setTippingPolicy("");
    setTippingPolicyOther("");
    setTippingDivision("");
    setTippingDivisionOther("");
    setTableCoverage("");
    setTableCoverageOther("");
    setServiceStations("");
    setTabletModel("");
    setTabletModelOther("");
    setMessagingService("");
    setMessagingServiceOther("");
    setReference("")
    setGoogleSubLocation(null)
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if(!googleSubLocation) {
      setError('"Search for your business" field must be filled.')
      return
    }

    // Create merchant profile with all fields
    const newLocation: AuthedLocation = {
      id: 0,
      google_id: googleSubLocation.google_id,
      owner_id: "",
      name: googleSubLocation.name,
      description: description,
      type: googleSubLocation.type,
      street: street,
      city: googleSubLocation.city,
      state: googleSubLocation.state,
      zip: googleSubLocation.zip,
      lat: googleSubLocation.lat,
      lng: googleSubLocation.lng,
      phone:businessPhone,
      email: businessEmail,
      admin_phone: primaryContactPhone,
      admin_email: primaryContactEmail,
      website: googleSubLocation.website,
      image_url: googleSubLocation.image_url,
      rating: googleSubLocation.rating,
      maps_page: googleSubLocation.maps_page,
      opening_hours: googleSubLocation.opening_hours,
      contact_firstname: primaryContactFirstName,
      contact_lastname: primaryContactLastName,
      contact_phone: primaryContactPhone,
      pos_system: posSystem === "Other" ? posSystemOther : posSystem,
      sole_proprietorship: soleProprietorship,
      tipping_policy: tippingPolicy === "Other" ? tippingPolicyOther : tippingPolicy,
      tipping_division: tippingDivision  === "Other" ? tippingDivisionOther: tippingDivision,
      table_coverage: tableCoverage  === "Other" ? tableCoverageOther : tableCoverage,
      service_stations: Number(serviceStations),
      tablet_model: tabletModel  === "Other" ? tabletModelOther : tabletModel,
      messaging_service: messagingService === "Other" ? messagingServiceOther : messagingService,
      reference: reference,
    }

    setIsSubmitting(true)
    await addLocation(newLocation)
    setSearchKey(prev => prev + 1)
    setIsSubmitting(false);
    resetForm()
    router.replace("/map")
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
                Search for Your Location Name
              </Label>
              <PlaceAutocomplete
              key={searchKey}
              setGoogleSubLocation={setGoogleSubLocation}
              setBusinessPhone={setBusinessPhone}
              setStreet={setStreet}/>
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

            {/* Business Contact Information */}
            <div>
              <h3 className="text-lg font-medium text-black dark:text-white mb-4">Business Contact Information (Publicly Available to Customers)</h3>
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="business-phone" className="text-black dark:text-white">
                    Business Phone Number
                  </Label>
                  <Input
                    id="business-phone"
                    value={businessPhone}
                    onChange={(e) => setBusinessPhone(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="business-email" className="text-black dark:text-white">
                    Business Email
                  </Label>
                  <Input
                    id="business-email"
                    value={businessEmail}
                    onChange={(e) => setBusinessEmail(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                  />
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="street" className="text-black dark:text-white">
                Street Address
              </Label>
              <Input
                id="street"
                value={street}
                onChange={(e) => setDescription(e.target.value)}
                className="text-black dark:text-white bg-secondary"
                required
              />
            </div>

            {/* Primary Contact Information */}
            <div>
              <h3 className="text-lg font-medium text-black dark:text-white mb-4">Primary Administrative Contact for Business (Only Visible to SFLuv Admin Team)</h3>
              <div className="grid gap-4 md:grid-cols-2">
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

                <div className="space-y-2">
                  <Label htmlFor="primary-contact-email" className="text-black dark:text-white">
                    Email
                  </Label>
                  <Input
                    id="primary-contact-email"
                    value={primaryContactEmail}
                    onChange={(e) => setPrimaryContactEmail(e.target.value)}
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

            <div className="space-y-2">
              <Label htmlFor="tablet-model" className="text-black dark:text-white">
                How are tips divided amongst staff members?
              </Label>
              <Select value={tippingDivision} onValueChange={setTippingDivision} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select tip divison style" />
                </SelectTrigger>
                <SelectContent>
                  {tippingDivisionOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {tippingDivision === "Other" && (
                <div className="space-y-2 mt-2">
                  <Label htmlFor="tipping-division-other" className="text-black dark:text-white">
                    Division of Tips
                  </Label>
                  <Input
                    id="tipping-division-other"
                    value={tippingDivisionOther}
                    onChange={(e) => setTippingDivisionOther(e.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Specify your tip divison style"
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
              <Select value={serviceStations} onValueChange={setServiceStations} required>
                <SelectTrigger className="text-black dark:text-white bg-secondary">
                  <SelectValue placeholder="Select # of service stations" />
                </SelectTrigger>
                <SelectContent>
                  {serviceStationOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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

            <div className="space-y-2">
              <Label htmlFor="reference" className="text-black dark:text-white">
                How did you hear about SFLuv?
              </Label>
              <Textarea
                id="reference"
                value={reference}
                onChange={(e) => setReference(e.target.value)}
                className="text-black dark:text-white bg-secondary min-h-[100px]"
                required
              />
            </div>
          </div>
          {error &&
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{error}</span>
            </div>
          }
          <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isSubmitting}>
            {isSubmitting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
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
