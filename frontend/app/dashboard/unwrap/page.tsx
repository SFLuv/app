"use client"

import type React from "react"

import { useState, useEffect } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ArrowDownToLine, Copy, CheckCircle, User, Plus, ArrowLeft } from "lucide-react"
import { AddContactModal } from "@/components/unwrap/add-contact-modal"
import Image from "next/image"

// Mock data
const mockBalance = 1250
const mockContacts = [
  { id: "1", name: "Personal Bank Account", address: "0x742d35Cc6634C0532925a3b844Bc454e4438f44e" },
  { id: "2", name: "Business Account", address: "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed" },
  { id: "3", name: "Investment Wallet", address: "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359" },
]

export default function UnwrapPage() {
  const router = useRouter()
  const { user } = useApp()
  const [amount, setAmount] = useState("")
  const [addressType, setAddressType] = useState("manual")
  const [manualAddress, setManualAddress] = useState("")
  const [selectedContact, setSelectedContact] = useState("")
  const [qrCode, setQrCode] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [copied, setCopied] = useState(false)
  const [currentStep, setCurrentStep] = useState(1) // 1 for details, 2 for QR code
  const [contacts, setContacts] = useState(mockContacts)
  const [isAddContactModalOpen, setIsAddContactModalOpen] = useState(false)

  // Check if user has merchant or admin role
  useEffect(() => {
    if (user && user.role !== "merchant" && user.role !== "admin") {
      router.push("/dashboard")
    }
  }, [user, router])

  // Handle form submission
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()

    if (!amount || (addressType === "manual" && !manualAddress) || (addressType === "contact" && !selectedContact)) {
      return
    }

    setIsSubmitting(true)

    // Simulate API call to generate QR code
    setTimeout(() => {
      setQrCode("/placeholder.svg?height=300&width=300")
      setIsSubmitting(false)
      setCurrentStep(2) // Move to QR code step
    }, 1500)
  }

  // Get the current address
  const getCurrentAddress = () => {
    if (addressType === "manual") {
      return manualAddress
    } else {
      const contact = contacts.find((c) => c.id === selectedContact)
      return contact ? contact.address : ""
    }
  }

  // Handle copy to clipboard
  const handleCopy = () => {
    const address = getCurrentAddress()
    if (address) {
      navigator.clipboard.writeText(address)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  // Go back to details step
  const handleBack = () => {
    setCurrentStep(1)
  }

  // Handle add contact
  const handleAddContact = (name: string, address: string) => {
    const newContact = {
      id: `${contacts.length + 1}`,
      name,
      address,
    }

    setContacts([...contacts, newContact])

    // Automatically select the new contact
    setAddressType("contact")
    setSelectedContact(newContact.id)
  }

  // If user is not authenticated or loading, show loading state
  if (!user) {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Unwrap SFLuv</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">
          Convert your SFLuv to USD by unwrapping it to an external account
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-black dark:text-white">Your Balance</CardTitle>
          <CardDescription>Available SFLuv for unwrapping</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="text-4xl font-bold text-black dark:text-white">{mockBalance} SFLuv</div>
          <p className="text-sm text-gray-600 dark:text-gray-400 mt-2">
            You can unwrap any amount up to your current balance
          </p>
        </CardContent>
      </Card>

      <Card>
        {currentStep === 1 ? (
          <>
            <CardHeader>
              <CardTitle className="text-black dark:text-white">Unwrap Details</CardTitle>
              <CardDescription>Specify amount and destination</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleSubmit} className="space-y-6">
                <div className="space-y-2">
                  <Label htmlFor="amount" className="text-black dark:text-white">
                    Amount to Unwrap
                  </Label>
                  <div className="relative">
                    <Input
                      id="amount"
                      type="number"
                      placeholder="Enter amount"
                      value={amount}
                      onChange={(e) => setAmount(e.target.value)}
                      min="1"
                      max={mockBalance.toString()}
                      className="text-black dark:text-white bg-secondary pr-16"
                      required
                    />
                    <div className="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none">
                      <span className="text-gray-500 dark:text-gray-400">SFLuv</span>
                    </div>
                  </div>
                  <p className="text-xs text-gray-500 dark:text-gray-400">Maximum: {mockBalance} SFLuv</p>
                </div>

                <div className="space-y-2">
                  <Label className="text-black dark:text-white">Destination Address</Label>
                  <Tabs defaultValue="manual" value={addressType} onValueChange={setAddressType}>
                    <TabsList className="grid grid-cols-2 mb-4 bg-secondary">
                      <TabsTrigger value="manual" className="text-black dark:text-white">
                        Manual Entry
                      </TabsTrigger>
                      <TabsTrigger value="contact" className="text-black dark:text-white">
                        Saved Contacts
                      </TabsTrigger>
                    </TabsList>
                    <TabsContent value="manual" className="space-y-2">
                      <Input
                        placeholder="Enter wallet address"
                        value={manualAddress}
                        onChange={(e) => setManualAddress(e.target.value)}
                        className="text-black dark:text-white bg-secondary"
                        required={addressType === "manual"}
                      />
                      <p className="text-xs text-gray-500 dark:text-gray-400">
                        Enter the full wallet address where you want to receive your funds
                      </p>
                    </TabsContent>
                    <TabsContent value="contact" className="space-y-2">
                      <Select value={selectedContact} onValueChange={setSelectedContact}>
                        <SelectTrigger className="text-black dark:text-white bg-secondary">
                          <SelectValue placeholder="Select a saved contact" />
                        </SelectTrigger>
                        <SelectContent>
                          {contacts.map((contact) => (
                            <SelectItem key={contact.id} value={contact.id}>
                              {contact.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <div className="flex justify-between items-center">
                        <p className="text-xs text-gray-500 dark:text-gray-400">Choose from your saved contacts</p>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2"
                          onClick={() => setIsAddContactModalOpen(true)}
                        >
                          <Plus className="h-4 w-4 mr-1" />
                          Add New
                        </Button>
                      </div>
                    </TabsContent>
                  </Tabs>
                </div>

                {(addressType === "manual" && manualAddress) || (addressType === "contact" && selectedContact) ? (
                  <div className="p-3 bg-secondary/50 rounded-md">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center">
                        <User className="h-4 w-4 mr-2 text-gray-500 dark:text-gray-400" />
                        <span className="text-sm text-black dark:text-white">
                          {addressType === "contact"
                            ? contacts.find((c) => c.id === selectedContact)?.name
                            : "Custom Address"}
                        </span>
                      </div>
                      <Button type="button" variant="ghost" size="sm" className="h-7 px-2" onClick={handleCopy}>
                        {copied ? <CheckCircle className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                      </Button>
                    </div>
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-1 break-all">{getCurrentAddress()}</p>
                  </div>
                ) : null}

                <Button
                  type="submit"
                  className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]"
                  disabled={
                    isSubmitting ||
                    !amount ||
                    Number(amount) > mockBalance ||
                    Number(amount) <= 0 ||
                    (addressType === "manual" && !manualAddress) ||
                    (addressType === "contact" && !selectedContact)
                  }
                >
                  {isSubmitting ? (
                    <>
                      <div className="animate-spin mr-2 h-4 w-4 border-2 border-white border-t-transparent rounded-full" />
                      Processing...
                    </>
                  ) : (
                    <>
                      <ArrowDownToLine className="mr-2 h-4 w-4" />
                      Generate Unwrap QR Code
                    </>
                  )}
                </Button>
              </form>
            </CardContent>
          </>
        ) : (
          <>
            <CardHeader>
              <div className="flex items-center">
                <Button
                  variant="ghost"
                  size="sm"
                  className="mr-2 h-8 w-8 p-0"
                  onClick={handleBack}
                  aria-label="Back to details"
                >
                  <ArrowLeft className="h-4 w-4" />
                </Button>
                <div>
                  <CardTitle className="text-black dark:text-white">QR Code</CardTitle>
                  <CardDescription>Scan to complete the unwrap process</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col items-center justify-center py-8">
              {qrCode ? (
                <div className="text-center">
                  <div className="bg-white p-4 rounded-lg inline-block mb-4">
                    <Image src={qrCode || "/placeholder.svg"} alt="QR Code" width={200} height={200} />
                  </div>
                  <div className="mt-4 max-w-md mx-auto">
                    <div className="p-3 bg-secondary/50 rounded-md mb-4">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center">
                          <span className="text-sm font-medium text-black dark:text-white">Amount:</span>
                        </div>
                        <span className="text-sm text-black dark:text-white">{amount} SFLuv</span>
                      </div>
                    </div>
                    <div className="p-3 bg-secondary/50 rounded-md">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center">
                          <User className="h-4 w-4 mr-2 text-gray-500 dark:text-gray-400" />
                          <span className="text-sm font-medium text-black dark:text-white">Destination:</span>
                        </div>
                        <Button type="button" variant="ghost" size="sm" className="h-7 px-2" onClick={handleCopy}>
                          {copied ? <CheckCircle className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                        </Button>
                      </div>
                      <p className="text-xs text-gray-500 dark:text-gray-400 mt-1 break-all">{getCurrentAddress()}</p>
                    </div>
                    <p className="text-sm text-gray-600 dark:text-gray-400 mt-4">
                      Scan this QR code with your wallet app to complete the unwrapping process. The code will expire in
                      15 minutes.
                    </p>
                  </div>
                </div>
              ) : (
                <div className="text-center py-12">
                  <ArrowDownToLine className="h-16 w-16 text-gray-300 dark:text-gray-600 mx-auto mb-4" />
                  <h3 className="text-xl font-medium text-black dark:text-white mb-2">No QR Code Generated</h3>
                  <p className="text-gray-600 dark:text-gray-400 max-w-md mx-auto">
                    Fill out the unwrap details and submit the form to generate a QR code for unwrapping your SFLuv.
                  </p>
                </div>
              )}
            </CardContent>
            <CardFooter className="flex justify-center">
              <Button variant="outline" className="text-black dark:text-white bg-secondary hover:bg-secondary/80">
                <Copy className="h-4 w-4 mr-2" />
                Copy Transaction Link
              </Button>
            </CardFooter>
          </>
        )}
      </Card>

      <AddContactModal
        isOpen={isAddContactModalOpen}
        onClose={() => setIsAddContactModalOpen(false)}
        onAddContact={handleAddContact}
      />
    </div>
  )
}
