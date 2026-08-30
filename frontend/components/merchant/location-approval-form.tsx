"use client"

import { useCallback, useMemo, useState } from "react"
import { z } from "zod"
import { Check, Info, Loader2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  OpeningHoursEditor,
  emptyWeek,
  validateWeek,
  weekSignature,
  type LocationDayHours,
} from "@/components/locations/opening-hours-editor"
import PlaceAutocomplete from "./google_place_finder"
import { useApp } from "@/context/AppProvider"
import { useLocation } from "@/context/LocationProvider"
import type { AuthedLocation, PlaceSelection } from "@/types/location"

/**
 * The three sections of the Location Approval Form, in order. Named here rather
 * than inline so the header, the step guard and the progress rail cannot drift
 * apart.
 */
export const LOCATION_FORM_STEPS = [
  {
    key: "public",
    title: "Public Information",
    blurb: "What customers see about this location on the SFLuv map.",
  },
  {
    key: "contact",
    title: "Contact",
    blurb: "How the SFLuv team reaches you about this location.",
  },
  {
    key: "payment",
    title: "Payment System",
    blurb: "How you take payment today, so we can set this location up properly.",
  },
] as const

type StepKey = (typeof LOCATION_FORM_STEPS)[number]["key"]

const OTHER = "Other"

const posOptions = [
  "Square",
  "Clover",
  "Toast",
  "Shopify",
  "SumUp",
  "Lightspeed",
  "Cash only",
  "No point of sale system",
  OTHER,
]

const referralOptions = [
  "A friend or another business",
  "An SFLuv team member",
  "A community or neighbourhood event",
  "Social media",
  "A search engine",
  "Press or a newsletter",
  OTHER,
]

/**
 * A stored answer is the resolved string, not the option that produced it: an
 * "Other" answer is flattened into its free text before it is sent. Anything
 * the list does not recognise is therefore an "Other" being read back, and has
 * to go into the write-in box or the merchant silently loses it.
 */
const fromResolvedOption = (stored: string, options: string[]): [string, string] => {
  const value = (stored || "").trim()
  if (!value) return ["", ""]
  if (value === OTHER) return [OTHER, ""]
  return options.includes(value) ? [value, ""] : [OTHER, value]
}

const resolveOther = (value: string, otherValue: string) =>
  value === OTHER ? otherValue.trim() : value

/** Yes/no answers are stored as booleans but picked as strings. */
type YesNo = "yes" | "no" | ""

const toYesNo = (value: boolean | null | undefined): YesNo => {
  if (value === true) return "yes"
  if (value === false) return "no"
  return ""
}

const fromYesNo = (value: YesNo): boolean | null => {
  if (value === "yes") return true
  if (value === "no") return false
  return null
}

type FormValues = {
  // Public Information
  locationName: string
  businessType: string
  description: string
  publicPhone: string
  // Contact
  contactName: string
  contactPhone: string
  contactEmail: string
  referralSource: string
  referralSourceOther: string
  // Payment System
  posSystem: string
  posSystemOther: string
  acceptsTips: YesNo
  hasStaffTablet: YesNo
}

const emptyForm: FormValues = {
  locationName: "",
  businessType: "",
  description: "",
  publicPhone: "",
  contactName: "",
  contactPhone: "",
  contactEmail: "",
  referralSource: "",
  referralSourceOther: "",
  posSystem: "",
  posSystemOther: "",
  acceptsTips: "",
  hasStaffTablet: "",
}

/**
 * What a second application inherits from one this merchant already filled in.
 *
 * Everything here describes the business rather than the premises: who we ring
 * about it, how they found us, what tills they run. The address, the Google
 * entry behind it, the description and the public phone are all about a
 * specific room, and carried over they would reach an admin as a claim about
 * that room rather than a leftover from somewhere else.
 *
 * The tips answer is carried too, and it is the one with teeth — it decides
 * whether approval mints this location a tipping wallet — so it is marked as
 * carried over in the UI like everything else.
 */
const CARRIED_OVER_FIELDS: (keyof FormValues)[] = [
  "contactName",
  "contactPhone",
  "contactEmail",
  "referralSource",
  "referralSourceOther",
  "posSystem",
  "posSystemOther",
  "acceptsTips",
  "hasStaffTablet",
]

const toFormValues = (location: AuthedLocation): FormValues => {
  const [posSystem, posSystemOther] = fromResolvedOption(location.pos_system, posOptions)
  const [referralSource, referralSourceOther] = fromResolvedOption(
    location.referral_source || location.reference || "",
    referralOptions,
  )

  return {
    locationName: location.name || "",
    businessType: location.type || "",
    description: location.description || "",
    publicPhone: location.phone || "",
    contactName:
      location.contact_name ||
      [location.contact_firstname, location.contact_lastname].filter(Boolean).join(" "),
    contactPhone: location.contact_phone || location.admin_phone || "",
    contactEmail: location.admin_email || "",
    referralSource,
    referralSourceOther,
    posSystem,
    posSystemOther,
    acceptsTips: toYesNo(location.accepts_tips),
    hasStaffTablet: toYesNo(location.has_staff_tablet),
  }
}

type CarriedOver = {
  sourceName: string
  values: FormValues
  fields: Set<keyof FormValues>
}

const carryOver = (location: AuthedLocation): CarriedOver => {
  const previous = toFormValues(location)
  const values = { ...emptyForm }
  const fields = new Set<keyof FormValues>()

  for (const field of CARRIED_OVER_FIELDS) {
    const answer = previous[field]
    if (!answer) continue
    values[field] = answer as never
    fields.add(field)
  }

  return { sourceName: (location.name || "").trim(), values, fields }
}

const requiredText = (label: string, max = 4000) =>
  z.string().trim().min(1, `${label} is required.`).max(max, `${label} must be ${max} characters or fewer.`)

const yesNo = (label: string) =>
  z.enum(["yes", "no"], { errorMap: () => ({ message: `${label} is required.` }) })

// Radix Select is not a native <select>, so `required` does nothing on it —
// these schemas are the only thing enforcing the dropdowns. One per step, so
// Next cannot carry somebody past a section they have not finished.
const stepSchemas: Record<StepKey, z.ZodTypeAny> = {
  public: z.object({
    locationName: requiredText("Location name", 512),
    businessType: requiredText("Business type", 512),
    description: requiredText("Location description"),
    publicPhone: z.string().trim().max(64, "Public phone is too long."),
  }),
  contact: z
    .object({
      contactName: requiredText("Contact name", 512),
      contactPhone: requiredText("Contact phone", 64),
      contactEmail: requiredText("Contact email", 320).email("Enter a valid email address."),
      referralSource: requiredText("How you heard about SFLuv"),
      referralSourceOther: z.string().trim(),
    })
    .superRefine((values, ctx) => {
      if (values.referralSource === OTHER && !values.referralSourceOther) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["referralSourceOther"],
          message: "Tell us how you heard about SFLuv.",
        })
      }
    }),
  payment: z
    .object({
      posSystem: requiredText("POS type", 512),
      posSystemOther: z.string().trim(),
      acceptsTips: yesNo("The tips answer"),
      hasStaffTablet: yesNo("The tablet answer"),
    })
    .superRefine((values, ctx) => {
      if (values.posSystem === OTHER && !values.posSystemOther) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["posSystemOther"],
          message: "Tell us which point of sale system you use.",
        })
      }
    }),
}

type FieldErrors = Partial<Record<keyof FormValues, string>>

function FieldError({ message }: { message?: string }) {
  if (!message) return null
  return <p className="text-xs text-red-600 dark:text-red-300">{message}</p>
}

/**
 * A carried-over answer is right often enough to be worth filling in and stale
 * often enough that submitting it unread is the failure worth guarding against.
 * The mark stays until the merchant edits that field, so leaving one as it
 * stands is a decision rather than something nobody looked at.
 */
function CarriedOverAnswer({ show }: { show?: boolean }) {
  if (!show) return null
  return (
    <p className="text-xs text-amber-700 dark:text-amber-400">
      Carried over from your other location — check it before submitting.
    </p>
  )
}

function YesNoField({
  id,
  label,
  hint,
  value,
  onChange,
  error,
  carriedOver,
  disabled,
}: {
  id: string
  label: string
  hint?: string
  value: YesNo
  onChange: (value: YesNo) => void
  error?: string
  carriedOver?: boolean
  disabled?: boolean
}) {
  return (
    <div className="space-y-2">
      <Label className="text-black dark:text-white">{label}</Label>
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      <div role="radiogroup" aria-label={label} className="flex gap-2">
        {(["yes", "no"] as const).map((option) => (
          <button
            key={option}
            id={option === "yes" ? id : undefined}
            type="button"
            role="radio"
            aria-checked={value === option}
            disabled={disabled}
            onClick={() => onChange(option)}
            className={`min-w-[88px] rounded-md border px-4 py-2 text-sm font-medium capitalize transition-colors disabled:opacity-60 ${
              value === option
                ? "border-[#eb6c6c] bg-[#eb6c6c]/10 text-[#b64a4a] dark:text-[#f0a0a0]"
                : "border-border/70 bg-secondary text-black hover:border-[#eb6c6c]/60 dark:text-white"
            }`}
          >
            {option}
          </button>
        ))}
      </div>
      <FieldError message={error} />
      <CarriedOverAnswer show={carriedOver} />
    </div>
  )
}

// Defined at module scope so its identity is stable across renders — a
// component built inside the form would remount on every keystroke and drop
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
  carriedOver,
}: {
  id: string
  label: string
  placeholder: string
  options: string[]
  field: keyof FormValues
  otherField: keyof FormValues
  otherLabel: string
  otherPlaceholder: string
  values: FormValues
  fieldErrors: FieldErrors
  setField: <K extends keyof FormValues>(field: K, value: FormValues[K]) => void
  carriedOver?: boolean
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id} className="text-black dark:text-white">
        {label}
      </Label>
      <Select
        value={values[field] as string}
        onValueChange={(value) => setField(field, value as FormValues[typeof field])}
      >
        <SelectTrigger id={id} className="bg-secondary text-black dark:text-white">
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
      <CarriedOverAnswer show={carriedOver} />
      {values[field] === OTHER && (
        <div className="mt-2 space-y-2">
          <Label htmlFor={`${id}-other`} className="text-black dark:text-white">
            {otherLabel}
          </Label>
          <Input
            id={`${id}-other`}
            value={values[otherField] as string}
            onChange={(event) =>
              setField(otherField, event.target.value as FormValues[typeof otherField])
            }
            className="bg-secondary text-black dark:text-white"
            placeholder={otherPlaceholder}
          />
          <FieldError message={fieldErrors[otherField]} />
        </div>
      )}
    </div>
  )
}

/**
 * The Location Approval Form: one location's application, in three steps.
 *
 * It replaces the single sheet that asked everything at once. The three
 * sections are not decoration — they are three different audiences. Public
 * Information ends up on the map, Contact never leaves the SFLuv team, and
 * Payment System is what an admin reads before walking the merchant through
 * setup. Splitting them is what lets each one say who will see it.
 *
 * Every answer belongs to this location and not to the merchant: two branches
 * can run different tills and take tips differently. What can sensibly be
 * shared is prefilled from an earlier application and marked as carried over,
 * so a second location is quick to file without any answer being assumed.
 */
export function LocationApprovalForm({
  prefillFrom,
  onSubmitted,
}: {
  /**
   * A listing this merchant already filled in, whose shared answers start the
   * form off. Absent on a first application.
   */
  prefillFrom?: AuthedLocation
  /** Called with the new listing's id once it has been accepted. */
  onSubmitted?: (locationId: number) => void
}) {
  const { addLocation } = useLocation()
  const { authFetch } = useApp()

  const [stepIndex, setStepIndex] = useState(0)
  const [placeSelection, setPlaceSelection] = useState<PlaceSelection | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})

  const carriedOver = useMemo<CarriedOver | null>(
    () => (prefillFrom ? carryOver(prefillFrom) : null),
    [prefillFrom],
  )
  const [values, setValues] = useState<FormValues>(() => carriedOver?.values ?? emptyForm)
  const [carriedOverFields, setCarriedOverFields] = useState<Set<keyof FormValues>>(
    () => new Set(carriedOver?.fields),
  )

  // Hours are optional, so they start as a blank week and are only sent when
  // the merchant has actually put times in one. On the Google path an untouched
  // week means "use whatever Google publishes", which the backend already does.
  const [week, setWeek] = useState<LocationDayHours[]>(() => emptyWeek())
  const [hoursError, setHoursError] = useState("")
  const blankWeekSignature = useMemo(() => weekSignature(emptyWeek()), [])
  const hoursEntered = weekSignature(week) !== blankWeekSignature

  const step = LOCATION_FORM_STEPS[stepIndex]

  const setField = useCallback(
    <K extends keyof FormValues>(field: K, value: FormValues[K]) => {
      setValues((current) => ({ ...current, [field]: value }))
      // Once it has been typed over it is this location's answer, not the other
      // location's, and the warning would be telling the merchant to check
      // their own work.
      setCarriedOverFields((current) => {
        if (!current.has(field)) return current
        const next = new Set(current)
        next.delete(field)
        return next
      })
      setFieldErrors((current) => {
        if (!current[field]) return current
        const next = { ...current }
        delete next[field]
        return next
      })
    },
    [],
  )

  /**
   * A Google business listing fills in the name, the category and the phone.
   * An address-only selection fills in nothing: that path exists precisely
   * because Google has no business record to copy, and inheriting the address
   * as a name is the failure it is built to prevent.
   *
   * The name and the category are overwritten rather than merely filled, and
   * their fields lock. They are what the backend stores for a Google listing
   * whatever the form sends, so a value left showing underneath a locked field
   * would be the screen disagreeing with the record.
   *
   * The phone is the opposite and only fills a gap: a merchant may publish a
   * different customer-facing number than the one on their Google listing, and
   * the backend treats Google's as a fallback for exactly that reason.
   */
  const applyPlaceSelection = (selection: PlaceSelection | null) => {
    setPlaceSelection(selection)
    setFormError(null)
    setFieldErrors((current) => ({
      ...current,
      locationName: undefined,
      businessType: undefined,
    }))
    if (selection?.source !== "google_place") return

    const place = selection.place
    setValues((current) => ({
      ...current,
      locationName: place.name || current.locationName,
      businessType: place.type || current.businessType,
      publicPhone: current.publicPhone || place.phone || "",
    }))
  }

  /**
   * Which of the name and the category Google is vouching for.
   *
   * Where it does, the field is filled in and locked: the backend re-fetches
   * the place server-side and overwrites both with Google's own values, so an
   * editable box there would take a change and silently drop it.
   *
   * Where it does not — the merchant ticked "Can't find my location", or the
   * Google result carries no primary type, which plenty do not — the field
   * opens up and the merchant answers it. That second case is the one that is
   * easy to miss, and it is why these are two separate checks rather than one
   * "is this a Google listing" flag.
   */
  const googleOwnsName =
    placeSelection?.source === "google_place" && !!placeSelection.place.name
  const googleOwnsType =
    placeSelection?.source === "google_place" && !!placeSelection.place.type

  const selectedAddressLine = useMemo(() => {
    if (!placeSelection) return ""
    const source =
      placeSelection.source === "google_place" ? placeSelection.place : placeSelection.address
    if (source.formatted_address) return source.formatted_address
    return [source.street, [source.city, source.state].filter(Boolean).join(", "), source.zip]
      .filter(Boolean)
      .join(", ")
  }, [placeSelection])

  const validateStep = (key: StepKey): boolean => {
    if (key === "public" && !placeSelection) {
      setFormError("Find your location and confirm the match before continuing.")
      return false
    }

    const parsed = stepSchemas[key].safeParse(values)
    if (!parsed.success) {
      const errors: FieldErrors = {}
      for (const issue of parsed.error.issues) {
        const field = issue.path[0] as keyof FormValues | undefined
        if (field && !errors[field]) errors[field] = issue.message
      }
      setFieldErrors((current) => ({ ...current, ...errors }))
      setFormError("Please fix the highlighted fields before continuing.")
      return false
    }

    if (key === "public" && hoursEntered) {
      const invalid = validateWeek(week)
      if (invalid) {
        setHoursError(invalid)
        setFormError("Please fix your opening hours before continuing.")
        return false
      }
      setHoursError("")
    }

    // The Shiba failure mode: a listing on the map named "1234 Main St". The
    // backend rejects it too, but the complaint belongs on the field rather
    // than arriving as a whole-form error after the last step.
    if (key === "public" && !googleOwnsName) {
      const typed = values.locationName.trim().toLowerCase()
      const address = placeSelection?.source === "manual" ? placeSelection.address : null
      const candidates = [address?.street, selectedAddressLine].filter(Boolean) as string[]
      if (candidates.some((candidate) => candidate.toLowerCase() === typed)) {
        setFieldErrors((current) => ({
          ...current,
          locationName: "That is your street address. Enter the name your customers know you by.",
        }))
        setFormError("Please fix the highlighted fields before continuing.")
        return false
      }
    }

    setFormError(null)
    return true
  }

  const goNext = () => {
    if (!validateStep(step.key)) return
    setStepIndex((current) => Math.min(current + 1, LOCATION_FORM_STEPS.length - 1))
  }

  const goBack = () => {
    setFormError(null)
    setStepIndex((current) => Math.max(current - 1, 0))
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setFormError(null)

    // Every step is re-checked, not just the last one. A merchant can walk back
    // and empty a field they already passed, and the submit button is the last
    // place that can catch it.
    for (const candidate of LOCATION_FORM_STEPS) {
      if (!validateStep(candidate.key)) {
        setStepIndex(LOCATION_FORM_STEPS.findIndex((entry) => entry.key === candidate.key))
        return
      }
    }
    if (!placeSelection) return

    const answers = {
      description: values.description.trim(),
      contact_name: values.contactName.trim(),
      // Both halves of the contact-phone split, so the admin panel and the
      // approval email agree with the form about which number this is.
      contact_phone: values.contactPhone.trim(),
      admin_phone: values.contactPhone.trim(),
      admin_email: values.contactEmail.trim(),
      referral_source: resolveOther(values.referralSource, values.referralSourceOther),
      pos_system: resolveOther(values.posSystem, values.posSystemOther),
      accepts_tips: fromYesNo(values.acceptsTips),
      has_staff_tablet: fromYesNo(values.hasStaffTablet),
    }

    // Google-derived fields come from the confirmed place, merchant-authored
    // ones from the form. Nothing is sourced from both, and the backend
    // re-fetches the Google half from the place id before storing it.
    const placeFields =
      placeSelection.source === "google_place"
        ? {
            google_id: placeSelection.place.google_id,
            listing_source: "google_place" as const,
            name: placeSelection.place.name || values.locationName.trim(),
            // The merchant's answer whenever Google has none. The backend's
            // place verification only overwrites a category it actually has, so
            // this survives the round trip rather than being wiped on the way in.
            type: placeSelection.place.type || values.businessType.trim(),
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
          }
        : {
            google_id: "",
            listing_source: "manual" as const,
            name: values.locationName.trim(),
            type: values.businessType.trim(),
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
          }

    const newLocation = {
      id: 0,
      owner_id: "",
      ...placeFields,
      ...answers,
      phone: values.publicPhone.trim(),
      payment_wallets: [],
    } as unknown as AuthedLocation

    setIsSubmitting(true)
    try {
      const locationId = await addLocation(newLocation)

      // Hours have their own endpoint, addressed by location id, so they can
      // only be written after the listing exists. hours_manual goes with them:
      // times the merchant typed are theirs, and the nightly Google sync must
      // not overwrite what they just told us.
      //
      // A failure here does not fail the application. The listing is already in
      // the queue, and losing the hours is a thing the merchant can fix from
      // their locations page; telling them the submission failed is not.
      if (hoursEntered && locationId > 0) {
        try {
          await authFetch(`/locations/${locationId}/hours`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ hours: week, hours_manual: true }),
          })
        } catch (error) {
          console.error("Unable to save the opening hours for the new location", error)
        }
      }

      onSubmitted?.(locationId)
    } catch (error) {
      setFormError(
        error instanceof Error ? error.message : "Something went wrong. Please try again.",
      )
      // Back to the first step: everything the backend refuses a submission
      // over — a duplicate business, a place outside the service area, a name
      // that is really an address — lives there.
      setStepIndex(0)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Card>
      <CardHeader className="space-y-4 pb-4">
        <CardTitle className="text-black dark:text-white">Location Approval Form</CardTitle>
        <ol className="grid gap-2 sm:grid-cols-3">
          {LOCATION_FORM_STEPS.map((entry, index) => {
            const done = index < stepIndex
            const current = index === stepIndex
            return (
              <li
                key={entry.key}
                aria-current={current ? "step" : undefined}
                className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-sm ${
                  current
                    ? "border-[#eb6c6c] bg-[#eb6c6c]/10 font-semibold text-black dark:text-white"
                    : "border-border/70 text-muted-foreground"
                }`}
              >
                <span
                  className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-xs ${
                    done || current ? "bg-[#eb6c6c] text-white" : "bg-muted text-muted-foreground"
                  }`}
                >
                  {done ? <Check className="h-3 w-3" /> : index + 1}
                </span>
                {entry.title}
              </li>
            )
          })}
        </ol>
        <p className="text-sm text-muted-foreground">{step.blurb}</p>
      </CardHeader>

      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-6" noValidate>
          {carriedOver && carriedOver.fields.size > 0 && (
            <div className="space-y-1 rounded-lg border border-amber-300/70 bg-amber-50 p-4 dark:border-amber-500/30 dark:bg-amber-500/10">
              <p className="text-sm font-medium text-black dark:text-white">
                Some answers are carried over from{" "}
                {carriedOver.sourceName || "your other location"}
              </p>
              <p className="text-xs leading-relaxed text-gray-600 dark:text-gray-400">
                Your contact and payment answers are filled in and marked where they were. Read
                them before submitting — anything that differs at this location now stands as this
                location&apos;s answer. Nothing about the premises is carried over: the address,
                the description and the opening hours are all yours to answer afresh for this one.
              </p>
            </div>
          )}

          {step.key === "public" && (
            <div className="space-y-5">
              <div className="space-y-2">
                <Label className="text-black dark:text-white">Location Address</Label>
                <PlaceAutocomplete value={placeSelection} onSelect={applyPlaceSelection} />
                <p className="text-xs text-muted-foreground">
                  We look the address up with Google so your pin lands in the right place on the
                  map.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="location-name" className="text-black dark:text-white">
                  Location Name
                </Label>
                <Input
                  id="location-name"
                  value={values.locationName}
                  onChange={(event) => setField("locationName", event.target.value)}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="The name your customers know you by"
                  readOnly={googleOwnsName}
                  disabled={googleOwnsName}
                />
                <FieldError message={fieldErrors.locationName} />
                {googleOwnsName && (
                  <p className="text-xs text-muted-foreground">
                    This is the name on your Google listing, and the name that goes on the map. To
                    change it, change it on Google — or pick your location again with
                    &ldquo;Can&apos;t find my location&rdquo; ticked and name it yourself.
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="business-type" className="text-black dark:text-white">
                  Business Type
                </Label>
                <Input
                  id="business-type"
                  value={values.businessType}
                  onChange={(event) => setField("businessType", event.target.value)}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="Cafe, bookshop, barber, bakery…"
                  readOnly={googleOwnsType}
                  disabled={googleOwnsType}
                />
                <FieldError message={fieldErrors.businessType} />
                {googleOwnsType ? (
                  <p className="text-xs text-muted-foreground">
                    The category on your Google listing.
                  </p>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    Your Google listing has no category, so tell us what kind of business this is.
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="location-description" className="text-black dark:text-white">
                  Location Description
                </Label>
                <Textarea
                  id="location-description"
                  value={values.description}
                  onChange={(event) => setField("description", event.target.value)}
                  className="min-h-[120px] bg-secondary text-black dark:text-white"
                  placeholder="What you sell, what makes this location worth a visit."
                />
                <FieldError message={fieldErrors.description} />
              </div>

              <div className="space-y-2">
                <Label htmlFor="public-phone" className="text-black dark:text-white">
                  Public Phone <span className="text-muted-foreground">(optional)</span>
                </Label>
                <Input
                  id="public-phone"
                  value={values.publicPhone}
                  onChange={(event) => setField("publicPhone", event.target.value)}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="The number customers should call"
                />
                <FieldError message={fieldErrors.publicPhone} />
                <p className="text-xs text-muted-foreground">
                  Shown on the map. This does not have to be the number we reach you on.
                </p>
              </div>

              <div className="space-y-2">
                <Label className="text-black dark:text-white">
                  Hours <span className="text-muted-foreground">(optional)</span>
                </Label>
                <p className="text-xs text-muted-foreground">
                  Leave this alone to use the hours published on your Google listing. Anything you
                  enter here is kept exactly as typed.
                </p>
                <OpeningHoursEditor
                  week={week}
                  onChange={(next) => {
                    setWeek(next)
                    setHoursError("")
                  }}
                  // The nightly-sync switch is not offered during an application:
                  // entering hours here is itself the decision to keep them, and
                  // a listing with no Google entry has nothing to sync from.
                  manual
                  onManualChange={() => undefined}
                  disabled={isSubmitting}
                />
                <FieldError message={hoursError || undefined} />
              </div>
            </div>
          )}

          {step.key === "contact" && (
            <div className="space-y-5">
              <div className="flex items-start gap-2 rounded-lg border border-border/70 bg-muted/30 p-3">
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        aria-label="Who sees these contact details"
                        className="mt-0.5 text-muted-foreground hover:text-foreground"
                      >
                        <Info className="h-4 w-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs">
                      These details are for the SFLuv team only. They are never shown on the map,
                      in the app, or to customers.
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
                <p className="text-xs leading-relaxed text-muted-foreground">
                  For internal contact only. Nothing in this section is public — it is how the
                  SFLuv team reaches you about this location, and it does not have to match the
                  public phone on the previous step.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="contact-name" className="text-black dark:text-white">
                  Contact Name
                </Label>
                <Input
                  id="contact-name"
                  value={values.contactName}
                  onChange={(event) => setField("contactName", event.target.value)}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="Who we should ask for"
                />
                <FieldError message={fieldErrors.contactName} />
                <CarriedOverAnswer show={carriedOverFields.has("contactName")} />
              </div>

              <div className="space-y-2">
                <Label htmlFor="contact-phone" className="text-black dark:text-white">
                  Contact Phone
                </Label>
                <Input
                  id="contact-phone"
                  value={values.contactPhone}
                  onChange={(event) => setField("contactPhone", event.target.value)}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="The best number to reach you on"
                />
                <FieldError message={fieldErrors.contactPhone} />
                <CarriedOverAnswer show={carriedOverFields.has("contactPhone")} />
              </div>

              <div className="space-y-2">
                <Label htmlFor="contact-email" className="text-black dark:text-white">
                  Contact Email
                </Label>
                <Input
                  id="contact-email"
                  type="email"
                  value={values.contactEmail}
                  onChange={(event) => setField("contactEmail", event.target.value)}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="you@yourbusiness.com"
                />
                <FieldError message={fieldErrors.contactEmail} />
                <CarriedOverAnswer show={carriedOverFields.has("contactEmail")} />
              </div>

              <SelectWithOther
                id="referral-source"
                label="How did you hear about SFLuv?"
                placeholder="Pick the closest answer"
                options={referralOptions}
                field="referralSource"
                otherField="referralSourceOther"
                otherLabel="Tell us how"
                otherPlaceholder="How you first heard about SFLuv"
                values={values}
                fieldErrors={fieldErrors}
                setField={setField}
                carriedOver={carriedOverFields.has("referralSource")}
              />
            </div>
          )}

          {step.key === "payment" && (
            <div className="space-y-5">
              <p className="text-xs leading-relaxed text-muted-foreground">
                This is what the SFLuv team reads before setting this location up, so the person
                helping you knows what they are looking at before they arrive.
              </p>

              <SelectWithOther
                id="pos-system"
                label="POS Type"
                placeholder="Pick your point of sale system"
                options={posOptions}
                field="posSystem"
                otherField="posSystemOther"
                otherLabel="Which system?"
                otherPlaceholder="The name of your point of sale system"
                values={values}
                fieldErrors={fieldErrors}
                setField={setField}
                carriedOver={carriedOverFields.has("posSystem")}
              />

              <YesNoField
                id="accepts-tips"
                label="Do you accept tips?"
                hint="If you do, this location gets a tipping wallet of its own when it is approved, kept separate from your takings."
                value={values.acceptsTips}
                onChange={(value) => setField("acceptsTips", value)}
                error={fieldErrors.acceptsTips}
                carriedOver={carriedOverFields.has("acceptsTips")}
                disabled={isSubmitting}
              />

              <YesNoField
                id="staff-tablet"
                label="Do you have a generic tablet or mobile phone available to staff?"
                hint="A shared device at the counter is the easiest way to take SFLuv payments."
                value={values.hasStaffTablet}
                onChange={(value) => setField("hasStaffTablet", value)}
                error={fieldErrors.hasStaffTablet}
                carriedOver={carriedOverFields.has("hasStaffTablet")}
                disabled={isSubmitting}
              />
            </div>
          )}

          {formError && (
            <p className="rounded-md border border-red-400/40 bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-500/10 dark:text-red-200">
              {formError}
            </p>
          )}

          <div className="flex flex-col gap-3 sm:flex-row sm:justify-between">
            <Button
              type="button"
              variant="outline"
              disabled={stepIndex === 0 || isSubmitting}
              onClick={goBack}
            >
              Back
            </Button>

            {stepIndex < LOCATION_FORM_STEPS.length - 1 ? (
              <Button
                type="button"
                className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                onClick={goNext}
                disabled={isSubmitting}
              >
                Continue
              </Button>
            ) : (
              <Button
                type="submit"
                className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                disabled={isSubmitting}
              >
                {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Submit application
              </Button>
            )}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
