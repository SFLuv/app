"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { QRCode } from "react-qrcode-logo"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent } from "@/components/ui/card"
import { Copy, CheckCircle, ChevronLeft, ChevronDown, ChevronRight, AlertTriangle } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { TabsTrigger, Tabs, TabsList } from "../ui/tabs"
import { Collapsible, CollapsibleTrigger } from "../ui/collapsible"
import { CollapsibleContent } from "@radix-ui/react-collapsible"
import { useContacts } from "@/context/ContactsProvider"
import { Form } from "../ui/form"
import { useApp } from "@/context/AppProvider"
import { validateAddress } from "@/lib/utils"
import { Contact } from "@/types/contact"
import { Event } from "@/types/event"
import { DateTimePicker } from "../ui/datetime-picker"
import { PonderSubscription } from "@/types/ponder"

interface NotificationModalProps {
  open: boolean
  id: number | undefined
  address: string
  emailAddress: string | undefined
  onOpenChange: (open: boolean) => void
}

export function NotificationModal({ open, id, address, emailAddress, onOpenChange }: NotificationModalProps) {
  const [email, setEmail] = useState<string>(emailAddress || "")
  const [amount, setAmount] = useState<number>(0)
  const [codes, setCodes] = useState<number>(0)
  const [expiration, setExpiration] = useState<number>(0)


  const [addError, setAddError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false)
  const [datePickerOpen, setDatePickerOpen] = useState<boolean>(false)
  const [timezone, setTimezone] = useState<string | undefined>(undefined)

  const {
    addPonderSubscription,
    getPonderSubscriptions,
    deletePonderSubscription
  } = useApp()

  useEffect(() => {
    setTimezone(Intl.DateTimeFormat().resolvedOptions().timeZone)
  }, [])

  useEffect(() => {
    setIsSubmitting(false)
  }, [open])

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

      if(email == "") {
        setAddError("Email must not be empty.")
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
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
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
              <Input
                value={email}
                className="font-mono text-xs sm:text-sm h-11"
                onChange={(e) => {
                  setEmail(e.target.value)
                }}
                autoComplete="off"
                />
            </div>

            {addError && (
              <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                <span>{addError}</span>
              </div>
            )}

            {/* Submit Button */}
            <div className="pt-2 text-center">
              <Button type="submit">
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
