"use client"

import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { AlertTriangle } from "lucide-react"
import { Contact } from "@/types/contact"

interface AddContactModalProps {
  open: boolean
  onOpenChange: () => void
  contact: Contact | undefined
  handleDeleteContact: (id: number) => Promise<void>
  deleteContactError: unknown
}

export function DeleteContactModal({ open, onOpenChange, contact, handleDeleteContact, deleteContactError }: AddContactModalProps) {
  if(!contact) return

  const [deleteError, setDeleteError]= useState<string | null>()
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false)

  useEffect(() => {
    setDeleteError(null)
    setIsSubmitting(false)
  }, [open])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {

    e.preventDefault()

    await handleDeleteContact(contact.id)
    if(!deleteContactError) {
      onOpenChange()
    } else {
      setDeleteError("Encountered a server error while deleting contact. Please try again later.")
    }

    setIsSubmitting(false)
  }


  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">Delete Contact</DialogTitle>
          <DialogDescription className="text-sm">
            Are you sure you want to delete {contact.name}?
          </DialogDescription>
        </DialogHeader>
        {isSubmitting ?
          <div className="min-h-screen flex items-center justify-center">
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
          </div>
        :
          <form
          className="space-y-4 sm:space-y-6"
          onSubmit={handleSubmit}
          >

            {/* Details */}
            <div className="space-y-2">

              {/* Submit Button */}
              <div className="pt-2 text-center">
                <Button type="submit">
                  Delete
                </Button>
              </div>

              {deleteError && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{deleteError}</span>
                </div>
              )}

            </div>
          </form>
        }
      </DialogContent>
    </Dialog>
  )
}
