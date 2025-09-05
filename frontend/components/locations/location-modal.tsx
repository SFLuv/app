"use client"

import { useState } from "react"
import Image from "next/image"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Badge } from "@/components/ui/badge"
import { Star, MapPin, Phone, Mail, Globe, ExternalLink } from "lucide-react"
import { Location } from "@/types/location"

interface LocationModalProps {
  location: Location | null
  isOpen: boolean
  onClose: () => void
}

export function LocationModal({ location, isOpen, onClose }: LocationModalProps) {
  const [activeTab, setActiveTab] = useState("info")

  if (!location) return null

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

  const getGoogleMapsUrl = (googleId: string) => {
    return `https://www.google.com/maps/place/?q=place_id:${googleId}`
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto space-y-4">
        <DialogHeader className="space-y-2">
          <DialogTitle className="text-2xl text-black dark:text-white">{location.name}</DialogTitle>
          <DialogDescription className="flex items-center gap-2 sr-only">{location.type.charAt(0).toUpperCase() + location.type.slice(1)}</DialogDescription>
            <Badge variant="outline" className="bg-secondary text-black dark:text-white">
            {location.type.charAt(0).toUpperCase() + location.type.slice(1)}
            </Badge>
            <div className="flex items-center ml-2">
              {renderStars(location.rating)}
              <span className="ml-1 text-sm text-gray-600 dark:text-gray-400">{location.rating.toFixed(1)}</span>
            </div>
        </DialogHeader>

        {/* <div className="my-4 flex justify-center">
          <Image
            src={location.image_url || "/placeholder.svg?height=300&width=600"}
            alt={location.name}
            width={400}
            height={200}
            className="object-cover rounded-md"
          />
        </div> */}

        <Tabs defaultValue="info" value={activeTab} onValueChange={setActiveTab}>
          <TabsList className={`grid ${!!location.opening_hours.length ? "grid-cols-3" : "grid-cols-2"} mb-4`}>
            <TabsTrigger value="info">Information</TabsTrigger>
            {!!location.opening_hours.length && <TabsTrigger value="hours">Hours</TabsTrigger>}
            <TabsTrigger value="contact">Contact</TabsTrigger>
          </TabsList>

          <TabsContent value="info" className="space-y-4">
            <p className="text-gray-700 dark:text-gray-300">{location.description}</p>

            <div className="flex items-start gap-2">
              <MapPin className="h-5 w-5 text-[#eb6c6c] mt-0.5" />
              <div>
                <p className="text-gray-700 dark:text-gray-300">{location.street}</p>
                <p className="text-gray-700 dark:text-gray-300">
                  {location.city}, {location.state} {location.zip}
                </p>
                <a
                  href={getGoogleMapsUrl(
                    location.google_id
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
          </TabsContent>

           <TabsContent value="hours" className="space-y-4">
            <h3 className="font-medium text-black dark:text-white">Hours of Operation</h3>
            <div className="space-y-2">
                <ul>
                  {location.opening_hours.map((hours) => (
                    <li key={hours}>{hours}</li>
                  ))}
                </ul>
            </div>
          </TabsContent>

          <TabsContent value="contact" className="space-y-4">
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Phone className="h-5 w-5 text-[#eb6c6c]" />
                <a href={`tel:${location.phone}`} className="text-gray-700 dark:text-gray-300">
                  {location.phone || "Not Available"}
                </a>
              </div>

              <div className="flex items-center gap-2">
                <Mail className="h-5 w-5 text-[#eb6c6c]" />
                <a href={`mailto:${location.email}`} className="text-gray-700 dark:text-gray-300">
                  {location.email || "Not Available"}
                </a>
              </div>

              {location.website && (
                <div className="flex items-center gap-2">
                  <Globe className="h-5 w-5 text-[#eb6c6c]" />
                  <a
                    href={location.website}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-gray-700 dark:text-gray-300"
                  >
                    {location.website || "Not Available"}
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
                  location.google_id
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
