"use client";

import { GOOGLE_MAPS_API_KEY } from "@/lib/constants";

export const hasGoogleMapsPlaces = () => {
  return (
    typeof window !== "undefined" &&
    !!(window as any).google?.maps?.importLibrary
  );
};

export const waitForGooglePlaces = async (timeoutMs = 15000): Promise<void> => {
  if (typeof window === "undefined") return;

  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    if (hasGoogleMapsPlaces()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  throw new Error("Google Maps script timed out");
};

export const ensureGooglePlacesScript = async (): Promise<void> => {
  if (typeof window === "undefined") return;
  if (hasGoogleMapsPlaces()) return;

  const src = `https://maps.googleapis.com/maps/api/js?key=${GOOGLE_MAPS_API_KEY}&libraries=places`;
  const existingScript = document.querySelector<HTMLScriptElement>(
    `script[src^="https://maps.googleapis.com/maps/api/js"]`,
  );

  if (!existingScript) {
    const script = document.createElement("script");
    script.src = src;
    script.async = true;
    script.defer = true;
    document.head.appendChild(script);
  }

  await waitForGooglePlaces();
};
