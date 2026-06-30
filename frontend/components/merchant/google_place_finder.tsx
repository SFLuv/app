"use client"; // for Next.js App Router

import { useEffect, useRef, useState } from "react";
import { LAT_DIF, LNG_DIF, MAP_CENTER } from "@/lib/constants";
import { GoogleSubLocation } from "@/types/location";

interface PlaceAutocompleteProps {
  setGoogleSubLocation: React.Dispatch<React.SetStateAction<GoogleSubLocation | null>>;
  setBusinessPhone: React.Dispatch<React.SetStateAction<string>>;
  setStreet: React.Dispatch<React.SetStateAction<string>>;
  onSelect?: (location: GoogleSubLocation) => void;
}

export default function PlaceAutocomplete({ setGoogleSubLocation, setBusinessPhone, setStreet, onSelect}: PlaceAutocompleteProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [textSearch, setTextSearch] = useState("");
  const [textSearchBusy, setTextSearchBusy] = useState(false);
  const [textSearchError, setTextSearchError] = useState("");

  const locationRestriction = {
      south: MAP_CENTER.lat - LAT_DIF,
      west: MAP_CENTER.lng - LNG_DIF,
      north: MAP_CENTER.lat + LAT_DIF,
      east: MAP_CENTER.lng + LNG_DIF,
  };

  const fields = [
      'id', 'displayName', 'addressComponents', 'location', 'rating', 'regularOpeningHours',
      'websiteURI', 'primaryTypeDisplayName', 'nationalPhoneNumber', 'googleMapsURI', 'photos',
      'svgIconMaskURI'
  ];

  const addressPart = (rawGoogleData: any, type: string) => {
      return rawGoogleData.addressComponents?.find((part: any) => part.types?.includes(type))?.longText || "";
  };

  const applyPlace = (rawGoogleData: any) => {
      const lat = rawGoogleData.location?.lat ?? rawGoogleData.location?.latitude;
      const lng = rawGoogleData.location?.lng ?? rawGoogleData.location?.longitude;
      if (!rawGoogleData.id || typeof lat !== "number" || typeof lng !== "number") {
          setTextSearchError("Google returned this place without a place ID or coordinates. Try another search.");
          return;
      }
      setTextSearchError("");
      const street = [addressPart(rawGoogleData, "street_number"), addressPart(rawGoogleData, "route")]
          .filter(Boolean)
          .join(" ");
      const displayName = typeof rawGoogleData.displayName === "string" ? rawGoogleData.displayName : rawGoogleData.displayName?.text || "";
      const primaryType = typeof rawGoogleData.primaryTypeDisplayName === "string" ? rawGoogleData.primaryTypeDisplayName : rawGoogleData.primaryTypeDisplayName?.text || "";
      const googleDetails: GoogleSubLocation = {
          google_id: rawGoogleData.id,
          name: displayName,
          type: primaryType,
          street,
          city: addressPart(rawGoogleData, "locality"),
          state: addressPart(rawGoogleData, "administrative_area_level_1"),
          zip: addressPart(rawGoogleData, "postal_code"),
          lat,
          lng,
          phone: rawGoogleData.nationalPhoneNumber,
          website: rawGoogleData.websiteURI,
          image_url: rawGoogleData.photos?.[0]?.googleMapsURI || "",
          rating: rawGoogleData.rating,
          maps_page: rawGoogleData.googleMapsURI,
          opening_hours: rawGoogleData.regularOpeningHours?.weekdayDescriptions || [],
      }
      setGoogleSubLocation(googleDetails)
      onSelect?.(googleDetails)
      if (typeof googleDetails.phone === "string") {
      setBusinessPhone(googleDetails.phone)
      } else {
        setBusinessPhone("")
      }
      if (typeof googleDetails.street === "string") {
      setStreet(googleDetails.street)
      } else {
        setStreet("")
      }
  };

  const searchByText = async () => {
      const query = textSearch.trim();
      if (!query) return;

      setTextSearchBusy(true);
      setTextSearchError("");
      try {
          const { Place } = await google.maps.importLibrary("places") as google.maps.PlacesLibrary;
          const { places } = await Place.searchByText({
              textQuery: query,
              fields,
              locationRestriction,
              maxResultCount: 1,
          } as any);
          if (!places?.length) {
              setTextSearchError("No Google place found. Try the name plus city or street address.");
              return;
          }
          applyPlace(places[0].toJSON());
      } catch (error) {
          console.error(error);
          setTextSearchError("Unable to search Google Places right now.");
      } finally {
          setTextSearchBusy(false);
      }
  };

  useEffect(() => {
    if (!containerRef.current) return;

    const importGoogleLibrary = async () => {
        await google.maps.importLibrary("places") as google.maps.PlacesLibrary;
    }

  const init = async () => {
    await importGoogleLibrary()
    //@ts-ignore
    const placeAutocomplete = new google.maps.places.PlaceAutocompleteElement({
    locationRestriction,
    });

    //@ts-ignore
    placeAutocomplete.addEventListener('gmp-select', async ({ placePrediction }) => {
        const place = placePrediction.toPlace();
        await place.fetchFields({ fields });
        applyPlace(place.toJSON())
    });
    placeAutocomplete.className="text-black dark:text-white border rounded-md bg-secondary px-3 py-2"

    if (containerRef.current?.querySelector("gmp-place-autocomplete")) {
    } else {
        //@ts-ignore
        containerRef.current.appendChild(placeAutocomplete)
    }
  };

    init();
  }
, [])

  return (
      <div className="space-y-2">
          <div ref={containerRef} style={{
      }}></div>
          <div className="flex gap-2">
              <input
                  className="min-w-0 flex-1 rounded-md border bg-secondary px-3 py-2 text-sm text-black dark:text-white"
                  placeholder="Exact name or address"
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
                  className="rounded-md border px-3 py-2 text-sm text-black disabled:opacity-50 dark:text-white"
                  disabled={textSearchBusy}
                  onClick={() => void searchByText()}
                  type="button"
              >
                  {textSearchBusy ? "Searching..." : "Search"}
              </button>
          </div>
          {textSearchError ? (
              <p className="text-xs text-red-600 dark:text-red-300">{textSearchError}</p>
          ) : null}
      </div>
  )
}
