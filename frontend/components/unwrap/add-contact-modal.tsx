"use client"

import type React from "react"

import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Loader2 } from "lucide-react"

interface AddContactModalProps {
  isOpen: boolean
  onClose: () => void
  onAddContact: (name: string, address: string) => void
}

export function AddContactModal({ isOpen, onClose, onAddContact }: AddContactModalProps) {
  const [contactName, setContactName] = useState("")
  const [walletAddress, setWalletAddress] = useState("")
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [nameError, setNameError] = useState("")
  const [addressError, setAddressError] = useState("")

  // Reset form on open
  const handleOpenChange = (open: boolean) => {
    if (!open) {
      onClose()
    }
  }

  // Validate form
  const validateForm = () => {
    let isValid = true

    if (!contactName.trim()) {
      setNameError("Contact name is required")
      isValid = false
    } else {
      setNameError("")
    }

    if (!walletAddress.trim()) {
      setAddressError("Wallet address is required")
      isValid = false
    } else if (!/^0x[a-fA-F0-9]{40}$/.test(walletAddress)) {
      setAddressError("Please enter a valid Ethereum wallet address")
      isValid = false
    } else {
      setAddressError("")
    }

    return isValid
  }

  // Handle form submission
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()

    if (!validateForm()) {
      return
    }

    setIsSubmitting(true)

    // Simulate API call
    setTimeout(() => {
      onAddContact(contactName.trim(), walletAddress.trim())
      setIsSubmitting(false)
      setContactName("")
      setWalletAddress("")
      onClose()
    }, 1500)
  }

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle className="text-black dark:text-white">Add New Contact</DialogTitle>
          <DialogDescription>Add a new contact to your saved wallet addresses.</DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="contact-name" className="text-black dark:text-white">
              Contact Name
            </Label>
            <Input
              id="contact-name"
              value={contactName}
              onChange={(e) => {
                setContactName(e.target.value)
                setNameError("")
              }}
              placeholder="e.g. Personal Bank Account"
              className="text-black dark:text-white bg-secondary"
            />
            {nameError && <p className="text-sm text-red-500">{nameError}</p>}
          </div>

          <div className="space-y-2">
            <Label htmlFor="wallet-address" className="text-black dark:text-white">
              Wallet Address
            </Label>
            <Input
              id="wallet-address"
              value={walletAddress}
              onChange={(e) => {
                setWalletAddress(e.target.value)
                setAddressError("")
              }}
              placeholder="0x..."
              className="text-black dark:text-white bg-secondary"
            />
            {addressError && <p className="text-sm text-red-500">{addressError}</p>}
            <p className="text-xs text-gray-500 dark:text-gray-400">
              Enter a valid Ethereum wallet address starting with 0x
            </p>
          </div>

          <DialogFooter className="pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={isSubmitting}
              className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
            >
              Cancel
            </Button>
            <Button type="submit" className="bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isSubmitting}>
              {isSubmitting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Adding...
                </>
              ) : (
                "Add Contact"
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
