"use client"

import { useState } from "react"
import { Search, SlidersHorizontal, X } from "lucide-react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import type { SortOption, SortDirection } from "@/types/opportunity"
import { mockOrganizers } from "@/data/mock-opportunities"

interface SearchFiltersProps {
  searchQuery: string
  setSearchQuery: (query: string) => void
  sortOption: SortOption
  setSortOption: (option: SortOption) => void
  sortDirection: SortDirection
  setSortDirection: (direction: SortDirection) => void
  selectedOrganizers: string[]
  setSelectedOrganizers: (organizers: string[]) => void
  userLocation: string
  setUserLocation: (location: string) => void
}

export function SearchFilters({
  searchQuery,
  setSearchQuery,
  sortOption,
  setSortOption,
  sortDirection,
  setSortDirection,
  selectedOrganizers,
  setSelectedOrganizers,
  userLocation,
  setUserLocation,
}: SearchFiltersProps) {
  const [isFiltersOpen, setIsFiltersOpen] = useState(false)

  const handleOrganizerChange = (organizer: string) => {
    if (selectedOrganizers.includes(organizer)) {
      setSelectedOrganizers(selectedOrganizers.filter((o) => o !== organizer))
    } else {
      setSelectedOrganizers([...selectedOrganizers, organizer])
    }
  }

  const clearFilters = () => {
    setSearchQuery("")
    setSortOption("date")
    setSortDirection("asc")
    setSelectedOrganizers([])
    setUserLocation("")
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
          <Input
            placeholder="Search opportunities..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10 text-black dark:text-white bg-secondary"
          />
          {searchQuery && (
            <button
              className="absolute right-3 top-1/2 transform -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              onClick={() => setSearchQuery("")}
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>

        <div className="flex gap-2">
          <Select value={sortOption} onValueChange={(value) => setSortOption(value as SortOption)}>
            <SelectTrigger className="w-[180px] text-black dark:text-white bg-secondary">
              <SelectValue placeholder="Sort by" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="date">Date</SelectItem>
              <SelectItem value="reward">Reward Amount</SelectItem>
              <SelectItem value="proximity">Proximity</SelectItem>
              <SelectItem value="organizer">Organizer</SelectItem>
            </SelectContent>
          </Select>

          <Select value={sortDirection} onValueChange={(value) => setSortDirection(value as SortDirection)}>
            <SelectTrigger className="w-[120px] text-black dark:text-white bg-secondary">
              <SelectValue placeholder="Order" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="asc">Ascending</SelectItem>
              <SelectItem value="desc">Descending</SelectItem>
            </SelectContent>
          </Select>

          <Popover open={isFiltersOpen} onOpenChange={setIsFiltersOpen}>
            <PopoverTrigger asChild>
              <Button variant="outline" className="text-black dark:text-white bg-secondary">
                <SlidersHorizontal className="h-4 w-4 mr-2" />
                Filters
                {selectedOrganizers.length > 0 && (
                  <span className="ml-1 bg-[#eb6c6c] text-white rounded-full w-5 h-5 flex items-center justify-center text-xs">
                    {selectedOrganizers.length}
                  </span>
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-80 bg-secondary">
              <div className="space-y-4">
                <h3 className="font-medium text-black dark:text-white">Filters</h3>

                {sortOption === "proximity" && (
                  <div className="space-y-2">
                    <Label htmlFor="location" className="text-black dark:text-white">
                      Your Location
                    </Label>
                    <Input
                      id="location"
                      placeholder="Enter your address"
                      value={userLocation}
                      onChange={(e) => setUserLocation(e.target.value)}
                      className="text-black dark:text-white bg-secondary"
                    />
                  </div>
                )}

                <div className="space-y-2">
                  <Label className="text-black dark:text-white">Organizers</Label>
                  <div className="space-y-2 max-h-40 overflow-y-auto pr-2">
                    {mockOrganizers.map((organizer) => (
                      <div key={organizer} className="flex items-center space-x-2">
                        <Checkbox
                          id={`organizer-${organizer}`}
                          checked={selectedOrganizers.includes(organizer)}
                          onCheckedChange={() => handleOrganizerChange(organizer)}
                        />
                        <Label
                          htmlFor={`organizer-${organizer}`}
                          className="text-sm text-black dark:text-white cursor-pointer"
                        >
                          {organizer}
                        </Label>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="flex justify-between pt-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={clearFilters}
                    className="text-black dark:text-white bg-secondary"
                  >
                    Clear All
                  </Button>
                  <Button size="sm" onClick={() => setIsFiltersOpen(false)} className="bg-[#eb6c6c] hover:bg-[#d55c5c]">
                    Apply Filters
                  </Button>
                </div>
              </div>
            </PopoverContent>
          </Popover>
        </div>
      </div>

      {selectedOrganizers.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {selectedOrganizers.map((organizer) => (
            <div
              key={organizer}
              className="bg-[#eb6c6c] bg-opacity-10 text-[#eb6c6c] px-3 py-1 rounded-full text-sm flex items-center"
            >
              {organizer}
              <button onClick={() => handleOrganizerChange(organizer)} className="ml-2">
                <X className="h-3 w-3" />
              </button>
            </div>
          ))}
          <button
            onClick={() => setSelectedOrganizers([])}
            className="text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 text-sm underline"
          >
            Clear all filters
          </button>
        </div>
      )}
    </div>
  )
}
