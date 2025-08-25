import { Contact } from "@/types/contact"
import { createContext, ReactNode, useContext, useEffect, useState } from "react"
import { useApp } from "./AppProvider"

export type ContactsStatus = "loading" | "available"

interface ContactsContextType {
  contacts: Contact[]
  contactsStatus: ContactsStatus
  addContact: (c: Contact) => Promise<void>
  updateContact: (c: Contact) => Promise<void>
  getContacts: () => Promise<void>
  deleteContact: (id: number) => Promise<void>
}

const ContactsContext = createContext<ContactsContextType | null>(null)

export default function ContactsProvider ({ children }: { children: ReactNode }) {
  const [contacts, setContacts] = useState<Contact[]>([])
  const [contactsStatus, setContactsStatus] = useState<ContactsStatus>("loading")
  const { status, authFetch, setError } = useApp()

  useEffect(() => {
    console.log(status)
    if(status === "loading") return

    if(status === "unauthenticated") {
      setContacts([])
      setContactsStatus("loading")
      return
    }

    console.log("getting contacts")
    getContacts()
  }, [status])


  const _addContact = async (c: Contact): Promise<Contact> => {
    const res = await authFetch("/contacts", {
      method: "POST",
      body: JSON.stringify(c)
    })

    if(res.status != 201) {
      throw new Error("error adding contact")
    }

    return await res.json() as Contact
  }

  const _updateContact = async (c: Contact) => {
    const res = await authFetch("/contacts", {
      method: "PUT",
      body: JSON.stringify(c)
    })

    if(res.status != 201) {
      throw new Error("error updating contact")
    }
  }

  const _getContacts = async (): Promise<Contact[]> => {
    const res = await authFetch("/contacts")

    if(res.status != 200) {
      throw new Error("error getting contacts")
    }

    return await res.json() as Contact[]
  }

  const _deleteContact = async (id: number) => {
    const res = await authFetch("/contacts?id=" + id, {
      method: "DELETE"
    })

    if(res.status != 200) {
      throw new Error("error deleting contact")
    }
  }


    const addContact = async (c: Contact) => {
      setContactsStatus("loading")
      try {
        const contact  = await _addContact(c)
        setContacts([...contacts, contact])
      }
      catch(err) {
        setError(err)
      }
      setContactsStatus("available")
    }

    const updateContact = async (c: Contact) => {
      setContactsStatus("loading")
      try {
        const index = contacts.findIndex((contact) => c.id === contact.id)
        if(index === -1) throw new Error("no contact found with id " + c.id)
        await _updateContact(c)
        contacts[index] = c
        setContacts([...contacts])
      }
      catch(err) {
        setError(err)
      }
      setContactsStatus("available")
    }

    const getContacts = async () => {
      setContactsStatus("loading")
      try {
        const cs = await _getContacts()
        setContacts(cs)
      }
      catch(err) {
        setError(err)
      }
      setContactsStatus("available")
    }

    const deleteContact = async (id: number) => {
      setContactsStatus("loading")
      try {
        const index = contacts.findIndex((contact) => contact.id === id)
        if(index === -1) throw new Error("no contact found with id " + id)
        await _deleteContact(id)
        contacts.splice(index, 1)
        setContacts([...contacts])
      }
      catch(err) {
        setError(err)
      }
      setContactsStatus("available")
    }

  return (
    <ContactsContext.Provider
      value={{
        contacts,
        contactsStatus,
        addContact,
        updateContact,
        getContacts,
        deleteContact
      }}
    >
      {children}
    </ContactsContext.Provider>
  )
}

export function useContacts() {
  const context = useContext(ContactsContext)
  if(!context) {
    throw new Error("useContacts must be used within a ContactsProvider")
  }
  return context
}