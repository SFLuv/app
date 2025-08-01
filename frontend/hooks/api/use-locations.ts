import { useState, useEffect, useCallback } from "react"
import type { Location } from "@/types/location"
import { mockMerchants } from "@/data/mock-merchants"
import { mockLocations } from "@/data/mock-locations"

export function useMerchants() {
  const [locations, setLocations] = useState<Location[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)

  // Fetch all locations
  useEffect(() => {
    const fetchLocations = async () => {
      try {
        setIsLoading(true)
        // Simulate API call
        const res = await fetch('/locations');
        
        setLocations(mockLocations)
        setError(null)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to fetch merchants"))
      } finally {
        setIsLoading(false)
      }
    }

    fetchLocations()
  }, [])

  // Get merchant by ID
  const getLocationById = useCallback(
    (id: number) => {
      return locations.find((location) => location.id === id) || null
    },
    [locations],
  )

}