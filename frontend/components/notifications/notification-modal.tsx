"use client"

import { useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { AlertTriangle, Loader2 } from "lucide-react"
import { useApp } from "@/context/AppProvider"
import { VerifiedEmailResponse } from "@/types/server"

interface NotificationModalProps {
  open: boolean
  id: number | undefined
  address: string
  emailAddress: string | undefined
  onOpenChange: (open: boolean) => void
}

export function NotificationModal({ open, id, address, emailAddress, onOpenChange }: NotificationModalProps) {
  const [email, setEmail] = useState<string>(emailAddress || "")
  const [verifiedEmails, setVerifiedEmails] = useState<VerifiedEmailResponse[]>([])
  const [addError, setAddError] = useState<string | null>(null)
  const [loadingEmails, setLoadingEmails] = useState<boolean>(false)
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false)

  const {
    addPonderSubscription,
    getPonderSubscriptions,
    deletePonderSubscription,
    authFetch,
  } = useApp()
  const router = useRouter()

  const verifiedEmailOptions = useMemo(
    () => verifiedEmails.filter((entry) => entry.status === "verified"),
    [verifiedEmails],
  )

  useEffect(() => {
    setIsSubmitting(false)
  }, [open])

  useEffect(() => {
    if (!open || Boolean(id)) return
    let ignore = false

    const loadVerifiedEmails = async () => {
      setLoadingEmails(true)
      setAddError(null)
      try {
        const res = await authFetch("/users/verified-emails")
        if (!res.ok) {
          const text = await res.text()
          throw new Error(text || "Unable to load verified emails.")
        }
        const data = (await res.json()) as VerifiedEmailResponse[]
        if (ignore) return
        setVerifiedEmails(data || [])
      } catch (err) {
        if (ignore) return
        setAddError(err instanceof Error ? err.message : "Unable to load verified emails.")
      } finally {
        if (!ignore) {
          setLoadingEmails(false)
        }
      }
    }

    void loadVerifiedEmails()
    return () => {
      ignore = true
    }
  }, [open, id, authFetch])

  useEffect(() => {
    if (id) {
      setEmail(emailAddress || "")
      return
    }
    if (verifiedEmailOptions.length === 0) {
      setEmail("")
      return
    }

    const existing = verifiedEmailOptions.some((entry) => entry.email === email)
    if (!existing) {
      setEmail(verifiedEmailOptions[0].email)
    }
  }, [id, emailAddress, verifiedEmailOptions, email])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    try {
      setIsSubmitting(true)

      if(id) {
        await deletePonderSubscription(id)
        await getPonderSubscriptions()
        onOpenChange(!open)
        return
      }

      if (email === "") {
        setAddError("Email must not be empty.")
        return
      }

      await addPonderSubscription(email, address)
      await getPonderSubscriptions()
      onOpenChange(!open)
    }
    catch {
      setAddError("Something went wrong. Please try again later.")
    }
    finally {
      setIsSubmitting(false)
    }
  }


  return (
    <Dialog open={open} onOpenChange={(open) => {
      onOpenChange(open)
    }}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">Email Notifications</DialogTitle>
          <DialogDescription className="text-sm">
            {id ? "Disable" : "Enable"} notifications for {address.slice(0, 8)}...{address.slice(-6)}.
          </DialogDescription>
        </DialogHeader>
        {isSubmitting ?
          <div className="min-h-64 flex items-center justify-center">
            <Loader2 className="h-8 w-8 animate-spin text-[#eb6c6c]" />
          </div>
        :
        <>{
          id ?
          <form
            className="space-y-4 sm:space-y-6"
            onSubmit={handleSubmit}
          >
            <div
              className="space-y-4 sm:space-y-6"
            >
              <Label className="text-sm font-medium">Email *</Label>
              <Input
                value={email}
                className="font-mono text-xs sm:text-sm h-11"
                autoComplete="off"
                readOnly
              />
            </div>

            <div className="pt-2 text-center">
              <Button type="submit">
                Disable
              </Button>
            </div>
          </form>
          :
          <form
            className="space-y-4 sm:space-y-6"
            onSubmit={handleSubmit}
          >
            {/* Name */}
            <div className="space-y-2">
              <Label className="text-sm font-medium">Email *</Label>
              {loadingEmails ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading verified emails...
                </div>
              ) : verifiedEmailOptions.length > 0 ? (
                <Select value={email} onValueChange={setEmail}>
                  <SelectTrigger className="font-mono text-xs sm:text-sm h-11">
                    <SelectValue placeholder="Select a verified email" />
                  </SelectTrigger>
                  <SelectContent>
                    {verifiedEmailOptions.map((entry) => (
                      <SelectItem key={entry.id} value={entry.email}>
                        {entry.email}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <div className="rounded border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-800 dark:border-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300 space-y-2">
                  <p>No verified emails found for your account.</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      onOpenChange(false)
                      router.push("/settings")
                    }}
                  >
                    Go to Settings
                  </Button>
                </div>
              )}
            </div>

            {addError && (
              <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                <span>{addError}</span>
              </div>
            )}

            {/* Submit Button */}
            <div className="pt-2 text-center">
              <Button type="submit" disabled={verifiedEmailOptions.length === 0 || loadingEmails}>
                Enable
              </Button>
            </div>
          </form>
        }</>
        }
      </DialogContent>
    </Dialog>
  )
}
