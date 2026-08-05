"use client"; // for Next.js App Router

import { useEffect, useRef, useState } from "react";
import { LAT_DIF, LNG_DIF, MAP_CENTER } from "@/lib/constants";
import { GoogleSubLocation } from "@/types/location";
import { Check, MapPin, Search } from "lucide-react";

interface PlaceAutocompleteProps {
  value: GoogleSubLocation | null;
  onSelect: (place: GoogleSubLocation | null) => void;
}

// Places types that describe a postal address rather than a business. Google
// returns the street address itself as `displayName` for these, so accepting one
// silently names the merchant after their address. Mirrors
// addressOnlyPlaceTypes in backend/handlers/google_places.go.
const ADDRESS_ONLY_TYPES = new Set([
  "street_address",
  "street_number",
  "route",
  "intersection",
  "premise",
  "subpremise",
  "plus_code",
  "postal_code",
  "postal_code_prefix",
  "postal_code_suffix",
  "geocode",
  "locality",
  "sublocality",
  "sublocality_level_1",
  "sublocality_level_2",
  "neighborhood",
  "administrative_area_level_1",
  "administrative_area_level_2",
  "administrative_area_level_3",
  "country",
  "political",
  "floor",
  "room",
  "post_box",
]);

const PLACE_FIELDS = [
  "id", "displayName", "addressComponents", "formattedAddress", "location", "rating",
  "regularOpeningHours", "websiteURI", "primaryTypeDisplayName", "nationalPhoneNumber",
  "googleMapsURI", "photos", "svgIconMaskURI", "types", "businessStatus",
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

// toGoogleSubLocation maps a raw Places result onto the shape the form submits.
// Returns null when the result cannot be used as a merchant record.
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

const formatAddress = (place: GoogleSubLocation): string => {
  if (place.formatted_address) return place.formatted_address;
  const cityLine = [place.city, place.state].filter(Boolean).join(", ");
  return [place.street, cityLine, place.zip].filter(Boolean).join(" · ");
};

export default function PlaceAutocomplete({ value, onSelect }: PlaceAutocompleteProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [textSearch, setTextSearch] = useState("");
  const [textSearchBusy, setTextSearchBusy] = useState(false);
  const [textSearchError, setTextSearchError] = useState("");
  const [candidates, setCandidates] = useState<GoogleSubLocation[]>([]);

  const locationRestriction = {
    south: MAP_CENTER.lat - LAT_DIF,
    west: MAP_CENTER.lng - LNG_DIF,
    north: MAP_CENTER.lat + LAT_DIF,
    east: MAP_CENTER.lng + LNG_DIF,
  };

  const selectPlace = (place: GoogleSubLocation | null) => {
    if (!place) {
      setTextSearchError("Google returned this place without a name, place ID, or coordinates. Try another search.");
      return;
    }
    if (!isBusinessPlace(place.types)) {
      setTextSearchError(
        "That result is a street address, not a business listing. Search for your business by name — for example \"Shiba SF\" instead of the street address.",
      );
      return;
    }
    setTextSearchError("");
    setCandidates([]);
    onSelect(place);
  };

  const searchByText = async () => {
    const query = textSearch.trim();
    if (!query) return;

    setTextSearchBusy(true);
    setTextSearchError("");
    setCandidates([]);
    try {
      const { Place } = await google.maps.importLibrary("places") as google.maps.PlacesLibrary;
      const { places } = await Place.searchByText({
        textQuery: query,
        fields: PLACE_FIELDS,
        locationRestriction,
        maxResultCount: 5,
      } as any);

      // Never auto-apply the top hit. Blindly taking places[0] is how a search
      // that resolves to an address became a merchant name.
      const usable = (places ?? [])
        .map((place) => toGoogleSubLocation(place.toJSON()))
        .filter((place): place is GoogleSubLocation => place !== null && isBusinessPlace(place.types));

      if (!usable.length) {
        setTextSearchError("No matching business found. Try the business name plus the city.");
        return;
      }
      setCandidates(usable);
    } catch (error) {
      console.error(error);
      setTextSearchError("Unable to search Google Places right now.");
    } finally {
      setTextSearchBusy(false);
    }
  };

  useEffect(() => {
    if (!containerRef.current) return;
    if (value) return;

    let cancelled = false;
    const container = containerRef.current;

    const init = async () => {
      await google.maps.importLibrary("places") as google.maps.PlacesLibrary;
      if (cancelled || container.querySelector("gmp-place-autocomplete")) return;

      // `includedPrimaryTypes` is not in the published typings yet, so the
      // options are built untyped. Establishments only: without this Google
      // mixes address (geocode) predictions into the list, and picking one
      // yields a place whose display name is the street address.
      const autocompleteOptions: any = {
        locationRestriction,
        includedPrimaryTypes: ["establishment"],
      };

      //@ts-ignore - PlaceAutocompleteElement is not in the published typings yet
      const placeAutocomplete = new google.maps.places.PlaceAutocompleteElement(autocompleteOptions);

      //@ts-ignore
      placeAutocomplete.addEventListener("gmp-select", async ({ placePrediction }) => {
        const place = placePrediction.toPlace();
        await place.fetchFields({ fields: PLACE_FIELDS });
        selectPlace(toGoogleSubLocation(place.toJSON()));
      });
      placeAutocomplete.className = "text-black dark:text-white border rounded-md bg-secondary px-3 py-2";

      container.appendChild(placeAutocomplete);
    };

    void init();

    return () => {
      cancelled = true;
    };
  }, [value]);

  const clearSelection = () => {
    setTextSearch("");
    setTextSearchError("");
    setCandidates([]);
    onSelect(null);
  };

  // Confirmation view — the merchant sees exactly what Google resolved to
  // before any of it is submitted.
  if (value) {
    return (
      <div className="rounded-md border border-green-600/40 bg-green-50 p-4 dark:bg-green-900/20">
        <div className="flex items-start gap-3">
          <Check className="mt-0.5 h-5 w-5 flex-shrink-0 text-green-600" />
          <div className="min-w-0 flex-1 space-y-1">
            <p className="font-semibold text-black dark:text-white">{value.name}</p>
            <p className="text-sm text-gray-600 dark:text-gray-300">{formatAddress(value)}</p>
            {value.type ? (
              <p className="text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">{value.type}</p>
            ) : null}
            {value.phone ? (
              <p className="text-sm text-gray-600 dark:text-gray-300">{value.phone}</p>
            ) : null}
            {value.maps_page ? (
              <a
                className="inline-block text-sm text-[#eb6c6c] underline"
                href={value.maps_page}
                rel="noreferrer"
                target="_blank"
              >
                View on Google Maps
              </a>
            ) : null}
          </div>
        </div>
        <button
          className="mt-3 rounded-md border px-3 py-1.5 text-sm text-black dark:text-white"
          onClick={clearSelection}
          type="button"
        >
          Not your business? Search again
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div ref={containerRef} />
      <div className="flex gap-2">
        <input
          className="min-w-0 flex-1 rounded-md border bg-secondary px-3 py-2 text-sm text-black dark:text-white"
          placeholder="Business name (add the city if the list is empty)"
          value={textSearch}
          onChange={(event) => setTextSearch(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              void searchByText();
            }
          }}
        />
        <button
          className="flex items-center gap-1.5 rounded-md border px-3 py-2 text-sm text-black disabled:opacity-50 dark:text-white"
          disabled={textSearchBusy}
          onClick={() => void searchByText()}
          type="button"
        >
          <Search className="h-4 w-4" />
          {textSearchBusy ? "Searching..." : "Search"}
        </button>
      </div>

      {candidates.length > 0 && (
        <ul className="divide-y rounded-md border">
          {candidates.map((candidate) => (
            <li key={candidate.google_id}>
              <button
                className="flex w-full items-start gap-2 px-3 py-2 text-left hover:bg-secondary"
                onClick={() => selectPlace(candidate)}
                type="button"
              >
                <MapPin className="mt-0.5 h-4 w-4 flex-shrink-0 text-gray-500" />
                <span className="min-w-0">
                  <span className="block text-sm font-medium text-black dark:text-white">{candidate.name}</span>
                  <span className="block text-xs text-gray-600 dark:text-gray-400">{formatAddress(candidate)}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {textSearchError ? (
        <p className="text-xs text-red-600 dark:text-red-300">{textSearchError}</p>
      ) : null}
    </div>
  );
}
