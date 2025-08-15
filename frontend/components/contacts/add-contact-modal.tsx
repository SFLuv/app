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

interface AddContactModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  handleAddContact: (c: Contact) => Promise<void>
  addContactError: unknown
}

export function AddContactModal({ open, onOpenChange, handleAddContact, addContactError }: AddContactModalProps) {
  const [name, setName] = useState<string>("")
  const [address, setAddress] = useState<string>("")
  const [addError, setAddError]= useState<string | null>()
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false)

  useEffect(() => {
    setAddError(null)
    setName("")
    setAddress("")
    setIsSubmitting(false)
  }, [open])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()

    if(!validateAddress(address)) {
      setAddError("Invalid contact address.")
      return
    }

    if(name.length < 2) {
      setAddError("Contact name must be at least 2 characters long.")
      return
    }

    setIsSubmitting(true)
    await handleAddContact({
      id: 0,
      owner: "",
      name,
      address,
      is_favorite: false
    })
    if(!addContactError) {
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
          <DialogTitle className="text-lg sm:text-xl">Add Contact</DialogTitle>
          <DialogDescription className="text-sm">
            Create a contact for token transfers and tip address specification.
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
              <Label className="text-sm font-medium">Name *</Label>
              <Input
                value={name}
                className="font-mono text-xs sm:text-sm h-11"
                onChange={(e) => {
                  setName(e.target.value)
                }}
                autoComplete="off"
                />
            </div>

            {/* Address */}
            <div className="space-y-2">
              <Label className="text-sm font-medium">Address *</Label>
              <Input
                value={address}
                className="font-mono text-xs sm:text-sm h-11"
                onChange={(e) => {
                  setAddress(e.target.value)
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
                Submit
              </Button>
            </div>
          </form>
        }
      </DialogContent>
    </Dialog>
  )
}
