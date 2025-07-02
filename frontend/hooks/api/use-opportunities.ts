"use client"

import { useState, useEffect, useCallback } from "react"
import { mockOpportunities } from "@/data/mock-opportunities"
import type { Opportunity } from "@/types/opportunity"

export function useOpportunities() {
  const [opportunities, setOpportunities] = useState<Opportunity[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)

  // Fetch all opportunities
  useEffect(() => {
    const fetchOpportunities = async () => {
      try {
        setIsLoading(true)
        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500))
        setOpportunities(mockOpportunities)
        setError(null)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to fetch opportunities"))
      } finally {
        setIsLoading(false)
      }
    }

    fetchOpportunities()
  }, [])

  // Get opportunity by ID
  const getOpportunityById = useCallback(
    (id: string) => {
      return opportunities.find((opp) => opp.id === id) || null
    },
    [opportunities],
  )

  // Create opportunity
  const createOpportunity = useCallback(async (opportunityData: Omit<Opportunity, "id">) => {
    try {
      setIsLoading(true)
      // Simulate API call
      await new Promise((resolve) => setTimeout(resolve, 500))

      const newOpportunity: Opportunity = {
        id: `opp-${Date.now()}`,
        ...opportunityData,
      }

      setOpportunities((prev) => [...prev, newOpportunity])
      return newOpportunity
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to create opportunity"))
      throw err
    } finally {
      setIsLoading(false)
    }
  }, [])

  // Update opportunity
  const updateOpportunity = useCallback(
    async (id: string, opportunityData: Partial<Opportunity>) => {
      try {
        setIsLoading(true)
        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500))

        setOpportunities((prev) => prev.map((opp) => (opp.id === id ? { ...opp, ...opportunityData } : opp)))

        return getOpportunityById(id)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to update opportunity"))
        throw err
      } finally {
        setIsLoading(false)
      }
    },
    [getOpportunityById],
  )

  // Delete opportunity
  const deleteOpportunity = useCallback(async (id: string) => {
    try {
      setIsLoading(true)
      // Simulate API call
      await new Promise((resolve) => setTimeout(resolve, 500))

      setOpportunities((prev) => prev.filter((opp) => opp.id !== id))
      return true
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to delete opportunity"))
      throw err
    } finally {
      setIsLoading(false)
    }
  }, [])

  // Get opportunities by organizer
  const getOpportunitiesByOrganizer = useCallback(
    (organizerName: string) => {
      return opportunities.filter((opp) => opp.organizer === organizerName)
    },
    [opportunities],
  )

  return {
    opportunities,
    isLoading,
    error,
    getOpportunityById,
    createOpportunity,
    updateOpportunity,
    deleteOpportunity,
    getOpportunitiesByOrganizer,
  }
}
