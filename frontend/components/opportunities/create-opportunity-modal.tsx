"use client"

import type React from "react"

import { useState } from "react"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Calendar } from "@/components/ui/calendar"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { CalendarIcon } from "lucide-react"
import { format } from "date-fns"
import { cn } from "@/lib/utils"

interface CreateOpportunityModalProps {
  isOpen: boolean
  onClose: () => void
  onCreateOpportunity: (opportunityData: any) => void
}

export function CreateOpportunityModal({ isOpen, onClose, onCreateOpportunity }: CreateOpportunityModalProps) {
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [date, setDate] = useState<Date | undefined>(undefined)
  const [location, setLocation] = useState("")
  const [rewardAmount, setRewardAmount] = useState("")
  const [volunteersNeeded, setVolunteersNeeded] = useState("")

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()

    // Validate form
    if (!title || !description || !date || !location || !rewardAmount || !volunteersNeeded) {
      alert("Please fill in all fields")
      return
    }

    // Create opportunity data
    const opportunityData = {
      title,
      description,
      date: date?.toISOString(),
      location: {
        address: location,
        city: "San Francisco", // Default values for demo
        state: "CA",
        zip: "94110",
        coordinates: {
          lat: 37.7599,
          lng: -122.4148,
        },
      },
      rewardAmount: Number.parseInt(rewardAmount),
      volunteersNeeded: Number.parseInt(volunteersNeeded),
      volunteersSignedUp: 0,
    }

    // Call the create function
    onCreateOpportunity(opportunityData)

    // Reset form
    setTitle("")
    setDescription("")
    setDate(undefined)
    setLocation("")
    setRewardAmount("")
    setVolunteersNeeded("")

    // Close modal
    onClose()
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>Create New Opportunity</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4 py-4">
          <div className="grid grid-cols-1 gap-4">
            <div className="space-y-2">
              <Label htmlFor="title">Title</Label>
              <Input
                id="title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="Enter opportunity title"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Enter detailed description"
                rows={4}
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Date</Label>
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      variant="outline"
                      className={cn("w-full justify-start text-left font-normal", !date && "text-muted-foreground")}
                    >
                      <CalendarIcon className="mr-2 h-4 w-4" />
                      {date ? format(date, "PPP") : "Select date"}
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-auto p-0">
                    <Calendar mode="single" selected={date} onSelect={setDate} initialFocus />
                  </PopoverContent>
                </Popover>
              </div>

              <div className="space-y-2">
                <Label htmlFor="location">Location</Label>
                <Input
                  id="location"
                  value={location}
                  onChange={(e) => setLocation(e.target.value)}
                  placeholder="Enter address"
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="rewardAmount">Reward Amount (SFLuv)</Label>
                <Input
                  id="rewardAmount"
                  type="number"
                  value={rewardAmount}
                  onChange={(e) => setRewardAmount(e.target.value)}
                  placeholder="Enter reward amount"
                  min="1"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="volunteersNeeded">Volunteers Needed</Label>
                <Input
                  id="volunteersNeeded"
                  type="number"
                  value={volunteersNeeded}
                  onChange={(e) => setVolunteersNeeded(e.target.value)}
                  placeholder="Enter number of volunteers"
                  min="1"
                />
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit">Create Opportunity</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
