"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { OpportunityModal } from "@/components/opportunities/opportunity-modal"
import { useRegisteredOpportunities } from "@/hooks/use-registered-opportunities"
import { mockOpportunities } from "@/data/mock-opportunities"
import { ChevronLeft, ChevronRight, CalendarIcon } from "lucide-react"
import type { Opportunity } from "@/types/opportunity"
import { DatePickerModal } from "@/components/calendar/date-picker-modal"
import { GoogleCalendarSync } from "@/components/calendar/google-calendar-sync"

export default function CalendarPage() {
  const router = useRouter()
  const { registeredOpportunities, isRegistered, cancelRegistration } = useRegisteredOpportunities()

  // Calendar state
  const [currentDate, setCurrentDate] = useState(new Date())
  const [selectedOpportunity, setSelectedOpportunity] = useState<Opportunity | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [isDatePickerOpen, setIsDatePickerOpen] = useState(false)

  // Get registered opportunities
  const userOpportunities = mockOpportunities.filter((opp) => registeredOpportunities.includes(opp.id))

  // Calendar navigation
  const goToPreviousMonth = () => {
    setCurrentDate((prev) => {
      const newDate = new Date(prev)
      newDate.setMonth(newDate.getMonth() - 1)
      return newDate
    })
  }

  const goToNextMonth = () => {
    setCurrentDate((prev) => {
      const newDate = new Date(prev)
      newDate.setMonth(newDate.getMonth() + 1)
      return newDate
    })
  }

  const goToCurrentMonth = () => {
    setCurrentDate(new Date())
  }

  const goToDatePicker = () => {
    setIsDatePickerOpen(true)
  }

  const handleDateSelection = (year: number, month: number) => {
    const newDate = new Date(currentDate)
    newDate.setFullYear(year)
    newDate.setMonth(month)
    setCurrentDate(newDate)
  }

  // Calendar helpers
  const getDaysInMonth = (year: number, month: number) => {
    return new Date(year, month + 1, 0).getDate()
  }

  const getFirstDayOfMonth = (year: number, month: number) => {
    return new Date(year, month, 1).getDay()
  }

  // Format date for comparison
  const formatDateForComparison = (date: Date) => {
    return date.toISOString().split("T")[0]
  }

  // Get opportunities for a specific day
  const getOpportunitiesForDay = (day: number) => {
    const date = new Date(currentDate.getFullYear(), currentDate.getMonth(), day)
    const dateString = formatDateForComparison(date)

    return userOpportunities.filter((opp) => {
      const oppDate = new Date(opp.date)
      return formatDateForComparison(oppDate) === dateString
    })
  }

  // Check if a date is in the past
  const isDateInPast = (day: number) => {
    const today = new Date()
    const date = new Date(currentDate.getFullYear(), currentDate.getMonth(), day)
    return date < new Date(today.setHours(0, 0, 0, 0))
  }

  // Handle opportunity click
  const handleOpportunityClick = (opportunity: Opportunity) => {
    setSelectedOpportunity(opportunity)
    setIsModalOpen(true)
  }

  // Handle modal close
  const handleModalClose = () => {
    setIsModalOpen(false)
    setSelectedOpportunity(null)
  }

  // Render calendar
  const renderCalendar = () => {
    const year = currentDate.getFullYear()
    const month = currentDate.getMonth()
    const daysInMonth = getDaysInMonth(year, month)
    const firstDayOfMonth = getFirstDayOfMonth(year, month)

    const days = []
    const weekdays = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]

    // Add weekday headers
    for (let i = 0; i < 7; i++) {
      days.push(
        <div key={`header-${i}`} className="text-center font-medium py-2 border-b">
          {weekdays[i]}
        </div>,
      )
    }

    // Add empty cells for days before the first day of the month
    for (let i = 0; i < firstDayOfMonth; i++) {
      days.push(<div key={`empty-${i}`} className="p-2 border min-h-[100px]"></div>)
    }

    // Add days of the month
    for (let day = 1; day <= daysInMonth; day++) {
      const opportunities = getOpportunitiesForDay(day)
      const isPast = isDateInPast(day)

      days.push(
        <div
          key={`day-${day}`}
          className={`p-2 border min-h-[100px] ${
            day === new Date().getDate() && month === new Date().getMonth() && year === new Date().getFullYear()
              ? "bg-secondary/30"
              : ""
          }`}
        >
          <div className="font-medium mb-1">{day}</div>
          <div className="space-y-1">
            {opportunities.map((opp) => (
              <div
                key={opp.id}
                onClick={() => handleOpportunityClick(opp)}
                className={`
                  text-xs p-1 rounded cursor-pointer truncate
                  ${
                    isPast
                      ? "bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-400 line-through"
                      : "bg-[#eb6c6c] bg-opacity-20 text-[#eb6c6c] hover:bg-opacity-30"
                  }
                `}
              >
                {isPast ? "✓ " : ""}
                {opp.title}
              </div>
            ))}
          </div>
        </div>,
      )
    }

    return days
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Volunteer Calendar</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Track your volunteer opportunities</p>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-black dark:text-white">
            {currentDate.toLocaleString("default", { month: "long", year: "numeric" })}
          </CardTitle>
          <div className="flex items-center space-x-2">
            <Button
              variant="outline"
              size="sm"
              onClick={goToPreviousMonth}
              className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={goToDatePicker}
              className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
            >
              <CalendarIcon className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={goToNextMonth}
              className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-7 gap-0">{renderCalendar()}</div>

          <div className="mt-6 flex items-center space-x-4">
            <div className="flex items-center">
              <div className="w-4 h-4 rounded bg-[#eb6c6c] bg-opacity-20 mr-2"></div>
              <span className="text-sm text-gray-600 dark:text-gray-400">Upcoming Opportunity</span>
            </div>
            <div className="flex items-center">
              <div className="w-4 h-4 rounded bg-gray-200 dark:bg-gray-700 mr-2"></div>
              <span className="text-sm text-gray-600 dark:text-gray-400">Completed Opportunity</span>
            </div>
            <div className="flex items-center">
              <div className="w-4 h-4 rounded bg-secondary/30 mr-2"></div>
              <span className="text-sm text-gray-600 dark:text-gray-400">Today</span>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="text-sm text-gray-500 dark:text-gray-400">
        Click on an opportunity to view details or manage your registration
      </div>

      <div className="flex justify-between items-center mt-6">
        <Button
          onClick={goToCurrentMonth}
          variant="outline"
          className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
        >
          Today
        </Button>

        <GoogleCalendarSync opportunities={userOpportunities} />
      </div>

      <DatePickerModal
        isOpen={isDatePickerOpen}
        onClose={() => setIsDatePickerOpen(false)}
        onSelectDate={handleDateSelection}
        currentDate={currentDate}
      />

      <OpportunityModal
        opportunity={selectedOpportunity}
        isOpen={isModalOpen}
        onClose={handleModalClose}
        isRegistered={selectedOpportunity ? isRegistered(selectedOpportunity.id) : false}
        onRegister={() => {}} // Already registered
        onCancelRegistration={() => {
          if (selectedOpportunity) {
            cancelRegistration(selectedOpportunity.id)
          }
        }}
      />
    </div>
  )
}
