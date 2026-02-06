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

interface AddEventModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  handleAddEvent: (e: Event) => Promise<void>
  addEventError: unknown
  currentBalance: number
}

export function AddEventModal({ open, onOpenChange, handleAddEvent, addEventError, currentBalance }: AddEventModalProps) {
  const [title, setTitle] = useState<string>("")
  const [description, setDescription] = useState<string>("")
  const [amount, setAmount] = useState<number>(0)
  const [codes, setCodes] = useState<number>(0)
  const [expiration, setExpiration] = useState<number>(0)
  const [addError, setAddError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false)
  const [datePickerOpen, setDatePickerOpen] = useState<boolean>(false)
  const [timezone, setTimezone] = useState<string | undefined>(undefined)

  useEffect(() => {
    setTimezone(Intl.DateTimeFormat().resolvedOptions().timeZone)
  }, [])

  useEffect(() => {
    setTitle("")
    setDescription("")
    setAmount(0)
    setIsSubmitting(false)
  }, [open])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()

    if(codes <= 0) {
      setAddError("At least one code required per event.")
      return
    }

    if(amount <= 0) {
      setAddError("Code amount must be greater than 0.")
      return
    }

    if(codes * amount > currentBalance) {
      setAddError("Codes should not exceed current faucet balance.")
      return
    }

    if(expiration <= 0) {
      setAddError("Expiration date must be set.")
      return
    }

    setIsSubmitting(true)
    await handleAddEvent({
      id: "",
      title,
      description,
      amount,
      codes,
      expiration
    })
    if(!addEventError) {
      onOpenChange(open)
    } else {
      setAddError("Encountered a server error while adding contact. Please try again later.")
    }

    setIsSubmitting(false)
  }


  return (
    <Dialog open={open} onOpenChange={(open) => {
      onOpenChange(open)
    }}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">New Event</DialogTitle>
          <DialogDescription className="text-sm">
            Create a volunteer event with SFLuv Rewards.
          </DialogDescription>
        </DialogHeader>
        {isSubmitting ?
          <div className="min-h-64 flex items-center justify-center">
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
          </div>
        :
          <form
            className="space-y-4 sm:space-y-6"
            onSubmit={handleSubmit}
          >
            {/* Name */}
            <div className="space-y-2">
              <Label className="text-sm font-medium">Title *</Label>
              <Input
                value={title}
                className="font-mono text-xs sm:text-sm h-11"
                onChange={(e) => {
                  setTitle(e.target.value)
                }}
                autoComplete="off"
                />
            </div>

            {/* Address */}
            <div className="space-y-2">
              <Label className="text-sm font-medium">Description *</Label>
              <Input
                value={description}
                className="font-mono text-xs sm:text-sm h-11"
                onChange={(e) => {
                  setDescription(e.target.value)
                }}
                autoComplete="off"
                />
            </div>

            <div className="space-y-2">
              <Label className="text-sm font-medium">Codes *</Label>
              <Input
                value={codes}
                className="font-mono text-xs sm:text-sm h-11"
                onChange={(e) => {
                  setCodes(Number(e.target.value))
                }}
                autoComplete="off"
                />
            </div>

            <div className="space-y-2">
              <Label className="text-sm font-medium">Amount *</Label>
              <Input
                value={amount}
                className="font-mono text-xs sm:text-sm h-11"
                onChange={(e) => {
                  setAmount(Number(e.target.value))
                }}
                autoComplete="off"
                />
            </div>

            <div className="space-y-2">
              <Label className="text-sm font-medium">Expiration *</Label>
              <DateTimePicker
                date={expiration}
                setDate={setExpiration}
                open={datePickerOpen}
                setOpen={setDatePickerOpen}
                timezone={timezone}
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
                Submit
              </Button>
            </div>
          </form>
        }
      </DialogContent>
    </Dialog>
  )
}
