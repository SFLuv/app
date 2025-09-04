"use client"

import { useState, useMemo } from "react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Label } from "@/components/ui/label"
import { Search, Filter } from "lucide-react"
import { type Merchant, type MerchantStatus, merchantStatusLabels } from "@/types/merchant"
import { MerchantDetailsModal } from "./merchant-details-modal"
import { useMerchants } from "@/hooks/api/use-merchants"

const merchantTypeLabels: { [key: string]: string } = {
  restaurant: "Restaurant",
  grocery: "Grocery Store",
  retail: "Retail Store",
  cafe: "Café",
  service: "Service Provider",
  entertainment: "Entertainment",
  health: "Health & Wellness",
  beauty: "Beauty & Spa",
  other: "Other",
}

export function MerchantManagement() {
  const [searchQuery, setSearchQuery] = useState("")
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [selectedMerchant, setSelectedMerchant] = useState<Merchant | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)

  // Use our custom hook
  const { merchants, isLoading, error, updateMerchantStatus, getMerchantsByStatus } = useMerchants()

  const filteredMerchants = useMemo(() => {
    // First filter by status
    const statusFiltered = statusFilter === "all" ? merchants : getMerchantsByStatus(statusFilter as MerchantStatus)

    // Then filter by search query
    return statusFiltered.filter((merchant) => merchant.name.toLowerCase().includes(searchQuery.toLowerCase()))
  }, [merchants, searchQuery, statusFilter, getMerchantsByStatus])

  const handleMerchantClick = (merchant: Merchant) => {
    setSelectedMerchant(merchant)
    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedMerchant(null)
  }

  const handleUpdateStatus = async (merchantId: string, status: MerchantStatus) => {
    try {
      await updateMerchantStatus(merchantId, status)
    } catch (err) {
      console.error("Failed to update merchant status:", err)
    }
  }

  const getStatusBadgeClass = (status: MerchantStatus) => {
    switch (status) {
      case "approved":
        return "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
      case "pending":
        return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
      case "rejected":
        return "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
      case "revoked":
        return "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200"
      default:
        return "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200"
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        <div className="h-10 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
        <div className="h-64 bg-gray-200 dark:bg-gray-700 rounded animate-pulse"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-8 text-center text-red-500">
        <p>Error loading merchants: {error.message}</p>
        <Button className="mt-4" onClick={() => window.location.reload()}>
          Retry
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col md:flex-row gap-4 items-start md:items-end">
        <div className="w-full md:w-1/2">
          <Label htmlFor="search" className="text-sm font-medium">
            Search Merchants
          </Label>
          <div className="relative mt-1">
            <Search className="absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-500 dark:text-gray-400" />
            <Input
              id="search"
              placeholder="Search by merchant name..."
              className="pl-8"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
        </div>
        <div className="w-full md:w-1/4">
          <Label htmlFor="status-filter" className="text-sm font-medium">
            Filter by Status
          </Label>
          <Select value={statusFilter} onValueChange={setStatusFilter}>
            <SelectTrigger id="status-filter" className="mt-1">
              <SelectValue placeholder="Filter by status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Statuses</SelectItem>
              {Object.entries(merchantStatusLabels).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button variant="outline" className="w-full md:w-auto" onClick={() => setStatusFilter("all")}>
          <Filter className="mr-2 h-4 w-4" />
          Clear Filters
        </Button>
      </div>

      <div className="border rounded-md bg-white dark:bg-[#2a2a2a]">
        <div className="grid grid-cols-12 gap-4 p-4 font-medium text-sm bg-secondary text-black dark:text-white">
          <div className="col-span-4">Merchant Name</div>
          <div className="col-span-3">Type</div>
          <div className="col-span-3">Contact</div>
          <div className="col-span-2">Status</div>
        </div>
        <div className="divide-y">
          {filteredMerchants.length > 0 ? (
            filteredMerchants.map((merchant) => (
              <div
                key={merchant.id}
                className="grid grid-cols-12 gap-4 p-4 hover:bg-secondary/50 dark:hover:bg-gray-800 cursor-pointer transition-colors"
                onClick={() => handleMerchantClick(merchant)}
              >
                <div className="col-span-4">
                  <div className="font-medium text-black dark:text-white">{merchant.name}</div>
                  <div className="text-sm text-gray-500 dark:text-gray-400 truncate">
                    {merchant.address.city}, {merchant.address.state}
                  </div>
                </div>
                <div className="col-span-3">
                  <div className="text-sm text-black dark:text-white">{merchantTypeLabels[merchant.type]}</div>
                </div>
                <div className="col-span-3">
                  <div className="text-sm text-black dark:text-white">{merchant.contactInfo.email}</div>
                  <div className="text-sm text-gray-500 dark:text-gray-400">{merchant.contactInfo.phone}</div>
                </div>
                <div className="col-span-2">
                  <span
                    className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${getStatusBadgeClass(merchant.status)}`}
                  >
                    {merchantStatusLabels[merchant.status]}
                  </span>
                </div>
              </div>
            ))
          ) : (
            <div className="p-8 text-center text-gray-500 dark:text-gray-400">
              No merchants found matching your search criteria.
            </div>
          )}
        </div>
      </div>

      <MerchantDetailsModal
        merchant={selectedMerchant}
        isOpen={isModalOpen}
        onClose={handleCloseModal}
        onUpdateStatus={handleUpdateStatus}
      />
    </div>
  )
}
