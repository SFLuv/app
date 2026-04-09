import { useEffect, useMemo, useState } from "react";
import { useContacts } from "@/context/ContactsProvider";
import { useLocation } from "@/context/LocationProvider";
import { AutocompleteInput, Suggestion } from "../ui/autocomplete-input";
import { getAddress, isAddress } from "viem";

export interface ResolvedRecipientOption {
  kind: "contact" | "merchant_location"
  address: string
  displayName: string
  tipToAddress?: string
}

interface ContactOrAddressInputProps  {
  onChange: (value: string, resolvedOption?: ResolvedRecipientOption | null) => void
  id: string
  className?: string
  initialValue?: string
  includeMerchantLocations?: boolean
}

const ContactOrAddressInput = (
  {
    onChange,
    id,
    className,
    initialValue,
    includeMerchantLocations = false,
  }: ContactOrAddressInputProps
) => {
  const [innerValue, setInnerValue] = useState<string>(initialValue ?? "")
  const [filteredValues, setFilteredValues] = useState<Suggestion[]>([])
  const {
    contacts
  } = useContacts()
  const { mapLocations } = useLocation()

  useEffect(() => {
    if (initialValue !== undefined) {
      setInnerValue(initialValue)
    }
  }, [initialValue])

  const resolvedOptions = useMemo<ResolvedRecipientOption[]>(() => {
    const contactOptions: ResolvedRecipientOption[] = contacts
      .filter((contact) => isAddress(contact.address))
      .map((contact) => ({
        kind: "contact",
        address: getAddress(contact.address),
        displayName: contact.name,
      }))

    if (!includeMerchantLocations) {
      return contactOptions
    }

    const locationOptions: ResolvedRecipientOption[] = mapLocations
      .filter((location) => isAddress(location.pay_to_address || ""))
      .map((location) => ({
        kind: "merchant_location",
        address: getAddress(location.pay_to_address as string),
        displayName: location.name,
        tipToAddress: isAddress(location.tip_to_address || "")
          ? getAddress(location.tip_to_address as string)
          : undefined,
      }))

    const deduped = new Map<string, ResolvedRecipientOption>()
    for (const option of [...contactOptions, ...locationOptions]) {
      const key = `${option.kind}:${option.address.toLowerCase()}`
      if (!deduped.has(key)) {
        deduped.set(key, option)
      }
    }

    return Array.from(deduped.values())
  }, [contacts, includeMerchantLocations, mapLocations])

  const optionEntries = useMemo(() => {
    const labelCounts = new Map<string, number>()
    for (const option of resolvedOptions) {
      const baseLabel =
        option.kind === "merchant_location"
          ? `${option.displayName} · Merchant`
          : option.displayName
      labelCounts.set(baseLabel, (labelCounts.get(baseLabel) || 0) + 1)
    }

    return resolvedOptions.map((option) => {
      const baseLabel =
        option.kind === "merchant_location"
          ? `${option.displayName} · Merchant`
          : option.displayName
      const label =
        (labelCounts.get(baseLabel) || 0) > 1
          ? `${baseLabel} · ${option.address.slice(0, 6)}...${option.address.slice(-4)}`
          : baseLabel
      const badge =
        option.kind === "merchant_location"
          ? option.tipToAddress
            ? "Tips"
            : "Merchant"
          : `${option.address.slice(0, 6)}...${option.address.slice(-4)}`
      return {
        option,
        label,
        suggestion: [label, badge] as Suggestion,
      }
    })
  }, [resolvedOptions])

  const suggestions: Suggestion[] = useMemo(
    () => optionEntries.map((entry) => entry.suggestion),
    [optionEntries],
  )

  const onValueChange = (value: string) => {
    const normalizedValue = value.trim().toLowerCase()
    const optionsByAddress = optionEntries.filter((entry) =>
      entry.option.address.toLowerCase().startsWith(normalizedValue),
    )
    setFilteredValues(optionsByAddress.map((entry) => entry.suggestion))

    const matchedEntry = optionEntries.find(
      (entry) =>
        entry.label === value ||
        entry.option.address.toLowerCase() === normalizedValue,
    )
    if(matchedEntry) {
      onChange(matchedEntry.option.address, matchedEntry.option)
      setInnerValue(matchedEntry.label)
      return
    }

    setInnerValue(value)
    onChange(value, null)
  }

  return (
    <AutocompleteInput
      placeholder={includeMerchantLocations ? "Contact, merchant, or 0x..." : "Contact name or 0x..."}
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
