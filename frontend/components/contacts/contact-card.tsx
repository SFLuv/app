import { Contact } from "@/types/contact"
import { Card, CardContent } from "../ui/card"
import { Badge } from "../ui/badge"
import { Star, Trash } from "lucide-react"
import { Button } from "../ui/button"

interface ContactCardProps {
  contact: Contact
  handleToggleIsFavorite: (c: Contact) => Promise<void>
  toggleDeleteContactModal: () => void
  setDeleteContactModalContact: (c: Contact) => void
}

const ContactCard = ({
  contact,
  handleToggleIsFavorite,
  toggleDeleteContactModal,
  setDeleteContactModalContact
}: ContactCardProps) => {
  const toggleModal = () => {
    setDeleteContactModalContact(contact)
    toggleDeleteContactModal()
  }

  return(
    <Card key={contact.id} className="overflow-hidden hover:shadow-md transition-shadow">
      <CardContent className="p-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div>
              <div className="flex items-center gap-2">
                <h3 className="font-medium text-black dark:text-white">
                  {contact.name}
                </h3>
                { contact.is_favorite &&
                  <Badge variant="outline" className="bg-secondary text-black dark:text-white">
                    Favorite
                  </Badge>
                }
              </div>
              <p className="text-sm text-gray-600 dark:text-gray-400 truncate max-w-[200px] md:max-w-[300px] font-mono">
                {contact.address.slice(0, 6)}...{contact.address.slice(-4)}
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