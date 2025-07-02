"use client"

import { useState, useEffect, useCallback } from "react"

// Define User type
export interface User {
  id: string
  name: string
  email: string
  role: "user" | "merchant" | "admin"
  isOrganizer: boolean
  avatar?: string
}

// Mock users data
const mockUsers: User[] = [
  { id: "user1", name: "John Doe", email: "john@example.com", role: "user", isOrganizer: false },
  { id: "user2", name: "Jane Smith", email: "jane@example.com", role: "user", isOrganizer: true },
  { id: "user3", name: "Bob Johnson", email: "bob@example.com", role: "merchant", isOrganizer: true },
  { id: "user4", name: "Alice Brown", email: "alice@example.com", role: "user", isOrganizer: false },
  { id: "user5", name: "Charlie Davis", email: "charlie@example.com", role: "merchant", isOrganizer: true },
  { id: "user6", name: "Eva Wilson", email: "eva@example.com", role: "user", isOrganizer: true },
  { id: "user7", name: "Frank Miller", email: "frank@example.com", role: "admin", isOrganizer: true },
  { id: "user8", name: "Grace Lee", email: "grace@example.com", role: "user", isOrganizer: false },
  { id: "user9", name: "Henry Garcia", email: "henry@example.com", role: "merchant", isOrganizer: false },
  { id: "user10", name: "Ivy Chen", email: "ivy@example.com", role: "user", isOrganizer: true },
  { id: "user11", name: "Jack Wilson", email: "jack@example.com", role: "user", isOrganizer: false },
  { id: "user12", name: "Karen Lopez", email: "karen@example.com", role: "merchant", isOrganizer: true },
  { id: "user13", name: "Leo Martin", email: "leo@example.com", role: "user", isOrganizer: false },
  { id: "user14", name: "Mia Thompson", email: "mia@example.com", role: "user", isOrganizer: true },
  { id: "user15", name: "Noah White", email: "noah@example.com", role: "merchant", isOrganizer: false },
]

export function useUsers() {
  const [users, setUsers] = useState<User[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)

  // Fetch all users
  useEffect(() => {
    const fetchUsers = async () => {
      try {
        setIsLoading(true)
        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500))
        setUsers(mockUsers)
        setError(null)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to fetch users"))
      } finally {
        setIsLoading(false)
      }
    }

    fetchUsers()
  }, [])

  // Get user by ID
  const getUserById = useCallback(
    (id: string) => {
      return users.find((user) => user.id === id) || null
    },
    [users],
  )

  // Update user
  const updateUser = useCallback(
    async (id: string, userData: Partial<User>) => {
      try {
        setIsLoading(true)
        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500))

        setUsers((prev) => prev.map((user) => (user.id === id ? { ...user, ...userData } : user)))

        return getUserById(id)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to update user"))
        throw err
      } finally {
        setIsLoading(false)
      }
    },
    [getUserById],
  )

  // Toggle organizer status
  const toggleOrganizerStatus = useCallback(
    async (id: string) => {
      const user = getUserById(id)
      if (!user) throw new Error("User not found")

      return updateUser(id, { isOrganizer: !user.isOrganizer })
    },
    [getUserById, updateUser],
  )

  // Get organizers
  const getOrganizers = useCallback(() => {
    return users.filter((user) => user.isOrganizer)
  }, [users])

  // Get non-organizers
  const getNonOrganizers = useCallback(() => {
    return users.filter((user) => !user.isOrganizer)
  }, [users])

  return {
    users,
    isLoading,
    error,
    getUserById,
    updateUser,
    toggleOrganizerStatus,
    getOrganizers,
    getNonOrganizers,
  }
}
