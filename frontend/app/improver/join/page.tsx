"use client"

import { FormEvent, useEffect, useMemo, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { VerifiedEmailResponse } from "@/types/server"
import { GlobalCredentialType } from "@/types/workflow"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

const slugify = (value: string) =>
  value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")

const toDisplayRole = (value: string) =>
  value
    .trim()
    .replace(/[-_]+/g, " ")
    .replace(/\s+/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase())

const findCredentialMatch = (role: string, types: GlobalCredentialType[]) => {
  const normalized = role.trim().toLowerCase()
  const roleSlug = slugify(role)
  return (
    types.find((type) => type.value.toLowerCase() === normalized) ||
    types.find((type) => type.label.toLowerCase() === normalized) ||
    types.find((type) => slugify(type.value) === roleSlug) ||
    types.find((type) => slugify(type.label) === roleSlug) ||
    null
  )
}

export default function ImproverJoinPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { status, login, user, authFetch, setImprover, updateUser, ensurePrimarySmartWallet } = useApp()

  const roleParam = (searchParams.get("role") || "").trim()

  const [firstName, setFirstName] = useState("")
  const [lastName, setLastName] = useState("")
  const [verifiedEmails, setVerifiedEmails] = useState<VerifiedEmailResponse[]>([])
  const [selectedEmail, setSelectedEmail] = useState("")
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [credentialTypesLoaded, setCredentialTypesLoaded] = useState(false)
  const [loadingData, setLoadingData] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    if (!user?.name) return
    if (firstName || lastName) return
    const parts = user.name.trim().split(/\s+/).filter(Boolean)
    if (parts.length === 0) return
    const parsedFirstName = parts[0]
    if (parsedFirstName.trim().toLowerCase() === "user") {
      return
    }
    setFirstName(parsedFirstName)
    setLastName(parts.slice(1).join(" "))
  }, [user?.name, firstName, lastName])

  useEffect(() => {
    if (status !== "authenticated") return
    let cancelled = false

    const loadFormData = async () => {
      setLoadingData(true)
      setError("")
      try {
        const [emailsRes, credentialTypesRes] = await Promise.all([
          authFetch("/users/verified-emails"),
          authFetch("/credentials/types"),
        ])
        if (!emailsRes.ok) {
          throw new Error((await emailsRes.text()) || "Unable to load verified emails.")
        }
        if (!credentialTypesRes.ok) {
          throw new Error((await credentialTypesRes.text()) || "Unable to load role data.")
        }

        const emails = ((await emailsRes.json()) as VerifiedEmailResponse[]) || []
        const types = ((await credentialTypesRes.json()) as GlobalCredentialType[]) || []
        const verifiedOnly = emails.filter((email) => email.status === "verified")

        if (cancelled) return
        setVerifiedEmails(verifiedOnly)
        setCredentialTypes(types)
        setCredentialTypesLoaded(true)
        setSelectedEmail((current) => current || verifiedOnly[0]?.email || "")
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : "Unable to load join form.")
      } finally {
        if (!cancelled) {
          setLoadingData(false)
        }
      }
    }

    void loadFormData()
    return () => {
      cancelled = true
    }
  }, [status])

  const credentialMatch = useMemo(
    () => (roleParam ? findCredentialMatch(roleParam, credentialTypes) : null),
    [roleParam, credentialTypes],
  )
  const roleNotFound = !roleParam || (status === "authenticated" && credentialTypesLoaded && !credentialMatch)
  const credentialTypeValue = credentialMatch?.value || roleParam
  const roleDisplayName = credentialMatch?.label || toDisplayRole(roleParam)
  const canSubmit = Boolean(
    roleParam &&
      firstName.trim() &&
      lastName.trim() &&
      selectedEmail &&
      verifiedEmails.length > 0 &&
      credentialMatch,
  )

  const handleJoin = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError("")

    if (!roleParam || !credentialMatch) {
      setError("Role not found.")
      return
    }
    if (!firstName.trim() || !lastName.trim()) {
      setError("First name and last name are required.")
      return
    }
    if (!selectedEmail) {
      setError("A verified email is required.")
      return
    }
    setSubmitting(true)
    try {
      const hasSmartWalletIndexZero = await ensurePrimarySmartWallet()
      if (!hasSmartWalletIndexZero) {
        throw new Error("Primary smart wallet is still initializing. Please wait a few seconds and try again.")
      }

      const improverRes = await authFetch("/improvers/request", {
        method: "POST",
        body: JSON.stringify({
          first_name: firstName.trim(),
          last_name: lastName.trim(),
          email: selectedEmail,
        }),
      })
      if (!improverRes.ok && improverRes.status !== 409) {
        throw new Error((await improverRes.text()) || "Unable to request improver status.")
      }
      if (improverRes.ok) {
        const improver = await improverRes.json()
        setImprover(improver)
      }
      updateUser({
        isImprover: true,
        name: `${firstName.trim()} ${lastName.trim()}`.trim(),
        contact_email: selectedEmail,
      })

      const credentialRes = await authFetch("/improvers/credential-requests", {
        method: "POST",
        body: JSON.stringify({ credential_type: credentialTypeValue }),
      })
      if (!credentialRes.ok && credentialRes.status !== 409) {
        throw new Error((await credentialRes.text()) || "Unable to request credential status.")
      }

      setSuccess(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to complete improver join request.")
    } finally {
      setSubmitting(false)
    }
  }

  if (roleNotFound) {
    return (
      <div className="mx-auto w-full max-w-xl py-8">
        <Card>
          <CardHeader>
            <CardTitle>Role not found</CardTitle>
          </CardHeader>
          <CardContent>
            <Button onClick={() => router.push("/map")} className="w-full sm:w-auto">
              Go back home
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (status === "loading") {
    return (
      <div className="min-h-[60vh] flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]" />
      </div>
    )
  }

  if (status === "unauthenticated") {
    return (
      <div className="mx-auto w-full max-w-xl py-8">
        <Card className="border-[#eb6c6c]/35 bg-[#eb6c6c]/5">
          <CardHeader>
            <CardTitle>Log in to continue</CardTitle>
            <CardDescription>
              Sign in to request {roleDisplayName || "this role"} status.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button onClick={login} className="w-full sm:w-auto bg-[#eb6c6c] hover:bg-[#d55c5c]">
              Create Account / Log In
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (success) {
    return (
      <div className="mx-auto w-full max-w-xl py-8">
        <Card className="border-green-300 dark:border-green-700">
          <CardHeader>
            <CardTitle>{roleDisplayName} status successfully requested!</CardTitle>
          </CardHeader>
          <CardContent>
            <Button onClick={() => router.push("/map")} className="w-full sm:w-auto">
              Return to app
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="mx-auto w-full max-w-xl py-8">
      <Card>
        <CardHeader>
          <CardTitle>Join as {roleDisplayName}</CardTitle>
          <CardDescription>
            Enter your details to request improver status and submit your credential request.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loadingData ? (
            <div className="min-h-[180px] flex items-center justify-center">
              <div className="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 border-[#eb6c6c]" />
            </div>
          ) : (
            <form className="space-y-4" onSubmit={handleJoin}>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="improver-join-first-name">First name</Label>
                  <Input
                    id="improver-join-first-name"
                    value={firstName}
                    onChange={(event) => setFirstName(event.target.value)}
                    placeholder="First name"
                    autoComplete="given-name"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="improver-join-last-name">Last name</Label>
                  <Input
                    id="improver-join-last-name"
                    value={lastName}
                    onChange={(event) => setLastName(event.target.value)}
                    placeholder="Last name"
                    autoComplete="family-name"
                  />
                </div>
              </div>

              {verifiedEmails.length > 1 ? (
                <div className="space-y-2">
                  <Label htmlFor="improver-join-email">Verified email</Label>
                  <Select value={selectedEmail} onValueChange={setSelectedEmail}>
                    <SelectTrigger id="improver-join-email">
                      <SelectValue placeholder="Select a verified email" />
                    </SelectTrigger>
                    <SelectContent>
                      {verifiedEmails.map((email) => (
                        <SelectItem key={email.id} value={email.email}>
                          {email.email}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ) : (
                <div className="space-y-2">
                  <Label htmlFor="improver-join-email-single">Verified email</Label>
                  <Input id="improver-join-email-single" value={selectedEmail} readOnly />
                </div>
              )}

              {verifiedEmails.length === 0 && (
                <p className="text-sm text-red-600">
                  No verified email found. Add and verify an email in settings, then reopen this link.
                </p>
              )}
              {error && <p className="text-sm text-red-600">{error}</p>}

              <Button type="submit" disabled={!canSubmit || submitting} className="w-full sm:w-auto">
                {submitting ? "Submitting..." : `Request ${roleDisplayName} Status`}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
