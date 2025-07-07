"use client"

import { useState } from "react"
import { Search, Plus, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Pagination } from "@/components/opportunities/pagination"
import { useUsers } from "@/hooks/api/use-users"

const ITEMS_PER_PAGE = 5

export function OrganizerManagement() {
  // State for organizer list
  const [searchQuery, setSearchQuery] = useState("")
  const [currentPage, setCurrentPage] = useState(1)

  // State for modals
  const [isAddOrganizerOpen, setIsAddOrganizerOpen] = useState(false)
  const [isViewOrganizerOpen, setIsViewOrganizerOpen] = useState(false)
  const [selectedUser, setSelectedUser] = useState<any>(null)
  const [addSearchQuery, setAddSearchQuery] = useState("")

  // Use our custom hook
  const { users, isLoading, error, getOrganizers, getNonOrganizers, toggleOrganizerStatus } = useUsers()

  // Get organizers
  const organizers = getOrganizers()

  // Filter organizers by search query
  const filteredOrganizers = organizers.filter(
    (organizer) =>
      organizer.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      organizer.email.toLowerCase().includes(searchQuery.toLowerCase()),
  )

  // Calculate pagination
  const totalPages = Math.ceil(filteredOrganizers.length / ITEMS_PER_PAGE)
  const paginatedOrganizers = filteredOrganizers.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)

  // Filter users for add organizer modal
  const nonOrganizers = getNonOrganizers()
  const filteredUsers = nonOrganizers.filter(
    (user) =>
      user.name.toLowerCase().includes(addSearchQuery.toLowerCase()) ||
      user.email.toLowerCase().includes(addSearchQuery.toLowerCase()),
  )

  // Handle page change
  const handlePageChange = (page: number) => {
    setCurrentPage(page)
  }

  // Handle view organizer
  const handleViewOrganizer = (organizer: any) => {
    setSelectedUser(organizer)
    setIsViewOrganizerOpen(true)
  }

  // Handle remove organizer
  const handleRemoveOrganizer = async () => {
    if (!selectedUser) return

    try {
      await toggleOrganizerStatus(selectedUser.id)
      setIsViewOrganizerOpen(false)
    } catch (err) {
      console.error("Failed to remove organizer:", err)
    }
  }

  // Handle add organizer
  const handleAddOrganizer = async (user: any) => {
    try {
      await toggleOrganizerStatus(user.id)
      setIsAddOrganizerOpen(false)
    } catch (err) {
      console.error("Failed to add organizer:", err)
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
        <p>Error loading organizers: {error.message}</p>
        <Button className="mt-4" onClick={() => window.location.reload()}>
          Retry
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="relative w-full max-w-sm">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-gray-500" />
          <Input
            type="search"
            placeholder="Search organizers..."
            className="pl-8"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
        <Button onClick={() => setIsAddOrganizerOpen(true)}>
          <Plus className="mr-2 h-4 w-4" /> Add Organizer
        </Button>
      </div>

      <div className="rounded-md border bg-white dark:bg-[#2a2a2a]">
        <div className="grid grid-cols-12 gap-4 p-4 font-medium border-b text-black dark:text-white">
          <div className="col-span-5">Name</div>
          <div className="col-span-4">Email</div>
          <div className="col-span-3">Role</div>
        </div>

        {paginatedOrganizers.length === 0 ? (
          <div className="p-8 text-center text-gray-500 dark:text-gray-400">No organizers found</div>
        ) : (
          <div>
            {paginatedOrganizers.map((organizer) => (
              <div
                key={organizer.id}
                className="grid grid-cols-12 gap-4 p-4 border-b hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer"
                onClick={() => handleViewOrganizer(organizer)}
              >
                <div className="col-span-5 flex items-center">
                  <Avatar className="h-8 w-8 mr-2">
                    <AvatarImage
                      src={`/abstract-geometric-shapes.png?key=3d4oi&height=32&width=32&query=${organizer.name}`}
                    />
                    <AvatarFallback>{organizer.name.substring(0, 2).toUpperCase()}</AvatarFallback>
                  </Avatar>
                  <span className="text-black dark:text-white">{organizer.name}</span>
                </div>
                <div className="col-span-4 flex items-center text-black dark:text-white">{organizer.email}</div>
                <div className="col-span-3 flex items-center">
                  <Badge
                    variant={
                      organizer.role === "admin" ? "destructive" : organizer.role === "merchant" ? "outline" : "default"
                    }
                  >
                    {organizer.role.charAt(0).toUpperCase() + organizer.role.slice(1)}
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

      {/* Add Organizer Modal */}
      <Dialog open={isAddOrganizerOpen} onOpenChange={setIsAddOrganizerOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Add Organizer</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-gray-500" />
              <Input
                type="search"
                placeholder="Search users..."
                className="pl-8"
                value={addSearchQuery}
                onChange={(e) => setAddSearchQuery(e.target.value)}
              />
            </div>

            <div className="max-h-[300px] overflow-y-auto border rounded-md">
              {filteredUsers.length === 0 ? (
                <div className="p-4 text-center text-gray-500">No users found</div>
              ) : (
                filteredUsers.map((user) => (
                  <div
                    key={user.id}
                    className="flex items-center justify-between p-3 border-b hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer"
                    onClick={() => handleAddOrganizer(user)}
                  >
                    <div className="flex items-center">
                      <Avatar className="h-8 w-8 mr-2">
                        <AvatarImage
                          src={`/abstract-geometric-shapes.png?key=kvo5r&height=32&width=32&query=${user.name}`}
                        />
                        <AvatarFallback>{user.name.substring(0, 2).toUpperCase()}</AvatarFallback>
                      </Avatar>
                      <div>
                        <div className="font-medium text-black dark:text-white">{user.name}</div>
                        <div className="text-sm text-gray-500 dark:text-gray-400">{user.email}</div>
                      </div>
                    </div>
                    <Badge
                      variant={user.role === "admin" ? "destructive" : user.role === "merchant" ? "outline" : "default"}
                    >
                      {user.role.charAt(0).toUpperCase() + user.role.slice(1)}
                    </Badge>
                  </div>
                ))
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* View Organizer Modal */}
      <Dialog open={isViewOrganizerOpen} onOpenChange={setIsViewOrganizerOpen}>
        <DialogContent className="sm:max-w-md">
          {selectedUser && (
            <>
              <DialogHeader>
                <DialogTitle>Organizer Details</DialogTitle>
              </DialogHeader>
              <div className="space-y-4 py-4">
                <div className="flex items-center space-x-4">
                  <Avatar className="h-12 w-12">
                    <AvatarImage
                      src={`/abstract-geometric-shapes.png?key=etukk&height=48&width=48&query=${selectedUser.name}`}
                    />
                    <AvatarFallback>{selectedUser.name.substring(0, 2).toUpperCase()}</AvatarFallback>
                  </Avatar>
                  <div>
                    <h3 className="font-medium text-lg text-black dark:text-white">{selectedUser.name}</h3>
                    <p className="text-gray-500 dark:text-gray-400">{selectedUser.email}</p>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-4 pt-4">
                  <div>
                    <p className="text-sm font-medium text-gray-500 dark:text-gray-400">User Role</p>
                    <p className="mt-1">
                      <Badge
                        variant={
                          selectedUser.role === "admin"
                            ? "destructive"
                            : selectedUser.role === "merchant"
                              ? "outline"
                              : "default"
                        }
                      >
                        {selectedUser.role.charAt(0).toUpperCase() + selectedUser.role.slice(1)}
                      </Badge>
                    </p>
                  </div>
                  <div>
                    <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Organizer Status</p>
                    <p className="mt-1">
                      <Badge variant="success">Active</Badge>
                    </p>
                  </div>
                </div>
              </div>
              <DialogFooter>
                <Button variant="destructive" onClick={handleRemoveOrganizer}>
                  <X className="mr-2 h-4 w-4" /> Remove Organizer Role
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
