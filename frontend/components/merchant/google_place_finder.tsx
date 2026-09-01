"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { CheckCircle2, Loader2, Search } from "lucide-react";

import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { LAT_DIF, LNG_DIF, MAP_CENTER } from "@/lib/constants";
import { GoogleSubLocation, ManualAddressDraft, PlaceSelection } from "@/types/location";

interface PlaceAutocompleteProps {
  value: PlaceSelection | null;
  onSelect: (selection: PlaceSelection | null) => void;
  /**
   * The "can't find my location" tick, owned by the caller.
   *
   * Controllable rather than always held here, because the Location Approval
   * Form moves this component between two positions when it is set, and a move
   * is an unmount and a remount — local state would reset to false on the very
   * render the tick caused, leaving the box unticked under a form that had
   * reorganised itself for a ticked box, and unticking impossible.
   *
   * Omitted by the callers that only need a picker — the admin editor, the
   * settings profile card, the older single-sheet form — which keep the state
   * here and never move the component.
   */
  manualEntry?: boolean;
  onManualEntryChange?: (manualEntry: boolean) => void;
  /**
   * Draw the search box only, leaving the caller to place the status and the
   * "can't find my location" tick.
   *
   * The Location Approval Form does this because that control must hold still
   * while the box itself moves: rendered inside, it travels with the box and
   * disappears mid-move, which is the one thing it must not do.
   */
  hideStatus?: boolean;
}

interface Suggestion {
  id: string;
  primary: string;
  secondary: string;
  /** Google's own prediction object, kept to resolve the full place on select. */
  prediction: any;
}

/**
 * Places types that describe a postal address rather than a business. Google
 * returns the street address itself as `displayName` for these, so accepting
 * one silently names the merchant after their address. Mirrors
 * addressOnlyPlaceTypes in backend/handlers/google_places.go.
 */
const ADDRESS_ONLY_TYPES = new Set([
  "street_address", "street_number", "route", "intersection", "premise",
  "subpremise", "plus_code", "postal_code", "postal_code_prefix",
  "postal_code_suffix", "geocode", "locality", "sublocality",
  "sublocality_level_1", "sublocality_level_2", "neighborhood",
  "administrative_area_level_1", "administrative_area_level_2",
  "administrative_area_level_3", "country", "political", "floor", "room",
  "post_box",
]);

// No iconography is requested, and none may be added.
//
// A location's logo is ours: it is uploaded by the merchant against that
// location and stored in location_icons, and where there is none the pin draws
// generated initials in SFLuv colours. Google's svgIconMaskURI used to be
// fetched here and was never read — a category glyph standing in for a
// merchant's mark is not a logo, it is a guess with Google's licensing
// attached, and a field nobody uses is how one quietly starts being used.
const PLACE_FIELDS = [
  "id", "displayName", "addressComponents", "formattedAddress", "location", "rating",
  "regularOpeningHours", "websiteURI", "primaryTypeDisplayName", "nationalPhoneNumber",
  "googleMapsURI", "photos", "types", "businessStatus",
];

const isBusinessPlace = (types: string[] | undefined): boolean => {
  if (!types?.length) return false;
  return types.some((type) => !ADDRESS_ONLY_TYPES.has(type));
};

const localizedText = (value: any): string => {
  if (typeof value === "string") return value;
  return value?.text || "";
};

const addressPart = (rawGoogleData: any, type: string): string =>
  rawGoogleData.addressComponents?.find((part: any) => part.types?.includes(type))?.longText || "";

/** Maps a raw Places result onto the shape the form submits, or null if unusable. */
const toGoogleSubLocation = (rawGoogleData: any): GoogleSubLocation | null => {
  const lat = rawGoogleData.location?.lat ?? rawGoogleData.location?.latitude;
  const lng = rawGoogleData.location?.lng ?? rawGoogleData.location?.longitude;
  if (!rawGoogleData.id || typeof lat !== "number" || typeof lng !== "number") return null;

  const name = localizedText(rawGoogleData.displayName);
  if (!name) return null;

  const street = [addressPart(rawGoogleData, "street_number"), addressPart(rawGoogleData, "route")]
    .filter(Boolean)
    .join(" ");

  return {
    google_id: rawGoogleData.id,
    name,
    type: localizedText(rawGoogleData.primaryTypeDisplayName),
    street,
    city: addressPart(rawGoogleData, "locality"),
    state: addressPart(rawGoogleData, "administrative_area_level_1"),
    zip: addressPart(rawGoogleData, "postal_code"),
    lat,
    lng,
    phone: rawGoogleData.nationalPhoneNumber || "",
    website: rawGoogleData.websiteURI || "",
    image_url: rawGoogleData.photos?.[0]?.googleMapsURI || "",
    rating: rawGoogleData.rating || 0,
    maps_page: rawGoogleData.googleMapsURI || "",
    opening_hours: rawGoogleData.regularOpeningHours?.weekdayDescriptions || [],
    types: rawGoogleData.types || [],
    formatted_address: rawGoogleData.formattedAddress || "",
  };
};

/** Keeps only the postal half of a geocode result. */
const toManualAddress = (rawGoogleData: any): ManualAddressDraft | null => {
  const lat = rawGoogleData.location?.lat ?? rawGoogleData.location?.latitude;
  const lng = rawGoogleData.location?.lng ?? rawGoogleData.location?.longitude;
  if (typeof lat !== "number" || typeof lng !== "number") return null;

  const street = [addressPart(rawGoogleData, "street_number"), addressPart(rawGoogleData, "route")]
    .filter(Boolean)
    .join(" ");
  if (!street) return null;

  return {
    street,
    city: addressPart(rawGoogleData, "locality"),
    state: addressPart(rawGoogleData, "administrative_area_level_1"),
    zip: addressPart(rawGoogleData, "postal_code"),
    lat,
    lng,
    formatted_address: rawGoogleData.formattedAddress || "",
  };
};

/**
 * The location finder, drawn by us.
 *
 * Google's `PlaceAutocompleteElement` renders its own input and dropdown inside
 * a shadow root, which cannot be styled past the handful of parts it exposes —
 * and on a narrow screen it takes the whole viewport with its own chrome, which
 * is a Google screen appearing in the middle of an SFLuv form. So this asks the
 * Places *data* API for predictions and renders them itself: one `<Input>` and
 * a list, in the app's own components, identical on every width.
 *
 * Nothing is shown about a place once it has been chosen. The input holds its
 * name and everything else — address, coordinates, category, hours, the place
 * id the backend re-verifies against — is carried in state to the submission.
 * A confirmation card restating Google's answer was information the merchant
 * had not asked for and could not act on.
 */
/** What the input should read for an already-confirmed selection. */
const queryFor = (selection: PlaceSelection | null): string => {
  if (!selection) return "";
  if (selection.source === "google_place") return selection.place.name;
  return selection.address.formatted_address || selection.address.street;
};

export default function PlaceAutocomplete({
  value,
  onSelect,
  manualEntry: controlledManualEntry,
  onManualEntryChange,
  hideStatus = false,
}: PlaceAutocompleteProps) {
  const [uncontrolledManualEntry, setUncontrolledManualEntry] = useState(false);
  const manualEntry = controlledManualEntry ?? uncontrolledManualEntry;
  // Seeded from the confirmed selection, so a remount — which is what moving
  // between the two positions is — comes back showing what was chosen rather
  // than an empty box beside a green "Location found".
  const [query, setQuery] = useState(() => queryFor(value));
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [open, setOpen] = useState(false);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");

  // One token spans a whole type-then-pick sequence and is replaced after each
  // selection; that is how Places bills a session rather than per keystroke.
  const sessionTokenRef = useRef<any>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  // Guards against a slow response for an earlier query overwriting a later one.
  const requestSeqRef = useRef(0);

  // A BIAS, not a restriction. As a restriction this box was a hard filter: a
  // business a few streets outside it simply never appeared, the merchant
  // concluded they were not on Google Maps, and typed their address into the
  // name field instead — the failure this whole component exists to prevent.
  const locationBias = {
    south: MAP_CENTER.lat - LAT_DIF,
    west: MAP_CENTER.lng - LNG_DIF,
    north: MAP_CENTER.lat + LAT_DIF,
    east: MAP_CENTER.lng + LNG_DIF,
  };

  useEffect(() => {
    const onPointerDown = (event: MouseEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    return () => document.removeEventListener("mousedown", onPointerDown);
  }, []);

  const fetchSuggestions = useCallback(
    async (input: string) => {
      const seq = ++requestSeqRef.current;
      if (input.trim().length < 2) {
        setSuggestions([]);
        setSearching(false);
        return;
      }

      try {
        setSearching(true);
        const places = (await google.maps.importLibrary("places")) as any;
        const { AutocompleteSuggestion, AutocompleteSessionToken } = places;
        if (!sessionTokenRef.current) {
          sessionTokenRef.current = new AutocompleteSessionToken();
        }

        // No type restriction: one box that takes a business or a street
        // address, because a merchant does not know in advance which of the two
        // Google holds for them. Which one they picked is decided on selection,
        // from the place's own types, and decides the path they are on.
        const response = await AutocompleteSuggestion.fetchAutocompleteSuggestions({
          input,
          sessionToken: sessionTokenRef.current,
          locationBias,
        });

        // A response for a query the merchant has already typed past is stale.
        if (seq !== requestSeqRef.current) return;

        setSuggestions(
          (response?.suggestions ?? [])
            .map((entry: any) => entry.placePrediction)
            .filter(Boolean)
            .map((prediction: any) => ({
              id: prediction.placeId,
              primary: localizedText(prediction.mainText) || localizedText(prediction.text),
              secondary: localizedText(prediction.secondaryText),
              prediction,
            })),
        );
        setSearchError("");
      } catch {
        if (seq !== requestSeqRef.current) return;
        setSuggestions([]);
        setSearchError("Could not reach Google to search. Check your connection and try again.");
      } finally {
        if (seq === requestSeqRef.current) setSearching(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- locationBias is a
    // fresh literal each render but its values are module constants.
    [],
  );

  // Debounced: a request per keystroke is both slow and billable.
  useEffect(() => {
    if (!open) return;
    const timer = setTimeout(() => void fetchSuggestions(query), 220);
    return () => clearTimeout(timer);
  }, [query, open, fetchSuggestions]);

  const choose = async (suggestion: Suggestion) => {
    setOpen(false);
    setSearching(true);
    setSearchError("");
    try {
      const place = suggestion.prediction.toPlace();
      await place.fetchFields({ fields: PLACE_FIELDS });
      const raw = place.toJSON();

      // The result's own types decide the path, not a mode the merchant had to
      // choose beforehand. A business carries its name, category, hours and
      // phone; a postal address carries none of those and Google returns the
      // street as its display name, so the name is dropped rather than
      // inherited — a listing on the map called "1234 Main St" is the exact
      // failure this whole component exists to prevent.
      const mapped = toGoogleSubLocation(raw);
      if (mapped && isBusinessPlace(mapped.types)) {
        setQuery(mapped.name);
        onSelect({ source: "google_place", place: mapped });
      } else {
        const address = toManualAddress(raw);
        if (!address) {
          setSearchError("Google returned no street address for that result. Try a more specific one.");
          return;
        }
        setQuery(address.formatted_address || address.street);
        onSelect({ source: "manual", address });
      }

      // A completed selection ends the billing session.
      sessionTokenRef.current = null;
      setSuggestions([]);
    } catch {
      setSearchError("Could not load that place. Please try again.");
    } finally {
      setSearching(false);
    }
  };

  const toggleManualEntry = (next: boolean) => {
    setSearchError("");
    if (controlledManualEntry === undefined) setUncontrolledManualEntry(next);
    onManualEntryChange?.(next);
  };

  return (
    <div className="space-y-2" ref={containerRef}>
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setOpen(true);
            // Typing past a confirmed place clears it: what is in the box and
            // what will be submitted must not disagree.
            if (value) onSelect(null);
          }}
          onFocus={() => setOpen(true)}
          className="bg-secondary pl-9 pr-9 text-black dark:text-white"
          placeholder={
            manualEntry ? "Start typing your street address" : "Search for your business or address"
          }
          autoComplete="off"
          spellCheck={false}
        />
        {searching && (
          <Loader2 className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
        )}

        {open && suggestions.length > 0 && (
          <ul className="absolute z-50 mt-1 max-h-72 w-full overflow-y-auto rounded-lg border border-border bg-popover py-1 shadow-lg">
            {suggestions.map((suggestion) => (
              <li key={suggestion.id}>
                <button
                  type="button"
                  className="w-full px-3 py-2 text-left transition-colors hover:bg-muted focus:bg-muted focus:outline-none"
                  onClick={() => void choose(suggestion)}
                >
                  <span className="block truncate text-sm text-foreground">{suggestion.primary}</span>
                  {suggestion.secondary && (
                    <span className="block truncate text-xs text-muted-foreground">
                      {suggestion.secondary}
                    </span>
                  )}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {searchError && <p className="text-xs text-red-600 dark:text-red-300">{searchError}</p>}

      {!hideStatus && (
        <PlaceSelectionStatus
          value={value}
          manualEntry={manualEntry}
          onManualEntryChange={toggleManualEntry}
        />
      )}
    </div>
  );
}

/**
 * Which of the two kinds of result was chosen, or the way out for a merchant
 * who knows there is none.
 *
 * One slot, two jobs. With nothing chosen it offers the tick; with something
 * chosen it says which kind it was, because that is the difference between the
 * form filling itself in and the merchant filling it in. The tick is only
 * reachable while nothing is selected — ticking it against a confirmed place
 * would be claiming not to have found the thing sitting in the box.
 */
export function PlaceSelectionStatus({
  value,
  manualEntry,
  onManualEntryChange,
}: {
  value: PlaceSelection | null;
  manualEntry: boolean;
  onManualEntryChange: (manualEntry: boolean) => void;
}) {
  if (value) {
    return (
      <div className="flex items-start gap-2">
        {value.source === "google_place" ? (
          <>
            <CheckCircle2 className="mt-px h-3.5 w-3.5 shrink-0 text-green-600 dark:text-green-500" />
            <span className="text-xs font-medium text-green-700 dark:text-green-500">
              Location found
            </span>
          </>
        ) : (
          <>
            <CheckCircle2 className="mt-px h-3.5 w-3.5 shrink-0 text-amber-500" />
            <div className="space-y-1">
              <span className="block text-xs font-medium text-amber-600 dark:text-amber-500">
                Address found
              </span>
              {/* Amber rather than green, and this sentence, because an address
                  is the weaker of the two answers: a business listing would
                  carry the name, the category, the hours and the phone, and put
                  the shop on the map under the name customers search for. */}
              <span className="block text-xs text-gray-600 dark:text-gray-400">
                If your business has its own Google listing, search for it by name instead — we can
                fill in far more for you.
              </span>
            </div>
          </>
        )}
      </div>
    );
  }

  return (
    <label className="flex cursor-pointer items-center gap-2">
      <Checkbox
        checked={manualEntry}
        onCheckedChange={(checked) => onManualEntryChange(checked === true)}
      />
      <span className="text-xs text-gray-600 dark:text-gray-400">Can&apos;t find my location</span>
    </label>
  );
}
