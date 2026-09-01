"use client"

import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { z } from "zod"
import { Check, Info, Loader2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Expand } from "@/components/ui/expand"
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
import PlaceAutocomplete, { PlaceSelectionStatus } from "./google_place_finder"
import { LocationLogoField } from "./location-logo-field"
import { uploadLocationLogo } from "@/lib/location-logo"
import { formatPhone, isValidEmail, isValidPhone, normalizeEmail } from "@/lib/contact-format"
import { useApp } from "@/context/AppProvider"
import { useLocation } from "@/context/LocationProvider"
import type { AuthedLocation, PlaceSelection } from "@/types/location"

/**
 * The three sections of the Location Approval Form, in order. Named here rather
 * than inline so the header, the step guard and the progress rail cannot drift
 * apart.
 */
export const LOCATION_FORM_STEPS = [
  { key: "public", title: "Public Information" },
  { key: "contact", title: "Contact" },
  { key: "payment", title: "Payment System" },
] as const

type StepKey = (typeof LOCATION_FORM_STEPS)[number]["key"]

const OTHER = "Other"

/** Kept in step with the duration on ui/expand, which does the sliding. */
const CHOOSER_MOVE_MS = 300

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
  // Name and type are absent on purpose. On the Google path they are Google's
  // answers, are not shown, and are overwritten server-side from the place id
  // regardless of what a client sends; on the manual path they are required and
  // checked in validateStep, which is the only place that knows which path the
  // merchant took. The description is optional — a shop with none still belongs
  // on the map.
  public: z.object({
    description: z.string().trim().max(4000, "Location description must be 4000 characters or fewer."),
    publicPhone: z
      .string()
      .trim()
      .refine((value) => value === "" || isValidPhone(value), "Enter a valid phone number."),
  }),
  contact: z
    .object({
      contactName: requiredText("Contact name", 512),
      contactPhone: requiredText("Contact phone", 64).refine(
        isValidPhone,
        "Enter a valid phone number.",
      ),
      contactEmail: requiredText("Contact email", 320).refine(
        isValidEmail,
        "Enter a valid email address.",
      ),
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

/**
 * A field label, with the context that field needs behind an info icon.
 *
 * Helper text under an input is read once and then becomes furniture the eye
 * skips on every later visit, while still taking the vertical space that pushes
 * the next field off the screen. On hover it costs nothing until it is wanted.
 *
 * Keep the hint to what somebody cannot work out from the field itself. If it
 * only restates the label, it should not be here at all.
 */
function LabelWithHint({
  htmlFor,
  children,
  hint,
  optional = false,
}: {
  htmlFor?: string
  children: React.ReactNode
  hint?: string
  optional?: boolean
}) {
  return (
    <div className="flex items-center gap-1.5">
      <Label htmlFor={htmlFor} className="text-black dark:text-white">
        {children}
        {optional && <span className="ml-1 font-normal text-muted-foreground">(optional)</span>}
      </Label>
      {hint && (
        <TooltipProvider delayDuration={200}>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={hint}
                className="text-muted-foreground transition-colors hover:text-foreground"
              >
                <Info className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="max-w-[16rem]">{hint}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
    </div>
  )
}

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
      Carried over — check it.
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
      <LabelWithHint hint={hint}>{label}</LabelWithHint>
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
  hint,
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
  /** Terse, and behind the label's info icon rather than under it. */
  hint?: string
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
      <LabelWithHint htmlFor={id} hint={hint}>
        {label}
      </LabelWithHint>
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
      <Expand open={values[field] === OTHER} gap="mt-2">
        <div className="space-y-2">
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
      </Expand>
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
  // The cropped logo, held rather than uploaded: a logo belongs to a location
  // and this one has no id yet. Posted after the listing is created, alongside
  // the hours, for the same reason and with the same forgiveness.
  //
  // Deliberately not carried over from an earlier application, unlike the
  // contact and payment answers: a logo is the mark on one shop's map pin, and
  // a second location silently inheriting the first one's is a claim about the
  // premises rather than a shared fact about the business.
  const [logo, setLogo] = useState<Blob | null>(null)
  // Grown to fit its own content rather than scrolled. Driven from an effect on
  // the value, not from the change handler, so it is also right after a reset
  // or a carried-over prefill — neither of which types anything.
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null)
  /**
   * The merchant has ticked "Can't find my location".
   *
   * Lifted out of the picker because it is one of the two things that opens the
   * rest of this step — the other being a confirmed place. Until one of them
   * happens the step is a single field, which is the whole of the happy path:
   * find your business, press Continue.
   */
  const [manualToggle, setManualToggle] = useState(false)
  /**
   * Whether the merchant wants to set hours at all. Off by default: hours are
   * optional, the Google path already publishes them, and seven days of time
   * pickers unfurled under an optional field is most of what made this step
   * look like a form to be endured.
   */
  const [settingHours, setSettingHours] = useState(false)
  const blankWeekSignature = useMemo(() => weekSignature(emptyWeek()), [])
  const hoursEntered = settingHours && weekSignature(week) !== blankWeekSignature

  useEffect(() => {
    const field = descriptionRef.current
    if (!field) return
    // Collapse first: scrollHeight only ever grows against a fixed height, so
    // deleting text would otherwise leave the box at its high-water mark.
    field.style.height = "auto"
    field.style.height = `${field.scrollHeight}px`
  }, [values.description, stepIndex])

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
      // "Fix the highlighted fields" is a summary of the field errors, so it
      // must go the moment one of them is being fixed — otherwise it sits there
      // pointing at a form with nothing highlighted.
      setFormError(null)
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
   * The merchant picked a plain street address rather than a business listing.
   *
   * Google returns the street as such a result's display name, so there is no
   * name, no category, no hours and no phone to carry — everything the Google
   * path gets for free has to be asked for instead. Reached either by picking
   * an address from the one search box or by saying up front there is nothing
   * to find.
   */
  const addressOnlySelection = placeSelection?.source === "manual"
  const manualEntry = manualToggle || addressOnlySelection
  /**
   * Whether to ask for what Google would otherwise answer.
   *
   * A confirmed business suppresses these even with the box ticked: the backend
   * re-fetches the place and overwrites name and category from it, so an
   * editable field there would take a change and silently drop it.
   */
  const showManualFields = manualEntry && placeSelection?.source !== "google_place"
  /**
   * Whether the address box drops below the name and type fields.
   *
   * Only when the merchant said up front that there is nothing to find. Then
   * they are naming the place themselves and the address is the last detail, so
   * it belongs last.
   *
   * Picking a street address from the search does NOT reorder anything, even
   * though it opens the same fields: the box is where they were just working
   * and where the amber note is pointing, and moving it out from under them
   * mid-task is disorienting. It also stays on top whenever the fields are shut
   * — a chooser that is neither above nor inside them is nowhere at all.
   */
  const chooserBelow = manualToggle && showManualFields

  /**
   * Which slot actually holds the chooser right now.
   *
   * Lags `chooserBelow` by one animation, which is what turns an instant jump
   * into a move: the slot it is leaving animates shut with the chooser still
   * inside it — sliding off to the right — and only once that has finished does
   * it reappear in the other slot and slide back in. Swapping immediately gave
   * two simultaneous half-animations of an element that was in neither place.
   */
  const [chooserSlot, setChooserSlot] = useState<"top" | "below">("top")
  useEffect(() => {
    const target = chooserBelow ? "below" : "top"
    if (target === chooserSlot) return
    const timer = setTimeout(() => setChooserSlot(target), CHOOSER_MOVE_MS)
    return () => clearTimeout(timer)
  }, [chooserBelow, chooserSlot])
  const chooserAtTop = !chooserBelow && chooserSlot === "top"
  const chooserAtBottom = chooserBelow && chooserSlot === "below"

  /**
   * Whether to offer setting hours by hand.
   *
   * Only where there is a gap to fill. A Google listing that publishes its hours
   * has already answered the question, and the nightly sync keeps that answer
   * current — offering the merchant a week of empty time pickers under it
   * invites them to retype what is already right and then diverge from it.
   *
   * The offer appears for a listing Google holds no hours for, for a plain
   * street address, and for a merchant who said up front there is nothing to
   * find. Before anything is chosen there is no gap to know about yet.
   */
  const googleSuppliedHours =
    placeSelection?.source === "google_place" && placeSelection.place.opening_hours.length > 0
  const showHoursToggle = Boolean(placeSelection || manualToggle) && !googleSuppliedHours

  // A merchant can tick the box and then pick a listing that publishes hours,
  // which takes the offer away — the answer has to go with it, or a week nobody
  // can see any more is submitted over Google's own.
  useEffect(() => {
    if (showHoursToggle) return
    setSettingHours(false)
    setHoursError("")
  }, [showHoursToggle])

  // Defined once and placed in one of two slots below, so the two positions
  // cannot drift apart — and so moving it never remounts the input, which would
  // drop focus and the typed query on the way.
  const locationChooser = (
    <div className="space-y-2">
      <LabelWithHint hint="Looked up with Google to place your map pin.">
        {manualEntry ? "Location Address" : "Find your location"}
      </LabelWithHint>
      <PlaceAutocomplete
        value={placeSelection}
        onSelect={applyPlaceSelection}
        manualEntry={manualToggle}
        onManualEntryChange={setManualToggle}
        hideStatus
      />
    </div>
  )

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

    // Only the manual path is asked for these, so only it is checked. On the
    // Google path they are not on screen, and complaining about a field the
    // merchant cannot see is the worst kind of dead end.
    if (key === "public" && showManualFields) {
      const missing: FieldErrors = {}
      if (!values.locationName.trim()) missing.locationName = "Location name is required."
      if (!values.businessType.trim()) missing.businessType = "Business type is required."
      if (Object.keys(missing).length > 0) {
        setFieldErrors((current) => ({ ...current, ...missing }))
        setFormError("Please fix the highlighted fields before continuing.")
        return false
      }
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
    if (key === "public" && showManualFields) {
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
    // Cleared on the way out as well as on the way in. A whole-form message
    // raised by step one has nothing to say about step three, and it was
    // outliving the step that raised it and reappearing under a form the
    // merchant had not touched yet.
    setFormError(null)
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
      contact_phone: formatPhone(values.contactPhone),
      admin_phone: formatPhone(values.contactPhone),
      admin_email: normalizeEmail(values.contactEmail),
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
      phone: values.publicPhone.trim() ? formatPhone(values.publicPhone) : "",
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

      // Same treatment as the hours, and for the same reason: the application
      // is already in the queue, a missing logo is something the merchant can
      // set from their locations page, and telling them the submission failed
      // over it would be false.
      if (logo && locationId > 0) {
        try {
          await uploadLocationLogo(authFetch, locationId, logo)
        } catch (error) {
          console.error("Unable to save the logo for the new location", error)
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
      {/* pb/pt carry sm: variants because Card's own defaults do (p-5 sm:p-6,
          p-5 pt-4 sm:pt-4) — an unqualified override is silently beaten by the
          breakpoint rule, which is what left 40px of air under the step rail. */}
      <CardHeader className="space-y-3 pb-3 sm:pb-3">
        <CardTitle className="text-black dark:text-white">Location Approval Form</CardTitle>
        {/* One row at every width. On a narrow screen the labels drop and the
            rail is just 1 - 2 - 3, which keeps the three steps visible as a
            sequence; stacking them cost three lines and stopped looking like
            progress at all. The title stays in the li for screen readers. */}
        {/* Two shapes from one list. Narrow: bare numbered circles joined by a
            rule, which is what a progress indicator looks like when there is no
            room for words — boxes around single digits read as three buttons.
            Wide: the boxed, labelled row. Every box class is sm:-prefixed so the
            mobile rail carries no border or fill at all, and the connectors are
            hidden the moment the boxes appear. */}
        <ol className="flex items-center sm:gap-2">
          {LOCATION_FORM_STEPS.map((entry, index) => {
            const done = index < stepIndex
            const current = index === stepIndex
            return (
              <Fragment key={entry.key}>
                {index > 0 && (
                  <li
                    aria-hidden
                    className={`mx-1.5 h-px flex-1 transition-colors sm:hidden ${
                      done || current ? "bg-[#eb6c6c]" : "bg-border"
                    }`}
                  />
                )}
                <li
                  aria-current={current ? "step" : undefined}
                  className={`flex items-center gap-2 text-sm sm:flex-1 sm:rounded-lg sm:border sm:px-3 sm:py-2 ${
                    current
                      ? "sm:border-[#eb6c6c] sm:bg-[#eb6c6c]/10 sm:font-semibold sm:text-black sm:dark:text-white"
                      : "sm:border-border/70 sm:text-muted-foreground"
                  }`}
                >
                  <span
                    className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full border text-xs font-medium transition-colors sm:h-5 sm:w-5 sm:font-normal ${
                      done || current
                        ? "border-[#eb6c6c] bg-[#eb6c6c] text-white"
                        : "border-border bg-transparent text-muted-foreground sm:border-transparent sm:bg-muted"
                    }`}
                  >
                    {done ? <Check className="h-3.5 w-3.5 sm:h-3 sm:w-3" /> : index + 1}
                  </span>
                  <span className="sr-only sm:not-sr-only">{entry.title}</span>
                </li>
              </Fragment>
            )
          })}
        </ol>
      </CardHeader>

      <CardContent className="pt-1 sm:pt-1">
        {/* space-y-4, not 6: with the helper paragraphs gone the fields are
            short, and the wider gap left the step looking emptier than it is. */}
        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          {carriedOver && carriedOver.fields.size > 0 && (
            <p className="rounded-lg border border-amber-300/70 bg-amber-50 p-3 text-sm text-black dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-white">
              Some answers carried over from {carriedOver.sourceName || "your other location"}.
              Check them before submitting.
            </p>
          )}

          {step.key === "public" && (
            <div className="space-y-4">
              {/* z-30 so the suggestion list paints over the fields below it,
                  the map-pin preview in particular.
                  
                  The dropdown's own z-50 cannot do this on its own: `slide`
                  applies a transform, a transform creates a stacking context,
                  and a z-index inside one only orders against its siblings
                  there. The context itself was at z-auto, so anything rendered
                  after it in the step simply painted on top. Ordering the
                  context is what the dropdown actually needed. */}
              <Expand open={chooserAtTop} gap="" slide className="relative z-30">
                {chooserSlot === "top" ? locationChooser : null}
              </Expand>

              {/* Never moves and never unmounts. It sits directly under the
                  search box while the box is at the top, and becomes the first
                  thing on the step — above the name and type fields — the
                  moment the box collapses away to its lower slot. One fixed
                  position gives both placements, so the control the merchant
                  just clicked does not vanish out from under the click. */}
              <PlaceSelectionStatus
                value={placeSelection}
                manualEntry={manualToggle}
                onManualEntryChange={setManualToggle}
              />

              {/* Ordered for the same reason as the slot inside it: the lower
                  chooser's z-30 only competes within whatever context encloses
                  it, so the enclosure needs to outrank the logo and description
                  below as well. */}
              <Expand open={showManualFields} className="relative z-20">
                <div className="space-y-4">
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div className="space-y-2">
                      <LabelWithHint htmlFor="location-name">Location Name</LabelWithHint>
                      <Input
                        id="location-name"
                        value={values.locationName}
                        onChange={(event) => setField("locationName", event.target.value)}
                        className="bg-secondary text-black dark:text-white"
                        placeholder="Your business name"
                      />
                      <FieldError message={fieldErrors.locationName} />
                    </div>

                    <div className="space-y-2">
                      <LabelWithHint htmlFor="business-type">Business Type</LabelWithHint>
                      <Input
                        id="business-type"
                        value={values.businessType}
                        onChange={(event) => setField("businessType", event.target.value)}
                        className="bg-secondary text-black dark:text-white"
                        placeholder="Cafe, bookshop, barber"
                      />
                      <FieldError message={fieldErrors.businessType} />
                    </div>
                  </div>

                  {/* The box's lower home. The status above stays where it is
                      throughout, so only the box itself travels. */}
                  <Expand open={chooserAtBottom} gap="" slide className="relative z-30">
                    {chooserSlot === "below" ? locationChooser : null}
                  </Expand>

                  <div className="space-y-2">
                    <LabelWithHint
                      htmlFor="public-phone"
                      optional
                      hint="Shown on the map. Need not match your contact phone."
                    >
                      Public Phone
                    </LabelWithHint>
                    <Input
                      id="public-phone"
                      value={values.publicPhone}
                      onChange={(event) => setField("publicPhone", event.target.value)}
                      onBlur={(event) => setField("publicPhone", formatPhone(event.target.value))}
                      className="bg-secondary text-black dark:text-white"
                      placeholder="Number for customers"
                      inputMode="tel"
                    />
                    <FieldError message={fieldErrors.publicPhone} />
                  </div>
                </div>
              </Expand>

              {/* Both of these are the merchant's own and nothing fills them in,
                  so they are here from the start on either path. */}
              <LocationLogoField
                locationName={values.locationName || (placeSelection?.source === "google_place"
                  ? placeSelection.place.name
                  : "")}
                value={logo}
                onChange={setLogo}
                disabled={isSubmitting}
              />

              <div className="space-y-2">
                <LabelWithHint htmlFor="location-description" optional hint="Shown on the map.">
                  Location Description
                </LabelWithHint>
                {/* One line to begin with, growing to fit whatever is typed.
                    Most descriptions are a phrase, and a fixed three-line box
                    reserved room for an essay nobody was writing — while still
                    capping anyone who wanted one. */}
                <Textarea
                  id="location-description"
                  ref={descriptionRef}
                  rows={1}
                  value={values.description}
                  onChange={(event) => setField("description", event.target.value)}
                  className="min-h-0 resize-none overflow-hidden bg-secondary text-black dark:text-white"
                  placeholder="What you sell"
                />
                <FieldError message={fieldErrors.description} />
              </div>

              <Expand open={showHoursToggle} gap="">
                {/* Collapsed behind its own tick, and only offered where Google has
                    left a gap: seven days of time pickers over hours Google
                    already publishes invites a merchant to retype what is
                    already right and then diverge from it. */}
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <Checkbox
                      id="set-location-hours"
                      checked={settingHours}
                      disabled={isSubmitting}
                      onCheckedChange={(checked) => {
                        setSettingHours(checked === true)
                        setHoursError("")
                      }}
                    />
                    <Label
                      htmlFor="set-location-hours"
                      className="cursor-pointer text-black dark:text-white"
                    >
                      Set location hours
                    </Label>
                    <TooltipProvider delayDuration={200}>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button
                            type="button"
                            aria-label="We have no hours for this location. Leave unticked to publish none."
                            className="text-muted-foreground transition-colors hover:text-foreground"
                          >
                            <Info className="h-3.5 w-3.5" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-[16rem]">
                          We have no hours for this location. Leave unticked to publish none.
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </div>

                  <Expand open={settingHours} gap="mt-3">
                    <div className="space-y-3">
                      <OpeningHoursEditor
                        week={week}
                        onChange={(next) => {
                          setWeek(next)
                          setHoursError("")
                        }}
                        // The nightly-sync switch is not offered during an
                        // application: ticking the box above is already the
                        // decision to keep these, and a listing with no Google
                        // entry has nothing to sync from.
                        manual
                        onManualChange={() => undefined}
                        showManualToggle={false}
                        disabled={isSubmitting}
                      />
                      <FieldError message={hoursError || undefined} />
                    </div>
                  </Expand>
                </div>
              </Expand>
            </div>
          )}

          {step.key === "contact" && (
            <div className="space-y-4">
              {/* Who sees this is the one thing somebody needs before typing a
                  personal number in, and it is a property of the whole step
                  rather than of any one field — so it hangs off the step's own
                  heading in the rail above, not over the first input. */}
              <div className="space-y-2">
                <LabelWithHint
                  htmlFor="contact-name"
                  hint="Internal only. Never shown publicly."
                >
                  Contact Name
                </LabelWithHint>
                <Input
                  id="contact-name"
                  value={values.contactName}
                  onChange={(event) => setField("contactName", event.target.value)}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="Who to ask for"
                />
                <FieldError message={fieldErrors.contactName} />
                <CarriedOverAnswer show={carriedOverFields.has("contactName")} />
              </div>

              <div className="space-y-2">
                <LabelWithHint
                  htmlFor="contact-phone"
                  hint="Internal only. Need not match your public phone."
                >
                  Contact Phone
                </LabelWithHint>
                <Input
                  id="contact-phone"
                  value={values.contactPhone}
                  onChange={(event) => setField("contactPhone", event.target.value)}
                  // Reformatted when they leave the field, not as they type:
                  // rewriting under a moving cursor fights whoever is typing.
                  onBlur={(event) => setField("contactPhone", formatPhone(event.target.value))}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="(415) 555-1234"
                  inputMode="tel"
                />
                <FieldError message={fieldErrors.contactPhone} />
                <CarriedOverAnswer show={carriedOverFields.has("contactPhone")} />
              </div>

              <div className="space-y-2">
                <LabelWithHint htmlFor="contact-email" hint="Internal only. Where approval mail goes.">
                  Contact Email
                </LabelWithHint>
                <Input
                  id="contact-email"
                  type="email"
                  value={values.contactEmail}
                  onChange={(event) => setField("contactEmail", event.target.value)}
                  onBlur={(event) => setField("contactEmail", normalizeEmail(event.target.value))}
                  className="bg-secondary text-black dark:text-white"
                  placeholder="you@yourbusiness.com"
                  inputMode="email"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                />
                <FieldError message={fieldErrors.contactEmail} />
                <CarriedOverAnswer show={carriedOverFields.has("contactEmail")} />
              </div>

              <SelectWithOther
                id="referral-source"
                label="How did you hear about SFLuv?"
                placeholder="Select"
                options={referralOptions}
                field="referralSource"
                otherField="referralSourceOther"
                otherLabel="How?"
                otherPlaceholder="How you heard about SFLuv"
                values={values}
                fieldErrors={fieldErrors}
                setField={setField}
                carriedOver={carriedOverFields.has("referralSource")}
              />
            </div>
          )}

          {step.key === "payment" && (
            <div className="space-y-4">
              <SelectWithOther
                id="pos-system"
                label="POS Type"
                placeholder="Select"
                options={posOptions}
                field="posSystem"
                otherField="posSystemOther"
                otherLabel="Which system?"
                otherPlaceholder="Your point of sale system"
                values={values}
                fieldErrors={fieldErrors}
                setField={setField}
                carriedOver={carriedOverFields.has("posSystem")}
              />

              <YesNoField
                id="accepts-tips"
                label="Do you accept tips?"
                hint="Yes gives this location its own tipping wallet at approval."
                value={values.acceptsTips}
                onChange={(value) => setField("acceptsTips", value)}
                error={fieldErrors.acceptsTips}
                carriedOver={carriedOverFields.has("acceptsTips")}
                disabled={isSubmitting}
              />

              <YesNoField
                id="staff-tablet"
                label="Tablet or phone available to staff?"
                hint="A shared device at the counter is how staff take SFLuv payments."
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
