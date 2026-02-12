import { Contact } from "@/types/contact"
import { Card, CardContent } from "../ui/card"
import { Badge } from "../ui/badge"
import { Check, CheckCircle, Copy, Pencil, Star, Trash, X } from "lucide-react"
import { Button } from "../ui/button"
import { useRef, useState } from "react"
import { useToast } from "@/hooks/use-toast"
import { Input } from "../ui/input"

interface ContactCardProps {
  contact: Contact
  handleToggleIsFavorite: (c: Contact) => Promise<void>
  toggleDeleteContactModal: () => void
  updateContact: (c: Contact) => Promise<void>
  setDeleteContactModalContact: (c: Contact) => void
}

const ContactCard = ({
  contact,
  handleToggleIsFavorite,
  toggleDeleteContactModal,
  updateContact,
  setDeleteContactModalContact
}: ContactCardProps) => {
  const [copied, setCopied] = useState<boolean>(false)
  const [contactName, setContactName] = useState<string>(contact.name)
  const [isSavingName, setIsSavingName] = useState<boolean>(false)
  const [isEditingName, setIsEditingName] = useState<boolean>(false)
  const nameInputRef = useRef<HTMLInputElement>(null)
  const { toast }= useToast()


  const toggleModal = () => {
    setDeleteContactModalContact(contact)
    toggleDeleteContactModal()
  }


  const handleSaveName = async () => {
    setIsSavingName(true)

    const newContact = { ...contact, name: contactName.trim() }

    try {
      // Simulate API call delay
      await updateContact(newContact)


      setIsEditingName(false)
      toast({
        title: "Wallet Renamed",
        description: `Wallet name updated to "${contactName.trim()}"`,
      })
    } catch (error) {
      toast({
        title: "Error",
        description: "Failed to update wallet name. Please try again.",
        variant: "destructive",
      })
    } finally {
      setIsSavingName(false)
    }
  }

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      handleSaveName()
    } else if (e.key === "Escape") {
      handleCancelEdit()
    }
  }

  const handleCancelEdit = () => {
    setIsEditingName(false)
    if (contact) {
      setContactName(contact.name)
    }
  }


  const handleEditName = () => {
    setIsEditingName(true)
  }

    // All handler functions
  const handleCopy = () => {
    if (!contact) return
    navigator.clipboard.writeText(contact.address)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }


  return(
    <Card key={contact.id} className="overflow-hidden hover:shadow-md transition-shadow">
      <CardContent className="p-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div>
              <div className="flex items-center gap-2">
                {isEditingName ? (
                  <div className="flex items-center gap-1 sm:gap-2">
                    <Input
                      ref={nameInputRef}
                      value={contactName}
                      onChange={(e) => setContactName(e.target.value)}
                      onKeyDown={handleKeyPress}
                      onBlur={handleSaveName}
                      className="h-7 sm:h-8 text-sm sm:text-base font-semibold px-2 py-1 min-w-0"
                      placeholder="Wallet name"
                      maxLength={30}
                      disabled={isSavingName}
                    />
                    <div className="flex gap-0.5 sm:gap-1 flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={handleSaveName}
                        disabled={isSavingName || !contactName.trim()}
                        className="h-6 w-6 sm:h-7 sm:w-7 p-0 text-green-600 hover:text-green-700 hover:bg-green-100 dark:hover:bg-green-900/20"
                      >
                        {isSavingName ? (
                          <div className="animate-spin h-3 w-3 sm:h-3.5 sm:w-3.5 border-2 border-green-600 border-t-transparent rounded-full" />
                        ) : (
                          <Check className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={handleCancelEdit}
                        disabled={isSavingName}
                        className="h-6 w-6 sm:h-7 sm:w-7 p-0 text-red-600 hover:text-red-700 hover:bg-red-100 dark:hover:bg-red-900/20"
                      >
                        <X className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center gap-1 sm:gap-2">
                    <h1 className="font-semibold text-base sm:text-lg truncate">{contact.name}</h1>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={handleEditName}
                      className="h-6 w-6 sm:h-7 sm:w-7 p-0 text-muted-foreground hover:text-foreground flex-shrink-0"
                    >
                      <Pencil className="h-3 w-3 sm:h-3.5 sm:w-3.5" />
                    </Button>
                  </div>
                )}
                { contact.is_favorite &&
                  <Badge variant="outline" className="bg-secondary text-black dark:text-white">
                    Favorite
                  </Badge>
                }
              </div>
              <p className="text-sm text-gray-600 dark:text-gray-400 truncate max-w-[200px] md:max-w-[300px] font-mono">
                {contact.address.slice(0, 6)}...{contact.address.slice(-4)}
                <Button variant="ghost" size="icon" className="h-6 w-6 ml-1" onClick={handleCopy}>
                  {copied ? <CheckCircle className="h-3 w-3 text-green-500" /> : <Copy className="h-3 w-3" />}
                </Button>
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button
              className="bg-transparent hover:bg-transparent"
              onClick={() => handleToggleIsFavorite(contact)}
            >
              { contact.is_favorite ?
                <Star fill="gold" className="fill-gold hover:opacity-50" strokeWidth={0}/>
                :
                <Star className="hover:bg-gold hover:opacity-50"/>
              }
            </Button>
            <Button
              className="bg-transparent hover:bg-transparent"
              onClick={toggleModal}
            >
              <Trash  color="red" className="hover:opacity-50"/>
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default ContactCard
