"use client"

import type React from "react"

import { useRouter } from "next/navigation"
import { useState } from "react"
import { useApp, User } from "@/context/AppProvider"

const mockUsers = [
  {
    id: "user1",
    name: "John Doe",
    email: "user@example.com",
    password: "password",
    isOrganizer: false,
  },
  {
    id: "merchant1",
    name: "Jane Smith",
    email: "merchant@example.com",
    password: "password",
    isOrganizer: false,
  },
  {
    id: "admin1",
    name: "Admin User",
    email: "admin@example.com",
    password: "password",
    isOrganizer: true,
  },
  {
    id: "organizer1",
    name: "Sam Organizer",
    email: "organizer@example.com",
    password: "password",
    isOrganizer: true,
  },
  {
    id: "merchant-organizer1",
    name: "Merchant Organizer",
    email: "merchant-organizer@example.com",
    password: "password",
    isOrganizer: true,
  },
]

// This is a mock function to simulate login for testing purposes
export function useMockAuth() {
  const { login } = useApp()
  const router = useRouter()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | null>(null)

  const handleLogin = (e: React.FormEvent) => {
    e.preventDefault()

    const user = mockUsers.find((u) => u.email === email && u.password === password)

    if (user) {
      login()
      router.push("/dashboard")
    } else {
      setError("Invalid email or password")
    }
  }

  const mockLogin = (email: string, password: string) => {
    // In a real app, this would be an API call
    const mockUser = {
      id: "user-123",
      name: email.split("@")[0],
      email,
      avatar: `https://ui-avatars.com/api/?name=${encodeURIComponent(email.split("@")[0])}&background=eb6c6c&color=fff`,
    }

    const userData: User = {
      id: "user-123",
      name: email.split("@")[0],
      contact_email: email,
      isAdmin: false,
      isMerchant: false,
      isOrganizer: false,
      isAffiliate: false,
    }


    login()
    router.push("/dashboard")
  }

  return { mockLogin, handleLogin, setEmail, setPassword, error }
}
