"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Search, Plus, Calendar } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Pagination } from "@/components/opportunities/pagination"
import { CreateOpportunityModal } from "@/components/opportunities/create-opportunity-modal"
import { useApp } from "@/context/AppProvider"
import { format } from "date-fns"
import { redirect } from "next/navigation"
import { useOpportunities } from "@/hooks/api/use-opportunities"

const ITEMS_PER_PAGE = 10

export default function YourOpportunitiesPage() {
  const router = useRouter()
  const { user } = useApp()
  const [searchQuery, setSearchQuery] = useState("")
  const [currentPage, setCurrentPage] = useState(1)
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)

  // Use our custom hook
  const { opportunities, isLoading, error, createOpportunity, getOpportunitiesByOrganizer } = useOpportunities()

  // Redirect if not an organizer
  if (!user?.isOrganizer) {
    redirect("/dashboard")
  }

  // Filter opportunities to only show those created by the current user
  // In a real app, this would be based on the user's ID
  // For demo purposes, we'll filter by organizer name
  const yourOpportunities = getOpportunitiesByOrganizer("SF Community Gardens").concat(
    getOpportunitiesByOrganizer("Clean SF Initiative"),
  )

  // Filter opportunities by search query
  const filteredOpportunities = yourOpportunities.filter(
    (opp) =>
      opp.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
      opp.description.toLowerCase().includes(searchQuery.toLowerCase()),
  )

  // Calculate pagination
  const totalPages = Math.ceil(filteredOpportunities.length / ITEMS_PER_PAGE)
  const paginatedOpportunities = filteredOpportunities.slice(
    (currentPage - 1) * ITEMS_PER_PAGE,
    currentPage * ITEMS_PER_PAGE,
  )

  // Handle page change
  const handlePageChange = (page: number) => {
    setCurrentPage(page)
  }

  // Handle opportunity click
  const handleOpportunityClick = (opportunityId: string) => {
    router.push(`/your-opportunities/${opportunityId}`)
  }

  // Handle create opportunity
  const handleCreateOpportunity = async (opportunityData: any) => {
    try {
      await createOpportunity(opportunityData)
      setIsCreateModalOpen(false)
    } catch (err) {
      console.error("Failed to create opportunity:", err)
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="h-10 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        <div className="h-16 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        <div className="h-64 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
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
        <h1 className="text-3xl font-bold text-black dark:text-white">Your Opportunities</h1>
        <div className="flex justify-between items-center">
          <p className="text-gray-600 dark:text-gray-400 mt-1">Manage the volunteer opportunities you've created</p>
          <Button onClick={() => setIsCreateModalOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Create Opportunity
          </Button>
        </div>
      </div>

      <div className="flex justify-between items-center">
        <div className="relative w-full max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-gray-500" />
          <Input
            type="search"
            placeholder="Search opportunities..."
            className="pl-8"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

      <div className="rounded-md border bg-white dark:bg-[#2a2a2a]">
        <div className="grid grid-cols-12 gap-4 p-4 font-medium border-b text-black dark:text-white">
          <div className="col-span-5">Title</div>
          <div className="col-span-2">Date</div>
          <div className="col-span-2">Reward</div>
          <div className="col-span-3">Registrations</div>
        </div>

        {paginatedOpportunities.length === 0 ? (
          <div className="p-8 text-center text-gray-500 dark:text-gray-400">No opportunities found</div>
        ) : (
          <div>
            {paginatedOpportunities.map((opportunity) => (
              <div
                key={opportunity.id}
                className="grid grid-cols-12 gap-4 p-4 border-b hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer"
                onClick={() => handleOpportunityClick(opportunity.id)}
              >
                <div className="col-span-5 flex items-center">
                  <div className="h-10 w-10 rounded-md bg-gray-100 dark:bg-gray-700 flex items-center justify-center mr-3">
                    <Calendar className="h-5 w-5 text-gray-500 dark:text-gray-400" />
                  </div>
                  <div>
                    <div className="font-medium text-black dark:text-white">{opportunity.title}</div>
                    <div className="text-sm text-gray-500 dark:text-gray-400 truncate max-w-xs">
                      {opportunity.description.substring(0, 60)}...
                    </div>
                  </div>
                </div>
                <div className="col-span-2 flex items-center text-black dark:text-white">
                  {format(new Date(opportunity.date), "MMM d, yyyy")}
                </div>
                <div className="col-span-2 flex items-center text-black dark:text-white">
                  {opportunity.rewardAmount} SFLuv
                </div>
                <div className="col-span-3 flex items-center">
                  <Badge
                    variant={
                      opportunity.volunteersSignedUp >= opportunity.volunteersNeeded
                        ? "success"
                        : opportunity.volunteersSignedUp >= opportunity.volunteersNeeded / 2
                          ? "default"
                          : "warning"
                    }
                  >
                    {opportunity.volunteersSignedUp} / {opportunity.volunteersNeeded}
                  </Badge>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {totalPages > 1 && (
        <div className="mt-4 flex justify-center">
          <Pagination currentPage={currentPage} totalPages={totalPages} onPageChange={handlePageChange} />
        </div>
      )}

      <CreateOpportunityModal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        onCreateOpportunity={handleCreateOpportunity}
      />
    </div>
  )
}
