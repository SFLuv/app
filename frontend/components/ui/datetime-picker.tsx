"use client"

import * as React from "react"
import { useState, useEffect } from "react"
import { ChevronDownIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

export function DateTimePicker({
  open,
  setOpen,
  date,
  setDate,
  timezone
}: {
  open: boolean,
  setOpen: React.Dispatch<React.SetStateAction<boolean>>,
  date: number,
  setDate: React.Dispatch<React.SetStateAction<number>>
  timezone: string | undefined
}) {
  const [dateTimestamp, setDateTimestamp] = useState<Date | undefined>(undefined)
  const [timeTimestamp, setTimeTimestamp] = useState<number>(0)

  useEffect(() => {
    if(dateTimestamp === undefined) {
      setDate(0)
      return
    }

    setDate(Math.floor(dateTimestamp.getTime() / 1000) + timeTimestamp)
  }, [dateTimestamp, timeTimestamp])

  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="flex flex-col gap-3">
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              id="date-picker"
              className="justify-between font-normal"
            >
              {dateTimestamp ? dateTimestamp.toLocaleDateString() : "Select date"}
              <ChevronDownIcon />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-auto overflow-hidden p-0" align="start">
            <Calendar
              mode="single"
              selected={dateTimestamp}
              fixedWeeks={true}
              onSelect={(dateTimestamp) => {
                setDateTimestamp(dateTimestamp)
                setOpen(false)
              }}
              timeZone={timezone}
              reverseYears={true}
              reverseMonths={true}
            />
          </PopoverContent>
        </Popover>
      </div>
      <div className="flex flex-col gap-3">
        <Input
          type="time"
          id="time-picker"
          step="1"
          defaultValue="00:00:00"
          onChange={(e) => {
            setTimeTimestamp(Math.floor(e.target.valueAsNumber / 1000))
          }}
          className="bg-background appearance-none [&::-webkit-calendar-picker-indicator]:hidden [&::-webkit-calendar-picker-indicator]:appearance-none"
        />
      </div>
    </div>
  )
}
