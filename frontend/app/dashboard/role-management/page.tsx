"use client"

import { useState } from "react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { MerchantManagement } from "@/components/role-management/merchant-management"
import { OrganizerManagement } from "@/components/role-management/organizer-management"
import { BlacklistedUsers } from "@/components/role-management/blacklisted-users"
import { useApp } from "@/context/app-context"
import { redirect } from "next/navigation"

export default function RoleManagementPage() {
  const [activeTab, setActiveTab] = useState("merchants")
  const { user } = useApp()

  // Redirect if not admin
  if (user?.role !== "admin") {
    redirect("/dashboard")
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Role Management</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">
          Manage user roles and permissions across the SFLuv platform
        </p>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="merchants">Merchant Management</TabsTrigger>
          <TabsTrigger value="organizers">Organizer Management</TabsTrigger>
          <TabsTrigger value="blacklisted">Blacklisted Users</TabsTrigger>
        </TabsList>
        <TabsContent value="merchants" className="mt-6">
          <MerchantManagement />
        </TabsContent>
        <TabsContent value="organizers" className="mt-6">
          <OrganizerManagement />
        </TabsContent>
        <TabsContent value="blacklisted" className="mt-6">
          <BlacklistedUsers />
        </TabsContent>
      </Tabs>
    </div>
  )
}
