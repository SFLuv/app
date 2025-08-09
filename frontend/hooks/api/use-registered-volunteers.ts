"use client"

import { useState, useEffect, useCallback } from "react"

interface RegisteredVolunteer {
  userId: string
  opportunityId: string
  name: string
  email: string
  registrationDate: string
}

// Mock registered volunteers data
const mockRegisteredVolunteers: RegisteredVolunteer[] = [
  {
    userId: "user1",
    opportunityId: "opp-1",
    name: "John Doe",
    email: "john@example.com",
    registrationDate: "2025-04-10T14:30:00",
  },
  {
    userId: "user4",
    opportunityId: "opp-1",
    name: "Alice Brown",
    email: "alice@example.com",
    registrationDate: "2025-04-12T09:15:00",
  },
  {
    userId: "user8",
    opportunityId: "opp-1",
    name: "Grace Lee",
    email: "grace@example.com",
    registrationDate: "2025-04-14T16:45:00",
  },
  {
    userId: "user2",
    opportunityId: "opp-2",
    name: "Jane Smith",
    email: "jane@example.com",
    registrationDate: "2025-04-11T11:20:00",
  },
  {
    userId: "user6",
    opportunityId: "opp-3",
    name: "Eva Wilson",
    email: "eva@example.com",
    registrationDate: "2025-04-13T13:10:00",
  },
]

export function useRegisteredVolunteers(opportunityId?: string) {
  const [volunteers, setVolunteers] = useState<RegisteredVolunteer[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)

  // Fetch registered volunteers
  useEffect(() => {
    const fetchVolunteers = async () => {
      try {
        setIsLoading(true)
        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500))

        if (opportunityId) {
          setVolunteers(mockRegisteredVolunteers.filter((v) => v.opportunityId === opportunityId))
        } else {
          setVolunteers(mockRegisteredVolunteers)
        }

        setError(null)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to fetch registered volunteers"))
      } finally {
        setIsLoading(false)
      }
    }

    fetchVolunteers()
  }, [opportunityId])

  // Get volunteers by opportunity ID
  const getVolunteersByOpportunityId = useCallback(
    (id: string) => {
      return volunteers.filter((volunteer) => volunteer.opportunityId === id)
    },
    [volunteers],
  )

  // Add volunteer to opportunity
  const addVolunteer = useCallback(async (volunteer: Omit<RegisteredVolunteer, "registrationDate">) => {
    try {
      setIsLoading(true)
      // Simulate API call
      await new Promise((resolve) => setTimeout(resolve, 500))

      const newVolunteer: RegisteredVolunteer = {
        ...volunteer,
        registrationDate: new Date().toISOString(),
      }

      setVolunteers((prev) => [...prev, newVolunteer])
      return newVolunteer
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to add volunteer"))
      throw err
    } finally {
      setIsLoading(false)
    }
  }, [])

  // Remove volunteer from opportunity
  const removeVolunteer = useCallback(async (userId: string, opportunityId: string) => {
    try {
      setIsLoading(true)
      // Simulate API call
      await new Promise((resolve) => setTimeout(resolve, 500))

      setVolunteers((prev) => prev.filter((v) => !(v.userId === userId && v.opportunityId === opportunityId)))

      return true
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to remove volunteer"))
      throw err
    } finally {
      setIsLoading(false)
    }
  }, [])

  return {
    volunteers,
    isLoading,
    error,
    getVolunteersByOpportunityId,
    addVolunteer,
    removeVolunteer,
  }
}
