"use client"

import { useState } from "react"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { type Merchant, type MerchantStatus, merchantStatusLabels, merchantTypeLabels } from "@/types/merchant"
import { Separator } from "@/components/ui/separator"

interface MerchantDetailsModalProps {
  merchant: Merchant | null
  isOpen: boolean
  onClose: () => void
  onUpdateStatus: (merchantId: string, status: MerchantStatus) => void
}

export function MerchantDetailsModal({ merchant, isOpen, onClose, onUpdateStatus }: MerchantDetailsModalProps) {
  const [status, setStatus] = useState<MerchantStatus | null>(merchant?.status || null)

  if (!merchant) return null

  const handleStatusChange = (value: MerchantStatus) => {
    setStatus(value)
  }

  const handleSave = () => {
    if (status && merchant) {
      onUpdateStatus(merchant.id, status)
      onClose()
    }
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[600px] max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold">{merchant.name}</DialogTitle>
        </DialogHeader>

        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 gap-4">
            <div className="col-span-4">
              <img
                src={merchant.imageUrl || "/placeholder.svg"}
                alt={merchant.name}
                className="w-full h-48 object-cover rounded-md"
              />
            </div>

            <div className="col-span-4">
              <Label className="text-sm font-medium">Description</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.description}</p>
            </div>

            <div className="col-span-2">
              <Label className="text-sm font-medium">Type</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchantTypeLabels[merchant.type]}</p>
            </div>

            <div className="col-span-2">
              <Label className="text-sm font-medium">Rating</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.rating} / 5</p>
            </div>

            <div className="col-span-4">
              <Separator className="my-2" />
              <h3 className="font-medium mb-2">Contact Information</h3>
            </div>

            <div className="col-span-2">
              <Label className="text-sm font-medium">Phone</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.contactInfo.phone}</p>
            </div>

            <div className="col-span-2">
              <Label className="text-sm font-medium">Email</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.contactInfo.email}</p>
            </div>

            {merchant.contactInfo.website && (
              <div className="col-span-4">
                <Label className="text-sm font-medium">Website</Label>
                <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">
                  <a
                    href={merchant.contactInfo.website}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-500 hover:underline"
                  >
                    {merchant.contactInfo.website}
                  </a>
                </p>
              </div>
            )}

            <div className="col-span-4">
              <Separator className="my-2" />
              <h3 className="font-medium mb-2">Address</h3>
            </div>

            <div className="col-span-4">
              <Label className="text-sm font-medium">Street</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.address.street}</p>
            </div>

            <div className="col-span-2">
              <Label className="text-sm font-medium">City</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.address.city}</p>
            </div>

            <div className="col-span-1">
              <Label className="text-sm font-medium">State</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.address.state}</p>
            </div>

            <div className="col-span-1">
              <Label className="text-sm font-medium">ZIP</Label>
              <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">{merchant.address.zip}</p>
            </div>

            <div className="col-span-4">
              <Separator className="my-2" />
              <h3 className="font-medium mb-2">Status Management</h3>
            </div>

            <div className="col-span-4">
              <Label htmlFor="status" className="text-sm font-medium">
                Merchant Status
              </Label>
              <Select
                value={status || merchant.status}
                onValueChange={(value) => handleStatusChange(value as MerchantStatus)}
              >
                <SelectTrigger className="w-full mt-1">
                  <SelectValue placeholder="Select status" />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(merchantStatusLabels).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-gray-500 mt-1">
                Current status: <span className="font-medium">{merchantStatusLabels[merchant.status]}</span>
              </p>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleSave} className="bg-[#eb6c6c] hover:bg-[#d55c5c]">
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
