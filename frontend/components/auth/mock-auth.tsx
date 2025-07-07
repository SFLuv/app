"use client"

import type React from "react"

import { useApp, type UserRole } from "@/context/app-context"
import { useRouter } from "next/navigation"
import { useState } from "react"

const mockUsers = [
  {
    id: "user1",
    name: "John Doe",
    email: "user@example.com",
    password: "password",
    role: "user" as UserRole,
    isOrganizer: false,
  },
  {
    id: "merchant1",
    name: "Jane Smith",
    email: "merchant@example.com",
    password: "password",
    role: "merchant" as UserRole,
    isOrganizer: false,
  },
  {
    id: "admin1",
    name: "Admin User",
    email: "admin@example.com",
    password: "password",
    role: "admin" as UserRole,
    isOrganizer: true,
  },
  {
    id: "organizer1",
    name: "Sam Organizer",
    email: "organizer@example.com",
    password: "password",
    role: "user" as UserRole,
    isOrganizer: true,
  },
  {
    id: "merchant-organizer1",
    name: "Merchant Organizer",
    email: "merchant-organizer@example.com",
    password: "password",
    role: "merchant" as UserRole,
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
      login({
        id: user.id,
        name: user.name,
        email: user.email,
        role: user.role,
        isOrganizer: user.isOrganizer,
      })
      router.push("/dashboard")
    } else {
      setError("Invalid email or password")
    }
  }

  const mockLogin = (email: string, password: string, role: UserRole = "user") => {
    // In a real app, this would be an API call
    const mockUser = {
      id: "user-123",
      name: email.split("@")[0],
      email,
      role,
      avatar: `https://ui-avatars.com/api/?name=${encodeURIComponent(email.split("@")[0])}&background=eb6c6c&color=fff`,
    }

    const userData = {
      id: "user-123",
      name: email.split("@")[0],
      email,
      role,
      avatar: `https://ui-avatars.com/api/?name=${encodeURIComponent(email.split("@")[0])}&background=eb6c6c&color=fff`,
    }

    // Set merchant status for merchant role
    if (role === "merchant") {
      userData.merchantStatus = "approved"
    }

    login(userData)
    router.push("/dashboard")
  }

  return { mockLogin, handleLogin, setEmail, setPassword, error }
}
