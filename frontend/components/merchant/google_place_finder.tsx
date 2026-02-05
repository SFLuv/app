"use client"; // for Next.js App Router

import { useEffect, useRef, useState } from "react";
import { LAT_DIF, LNG_DIF, MAP_CENTER, MAP_RADIUS } from "@/lib/constants";
import { useApp } from "@/context/AppProvider";
import { GoogleSubLocation } from "@/types/location";

interface PlaceAutocompleteProps {
  setGoogleSubLocation: React.Dispatch<React.SetStateAction<GoogleSubLocation | null>>;
  setBusinessPhone: React.Dispatch<React.SetStateAction<string>>;
  setStreet: React.Dispatch<React.SetStateAction<string>>;
}

export default function PlaceAutocomplete({ setGoogleSubLocation, setBusinessPhone, setStreet}: PlaceAutocompleteProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const importGoogleLibrary = async () => {
        await google.maps.importLibrary("places") as google.maps.PlacesLibrary;
    }

  const init = async () => {
    await importGoogleLibrary()
    //@ts-ignore
    const placeAutocomplete = new google.maps.places.PlaceAutocompleteElement({
    locationRestriction: {
        south: MAP_CENTER.lat - LAT_DIF,
        west: MAP_CENTER.lng - LNG_DIF,
        north: MAP_CENTER.lat + LAT_DIF,
        east: MAP_CENTER.lng + LNG_DIF,},
    });

    //@ts-ignore
    placeAutocomplete.addEventListener('gmp-select', async ({ placePrediction }) => {
        const place = placePrediction.toPlace();
        await place.fetchFields({ fields: [
            'displayName', 'addressComponents', 'location', 'rating', 'regularOpeningHours',
            'websiteURI', 'primaryTypeDisplayName', 'nationalPhoneNumber', 'googleMapsURI', 'photos',
            'svgIconMaskURI'


        ] });
        const rawGoogleData = place.toJSON()
        const googleDetails: GoogleSubLocation = {
            google_id: rawGoogleData.id,
            name: rawGoogleData.displayName,
            type: rawGoogleData.primaryTypeDisplayName,
            street: (rawGoogleData.addressComponents[0]?.longText || "") + " " + (rawGoogleData.addressComponents[1]?.longText || ""),
            city: rawGoogleData.addressComponents[3]?.longText || "",
            state: rawGoogleData.addressComponents[5]?.longText || "",
            zip: rawGoogleData.addressComponents[7]?.longText || "",
            lat: rawGoogleData.location.lat,
            lng: rawGoogleData.location.lng,
            phone: rawGoogleData.nationalPhoneNumber,
            website: rawGoogleData.websiteURI,
            image_url: rawGoogleData.photos[0]?.googleMapsURI || "",
            rating: rawGoogleData.rating,
            maps_page: rawGoogleData.googleMapsURI,
            opening_hours: rawGoogleData.regularOpeningHours?.weekdayDescriptions || [],
        }
        setGoogleSubLocation(googleDetails)
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
      <div ref={containerRef} style={{
  }}></div>
  )
}
