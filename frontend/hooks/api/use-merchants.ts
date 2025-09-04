"use client"

import { useState, useEffect, useCallback } from "react"
import { mockMerchants } from "@/data/mock-merchants"
import type { Merchant, MerchantStatus } from "@/types/merchant"

export function useMerchants() {
  const [merchants, setMerchants] = useState<Merchant[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)

  // Fetch all merchants
  useEffect(() => {
    const fetchMerchants = async () => {
      try {
        setIsLoading(true)
        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500))
        setMerchants(mockMerchants)
        setError(null)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to fetch merchants"))
      } finally {
        setIsLoading(false)
      }
    }

    fetchMerchants()
  }, [])

  // Get merchant by ID
  const getMerchantById = useCallback(
    (id: string) => {
      return merchants.find((merchant) => merchant.id === id) || null
    },
    [merchants],
  )

  // Update merchant status
  const updateMerchantStatus = useCallback(
    async (id: string, status: MerchantStatus) => {
      try {
        setIsLoading(true)
        // Simulate API call
        await new Promise((resolve) => setTimeout(resolve, 500))

        setMerchants((prev) => prev.map((merchant) => (merchant.id === id ? { ...merchant, status } : merchant)))

        return getMerchantById(id)
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to update merchant status"))
        throw err
      } finally {
        setIsLoading(false)
      }
    },
    [getMerchantById],
  )

  // Filter merchants by status
  const getMerchantsByStatus = useCallback(
    (status: MerchantStatus | "all") => {
      if (status === "all") return merchants
      return merchants.filter((merchant) => merchant.status === status)
    },
    [merchants],
  )

  return {
    merchants,
    isLoading,
    error,
    getMerchantById,
    updateMerchantStatus,
    getMerchantsByStatus,
  }
}
