"use client"

import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Badge } from "@/components/ui/badge"
import { Star, MapPin, Phone, Mail, Globe, Navigation, Wallet } from "lucide-react"
import { Location } from "@/types/location"
import { currentWeekdayIndex, isTodayHoursLine } from "@/lib/opening-hours"
import { MerchantIcon, OpenStatusBadge } from "@/components/locations/merchant-pin"
import { useOpenState } from "@/hooks/use-open-state"
import { isAddress } from "viem"

interface LocationModalProps {
  location: Location | null
  isOpen: boolean
  onClose: () => void
  isPayEnabled: boolean
  onPayLocation: (location: Location) => void
}

export function LocationModal({ location: selected, isOpen, onClose, isPayEnabled, onPayLocation }: LocationModalProps) {
  const [activeTab, setActiveTab] = useState("info")

  // The last merchant is held through the exit animation. Callers clear their
  // selection the moment the dialog closes, and rendering null on that tick
  // tore the content out before it could animate away — so the close was
  // instant while the open was not.
  const [shown, setShown] = useState<Location | null>(selected)
  useEffect(() => {
    if (selected) setShown(selected)
  }, [selected])

  // Called before the early return: hooks cannot be conditional, and this modal
  // renders with no merchant at all until one is picked.
  const openState = useOpenState(shown?.hours)

  if (!shown) return null
  const location = shown

  const openingHours = location.opening_hours ?? []
  // Computed once per render rather than per row, so every line is judged
  // against the same day even if the render straddles midnight.
  const today = currentWeekdayIndex()
  const canPay = isPayEnabled && isAddress((location.pay_to_address || "").trim())

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
          <div className="flex items-start gap-3">
            <div className="h-12 w-12 shrink-0 overflow-hidden rounded-xl border border-border/60 shadow-sm">
              <MerchantIcon name={location.name} iconUrl={location.icon_url} size={48} state={openState} />
            </div>
            <div className="min-w-0 flex-1 space-y-1">
              <DialogTitle className="text-2xl text-black dark:text-white">{location.name}</DialogTitle>
              <OpenStatusBadge state={openState} />
            </div>
          </div>
          <DialogDescription className="flex items-center gap-2 sr-only">{location.type.charAt(0).toUpperCase() + location.type.slice(1)}</DialogDescription>
            <Badge variant="outline" className="bg-secondary text-black dark:text-white">
            {location.type.charAt(0).toUpperCase() + location.type.slice(1)}
            </Badge>
            <div className="flex items-center ml-2">
              {location.rating > 0 ? (
                <>
                  {renderStars(location.rating)}
                  <span className="ml-1 text-sm text-gray-600 dark:text-gray-400">{location.rating.toFixed(1)}</span>
                </>
              ) : (
                <span className="text-sm text-gray-600 dark:text-gray-400">No Reviews</span>
              )}
            </div>
        </DialogHeader>

        <Tabs defaultValue="info" value={activeTab} onValueChange={setActiveTab}>
          <TabsList className={`grid ${openingHours.length ? "grid-cols-3" : "grid-cols-2"} mb-4`}>
            <TabsTrigger value="info">Information</TabsTrigger>
            {!!openingHours.length && <TabsTrigger value="hours">Hours</TabsTrigger>}
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
                {/* Directions sit with the address rather than in the footer:
                    it is an action on this block of text, and it was competing
                    with Pay for attention down there. */}
                <Button
                  size="sm"
                  className="mt-2 bg-[#eb6c6c] hover:bg-[#d55c5c]"
                  onClick={() => window.open(getGoogleMapsUrl(location.google_id), "_blank")}
                >
                  <Navigation className="mr-2 h-4 w-4" />
                  Get Directions
                </Button>
              </div>
            </div>
          </TabsContent>

           <TabsContent value="hours" className="space-y-4">
            <h3 className="font-medium text-black dark:text-white">Hours of Operation</h3>
            <div className="space-y-2">
                <ul>
                  {openingHours.map((hours, index) => (
                    <li
                      key={hours}
                      className={isTodayHoursLine(hours, index, today) ? "font-semibold text-foreground" : undefined}
                    >
                      {hours}
                    </li>
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

        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <Button
            onClick={onClose}
            variant="outline"
            className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
          >
            Close
          </Button>
          {canPay ? (
            <Button
              className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
              onClick={() => onPayLocation(location)}
            >
              <Wallet className="mr-2 h-4 w-4" />
              Pay
            </Button>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
