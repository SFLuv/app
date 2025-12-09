"use client"


import { AuthedLocationResponse, LocationResponse } from "@/types/server";
import { User } from "./AppProvider";
import { createContext, ReactNode, useContext, useEffect, useState } from "react";
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

export default function LocationProvider({ children }: { children: ReactNode }) {
    const [mapLocations, setMapLocations] = useState<Location[]>([])
    const [authedMapLocations, setAuthedMapLocations] = useState<AuthedLocation[]>([])
    const [mapLocationsStatus, setMapLocationsStatus] = useState<LocationsStatus>("loading")
    const [locationTypes, setLocationTypes] = useState<string[]>([])
    const { authFetch, userLocations, setUserLocations } = useApp()

    useEffect(() => {
      getMapLocations()
    }, [])


    const _getMapLocations = async (): Promise<LocationResponse> => {
        const res = await fetch(BACKEND + "/locations")
        if(res.status != 200) {
            throw new Error("error getting locations")
        }
        return await res.json() as LocationResponse
    }

    const _getAuthedMapLocations = async (): Promise<AuthedLocationResponse> => {
        const res = await authFetch("/admin/locations")
        if(res.status != 200) {
            throw new Error("error getting authed locations")
        }
        return await res.json() as AuthedLocationResponse
    }

    const _updateLocation = async (location: AuthedLocation) => {
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


    const _addLocation = async (location: AuthedLocation) => {
        const res = await authFetch("/locations", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
                },
            body: JSON.stringify(location)
        })
        if(res.status != 201) {
            throw new Error("error adding new location, from controller")
        }
      }

      const _updateLocationApproval = async (req: UpdateLocationApprovalRequest) => {
        console.log(req)
        const res = await authFetch("/admin/locations", {
            method: "PUT",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify(req)
        })
        if(res.status != 201) {
            throw new Error("error updating location approval")
        }
      }


    const addLocation = async (location: AuthedLocation) => {
        setMapLocationsStatus("loading")
        try {
            await _addLocation(location)
            setUserLocations([...userLocations, location])
        }
        catch {
            setMapLocationsStatus("unavailable")
            console.error("error adding new location")
        }
        setMapLocationsStatus("available")
      }

    const getMapLocations = async () => {
        setMapLocationsStatus("loading")
        try {
            const l = await _getMapLocations()
            setMapLocations(l.locations)
            setMapLocationsStatus("available")
            const tempTypes: string[] = [];
            for (let i = 0; i < l.locations.length; i++) {
                if (!tempTypes.includes(l.locations[i].type)) {
                tempTypes.push(l.locations[i].type)
                }
            }
            tempTypes.push("All Locations")
            setLocationTypes(tempTypes)
        }
        catch {
            setMapLocationsStatus("unavailable")
            console.error("error getting locations")
        }
    }

    const getAuthedMapLocations = async () => {
        setMapLocationsStatus("loading")
        try {
            const l = await _getAuthedMapLocations()
            setAuthedMapLocations(l.locations)
        } catch {
            console.log("error getting authed locations")
        }
    }


    const updateLocation = async (location: AuthedLocation) => {
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

     const updateLocationApproval = async (req: UpdateLocationApprovalRequest) => {
         setMapLocationsStatus("loading")
        try {
            await _updateLocationApproval(req)
            const l = await _getMapLocations()
            setMapLocations(l.locations)
            const al = await _getAuthedMapLocations()
            setAuthedMapLocations(al.locations)
            setMapLocationsStatus("available")
        }
        catch {
            setMapLocationsStatus("unavailable")
            console.error("error updating location approval")
        }
      }

    return (
        <LocationContext.Provider
        value ={{
            mapLocations,
            authedMapLocations,
            locationTypes,
            mapLocationsStatus,
            getMapLocations,
            getAuthedMapLocations,
            updateLocation,
            updateLocationApproval,
            addLocation,
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
