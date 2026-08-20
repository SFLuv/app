"use client"


import { AuthedLocationResponse, LocationResponse } from "@/types/server";
import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { AuthedLocation, Location, UpdateLocationApprovalRequest } from "@/types/location";
import { useApp } from "@/context/AppProvider";
import { BACKEND } from "@/lib/constants";

export type LocationsStatus = "loading" | "available" | "unavailable"

interface LocationContextType {
    mapLocations: Location[]
    authedMapLocations: AuthedLocation[]
    locationTypes: string[]
    mapLocationsStatus: LocationsStatus
    getMapLocations: () => Promise<void>
    getAuthedMapLocations: () => Promise<void>
    updateLocation: (location: AuthedLocation) => Promise<void>
    updateLocationApproval: (req: UpdateLocationApprovalRequest) => Promise<void>
    addLocation: (location: AuthedLocation) => Promise<void>
}

const LocationContext = createContext<LocationContextType | null>(null)
let mapLocationsInFlight: Promise<LocationResponse> | null = null

const getLocationTypes = (locations: Location[]): string[] => {
    const uniqueTypes = new Set<string>()
    for (const location of locations) {
        const normalizedType = (location.type || "").trim()
        if (!normalizedType) continue
        uniqueTypes.add(normalizedType)
    }
    return [...uniqueTypes, "All Locations"]
}

// readErrorMessage pulls the backend's {"error": "..."} message off a failed
// response, falling back to a generic message for non-JSON bodies.
const readErrorMessage = async (
    res: Response,
    fallback = "Something went wrong submitting your application. Please try again.",
): Promise<string> => {
    try {
        const body = await res.clone().json()
        if (body && typeof body.error === "string" && body.error.trim()) {
            return body.error.trim()
        }
    } catch {
        // not a JSON body
    }
    return fallback
}

const fetchMapLocations = async (): Promise<LocationResponse> => {
    const controller = new AbortController()
    const timeoutId = setTimeout(() => controller.abort(), 10000)
    try {
        const res = await fetch(BACKEND + "/locations", { signal: controller.signal })
        if(res.status != 200) {
            throw new Error("error getting locations")
        }
        return await res.json() as LocationResponse
    } finally {
        clearTimeout(timeoutId)
    }
}

const getMapLocationsDeduped = async (): Promise<LocationResponse> => {
    if (mapLocationsInFlight) return mapLocationsInFlight
    mapLocationsInFlight = (async () => {
        try {
            return await fetchMapLocations()
        } finally {
            mapLocationsInFlight = null
        }
    })()
    return mapLocationsInFlight
}

export default function LocationProvider({ children }: { children: ReactNode }) {
    const [mapLocations, setMapLocations] = useState<Location[]>([])
    const [authedMapLocations, setAuthedMapLocations] = useState<AuthedLocation[]>([])
    const [mapLocationsStatus, setMapLocationsStatus] = useState<LocationsStatus>("loading")
    const [locationTypes, setLocationTypes] = useState<string[]>([])
    const { authFetch, setUserLocations } = useApp()
    const authFetchRef = useRef(authFetch)
    const setUserLocationsRef = useRef(setUserLocations)
    const mapLocationsRequestRef = useRef<Promise<void> | null>(null)

    useEffect(() => {
        authFetchRef.current = authFetch
    }, [authFetch])

    useEffect(() => {
        setUserLocationsRef.current = setUserLocations
    }, [setUserLocations])

    const getMapLocations = useCallback(async () => {
        if (mapLocationsRequestRef.current) {
            return mapLocationsRequestRef.current
        }

        const request = (async () => {
            setMapLocationsStatus((currentStatus) => currentStatus === "loading" ? currentStatus : "loading")
            try {
                const response = await getMapLocationsDeduped()
                setMapLocations(response.locations)
                setLocationTypes(getLocationTypes(response.locations))
                setMapLocationsStatus("available")
            }
            catch {
                setMapLocationsStatus("unavailable")
                console.error("error getting locations")
            }
            finally {
                mapLocationsRequestRef.current = null
            }
        })()

        mapLocationsRequestRef.current = request
        return request
    }, [])

    useEffect(() => {
        void getMapLocations()
    }, [getMapLocations])

    const getAuthedMapLocations = useCallback(async () => {
        try {
            const res = await authFetchRef.current("/admin/locations")
            if(res.status != 200) {
                throw new Error("error getting authed locations")
            }
            const response = await res.json() as AuthedLocationResponse
            setAuthedMapLocations(response.locations)
        } catch {
            console.log("error getting authed locations")
        }
    }, [])

    // Surfaces the backend's message so submission failures (duplicate business,
    // rejected place, validation) reach the merchant instead of being swallowed
    // while the form navigates away as if it had succeeded.
    const addLocation = useCallback(async (location: AuthedLocation) => {
        setMapLocationsStatus("loading")
        try {
            const res = await authFetchRef.current("/locations", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                    },
                body: JSON.stringify(location)
            })
            if(res.status != 201) {
                const message = await readErrorMessage(res)
                throw new Error(message)
            }
            setUserLocationsRef.current((currentLocations) => [...currentLocations, location])
            setMapLocationsStatus("available")
        }
        catch (error) {
            setMapLocationsStatus("unavailable")
            console.error("error adding new location", error)
            throw error instanceof Error
                ? error
                : new Error("Something went wrong submitting your application. Please try again.")
        }
      }, [])

    // Rethrows for the same reason addLocation does: a merchant correcting a
    // rejected application has no other signal that the save failed, and a
    // silent failure here reads exactly like a successful one.
    const updateLocation = useCallback(async (location: AuthedLocation) => {
        setMapLocationsStatus("loading")
        try {
            const res = await authFetchRef.current("/locations", {
                method: "PUT",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({location})
            })
            if(res.status != 201) {
                const message = await readErrorMessage(
                    res,
                    "Something went wrong saving your changes. Please try again.",
                )
                throw new Error(message)
            }
            const updatedLocations = await getMapLocationsDeduped()
            setMapLocations(updatedLocations.locations)
            setLocationTypes(getLocationTypes(updatedLocations.locations))
            setMapLocationsStatus("available")
        }
        catch (error) {
            setMapLocationsStatus("unavailable")
            console.error("error updating location", error)
            throw error instanceof Error
                ? error
                : new Error("Something went wrong saving your changes. Please try again.")
        }
    }, [])

    const updateLocationApproval = useCallback(async (req: UpdateLocationApprovalRequest) => {
        setMapLocationsStatus("loading")
        try {
            const updateRes = await authFetchRef.current("/admin/locations", {
                method: "PUT",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify(req)
            })
            if(updateRes.status != 201) {
                throw new Error("error updating location approval")
            }

            const [locationsRes, authedLocationsRes] = await Promise.all([
                getMapLocationsDeduped(),
                authFetchRef.current("/admin/locations")
            ])
            if(authedLocationsRes.status != 200) {
                throw new Error("error getting authed locations")
            }
            const authedLocations = await authedLocationsRes.json() as AuthedLocationResponse

            setMapLocations(locationsRes.locations)
            setLocationTypes(getLocationTypes(locationsRes.locations))
            setAuthedMapLocations(authedLocations.locations)
            setMapLocationsStatus("available")
        }
        catch {
            setMapLocationsStatus("unavailable")
            console.error("error updating location approval")
        }
      }, [])

    const contextValue = useMemo<LocationContextType>(() => ({
        mapLocations,
        authedMapLocations,
        locationTypes,
        mapLocationsStatus,
        getMapLocations,
        getAuthedMapLocations,
        updateLocation,
        updateLocationApproval,
        addLocation,
    }), [
        mapLocations,
        authedMapLocations,
        locationTypes,
        mapLocationsStatus,
        getMapLocations,
        getAuthedMapLocations,
        updateLocation,
        updateLocationApproval,
        addLocation
    ])

    return (
        <LocationContext.Provider
        value ={contextValue}
        >
            {children}
        </LocationContext.Provider>
    )
}

export function useLocation() {
    const context = useContext(LocationContext);
      if (!context) {
        throw new Error("useLocation must be used within a LocationProvider");
      }
      return context;
}
