"use client"; // for Next.js App Router

import { useEffect, useRef, useState } from "react";
import { LAT_DIF, LNG_DIF, MAP_CENTER } from "@/lib/constants";
import { GoogleSubLocation, ManualAddressDraft, PlaceSelection } from "@/types/location";
import { Check, MapPin } from "lucide-react";

interface PlaceAutocompleteProps {
  value: PlaceSelection | null;
  onSelect: (selection: PlaceSelection | null) => void;
}

type Mode = "business" | "address";

// Address-mode fields. No displayName: a geocode result's display name IS the
// street address, and inheriting it is precisely the bug this mode exists to
// avoid. The merchant types their business name into the form instead.
const ADDRESS_FIELDS = ["id", "addressComponents", "formattedAddress", "location"];

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

const formatAddress = (place: GoogleSubLocation | ManualAddressDraft): string => {
  if (place.formatted_address) return place.formatted_address;
  const cityLine = [place.city, place.state].filter(Boolean).join(", ");
  return [place.street, cityLine, place.zip].filter(Boolean).join(" · ");
};

// toManualAddress keeps only the postal half of a geocode result. Returns null
// when Google gave us nothing usable to place a pin with.
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

export default function PlaceAutocomplete({ value, onSelect }: PlaceAutocompleteProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [mode, setMode] = useState<Mode>("business");
  const [searchError, setSearchError] = useState("");

  // A BIAS, not a restriction. As a restriction this box was a hard filter:
  // a business a few streets outside it simply never appeared, the merchant
  // concluded they were not on Google Maps, and typed their address into the
  // name field instead — the failure this whole component exists to prevent.
  // Biasing ranks nearby results first while still finding everything, and the
  // service-area check at submit is what actually enforces where we operate.
  const locationBias = {
    south: MAP_CENTER.lat - LAT_DIF,
    west: MAP_CENTER.lng - LNG_DIF,
    north: MAP_CENTER.lat + LAT_DIF,
    east: MAP_CENTER.lng + LNG_DIF,
  };

  const selectPlace = (place: GoogleSubLocation | null) => {
    if (!place) {
      setSearchError("Google returned this place without a name, place ID, or coordinates. Try another search.");
      return;
    }
    if (!isBusinessPlace(place.types)) {
      setSearchError(
        "That result is a street address, not a business listing. Pick your business by name, or use \"My business isn't on Google Maps\" below.",
      );
      return;
    }
    setSearchError("");
    onSelect({ source: "google_place", place });
  };

  const selectAddress = (address: ManualAddressDraft | null) => {
    if (!address) {
      setSearchError("Google returned no street address or coordinates for that result. Try a more specific address.");
      return;
    }
    setSearchError("");
    onSelect({ source: "manual", address });
  };

  const switchMode = (next: Mode) => {
    setMode(next);
    setSearchError("");
  };

  useEffect(() => {
    if (!containerRef.current) return;
    if (value) return;

    let cancelled = false;
    const container = containerRef.current;

    // Rebuilt whenever the mode changes: the element's type restriction is fixed
    // at construction, so business and address modes need separate elements.
    container.replaceChildren();

    const init = async () => {
      await google.maps.importLibrary("places") as google.maps.PlacesLibrary;
      if (cancelled || container.querySelector("gmp-place-autocomplete")) return;

      // `includedPrimaryTypes` is not in the published typings yet, so the
      // options are built untyped.
      //
      // Business mode is establishments only: without that restriction Google
      // mixes address (geocode) predictions into the list, and picking one
      // yields a place whose display name is the street address.
      //
      // Address mode is the deliberate inverse — it asks for addresses, and the
      // caller uses only the postal fields, never a name.
      const autocompleteOptions: any = {
        locationBias,
        includedPrimaryTypes: mode === "business" ? ["establishment"] : ["street_address", "premise", "subpremise"],
      };

      //@ts-ignore - PlaceAutocompleteElement is not in the published typings yet
      const placeAutocomplete = new google.maps.places.PlaceAutocompleteElement(autocompleteOptions);

      //@ts-ignore
      placeAutocomplete.addEventListener("gmp-select", async ({ placePrediction }) => {
        const place = placePrediction.toPlace();
        if (mode === "business") {
          await place.fetchFields({ fields: PLACE_FIELDS });
          selectPlace(toGoogleSubLocation(place.toJSON()));
          return;
        }
        await place.fetchFields({ fields: ADDRESS_FIELDS });
        selectAddress(toManualAddress(place.toJSON()));
      });
      placeAutocomplete.className = "text-black dark:text-white border rounded-md bg-secondary px-3 py-2";

      container.appendChild(placeAutocomplete);
    };

    void init();

    return () => {
      cancelled = true;
    };
  }, [value, mode]);

  const clearSelection = () => {
    setSearchError("");
    onSelect(null);
  };

  // Confirmation view — the merchant sees exactly what was resolved before any
  // of it is submitted. The manual variant deliberately shows no business name:
  // there isn't one yet, and saying so here is what sends them to the name field.
  if (value?.source === "manual") {
    const address = value.address;
    return (
      <div className="rounded-md border border-amber-600/40 bg-amber-50 p-4 dark:bg-amber-900/20">
        <div className="flex items-start gap-3">
          <MapPin className="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-600" />
          <div className="min-w-0 flex-1 space-y-1">
            <p className="font-semibold text-black dark:text-white">Address confirmed</p>
            <p className="text-sm text-gray-600 dark:text-gray-300">{formatAddress(address)}</p>
            <p className="text-xs text-gray-600 dark:text-gray-400">
              This address has no Google business listing, so enter your business name and category below. They will
              appear on the SFLuv map exactly as you type them.
            </p>
          </div>
        </div>
        <button
          className="mt-3 rounded-md border px-3 py-1.5 text-sm text-black dark:text-white"
          onClick={clearSelection}
          type="button"
        >
          Wrong address? Search again
        </button>
      </div>
    );
  }

  if (value?.source === "google_place") {
    const place = value.place;
    return (
      <div className="rounded-md border border-green-600/40 bg-green-50 p-4 dark:bg-green-900/20">
        <div className="flex items-start gap-3">
          <Check className="mt-0.5 h-5 w-5 flex-shrink-0 text-green-600" />
          <div className="min-w-0 flex-1 space-y-1">
            <p className="font-semibold text-black dark:text-white">{place.name}</p>
            <p className="text-sm text-gray-600 dark:text-gray-300">{formatAddress(place)}</p>
            {place.type ? (
              <p className="text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">{place.type}</p>
            ) : null}
            {place.phone ? (
              <p className="text-sm text-gray-600 dark:text-gray-300">{place.phone}</p>
            ) : null}
            {place.maps_page ? (
              <a
                className="inline-block text-sm text-[#eb6c6c] underline"
                href={place.maps_page}
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
      {/* Business mode needs no instruction — the field is self-evident. Address
          mode keeps one, because "you name the business yourself next" is the
          part of that flow nobody would guess. */}
      {mode === "address" ? (
        <p className="text-xs text-gray-600 dark:text-gray-400">
          Start typing your street address and pick it from the list. You will enter the business name yourself on the
          next step.
        </p>
      ) : null}

      <div ref={containerRef} />

      {searchError ? (
        <p className="text-xs text-red-600 dark:text-red-300">{searchError}</p>
      ) : null}

      <button
        className="text-xs text-[#eb6c6c] underline"
        onClick={() => switchMode(mode === "business" ? "address" : "business")}
        type="button"
      >
        {mode === "business"
          ? "My business isn't on Google Maps — enter your address manually"
          : "Back to searching for a Google business listing"}
      </button>
    </div>
  );
}
