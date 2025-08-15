"use client"


import { LocationResponse } from "@/types/server";
import { User } from "./AppProvider";
import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { Location } from "@/types/location";
import { mockLocations } from "@/data/mock_locations";
import { useApp } from "@/context/AppProvider";

export type LocationsStatus = "loading" | "available" | "unavailable"

interface LocationContextType {
    mapLocations: Location[]
    mapLocationsStatus: LocationsStatus
    getMapLocations: () => Promise<void>
    updateLocation: (location: Location) => Promise<void>
    addLocation: (location: Location) => Promise<void>
    loadLocations: () => Promise<void>
}

const LocationContext = createContext<LocationContextType | null>(null)

export default function LocationProvider({ children }: { children: ReactNode }) {
    const [mapLocations, setMapLocations] = useState<Location[]>([])
    const [mapLocationsStatus, setMapLocationsStatus] = useState<LocationsStatus>("loading")
    const { authFetch } = useApp()

    useEffect(() => {
      getMapLocations()
    }, [])


    const _getMapLocations = async (): Promise<LocationResponse> => {
        const res = await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + "/locations")
        if(res.status != 200) {
            throw new Error("error getting locations")
        }
        return await res.json() as LocationResponse
    }

    const _updateLocation = async (location: Location) => {
        const res = await authFetch("/locations", {
            method: "PUT",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({location})
        })
        if(res.status != 201) {
            throw new Error("error updating location")
        }
      }

    const _getLocationById = async (id: number): Promise<Location> => {
        const res = await authFetch("/locations{id}")
        if(res.status != 200) {
            throw new Error("error getting location by id")
        }
        return await res.json() as Location
    }


    const _addLocation = async (location: Location) => {
        const res = await authFetch("/locations", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
                },
            body: JSON.stringify({location})
        })
        if(res.status != 201) {
            throw new Error("error adding new location, from controller")
        }
      }

    const addLocation = async (location: Location) => {
        setMapLocationsStatus("loading")
        try {
            await _addLocation(location)
        }
        catch {
            setMapLocationsStatus("unavailable")
            console.error("error adding new location")
        }

      }

    const getMapLocations = async () => {
        setMapLocationsStatus("loading")
        try {
            const l = await _getMapLocations()
            setMapLocations(l.locations)
            setMapLocationsStatus("available")
        }
        catch {
            setMapLocationsStatus("unavailable")
            console.error("error getting locations")
        }
    }

    const loadLocations = async () => {
        for (const loc of mockLocations) {
            await addLocation(loc)
            console.log(loc)
        }
    }

    const updateLocation = async (location: Location) => {
        setMapLocationsStatus("loading")
        try {
            await _updateLocation(location)
            const l = await _getMapLocations()
            setMapLocations(l.locations)
            setMapLocationsStatus("available")
        }
        catch {
            setMapLocationsStatus("unavailable")
            console.error("error updating locations")
        }
    }

    return (
        <LocationContext.Provider
        value ={{
            mapLocations,
            mapLocationsStatus,
            getMapLocations,
            updateLocation,
            addLocation,
            loadLocations
        }}
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
