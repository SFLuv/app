"use client"

import { useState } from "react"
import Image from "next/image"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Badge } from "@/components/ui/badge"
import { Star, MapPin, Phone, Mail, Globe, ExternalLink } from "lucide-react"
import type { Merchant } from "@/types/merchant"

interface MerchantModalProps {
  merchant: Merchant | null
  isOpen: boolean
  onClose: () => void
}

export function MerchantModal({ merchant, isOpen, onClose }: MerchantModalProps) {
  const [activeTab, setActiveTab] = useState("info")

  if (!merchant) return null

  const renderStars = (rating: number) => {
    return Array(5)
      .fill(0)
      .map((_, i) => (
        <Star
          key={i}
          className={`h-4 w-4 ${i < Math.floor(rating) ? "text-yellow-400 fill-yellow-400" : "text-gray-300"}`}
        />
      ))
  }

  const getGoogleMapsUrl = (address: string, city: string, state: string, zip: string) => {
    const formattedAddress = encodeURIComponent(`${address}, ${city}, ${state} ${zip}`)
    return `https://www.google.com/maps/search/?api=1&query=${formattedAddress}`
  }

  console.log(merchant)

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="text-2xl text-black dark:text-white">{merchant.name}</DialogTitle>
          <DialogDescription className="flex items-center gap-2">
            <Badge variant="outline" className="bg-secondary text-black dark:text-white">
            {merchant.type.charAt(0).toUpperCase() + merchant.type.slice(1)}
            </Badge>
            <div className="flex items-center ml-2">
              {renderStars(merchant.rating)}
              <span className="ml-1 text-sm text-gray-600 dark:text-gray-400">{merchant.rating.toFixed(1)}</span>
            </div>
          </DialogDescription>
        </DialogHeader>

        <div className="relative h-64 w-full my-4">
          <Image
            src={merchant.imageUrl || "/placeholder.svg?height=300&width=600"}
            alt={merchant.name}
            fill
            className="object-cover rounded-md"
          />
        </div>

        <Tabs defaultValue="info" value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="grid grid-cols-3 mb-4">
            <TabsTrigger value="info">Information</TabsTrigger>
            <TabsTrigger value="hours">Hours</TabsTrigger>
            <TabsTrigger value="contact">Contact</TabsTrigger>
          </TabsList>

          <TabsContent value="info" className="space-y-4">
            <p className="text-gray-700 dark:text-gray-300">{merchant.description}</p>

            <div className="flex items-start gap-2">
              <MapPin className="h-5 w-5 text-[#eb6c6c] mt-0.5" />
              <div>
                <p className="text-gray-700 dark:text-gray-300">{merchant.address.street}</p>
                <p className="text-gray-700 dark:text-gray-300">
                  {merchant.address.city}, {merchant.address.state} {merchant.address.zip}
                </p>
                <a
                  href={getGoogleMapsUrl(
                    merchant.address.street,
                    merchant.address.city,
                    merchant.address.state,
                    merchant.address.zip,
                  )}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-[#eb6c6c] hover:underline text-sm flex items-center mt-1"
                >
                  View on Google Maps
                  <ExternalLink className="h-3 w-3 ml-1" />
                </a>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <Badge className="bg-[#eb6c6c]">Accepts SFLuv</Badge>
            </div>
          </TabsContent>

          <TabsContent value="hours" className="space-y-4">
            <h3 className="font-medium text-black dark:text-white">Hours of Operation</h3>
            <div className="space-y-2">
              {merchant.opening_hours?.map(item => <li>{item}</li>)}
            </div>
          </TabsContent>

          <TabsContent value="contact" className="space-y-4">
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Phone className="h-5 w-5 text-[#eb6c6c]" />
                <a href={`tel:${merchant.contactInfo.phone}`} className="text-gray-700 dark:text-gray-300">
                  {merchant.contactInfo.phone}
                </a>
              </div>

              <div className="flex items-center gap-2">
                <Mail className="h-5 w-5 text-[#eb6c6c]" />
                <a href={`mailto:${merchant.contactInfo.email}`} className="text-gray-700 dark:text-gray-300">
                  {merchant.contactInfo.email}
                </a>
              </div>

              {merchant.contactInfo.website && (
                <div className="flex items-center gap-2">
                  <Globe className="h-5 w-5 text-[#eb6c6c]" />
                  <a
                    href={`https://${merchant.contactInfo.website}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-gray-700 dark:text-gray-300"
                  >
                    {merchant.contactInfo.website}
                  </a>
                </div>
              )}
            </div>
          </TabsContent>
        </Tabs>

        <div className="mt-6 flex justify-end">
          <Button
            onClick={onClose}
            variant="outline"
            className="mr-2 text-black dark:text-white bg-secondary hover:bg-secondary/80"
          >
            Close
          </Button>
          <Button
            className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
            onClick={() =>
              window.open(
                getGoogleMapsUrl(
                  merchant.address.street,
                  merchant.address.city,
                  merchant.address.state,
                  merchant.address.zip,
                ),
                "_blank",
              )
            }
          >
            Get Directions
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
