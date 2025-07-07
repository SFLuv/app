"use client"

import { createContext, useContext, useState, useEffect, type ReactNode } from "react"

export type UserRole = "user" | "merchant" | "admin" | null
export type UserStatus = "loading" | "authenticated" | "unauthenticated"
export type MerchantApprovalStatus = "pending" | "approved" | "rejected" | null

interface User {
  id: string
  name: string
  email: string
  role: UserRole
  avatar?: string
  isOrganizer?: boolean
  merchantStatus?: MerchantApprovalStatus
  merchantProfile?: {
    businessName: string
    description: string
    address: {
      street: string
      city: string
      state: string
      zip: string
    }
    contactInfo: {
      phone: string
      website?: string
    }
    businessType: string
  }
}

interface AppContextType {
  user: User | null
  status: UserStatus
  login: (user: User) => void
  logout: () => void
  updateUser: (data: Partial<User>) => void
  requestMerchantStatus: (merchantProfile: User["merchantProfile"]) => void
  approveMerchantStatus: () => void
  rejectMerchantStatus: () => void
}

const AppContext = createContext<AppContextType | undefined>(undefined)

export function AppProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [status, setStatus] = useState<UserStatus>("loading")

  // Check for existing session on mount
  useEffect(() => {
    const checkSession = async () => {
      try {
        // In a real app, this would be an API call to verify the session
        const storedUser = localStorage.getItem("sfluv_user")

        if (storedUser) {
          const userData = JSON.parse(storedUser)
          setUser(userData)
          setStatus("authenticated")
        } else {
          setStatus("unauthenticated")
        }
      } catch (error) {
        console.error("Failed to restore session:", error)
        setStatus("unauthenticated")
      }
    }

    checkSession()
  }, [])

  const login = (userData: User) => {
    setUser(userData)
    // Set isOrganizer to true for admins by default
    if (userData.role === "admin" && !userData.isOrganizer) {
      userData.isOrganizer = true
    }
    setStatus("authenticated")
    localStorage.setItem("sfluv_user", JSON.stringify(userData))
  }

  const logout = () => {
    setUser(null)
    setStatus("unauthenticated")
    localStorage.removeItem("sfluv_user")
  }

  const updateUser = (data: Partial<User>) => {
    if (user) {
      const updatedUser = { ...user, ...data }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  const requestMerchantStatus = (merchantProfile: User["merchantProfile"]) => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "pending" as MerchantApprovalStatus,
        merchantProfile,
      }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  const approveMerchantStatus = () => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "approved" as MerchantApprovalStatus,
        role: "merchant" as UserRole,
      }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  const rejectMerchantStatus = () => {
    if (user) {
      const updatedUser = {
        ...user,
        merchantStatus: "rejected" as MerchantApprovalStatus,
      }
      setUser(updatedUser)
      localStorage.setItem("sfluv_user", JSON.stringify(updatedUser))
    }
  }

  return (
    <AppContext.Provider
      value={{
        user,
        status,
        login,
        logout,
        updateUser,
        requestMerchantStatus,
        approveMerchantStatus,
        rejectMerchantStatus,
      }}
    >
      {children}
    </AppContext.Provider>
  )
}

export function useApp() {
  const context = useContext(AppContext)
  if (context === undefined) {
    throw new Error("useApp must be used within an AppProvider")
  }
  return context
}
