"use client"; // for Next.js App Router

import { useEffect, useRef, useState } from "react";
import { LAT_DIF, LNG_DIF, MAP_CENTER, MAP_RADIUS } from "@/lib/constants";
import { useApp } from "@/context/AppProvider";



export default function PlaceAutocomplete() {
  const { status } = useApp()
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const importGoogleLibrary = async () => {
        console.log("google maps imported")
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
            'websiteURI', 'primaryTypeDisplayName', 'nationalPhoneNumber', 'googleMapsURI',
            'googleMapsURI',

        ] });
        console.log(JSON.stringify(place.toJSON(), /* replacer */ null, /* space */ 2))
    });
    placeAutocomplete.className="text-black dark:text-white border rounded-md bg-secondary px-3 py-2"

    if (containerRef.current?.querySelector("gmp-place-autocomplete")) {
        console.log("Element is already inside container");
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
