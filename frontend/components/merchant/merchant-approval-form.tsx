"use client"

import type React from "react"
import { useCallback, useState } from "react"
import { useRouter } from "next/navigation"
import { z } from "zod"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { AlertTriangle, Loader2 } from "lucide-react"
import PlaceAutocomplete from "./google_place_finder"
import { AuthedLocation, PlaceSelection } from "@/types/location"
import { useLocation } from "@/context/LocationProvider"

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
  "Other",
]

const serviceStationOptions = ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10"]

const messagingServiceOptions = [
  "Zapier",
  "Google messaging",
  "We do not currently use a messaging service",
  "I'm not sure",
  "Other",
]

const OTHER = "Other"

type MerchantFormValues = {
  description: string
  businessPhone: string
  businessEmail: string
  manualBusinessName: string
  manualBusinessType: string
  contactFirstName: string
  contactLastName: string
  contactPhone: string
  contactEmail: string
  posSystem: string
  posSystemOther: string
  soleProprietorship: string
  tippingPolicy: string
  tippingPolicyOther: string
  tippingDivision: string
  tippingDivisionOther: string
  tableCoverage: string
  tableCoverageOther: string
  serviceStations: string
  tabletModel: string
  tabletModelOther: string
  messagingService: string
  messagingServiceOther: string
  reference: string
}

const emptyForm: MerchantFormValues = {
  description: "",
  businessPhone: "",
  businessEmail: "",
  manualBusinessName: "",
  manualBusinessType: "",
  contactFirstName: "",
  contactLastName: "",
  contactPhone: "",
  contactEmail: "",
  posSystem: "",
  posSystemOther: "",
  soleProprietorship: "",
  tippingPolicy: "",
  tippingPolicyOther: "",
  tippingDivision: "",
  tippingDivisionOther: "",
  tableCoverage: "",
  tableCoverageOther: "",
  serviceStations: "",
  tabletModel: "",
  tabletModelOther: "",
  messagingService: "",
  messagingServiceOther: "",
  reference: "",
}

const requiredText = (label: string, max = 4000) =>
  z.string().trim().min(1, `${label} is required.`).max(max, `${label} must be ${max} characters or fewer.`)

const optionalEmail = z
  .string()
  .trim()
  .refine((value) => value === "" || z.string().email().safeParse(value).success, "Enter a valid business email.")

// Radix Select is not a native <select>, so the `required` attribute has no
// effect on it — every dropdown on this form was silently optional. This schema
// is the only thing enforcing them.
const merchantFormSchema = z
  .object({
    description: requiredText("Business description"),
    businessPhone: z.string().trim().max(64, "Business phone is too long."),
    businessEmail: optionalEmail,
    // Required only on the manual path; enforced in handleSubmit, which is the
    // only place that knows which path the merchant took.
    manualBusinessName: z.string().trim().max(512, "Business name must be 512 characters or fewer."),
    manualBusinessType: z.string().trim().max(512, "Business category must be 512 characters or fewer."),
    contactFirstName: requiredText("First name", 128),
    contactLastName: requiredText("Last name", 128),
    contactPhone: requiredText("Phone number", 64),
    contactEmail: requiredText("Email", 320).email("Enter a valid email address."),
    posSystem: requiredText("Point of Sale system"),
    posSystemOther: z.string().trim(),
    soleProprietorship: requiredText("Sole proprietorship answer"),
    tippingPolicy: requiredText("Tipping policy"),
    tippingPolicyOther: z.string().trim(),
    tippingDivision: requiredText("Tip division style"),
    tippingDivisionOther: z.string().trim(),
    tableCoverage: requiredText("Table coverage method"),
    tableCoverageOther: z.string().trim(),
    serviceStations: requiredText("Number of tables or service stations"),
    tabletModel: requiredText("Tablet model"),
    tabletModelOther: z.string().trim(),
    messagingService: requiredText("Messaging service"),
    messagingServiceOther: z.string().trim(),
    reference: requiredText("How you heard about SFLuv"),
  })
  .superRefine((values, ctx) => {
    const otherPairs: [keyof MerchantFormValues, keyof MerchantFormValues, string][] = [
      ["posSystem", "posSystemOther", "Specify your Point of Sale system."],
      ["tippingPolicy", "tippingPolicyOther", "Specify your tipping policy."],
      ["tippingDivision", "tippingDivisionOther", "Specify your tip division style."],
      ["tableCoverage", "tableCoverageOther", "Specify your table coverage method."],
      ["tabletModel", "tabletModelOther", "Specify your tablet model."],
      ["messagingService", "messagingServiceOther", "Specify your messaging service."],
    ]

    for (const [selectField, otherField, message] of otherPairs) {
      if (values[selectField] === OTHER && !values[otherField]) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, path: [otherField], message })
      }
    }
  })

type FieldErrors = Partial<Record<keyof MerchantFormValues, string>>

const resolveOther = (value: string, otherValue: string) => (value === OTHER ? otherValue.trim() : value)

function FieldError({ message }: { message?: string }) {
  if (!message) return null
  return <p className="text-xs text-red-600 dark:text-red-300">{message}</p>
}

// A select-plus-"Other" pair appears six times on this form; rendering it from
// one place keeps the option list, the conditional input and its error message
// in sync. Defined at module scope so its identity is stable across renders —
// a component built inside the form would remount on every keystroke and drop
// focus out of the "Other" input.
function SelectWithOther({
  id,
  label,
  placeholder,
  options,
  field,
  otherField,
  otherLabel,
  otherPlaceholder,
  values,
  fieldErrors,
  setField,
}: {
  id: string
  label: string
  placeholder: string
  options: string[]
  field: keyof MerchantFormValues
  otherField?: keyof MerchantFormValues
  otherLabel?: string
  otherPlaceholder?: string
  values: MerchantFormValues
  fieldErrors: FieldErrors
  setField: <K extends keyof MerchantFormValues>(field: K, value: MerchantFormValues[K]) => void
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id} className="text-black dark:text-white">
        {label}
      </Label>
      <Select value={values[field]} onValueChange={(value) => setField(field, value)}>
        <SelectTrigger id={id} className="text-black dark:text-white bg-secondary">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <FieldError message={fieldErrors[field]} />
      {otherField && values[field] === OTHER && (
        <div className="space-y-2 mt-2">
          <Label htmlFor={`${id}-other`} className="text-black dark:text-white">
            {otherLabel}
          </Label>
          <Input
            id={`${id}-other`}
            value={values[otherField]}
            onChange={(event) => setField(otherField, event.target.value)}
            className="text-black dark:text-white bg-secondary"
            placeholder={otherPlaceholder}
          />
          <FieldError message={fieldErrors[otherField]} />
        </div>
      )}
    </div>
  )
}

export function MerchantApprovalForm() {
  const { addLocation } = useLocation()
  const router = useRouter()

  const [isSubmitting, setIsSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [placeSelection, setPlaceSelection] = useState<PlaceSelection | null>(null)
  const [values, setValues] = useState<MerchantFormValues>(emptyForm)

  const setField = useCallback(<K extends keyof MerchantFormValues>(field: K, value: MerchantFormValues[K]) => {
    setValues((current) => ({ ...current, [field]: value }))
    setFieldErrors((current) => {
      if (!current[field]) return current
      const next = { ...current }
      delete next[field]
      return next
    })
  }, [])

  const resetForm = () => {
    setValues(emptyForm)
    setFieldErrors({})
    setFormError(null)
    setPlaceSelection(null)
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setFormError(null)

    if (!placeSelection) {
      setFormError('Find your business or address and confirm the match before submitting.')
      return
    }

    const parsed = merchantFormSchema.safeParse(values)
    if (!parsed.success) {
      const errors: FieldErrors = {}
      for (const issue of parsed.error.issues) {
        const field = issue.path[0] as keyof MerchantFormValues | undefined
        if (field && !errors[field]) errors[field] = issue.message
      }
      setFieldErrors(errors)
      setFormError("Please fix the highlighted fields before submitting.")
      return
    }

    const form = parsed.data

    // On the manual path nothing vouches for the business name but the merchant,
    // so it is required here and must not simply echo the address they just
    // picked — that is the exact mistake this path is built to prevent, and the
    // backend rejects it too.
    if (placeSelection.source === "manual") {
      const address = placeSelection.address
      const typedName = form.manualBusinessName.trim()
      const addressLine = [address.street, [address.city, address.state].filter(Boolean).join(", ")]
        .filter(Boolean)
        .join(", ")

      const nameError = !typedName
        ? "Enter your business name."
        : [address.street, addressLine, address.formatted_address].some(
              (candidate) => candidate && candidate.toLowerCase() === typedName.toLowerCase(),
            )
          ? "That is your street address. Enter the name your customers know you by."
          : null

      if (nameError) {
        setFieldErrors((current) => ({ ...current, manualBusinessName: nameError }))
        setFormError("Please fix the highlighted fields before submitting.")
        return
      }
    }

    // Google-derived fields come from the confirmed place, merchant-authored
    // fields from the form. Nothing is sourced from both, and the backend
    // re-fetches the Google half from the place id before storing it.
    //
    // A manual listing has no place id and no Google-owned half at all: its
    // address comes from the geocode autocomplete and everything else is typed.
    const placeFields = placeSelection.source === "google_place"
      ? {
          google_id: placeSelection.place.google_id,
          listing_source: "google_place" as const,
          name: placeSelection.place.name,
          type: placeSelection.place.type,
          street: placeSelection.place.street,
          city: placeSelection.place.city,
          state: placeSelection.place.state,
          zip: placeSelection.place.zip,
          lat: placeSelection.place.lat,
          lng: placeSelection.place.lng,
          website: placeSelection.place.website,
          image_url: placeSelection.place.image_url,
          rating: placeSelection.place.rating,
          maps_page: placeSelection.place.maps_page,
          opening_hours: placeSelection.place.opening_hours,
          googlePhone: placeSelection.place.phone,
        }
      : {
          google_id: "",
          listing_source: "manual" as const,
          name: form.manualBusinessName,
          type: form.manualBusinessType,
          street: placeSelection.address.street,
          city: placeSelection.address.city,
          state: placeSelection.address.state,
          zip: placeSelection.address.zip,
          lat: placeSelection.address.lat,
          lng: placeSelection.address.lng,
          website: "",
          image_url: "",
          rating: 0,
          maps_page: "",
          opening_hours: [] as string[],
          googlePhone: "",
        }

    const { googlePhone, ...locationPlaceFields } = placeFields

    const newLocation: AuthedLocation = {
      id: 0,
      owner_id: "",
      ...locationPlaceFields,
      description: form.description,
      phone: form.businessPhone || googlePhone || "",
      email: form.businessEmail,
      admin_phone: form.contactPhone,
      admin_email: form.contactEmail,
      contact_firstname: form.contactFirstName,
      contact_lastname: form.contactLastName,
      contact_phone: form.contactPhone,
      pos_system: resolveOther(form.posSystem, form.posSystemOther),
      sole_proprietorship: form.soleProprietorship,
      tipping_policy: resolveOther(form.tippingPolicy, form.tippingPolicyOther),
      tipping_division: resolveOther(form.tippingDivision, form.tippingDivisionOther),
      table_coverage: resolveOther(form.tableCoverage, form.tableCoverageOther),
      service_stations: Number(form.serviceStations),
      tablet_model: resolveOther(form.tabletModel, form.tabletModelOther),
      messaging_service: resolveOther(form.messagingService, form.messagingServiceOther),
      payment_wallets: [],
      reference: form.reference,
    }

    setIsSubmitting(true)
    try {
      await addLocation(newLocation)
      resetForm()
      router.replace("/map")
    } catch (error) {
      // Navigating on failure used to make a rejected application look accepted.
      setFormError(error instanceof Error ? error.message : "Something went wrong. Please try again.")
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Card>
      {/* pb trimmed: with the description gone the header's own padding plus
          the content's left a large empty band under the title. */}
      <CardHeader className="pb-2">
        <CardTitle className="text-black dark:text-white">Merchant Application</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-6" noValidate>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="business-name" className="text-black dark:text-white">
                Search for Your Location Name
              </Label>
              <PlaceAutocomplete value={placeSelection} onSelect={setPlaceSelection} />
            </div>

            {placeSelection?.source === "google_place" && (
              <div className="space-y-2">
                <Label className="text-black dark:text-white">Street Address</Label>
                <Input
                  value={placeSelection.place.street}
                  className="text-black dark:text-white bg-secondary"
                  readOnly
                  disabled
                />
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  Address, business name and map position come from your Google listing. Update them in Google Business
                  Profile and they will follow here.
                </p>
              </div>
            )}

            {/* Manual path: the address came from Google but the identity did
                not, so these two are the only place a name and category exist. */}
            {placeSelection?.source === "manual" && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="manual-business-name" className="text-black dark:text-white">
                    Business Name
                  </Label>
                  <Input
                    id="manual-business-name"
                    value={values.manualBusinessName}
                    onChange={(event) => setField("manualBusinessName", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="The name your customers know you by"
                  />
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    This is what appears on the SFLuv map. Do not enter your street address here.
                  </p>
                  <FieldError message={fieldErrors.manualBusinessName} />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="manual-business-type" className="text-black dark:text-white">
                    Business Category
                  </Label>
                  <Input
                    id="manual-business-type"
                    value={values.manualBusinessType}
                    onChange={(event) => setField("manualBusinessType", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder="Cafe, barber shop, bookstore…"
                  />
                  <FieldError message={fieldErrors.manualBusinessType} />
                </div>

                <div className="space-y-2">
                  <Label className="text-black dark:text-white">Street Address</Label>
                  <Input
                    value={placeSelection.address.street}
                    className="text-black dark:text-white bg-secondary"
                    readOnly
                    disabled
                  />
                </div>
              </>
            )}

            <div className="space-y-2">
              <Label htmlFor="description" className="text-black dark:text-white">
                Business Description
              </Label>
              <Textarea
                id="description"
                value={values.description}
                onChange={(event) => setField("description", event.target.value)}
                className="text-black dark:text-white bg-secondary min-h-[100px]"
              />
              <FieldError message={fieldErrors.description} />
            </div>

            {/* Business Contact Information */}
            <div>
              <h3 className="text-lg font-medium text-black dark:text-white mb-4">
                Business Contact Information (Publicly Available to Customers)
              </h3>
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="business-phone" className="text-black dark:text-white">
                    Business Phone Number
                  </Label>
                  <Input
                    id="business-phone"
                    value={values.businessPhone}
                    onChange={(event) => setField("businessPhone", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                    placeholder={placeSelection?.source === "google_place" ? placeSelection.place.phone : ""}
                  />
                  <FieldError message={fieldErrors.businessPhone} />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="business-email" className="text-black dark:text-white">
                    Business Email
                  </Label>
                  <Input
                    id="business-email"
                    type="email"
                    value={values.businessEmail}
                    onChange={(event) => setField("businessEmail", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                  />
                  <FieldError message={fieldErrors.businessEmail} />
                </div>
              </div>
            </div>

            {/* Primary Contact Information */}
            <div>
              <h3 className="text-lg font-medium text-black dark:text-white mb-4">
                Primary Administrative Contact for Business (Only Visible to SFLuv Admin Team)
              </h3>
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="primary-contact-first-name" className="text-black dark:text-white">
                    First Name
                  </Label>
                  <Input
                    id="primary-contact-first-name"
                    value={values.contactFirstName}
                    onChange={(event) => setField("contactFirstName", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                  />
                  <FieldError message={fieldErrors.contactFirstName} />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="primary-contact-last-name" className="text-black dark:text-white">
                    Last Name
                  </Label>
                  <Input
                    id="primary-contact-last-name"
                    value={values.contactLastName}
                    onChange={(event) => setField("contactLastName", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                  />
                  <FieldError message={fieldErrors.contactLastName} />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="primary-contact-phone" className="text-black dark:text-white">
                    Phone Number
                  </Label>
                  <Input
                    id="primary-contact-phone"
                    value={values.contactPhone}
                    onChange={(event) => setField("contactPhone", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                  />
                  <FieldError message={fieldErrors.contactPhone} />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="primary-contact-email" className="text-black dark:text-white">
                    Email
                  </Label>
                  <Input
                    id="primary-contact-email"
                    type="email"
                    value={values.contactEmail}
                    onChange={(event) => setField("contactEmail", event.target.value)}
                    className="text-black dark:text-white bg-secondary"
                  />
                  <FieldError message={fieldErrors.contactEmail} />
                </div>
              </div>
            </div>

            <SelectWithOther
              id="pos-system"
              label="What Point of Sale System do you use?"
              placeholder="Select POS system"
              options={posOptions}
              field="posSystem"
              otherField="posSystemOther"
              otherLabel="Point of Sale System"
              otherPlaceholder="Specify your POS system"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <SelectWithOther
              id="sole-proprietorship"
              label="Is your business a sole proprietorship?"
              placeholder="Select option"
              options={soleProprietorshipOptions}
              field="soleProprietorship"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <SelectWithOther
              id="tipping-policy"
              label="Do you add tips to the bill automatically, or do customers tip at their discretion?"
              placeholder="Select tipping policy"
              options={tippingOptions}
              field="tippingPolicy"
              otherField="tippingPolicyOther"
              otherLabel="Tipping Policy"
              otherPlaceholder="Specify your tipping policy"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <SelectWithOther
              id="tipping-division"
              label="How are tips divided amongst staff members?"
              placeholder="Select tip division style"
              options={tippingDivisionOptions}
              field="tippingDivision"
              otherField="tippingDivisionOther"
              otherLabel="Division of Tips"
              otherPlaceholder="Specify your tip division style"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <SelectWithOther
              id="table-coverage"
              label="Are servers assigned to specific sections, or is table coverage managed differently?"
              placeholder="Select table coverage method"
              options={tableCoverageOptions}
              field="tableCoverage"
              otherField="tableCoverageOther"
              otherLabel="Table Coverage Method"
              otherPlaceholder="Specify your table coverage method"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <SelectWithOther
              id="tables-or-stations"
              label="How many tables or service stations do you have?"
              placeholder="Select # of service stations"
              options={serviceStationOptions}
              field="serviceStations"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <SelectWithOther
              id="tablet-model"
              label="If you currently have a tablet or similar device available for servers near the register, please specify which model they use:"
              placeholder="Select tablet model"
              options={tabletOptions}
              field="tabletModel"
              otherField="tabletModelOther"
              otherLabel="Tablet Model"
              otherPlaceholder="Specify your tablet model"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <SelectWithOther
              id="messaging-service"
              label="If your business regularly receives notifications from a system like Zapier, what service do you use:"
              placeholder="Select messaging service"
              options={messagingServiceOptions}
              field="messagingService"
              otherField="messagingServiceOther"
              otherLabel="Messaging Service"
              otherPlaceholder="Specify your messaging service"
              values={values}
              fieldErrors={fieldErrors}
              setField={setField}
            />

            <div className="space-y-2">
              <Label htmlFor="reference" className="text-black dark:text-white">
                How did you hear about SFLuv?
              </Label>
              <Textarea
                id="reference"
                value={values.reference}
                onChange={(event) => setField("reference", event.target.value)}
                className="text-black dark:text-white bg-secondary min-h-[100px]"
              />
              <FieldError message={fieldErrors.reference} />
            </div>
          </div>

          {formError && (
            <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{formError}</span>
            </div>
          )}

          <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isSubmitting}>
            {isSubmitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : "Submit Merchant Application"}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
