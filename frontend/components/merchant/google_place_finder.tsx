"use client"; // for Next.js App Router

import { useEffect, useRef, useState } from "react";
import { GOOGLE_MAPS_API_KEY } from "@/lib/constants";
import { useApp } from "@/context/AppProvider";


export default function PlaceAutocomplete() {
  const { status } = useApp()


  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const addGoogleScript = async () => {
        const existingScript = document.querySelector<HTMLScriptElement>(
            `script[src^="https://maps.googleapis.com/maps/api/js"]`);
        if (!existingScript) {
        const script = document.createElement("script");
        script.src = `https://maps.googleapis.com/maps/api/js?key=${GOOGLE_MAPS_API_KEY}&libraries=places`;
        script.async = true;
        script.defer = true;
        document.head.appendChild(script);
        console.log("Script appended")
        }
    }

    const importGoogleLibrary = async () => {
        console.log("google maps imported")
        await google.maps.importLibrary("places") as google.maps.PlacesLibrary;
    }

  const init = async () => {
    await addGoogleScript()
    await importGoogleLibrary()
    //@ts-ignore
    const placeAutocomplete = new google.maps.places.PlaceAutocompleteElement();
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

  return <div ref={containerRef}></div>;
}
