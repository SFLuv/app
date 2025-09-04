"use client"

import { useState, useEffect } from "react"

export function useRegisteredOpportunities() {
  const [registeredOpportunities, setRegisteredOpportunities] = useState<string[]>([])

  // Load registered opportunities from localStorage on mount
  useEffect(() => {
    const stored = localStorage.getItem("sfluv_registered_opportunities")
    if (stored) {
      setRegisteredOpportunities(JSON.parse(stored))
    }
  }, [])

  // Save to localStorage whenever the array changes
  useEffect(() => {
    localStorage.setItem("sfluv_registered_opportunities", JSON.stringify(registeredOpportunities))
  }, [registeredOpportunities])

  const registerForOpportunity = (opportunityId: string) => {
    setRegisteredOpportunities((prev) => [...prev, opportunityId])
  }

  const cancelRegistration = (opportunityId: string) => {
    setRegisteredOpportunities((prev) => prev.filter((id) => id !== opportunityId))
  }

  const isRegistered = (opportunityId: string) => {
    return registeredOpportunities.includes(opportunityId)
  }

  return {
    registeredOpportunities,
    registerForOpportunity,
    cancelRegistration,
    isRegistered,
  }
}
