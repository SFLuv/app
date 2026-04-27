"use client"

import { useEffect, useMemo, useState } from "react"
import { useParams, useRouter } from "next/navigation"
import { getAddress, isAddress } from "viem"
import { AlertTriangle, CheckCircle, Contact, Loader2 } from "lucide-react"
import { AddContactModal } from "@/components/contacts/add-contact-modal"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { useContacts } from "@/context/ContactsProvider"

function shortAddress(address: string): string {
  return address.length <= 16 ? address : `${address.slice(0, 8)}...${address.slice(-6)}`
}

export default function AddContactLinkPage() {
  const router = useRouter()
  const params = useParams()
  const rawAddress = typeof params.address === "string" ? decodeURIComponent(params.address) : ""
  const normalizedAddress = useMemo(() => (isAddress(rawAddress) ? getAddress(rawAddress) : ""), [rawAddress])
  const { status, login, setError } = useApp()
  const { contacts, contactsStatus, addContact } = useContacts()
  const [modalOpen, setModalOpen] = useState(false)

  const existingContact = useMemo(
    () => contacts.find((contact) => contact.address.toLowerCase() === normalizedAddress.toLowerCase()),
    [contacts, normalizedAddress],
  )

  useEffect(() => {
    if (!normalizedAddress || status !== "authenticated" || contactsStatus !== "available" || existingContact) {
      return
    }
    setError(null)
    setModalOpen(true)
  }, [contactsStatus, existingContact, normalizedAddress, setError, status])

  const handleModalOpenChange = (open: boolean) => {
    setModalOpen(open)
    if (!open) {
      router.replace("/contacts")
    }
  }

  if (!normalizedAddress) {
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

  if (status === "loading") {
    return (
      <main className="flex min-h-[70vh] items-center justify-center">
        <Loader2 className="h-10 w-10 animate-spin text-[#eb6c6c]" />
      </main>
    )
  }

  if (status === "unauthenticated") {
    return (
      <main className="mx-auto flex min-h-[70vh] max-w-lg items-center justify-center px-4">
        <Card className="w-full">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Contact className="h-5 w-5 text-[#eb6c6c]" />
              Add SFLuv contact
            </CardTitle>
            <CardDescription>Sign in to save {shortAddress(normalizedAddress)} to your SFLuv contacts.</CardDescription>
          </CardHeader>
          <CardContent>
            <Button onClick={() => void login()}>Sign in to add contact</Button>
          </CardContent>
        </Card>
      </main>
    )
  }

  return (
    <main className="mx-auto flex min-h-[70vh] max-w-lg items-center justify-center px-4">
      <Card className="w-full">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {existingContact ? (
              <CheckCircle className="h-5 w-5 text-green-600" />
            ) : (
              <Contact className="h-5 w-5 text-[#eb6c6c]" />
            )}
            {existingContact ? "Contact already saved" : "Add SFLuv contact"}
          </CardTitle>
          <CardDescription>
            {existingContact
              ? `${existingContact.name} is already saved in your contacts.`
              : `Save ${shortAddress(normalizedAddress)} as a contact for future payments.`}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex gap-3">
          {existingContact ? null : <Button onClick={() => setModalOpen(true)}>Add contact</Button>}
          <Button variant="outline" onClick={() => router.replace("/contacts")}>View contacts</Button>
        </CardContent>
      </Card>
      <AddContactModal
        open={modalOpen}
        onOpenChange={handleModalOpenChange}
        handleAddContact={addContact}
        initialAddress={normalizedAddress}
      />
    </main>
  )
}
