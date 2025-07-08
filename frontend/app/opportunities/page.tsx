"use client"

import { useState, useEffect } from "react"
import { useSearchParams, useRouter } from "next/navigation"
import { OpportunityCard } from "@/components/opportunities/opportunity-card"
import { OpportunityModal } from "@/components/opportunities/opportunity-modal"
import { SearchFilters } from "@/components/opportunities/search-filters"
import { Pagination } from "@/components/opportunities/pagination"
import { useRegisteredOpportunities } from "@/hooks/use-registered-opportunities"
import { calculateDistance, defaultLocation } from "@/utils/location"
import type { Opportunity, SortOption, SortDirection, UserLocation } from "@/types/opportunity"
import { CreateOpportunityModal } from "@/components/opportunities/create-opportunity-modal"
import { Button } from "@/components/ui/button"
import { Plus } from "lucide-react"
import { useOpportunities } from "@/hooks/api/use-opportunities"
import { useApp } from "@/context/AppProvider"

const ITEMS_PER_PAGE = 6

export default function OpportunitiesPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const opportunityId = searchParams.get("id")
  const { user } = useApp()

  // State for search and filters
  const [searchQuery, setSearchQuery] = useState("")
  const [sortOption, setSortOption] = useState<SortOption>("date")
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc")
  const [selectedOrganizers, setSelectedOrganizers] = useState<string[]>([])
  const [userLocationInput, setUserLocationInput] = useState(defaultLocation.address || "")
  const [userLocation, setUserLocation] = useState<UserLocation>(defaultLocation)

  // State for pagination
  const [currentPage, setCurrentPage] = useState(1)

  // State for modal
  const [selectedOpportunity, setSelectedOpportunity] = useState<Opportunity | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)

  // Get registered opportunities
  const { isRegistered, registerForOpportunity, cancelRegistration } = useRegisteredOpportunities()

  // Use our custom hook
  const { opportunities, isLoading, error, createOpportunity } = useOpportunities()

  // Open modal if opportunityId is in URL
  useEffect(() => {
    if (opportunityId) {
      const opportunity = opportunities.find((opp) => opp.id === opportunityId)
      if (opportunity) {
        setSelectedOpportunity(opportunity)
        setIsModalOpen(true)
      }
    }
  }, [opportunityId, opportunities])

  const handleCreateOpportunity = async (opportunityData: any) => {
    try {
      await createOpportunity(opportunityData)
      setIsCreateModalOpen(false)
    } catch (err) {
      console.error("Failed to create opportunity:", err)
    }
  }

  // Filter and sort opportunities
  const filteredOpportunities = opportunities
    .filter((opportunity) => {
      // Filter by search query
      const matchesSearch =
        searchQuery === "" ||
        opportunity.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
        opportunity.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
        opportunity.organizer.toLowerCase().includes(searchQuery.toLowerCase())

      // Filter by organizer
      const matchesOrganizer = selectedOrganizers.length === 0 || selectedOrganizers.includes(opportunity.organizer)

      return matchesSearch && matchesOrganizer
    })
    .sort((a, b) => {
      // Sort by selected option
      if (sortOption === "date") {
        return sortDirection === "asc"
          ? new Date(a.date).getTime() - new Date(b.date).getTime()
          : new Date(b.date).getTime() - new Date(a.date).getTime()
      }

      if (sortOption === "reward") {
        return sortDirection === "asc" ? a.rewardAmount - b.rewardAmount : b.rewardAmount - a.rewardAmount
      }

      if (sortOption === "proximity") {
        const distanceA = calculateDistance(
          userLocation.lat,
          userLocation.lng,
          a.location.coordinates.lat,
          a.location.coordinates.lng,
        )
        const distanceB = calculateDistance(
          userLocation.lat,
          userLocation.lng,
          b.location.coordinates.lat,
          b.location.coordinates.lng,
        )
        return sortDirection === "asc" ? distanceA - distanceB : distanceB - distanceA
      }

      if (sortOption === "organizer") {
        return sortDirection === "asc" ? a.organizer.localeCompare(b.organizer) : b.organizer.localeCompare(a.organizer)
      }

      return 0
    })

  // Calculate pagination
  const totalPages = Math.ceil(filteredOpportunities.length / ITEMS_PER_PAGE)
  const paginatedOpportunities = filteredOpportunities.slice(
    (currentPage - 1) * ITEMS_PER_PAGE,
    currentPage * ITEMS_PER_PAGE,
  )

  // Handle page change
  const handlePageChange = (page: number) => {
    setCurrentPage(page)
    window.scrollTo({ top: 0, behavior: "smooth" })
  }

  // Handle opportunity click
  const handleOpportunityClick = (opportunity: Opportunity) => {
    setSelectedOpportunity(opportunity)
    setIsModalOpen(true)
    // Update URL with opportunity ID
    router.push(`/opportunities?id=${opportunity.id}`, { scroll: false })
  }

  // Handle modal close
  const handleModalClose = () => {
    setIsModalOpen(false)
    setSelectedOpportunity(null)
    // Remove opportunity ID from URL
    router.push("/opportunities", { scroll: false })
  }

  // Handle registration
  const handleRegister = () => {
    if (selectedOpportunity) {
      registerForOpportunity(selectedOpportunity.id)
    }
  }

  // Handle cancel registration
  const handleCancelRegistration = () => {
    if (selectedOpportunity) {
      cancelRegistration(selectedOpportunity.id)
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="h-10 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        <div className="h-16 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="h-64 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
          ))}
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-8 text-center text-red-500">
        <p>Error loading opportunities: {error.message}</p>
        <Button className="mt-4" onClick={() => window.location.reload()}>
          Retry
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Volunteer Opportunities</h1>
        <div className="flex justify-between items-center">
          <p className="text-gray-600 dark:text-gray-400 mt-1">Find opportunities to volunteer and earn SFLuv</p>
          {user?.isOrganizer && (
            <Button onClick={() => setIsCreateModalOpen(true)}>
              <Plus className="mr-2 h-4 w-4" /> Create Opportunity
            </Button>
          )}
        </div>
      </div>

      <SearchFilters
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        sortOption={sortOption}
        setSortOption={setSortOption}
        sortDirection={sortDirection}
        setSortDirection={setSortDirection}
        selectedOrganizers={selectedOrganizers}
        setSelectedOrganizers={setSelectedOrganizers}
        userLocation={userLocationInput}
        setUserLocation={setUserLocationInput}
      />

      {paginatedOpportunities.length === 0 ? (
        <div className="text-center py-12">
          <h3 className="text-xl font-medium text-black dark:text-white">No opportunities found</h3>
          <p className="text-gray-600 dark:text-gray-400 mt-2">
            Try adjusting your search or filters to find opportunities
          </p>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {paginatedOpportunities.map((opportunity) => (
              <OpportunityCard
                key={opportunity.id}
                opportunity={opportunity}
                onClick={() => handleOpportunityClick(opportunity)}
              />
            ))}
          </div>

          {totalPages > 1 && (
            <div className="mt-8">
              <Pagination currentPage={currentPage} totalPages={totalPages} onPageChange={handlePageChange} />
            </div>
          )}
        </>
      )}

      <OpportunityModal
        opportunity={selectedOpportunity}
        isOpen={isModalOpen}
        onClose={handleModalClose}
        isRegistered={selectedOpportunity ? isRegistered(selectedOpportunity.id) : false}
        onRegister={handleRegister}
        onCancelRegistration={handleCancelRegistration}
      />
      {user?.isOrganizer && (
        <CreateOpportunityModal
          isOpen={isCreateModalOpen}
          onClose={() => setIsCreateModalOpen(false)}
          onCreateOpportunity={handleCreateOpportunity}
        />
      )}
    </div>
  )
}
