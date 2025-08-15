import { cn } from "@/lib/utils";
import { InputProps } from "../ui/input";
import { ChangeEventHandler, ForwardedRef, forwardRef, HTMLInputAutoCompleteAttribute, useMemo, useState } from "react";
import { useContacts } from "@/context/ContactsProvider";
import { AutocompleteInput, Suggestion } from "../ui/autocomplete-input";

interface ContactOrAddressInputProps  {
  onChange: (value: string) => void
  id: string
  className?: string
}

const ContactOrAddressInput = (
  {
    onChange,
    id,
    className
  }: ContactOrAddressInputProps
) => {
  const [innerValue, setInnerValue] = useState<string>("")
  const [filteredValues, setFilteredValues] = useState<Suggestion[]>([])
  const {
    contacts
  } = useContacts()

  const suggestions: Suggestion[] = useMemo(() => contacts.map((c) => [c.name, `${c.address.slice(0, 6)}...${c.address.slice(-4)}`]), [contacts])

  const onValueChange = (value: string) => {

    const contactsByAddress = contacts.filter((c) => c.address.startsWith(value))
    setFilteredValues(contactsByAddress.map((c) => [c.name, `${c.address.slice(0, 6)}...${c.address.slice(-4)}`]))

    const contact = contacts.find((c) => c.name === value || c.address === value)
    if(contact) {
      onChange(contact.address)
      setInnerValue(contact.name)
      return
    }

    setInnerValue(value)
    onChange(value)
  }

  return (
    <AutocompleteInput
      placeholder="Contact name or 0x..."
      value={innerValue}
      onValueChange={onValueChange}
      allowCustomInput={true}
      suggestions={suggestions}
      filteredSuggestions={filteredValues}
      id={id}
      className={className}
    />
  )
}

ContactOrAddressInput.displayName = "ContactOrAddressInput"
export default ContactOrAddressInput