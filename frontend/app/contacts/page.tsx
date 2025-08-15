"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Plus, Wallet, Settings, ArrowRight } from "lucide-react"
import { WalletDetailModal } from "@/components/wallets/wallet-detail-modal"
import { useWallets, usePrivy } from "@privy-io/react-auth"
import type { ConnectedWallet } from "@/types/privy-wallet"
import { useApp } from "@/context/AppProvider"
import { AppWallet } from "@/lib/wallets/wallets"
import { ConnectWalletModal } from "@/components/wallets/connect-wallet-modal"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { NewWalletModal } from "@/components/wallets/new-wallet-modal"
import { useContacts } from "@/context/ContactsProvider"
import { AddContactModal } from "@/components/contacts/add-contact-modal"
import ContactCard from "@/components/contacts/contact-card"
import { Contact } from "@/types/contact"
import { DeleteContactModal } from "@/components/contacts/delete-contact-modal"

export default function ContactsPage() {
  const router = useRouter()
  const [addContactModalOpen, setAddContactModalOpen] = useState(false)
  const [deleteContactModalOpen, setDeleteContactModalOpen] = useState(false)
  const [deleteContactModalContact, setDeleteContactModalContact] = useState<Contact>()
  const [onlyFavorites, setOnlyFavorites] = useState(false)
  const {
    contacts,
    contactsStatus,
    addContact,
    updateContact,
    getContacts,
    deleteContact
  } = useContacts()
  const {
    status,
    error,
    setError
  } = useApp()

  useEffect(() => {
    if(status === "unauthenticated") {
      router.replace("/")
    }
  }, [status])



  const toggleAddContactModal = () => {
    setError(null)
    setAddContactModalOpen(!addContactModalOpen)
  }

  const handleToggleIsFavorite = async (c: Contact) => {
    c.is_favorite = !c.is_favorite
    await updateContact(c)
  }

  const toggleDeleteContactModal = () => {
    setError(null)
    setDeleteContactModalOpen(!deleteContactModalOpen)
  }

  const handleDeleteContact = async (id: number) => {
    await deleteContact(id)
  }

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <DeleteContactModal
        open={deleteContactModalOpen}
        onOpenChange={toggleDeleteContactModal}
        contact={deleteContactModalContact}
        handleDeleteContact={handleDeleteContact}
        deleteContactError={error}
      />
      <AddContactModal open={addContactModalOpen} onOpenChange={toggleAddContactModal} handleAddContact={addContact} addContactError={error} />
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Contacts</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Manage your contacts</p>
      </div>
      <div className="flex items-center space-x-2">
        <Checkbox
          id={`only-favorites`}
          checked={onlyFavorites}
          onCheckedChange={() => setOnlyFavorites(!onlyFavorites)}
        />
        <Label
          htmlFor={`only-favorites`}
          className="text-sm text-black dark:text-white cursor-pointer"
        >
          Filter by favorites
        </Label>
      </div>
      <div className="space-y-4">
        {contacts.length === 0 ? (
          <div className="text-center py-12">
            <h3 className="text-xl font-medium text-black dark:text-white mb-2">No contacts.</h3>
          </div>
        ) : (
          contacts.map((contact, index) => {
            if(onlyFavorites && !contact.is_favorite) return
            return (
              <ContactCard
                key={contact.id}
                contact={contact}
                handleToggleIsFavorite={handleToggleIsFavorite}
                toggleDeleteContactModal={toggleDeleteContactModal}
                setDeleteContactModalContact={setDeleteContactModalContact}
              />
            )
          })
        )}
      </div>

      <div className="flex flex-col sm:flex-row gap-4">
        <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={toggleAddContactModal}>
          <Plus className="h-4 w-4 mr-2" />
          Add Contact
        </Button>
      </div>

      <div className="text-sm text-gray-500 dark:text-gray-400">
        Showing {contacts.length} {!onlyFavorites ? "" : "favorite"} contact{(contacts.filter((contact) => contact.is_favorite === true || !onlyFavorites)).length !== 1 ? "s" : ""}
      </div>
    </div>
  )
}
