"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { buildCredentialLabelMap, formatCredentialLabel } from "@/lib/credential-labels"
import { CredentialRequest, IssuerWithScopes } from "@/types/issuer"
import { GlobalCredentialType } from "@/types/workflow"
import { AlertTriangle, CheckCircle2, ChevronLeft, ChevronRight, ClipboardList, Search, ShieldCheck } from "lucide-react"

export default function IssuerPage() {
  const { authFetch, status, user } = useApp()
  const [scopes, setScopes] = useState<IssuerWithScopes | null>(null)
  const [requests, setRequests] = useState<CredentialRequest[]>([])
  const [credentialTypes, setCredentialTypes] = useState<GlobalCredentialType[]>([])
  const [statusFilter, setStatusFilter] = useState<"all" | "pending" | "approved" | "rejected">("all")
  const [credentialFilter, setCredentialFilter] = useState<string>("all")
  const [issuerSearch, setIssuerSearch] = useState<string>("")
  const [issuerPage, setIssuerPage] = useState<number>(0)
  const [modalOpen, setModalOpen] = useState<boolean>(false)
  const [selectedRequest, setSelectedRequest] = useState<CredentialRequest | null>(null)
  const [statusDraft, setStatusDraft] = useState<CredentialRequest["status"]>("pending")
  const [loading, setLoading] = useState<boolean>(true)
  const [saving, setSaving] = useState<boolean>(false)
  const [error, setError] = useState<string>("")
  const [notice, setNotice] = useState<string>("")

  const credentialLabelMap = useMemo(
    () => buildCredentialLabelMap(credentialTypes),
    [credentialTypes],
  )

  const getCredentialLabel = useCallback(
    (credential: string) => formatCredentialLabel(credential, credentialLabelMap),
    [credentialLabelMap],
  )

  const canUsePanel = Boolean(user?.isIssuer || user?.isAdmin)

  const loadIssuerMetadata = useCallback(async () => {
    if (!canUsePanel) {
      setLoading(false)
      return
    }

    try {
      const [scopeRes, credentialTypesRes] = await Promise.all([
        authFetch("/issuers/scopes"),
        authFetch("/credentials/types"),
      ])
      if (!scopeRes.ok) throw new Error("Unable to load issuer scope.")

      const scopeData = (await scopeRes.json()) as IssuerWithScopes
      const credentialTypeData = credentialTypesRes.ok
        ? ((await credentialTypesRes.json()) as GlobalCredentialType[])
        : []

      setScopes(scopeData)
      setCredentialTypes(credentialTypeData || [])
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load issuer data.")
    } finally {
      setLoading(false)
    }
  }, [authFetch, canUsePanel])

  const loadRequests = useCallback(async (search: string, page: number) => {
    if (!canUsePanel) return
    try {
      const params = new URLSearchParams({ page: String(page), count: "20" })
      if (search) params.set("search", search)
      const res = await authFetch(`/issuers/credential-requests?${params}`)
      if (!res.ok) throw new Error("Unable to load credential requests.")
      const data = (await res.json()) as CredentialRequest[]
      setRequests(data || [])
      setError("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load credential requests.")
    }
  }, [authFetch, canUsePanel])

  useEffect(() => {
    if (status !== "authenticated") return
    loadIssuerMetadata()
  }, [status, loadIssuerMetadata])

  useEffect(() => {
    if (status !== "authenticated") return
    loadRequests(issuerSearch, issuerPage)
  }, [status, issuerSearch, issuerPage, loadRequests])

  const credentialFilterOptions = useMemo(() => {
    const scoped = scopes?.allowed_credentials || []
    const deduped = new Set<string>()
    scoped.forEach((credential) => {
      const value = credential.trim()
      if (value) deduped.add(value)
    })

    if (deduped.size === 0 && user?.isAdmin) {
      credentialTypes.forEach((credentialType) => {
        const value = credentialType.value.trim()
        if (value) deduped.add(value)
      })
    }

    return Array.from(deduped)
  }, [credentialTypes, scopes?.allowed_credentials, user?.isAdmin])

  useEffect(() => {
    if (credentialFilter === "all") return
    if (!credentialFilterOptions.includes(credentialFilter)) {
      setCredentialFilter("all")
    }
  }, [credentialFilter, credentialFilterOptions])

  const filteredRequests = useMemo(() => {
    return requests.filter((request) => {
      const matchesStatus = statusFilter === "all" || request.status === statusFilter
      const matchesCredential = credentialFilter === "all" || request.credential_type === credentialFilter
      return matchesStatus && matchesCredential
    })
  }, [credentialFilter, requests, statusFilter])

  const openRequestModal = (request: CredentialRequest) => {
    setSelectedRequest(request)
    setStatusDraft(request.status)
    setError("")
    setNotice("")
    setModalOpen(true)
  }

  const saveDecision = async () => {
    if (!selectedRequest) return

    setSaving(true)
    try {
      const res = await authFetch(`/issuers/credential-requests/${encodeURIComponent(selectedRequest.id)}/decision`, {
        method: "POST",
        body: JSON.stringify({ status: statusDraft }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to resolve credential request.")
      }
      const updated = (await res.json()) as CredentialRequest
      setRequests((prev) => prev.map((request) => (request.id === updated.id ? updated : request)))
      setSelectedRequest(updated)
      setNotice(
        updated.status === "approved"
          ? "Credential request status updated to approved."
          : updated.status === "rejected"
            ? "Credential request status updated to rejected."
            : "Credential request status updated to pending."
      )
      setError("")
      setModalOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to resolve credential request.")
    } finally {
      setSaving(false)
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
        <p className="text-muted-foreground">Review credential requests and approve or reject them.</p>
      </div>

      {error && (
        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {notice && (
        <div className="flex items-center gap-2 text-green-700 text-sm p-3 bg-green-50 dark:bg-green-900/20 rounded-lg">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          <span>{notice}</span>
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
            scopes?.allowed_credentials.map((credential) => <Badge key={credential}>{getCredentialLabel(credential)}</Badge>)
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ClipboardList className="h-5 w-5" />
            Credential Requests
          </CardTitle>
          <CardDescription>
            Requests are shown only for credential types your issuer account can grant.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-3">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by name or email..."
                value={issuerSearch}
                onChange={(e) => { setIssuerSearch(e.target.value); setIssuerPage(0) }}
                className="pl-9"
              />
            </div>
            <div className="grid w-full gap-3 sm:grid-cols-2">
              <Select value={statusFilter} onValueChange={(value) => setStatusFilter(value as "all" | "pending" | "approved" | "rejected")}>
                <SelectTrigger>
                  <SelectValue placeholder="Filter by status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Statuses</SelectItem>
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="approved">Approved</SelectItem>
                  <SelectItem value="rejected">Rejected</SelectItem>
                </SelectContent>
              </Select>
              <Select value={credentialFilter} onValueChange={setCredentialFilter}>
                <SelectTrigger>
                  <SelectValue placeholder="Filter by credential type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Credential Types</SelectItem>
                  {credentialFilterOptions.map((credential) => (
                    <SelectItem key={credential} value={credential}>
                      {getCredentialLabel(credential)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {filteredRequests.length === 0 ? (
            <p className="text-sm text-muted-foreground">No credential requests match the selected filter.</p>
          ) : (
            <div className="space-y-3">
              {filteredRequests.map((request) => (
                <Card key={request.id} className="cursor-pointer hover:border-primary/40 transition-colors" onClick={() => openRequestModal(request)}>
                  <CardContent className="p-4 text-sm space-y-2">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="font-medium">{request.requester_name}</span>
                      <Badge variant={request.status === "approved" ? "default" : request.status === "rejected" ? "destructive" : "outline"}>
                        {request.status}
                      </Badge>
                    </div>
                    <p className="text-xs text-muted-foreground">{request.requester_email}</p>
                    <p className="text-xs text-muted-foreground">Credential: {getCredentialLabel(request.credential_type)}</p>
                    <p className="text-xs text-muted-foreground">Requested: {new Date(request.requested_at).toLocaleString()}</p>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          <div className="flex items-center justify-between pt-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIssuerPage((p) => Math.max(0, p - 1))}
              disabled={issuerPage === 0}
            >
              <ChevronLeft className="h-4 w-4" />
              Previous
            </Button>
            <span className="text-sm text-muted-foreground">Page {issuerPage + 1}</span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIssuerPage((p) => p + 1)}
              disabled={requests.length < 20}
            >
              Next
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Credential Request</DialogTitle>
            <DialogDescription>
              Review requester details and set the request outcome.
            </DialogDescription>
          </DialogHeader>

          {selectedRequest && (
            <div className="space-y-3">
              <div className="space-y-1">
                <Label>Requester</Label>
                <p className="text-sm">{selectedRequest.requester_name}</p>
              </div>
              <div className="space-y-1">
                <Label>Requester Email</Label>
                <p className="text-sm">{selectedRequest.requester_email}</p>
              </div>
              <div className="space-y-1">
                <Label>Requester User ID</Label>
                <p className="text-xs font-mono break-all">{selectedRequest.user_id}</p>
              </div>
              <div className="space-y-1">
                <Label>Credential Type</Label>
                <p className="text-sm">{getCredentialLabel(selectedRequest.credential_type)}</p>
              </div>
              <div className="space-y-1">
                <Label>Current Status</Label>
                <Badge variant={selectedRequest.status === "approved" ? "default" : selectedRequest.status === "rejected" ? "destructive" : "outline"}>
                  {selectedRequest.status}
                </Badge>
              </div>

              <div className="space-y-1">
                <Label>Change Approval Status</Label>
                <Select value={statusDraft} onValueChange={(value) => setStatusDraft(value as CredentialRequest["status"])}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select status" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="pending">Pending</SelectItem>
                    <SelectItem value="approved">Approved</SelectItem>
                    <SelectItem value="rejected">Rejected</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          <DialogFooter>
            <Button variant="outline" onClick={() => setModalOpen(false)}>
              Close
            </Button>
            <Button onClick={saveDecision} disabled={saving}>
              {saving ? "Saving..." : "Save Changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
