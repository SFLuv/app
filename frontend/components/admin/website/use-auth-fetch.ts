"use client"

import { useCallback, useRef } from "react"

import { useApp } from "@/context/AppProvider"

/**
 * `authFetch` with a stable identity.
 *
 * The app's provider builds a new `authFetch` every time it re-renders, which is
 * often. These editors keep unsaved drafts in state and load from effects that
 * depend on the fetcher, so a changing identity would reload — and wipe — what
 * someone is in the middle of typing. This always calls the latest one, but never
 * changes itself.
 */
export function useAuthFetch() {
  const { authFetch } = useApp()
  const latest = useRef(authFetch)
  latest.current = authFetch
  return useCallback((endpoint: string, options?: RequestInit) => latest.current(endpoint, options), [])
}
