"use client"

import { useEffect, useMemo } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { getAddress, isAddress } from "viem"
import { AlertTriangle, Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"

export default function AddContactQueryPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const rawAddress = searchParams.get("address") || searchParams.get("addContact") || searchParams.get("contact") || ""
  const normalizedAddress = useMemo(() => (isAddress(rawAddress) ? getAddress(rawAddress) : ""), [rawAddress])

  useEffect(() => {
    if (!normalizedAddress) return
    router.replace(`/addcontact/${normalizedAddress}`)
  }, [normalizedAddress, router])

  if (normalizedAddress) {
    return (
      <main className="flex min-h-[70vh] items-center justify-center">
        <Loader2 className="h-10 w-10 animate-spin text-[#eb6c6c]" />
      </main>
    )
  }

  return (
    <main className="mx-auto flex min-h-[70vh] max-w-lg items-center justify-center px-4">
      <Card className="w-full">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive" />
            Invalid contact link
          </CardTitle>
          <CardDescription>This contact link does not contain a valid wallet address.</CardDescription>
        </CardHeader>
        <CardContent>
          <Button onClick={() => router.replace("/contacts")}>Go to contacts</Button>
        </CardContent>
      </Card>
    </main>
  )
}
