"use client"

import { useCallback, useEffect, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { CredentialType } from "@/types/workflow"
import { IssuerWithScopes, UserCredential } from "@/types/issuer"
import { AlertTriangle, ShieldCheck } from "lucide-react"

const CREDENTIAL_OPTIONS: Array<{ value: CredentialType; label: string }> = [
  { value: "dpw_certified", label: "DPW Certified" },
  { value: "sfluv_verifier", label: "SFLuv Verifier" },
]

export default function IssuerPage() {
  const { authFetch, status, user } = useApp()
  const [scopes, setScopes] = useState<IssuerWithScopes | null>(null)
  const [lookupUserId, setLookupUserId] = useState<string>("")
  const [credentialType, setCredentialType] = useState<CredentialType>("dpw_certified")
  const [credentials, setCredentials] = useState<UserCredential[]>([])
  const [loading, setLoading] = useState<boolean>(true)
  const [submitting, setSubmitting] = useState<boolean>(false)
  const [error, setError] = useState<string>("")

  const canUsePanel = Boolean(user?.isIssuer || user?.isAdmin)

  const loadScopes = useCallback(async () => {
    if (!canUsePanel) {
      setLoading(false)
      return
    }

    try {
      const res = await authFetch("/issuers/scopes")
      if (!res.ok) throw new Error()
      const data = (await res.json()) as IssuerWithScopes
      setScopes(data)
      setError("")
    } catch {
      setError("Unable to load issuer scopes.")
    } finally {
      setLoading(false)
    }
  }, [authFetch, canUsePanel])

  useEffect(() => {
    if (status !== "authenticated") return
    loadScopes()
  }, [status, loadScopes])

  const issueCredential = async () => {
    if (!lookupUserId.trim()) {
      setError("User ID is required.")
      return
    }

    setSubmitting(true)
    try {
      const res = await authFetch("/issuers/credentials", {
        method: "POST",
        body: JSON.stringify({
          user_id: lookupUserId.trim(),
          credential_type: credentialType,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to issue credential.")
      }
      await fetchUserCredentials()
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to issue credential.")
    } finally {
      setSubmitting(false)
    }
  }

  const revokeCredential = async () => {
    if (!lookupUserId.trim()) {
      setError("User ID is required.")
      return
    }

    setSubmitting(true)
    try {
      const res = await authFetch("/issuers/credentials", {
        method: "DELETE",
        body: JSON.stringify({
          user_id: lookupUserId.trim(),
          credential_type: credentialType,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to revoke credential.")
      }
      await fetchUserCredentials()
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to revoke credential.")
    } finally {
      setSubmitting(false)
    }
  }

  const fetchUserCredentials = async () => {
    if (!lookupUserId.trim()) {
      setError("Enter a user ID to view credentials.")
      return
    }

    setSubmitting(true)
    try {
      const res = await authFetch(`/issuers/credentials/${lookupUserId.trim()}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load credentials.")
      }
      const data = (await res.json()) as UserCredential[]
      setCredentials(data || [])
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load credentials.")
    } finally {
      setSubmitting(false)
    }
  }

  if (status === "loading" || loading) {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (!canUsePanel) {
    return (
      <div className="container mx-auto p-4 space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Issuer Access Required</CardTitle>
            <CardDescription>
              Your account is not approved for credential issuing. Admins can grant issuer role and credential scopes.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Issuer Panel</h1>
        <p className="text-muted-foreground">Grant and revoke workflow credentials for users.</p>
      </div>

      {error && (
        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5" />
            Your Credential Scope
          </CardTitle>
          <CardDescription>Only credentials in your scope can be granted.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {(scopes?.allowed_credentials || []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No credential scopes assigned yet.</p>
          ) : (
            scopes?.allowed_credentials.map((credential) => <Badge key={credential}>{credential}</Badge>)
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Credential Actions</CardTitle>
          <CardDescription>Issue or revoke credentials for a specific user ID.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="issuer-user-id">User ID</Label>
            <Input
              id="issuer-user-id"
              value={lookupUserId}
              onChange={(e) => setLookupUserId(e.target.value)}
              placeholder="did:privy:..."
            />
          </div>

          <div className="space-y-2">
            <Label>Credential Type</Label>
            <Select value={credentialType} onValueChange={(value: CredentialType) => setCredentialType(value)}>
              <SelectTrigger>
                <SelectValue placeholder="Select credential type" />
              </SelectTrigger>
              <SelectContent>
                {CREDENTIAL_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button onClick={issueCredential} disabled={submitting}>
              Issue Credential
            </Button>
            <Button variant="outline" onClick={revokeCredential} disabled={submitting}>
              Revoke Credential
            </Button>
            <Button variant="secondary" onClick={fetchUserCredentials} disabled={submitting}>
              Refresh User Credentials
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>User Credential History</CardTitle>
          <CardDescription>Latest credential records for the selected user.</CardDescription>
        </CardHeader>
        <CardContent>
          {credentials.length === 0 ? (
            <p className="text-sm text-muted-foreground">No credentials found for the selected user.</p>
          ) : (
            <div className="space-y-3">
              {credentials.map((credential) => (
                <Card key={credential.id}>
                  <CardContent className="p-3 text-sm">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="font-medium">{credential.credential_type}</span>
                      <Badge variant={credential.is_revoked ? "destructive" : "default"}>
                        {credential.is_revoked ? "revoked" : "active"}
                      </Badge>
                    </div>
                    <p className="text-xs text-muted-foreground mt-1">
                      Issued: {new Date(credential.issued_at).toLocaleString()}
                    </p>
                    {credential.revoked_at && (
                      <p className="text-xs text-muted-foreground">
                        Revoked: {new Date(credential.revoked_at).toLocaleString()}
                      </p>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
