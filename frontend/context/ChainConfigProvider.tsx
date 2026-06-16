"use client"

import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from "react"
import { BACKEND } from "@/lib/constants"
import {
  CommunityConfigPayload,
  ResolvedCommunityConfig,
  resolveCommunityConfig,
} from "@/lib/community-config"

type ChainConfigState =
  | { status: "loading" }
  | { status: "ready"; config: ResolvedCommunityConfig }
  | { status: "error"; error: string }

const ChainConfigContext = createContext<ResolvedCommunityConfig | null>(null)

export function ChainConfigProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ChainConfigState>({ status: "loading" })

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const response = await fetch(`${BACKEND.replace(/\/+$/, "")}/config`, {
          headers: { Accept: "application/json" },
          cache: "no-store",
        })
        if (!response.ok) {
          throw new Error(`/config returned ${response.status}`)
        }
        const payload = (await response.json()) as CommunityConfigPayload
        const config = resolveCommunityConfig(payload)
        if (!cancelled) {
          setState({ status: "ready", config })
        }
      } catch (error) {
        console.error("[ChainConfig] failed to load", error)
        if (!cancelled) {
          setState({
            status: "error",
            error: error instanceof Error ? error.message : "Unable to load blockchain config",
          })
        }
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [])

  const value = useMemo(() => (state.status === "ready" ? state.config : null), [state])

  if (state.status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]" />
      </div>
    )
  }

  if (state.status === "error") {
    return (
      <div className="min-h-screen flex items-center justify-center p-6 text-center">
        <div>
          <h1 className="text-lg font-semibold">Unable to start SFLuv</h1>
          <p className="mt-2 text-sm text-muted-foreground">{state.error}</p>
        </div>
      </div>
    )
  }

  return <ChainConfigContext.Provider value={value}>{children}</ChainConfigContext.Provider>
}

export function useChainConfig(): ResolvedCommunityConfig {
  const config = useContext(ChainConfigContext)
  if (!config) {
    throw new Error("useChainConfig must be used inside ChainConfigProvider")
  }
  return config
}
