"use client"

import { useRouter, usePathname } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { cn } from "@/lib/utils"
import {
  BarChart3,
  Home,
  LogOut,
  Map,
  Settings,
  ShoppingBag,
  Users,
  Wallet,
  CalendarClock,
  FileCheck,
  Calendar,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"

export function DashboardSidebar() {
  const router = useRouter()
  const pathname = usePathname()
  const { user, logout } = useApp()

  const isActive = (path: string) => pathname === path

  const handleLogout = () => {
    logout()
    router.push("/")
  }

  // Define navigation items based on user role
  const getNavItems = () => {
    const baseItems = [
      {
        title: "Dashboard",
        icon: Home,
        path: "/dashboard",
      },
      {
        title: "Opportunities",
        icon: CalendarClock,
        path: "/dashboard/opportunities",
      },
      {
        title: "Calendar",
        icon: Calendar,
        path: "/dashboard/calendar",
      },
      {
        title: "Merchant Map",
        icon: Map,
        path: "/dashboard/map",
      },
      {
        title: "Connected Wallets",
        icon: Wallet,
        path: "/dashboard/wallets",
      },
    ]

    const merchantItems = [
      {
        title: "Transactions",
        icon: ShoppingBag,
        path: "/dashboard/transactions",
      },
      {
        title: "Unwrap Currency",
        icon: Wallet,
        path: "/dashboard/unwrap",
      },
    ]

    const organizerItems = [
      {
        title: "Your Opportunities",
        icon: CalendarClock,
        path: "/dashboard/your-opportunities",
      },
    ]

    const adminItems = [
      {
        title: "Users",
        icon: Users,
        path: "/dashboard/users",
      },
      {
        title: "Role Management",
        icon: FileCheck,
        path: "/dashboard/role-management",
      },
      {
        title: "Metrics",
        icon: BarChart3,
        path: "/dashboard/metrics",
      },
    ]

    let items = [...baseItems]

    // Only show merchant items if user is a merchant with approved status
    if (user?.role === "merchant" && user?.merchantStatus === "approved") {
      items = [...items, ...merchantItems]
    }

    // Add merchant status link for users with any merchant status
    if (user?.merchantStatus) {
      items.push({
        title: "Merchant Status",
        icon: FileCheck,
        path: "/dashboard/merchant-status",
      })
    }

    if (user?.isOrganizer) {
      items = [...items, ...organizerItems]
    }

    if (user?.role === "admin") {
      items = [...items, ...merchantItems, ...adminItems]
    }

    return items
  }

  return (
    <Sidebar className="bg-secondary dark:bg-secondary">
      <SidebarHeader className="border-b bg-secondary dark:bg-secondary">
        <div
          className="flex items-center p-2 cursor-pointer hover:bg-secondary/80 transition-colors"
          onClick={() => router.push("/dashboard")}
        >
          <div className="flex-1 overflow-hidden">
            <h2 className="text-lg font-semibold text-black dark:text-white truncate">SFLuv Dashboard</h2>
            <p className="text-xs text-gray-500 dark:text-gray-400 truncate">
              {user?.role && `Logged in as ${user.role}`}
            </p>
          </div>
        </div>
      </SidebarHeader>
      <SidebarContent
        data-mobile="true"
        className="w-[--sidebar-width] bg-secondary dark:bg-secondary p-0 text-sidebar-foreground [&>button]:hidden"
      >
        <SidebarMenu>
          {getNavItems().map((item) => (
            <SidebarMenuItem key={item.path}>
              <SidebarMenuButton asChild isActive={isActive(item.path)} tooltip={item.title}>
                <Button
                  variant="ghost"
                  className={cn(
                    "w-full justify-start transition-colors hover:bg-secondary/80",
                    isActive(item.path)
                      ? "bg-[#eb6c6c] text-white hover:bg-[#d55c5c] rounded-none"
                      : "text-black dark:text-white",
                  )}
                  onClick={() => router.push(item.path)}
                >
                  <item.icon className="mr-2 h-4 w-4" />
                  <span>{item.title}</span>
                </Button>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarContent>
      <SidebarFooter className="border-t p-2 bg-secondary dark:bg-secondary">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild tooltip="Settings" isActive={isActive("/dashboard/settings")}>
              <Button
                variant="ghost"
                className={cn(
                  "w-full justify-start transition-colors hover:bg-secondary/80",
                  isActive("/dashboard/settings")
                    ? "bg-[#eb6c6c] text-white hover:bg-[#d55c5c] rounded-md"
                    : "text-black dark:text-white",
                )}
                onClick={() => router.push("/dashboard/settings")}
              >
                <Settings className="mr-2 h-4 w-4" />
                <span>Settings</span>
              </Button>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton asChild tooltip="Logout">
              <Button
                variant="ghost"
                className="w-full justify-start text-red-500 hover:text-white hover:bg-red-500 transition-colors rounded-md"
                onClick={handleLogout}
              >
                <LogOut className="mr-2 h-4 w-4" />
                <span>Logout</span>
              </Button>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
