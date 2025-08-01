
import { LocationResponse } from "@/types/server";
import { User } from "./AppProvider";
import { createContext, ReactNode, useContext, useState } from "react";

interface LocationContextType {
    mapLocations: LocationResponse[]
    getMapLocations: () => Promise<LocationResponse[]>
    getLocationById: (id: number) => Promise<LocationResponse>
    updateLocation: (location: LocationResponse) => void
    addLocation: (location: LocationResponse) => void
}

const LocationContext = createContext<LocationContextType | null>(null)

export default function LocationProvider({ children }: { children: ReactNode }) {
    const [mapLocations, setMapLocations] = useState<LocationResponse[]>([])

    const getMapLocations = async (): Promise<LocationResponse[]> => {
        const res = await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + "/locations")
        return await res.json() as LocationResponse[]
      }

    const updateLocation = async (location: LocationResponse) => {
        const res = await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + "/locations", {
            method: "PUT",
            headers: {
                "Content-Type": "application/json"
                },
            body: JSON.stringify({location})
        }
        )
      }

    const addLocation = async (location: LocationResponse) => {
        const res = await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + "/locations", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
                },
            body: JSON.stringify({location})
        })
      }

    const getLocationById = async (id: number): Promise<LocationResponse> => {
        const res = await fetch(process.env.NEXT_PUBLIC_BACKEND_BASE_URL + "/locations/{id}")
        return await res.json() as LocationResponse
    }


    return (
        <LocationContext.Provider
        value ={{
            mapLocations,
            getLocationById,
            getMapLocations,
            updateLocation,
            addLocation
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
