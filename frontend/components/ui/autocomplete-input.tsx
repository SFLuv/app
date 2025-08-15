
"use client"

import * as React from "react"
import { Check } from "lucide-react"
import { cn } from "@/lib/utils"
import { Input } from "@/components/ui/input"
import { Badge } from "./badge"

export type Suggestion = [string, string]

interface AutocompleteInputProps {
  value?: string
  onValueChange?: (value: string) => void
  suggestions?: Suggestion[]
  filteredSuggestions?: Suggestion[]
  placeholder?: string
  emptyMessage?: string
  className?: string
  disabled?: boolean
  allowCustomInput?: boolean
  id?: string
}

export function AutocompleteInput({
  value = "",
  onValueChange,
  suggestions = [],
  filteredSuggestions = [],
  placeholder = "Type or select...",
  emptyMessage = "No suggestions found.",
  className,
  disabled = false,
  allowCustomInput = true,
  id = "auto-complete"
}: AutocompleteInputProps) {
  const [inputValue, setInputValue] = React.useState(value)
  const [showDropdown, setShowDropdown] = React.useState(false)
  const [highlightedIndex, setHighlightedIndex] = React.useState(-1)
  const inputRef = React.useRef<HTMLInputElement>(null)
  const dropdownRef = React.useRef<HTMLDivElement>(null)

  React.useEffect(() => {
    setInputValue(value)
  }, [value])

  const fSuggestions = React.useMemo(() => {
    const n = [...new Set([
      ...suggestions
      .filter((suggestion) =>
        suggestion[0].toLowerCase().includes(inputValue.toLowerCase())
      )
      .map((s) =>
        s[0]
      ),
      ...filteredSuggestions
      .map((s) => s[0])
    ])]
    .slice(0, 4)
    .map((e) => [...suggestions, ...filteredSuggestions].find((s) => s[0] === e))

    return n.filter((e) => e !== undefined)
  }, [suggestions, inputValue, filteredSuggestions])

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value
    setInputValue(newValue)
    setShowDropdown(true)
    setHighlightedIndex(-1)
    if (allowCustomInput) {
      onValueChange?.(newValue)
    }
  }

  const handleSuggestionClick = (suggestion: string) => {
    setInputValue(suggestion)
    onValueChange?.(suggestion)
    setShowDropdown(false)
    setHighlightedIndex(-1)
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!showDropdown || fSuggestions.length === 0) return

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault()
        setHighlightedIndex((prev) => (prev < fSuggestions.length - 1 ? prev + 1 : 0))
        break
      case "ArrowUp":
        e.preventDefault()
        setHighlightedIndex((prev) => (prev > 0 ? prev - 1 : fSuggestions.length - 1))
        break
      case "Enter":
        e.preventDefault()
        if (highlightedIndex >= 0) {
          handleSuggestionClick(fSuggestions[highlightedIndex][0])
        }
        break
      case "Escape":
        setShowDropdown(false)
        setHighlightedIndex(-1)
        break
    }
  }

  const handleFocus = () => {
    if (fSuggestions.length > 0) {
      setShowDropdown(true)
    }
  }

  const handleBlur = (e: React.FocusEvent<HTMLInputElement>) => {
    // Delay hiding dropdown to allow for clicks on suggestions
    setTimeout(() => {
      if (!dropdownRef.current?.contains(document.activeElement)) {
        setShowDropdown(false)
        setHighlightedIndex(-1)
      }
    }, 150)
  }

  return (
    <div className="relative w-full">
      <Input
        ref={inputRef}
        type="text"
        value={inputValue}
        onChange={handleInputChange}
        onKeyDown={handleKeyDown}
        onFocus={handleFocus}
        onBlur={handleBlur}
        placeholder={placeholder}
        disabled={disabled}
        className={className}
        autoComplete="off"
        id={id}
      />

      {showDropdown && fSuggestions.length > 0 && (
        <div
          ref={dropdownRef}
          className="absolute top-full m-auto max-w-[98%] left-0 right-0 z-50 mt-1 max-h-60 overflow-auto rounded-md rounded-t-none border bg-popover text-popover-foreground shadow-md"
        >
          {fSuggestions.map((suggestion, index) => (
            <div
              key={suggestion[0]}
              className={cn(
                "relative flex cursor-pointer space-x-2 select-none items-center px-3 py-2 text-sm outline-none hover:bg-[#eb6c6c] hover:text-accent-foreground",
                index === highlightedIndex && "bg-accent text-accent-foreground",
              )}
              onClick={() => handleSuggestionClick(suggestion[0])}
              onMouseEnter={() => setHighlightedIndex(index)}
            >
              <Check className={cn("mr-2 h-4 w-4", inputValue === suggestion[0] ? "opacity-100" : "opacity-0")} />
              {suggestion[0]}
              { suggestion[1] &&
                <Badge variant="outline" className="bg-secondary text-black dark:text-white">
                  {suggestion[1]}
                </Badge>
              }
            </div>
          ))}
        </div>
      )}

      {/* {showDropdown && fSuggestions.length === 0 && inputValue && (
        <div
          ref={dropdownRef}
          className="absolute top-full left-0 right-0 z-50 mt-1 rounded-md border bg-popover p-3 text-sm text-muted-foreground shadow-md"
        >
          {emptyMessage}
        </div>
      )} */}
    </div>
  )
}

