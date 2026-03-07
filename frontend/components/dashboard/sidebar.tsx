"use client"

import { useRouter, usePathname } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { cn } from "@/lib/utils"
import { useIsMobile } from "@/hooks/use-mobile"
import {
  LogOut,
  Map,
  Settings,
  Wallet,
  Handshake,
  FileCheck,
  SquareUserIcon,
  Shield,
  ClipboardList,
  ClipboardCheck,
  Vote,
  ShieldCheck,
  Wrench,
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
import { ForwardRefExoticComponent } from "react"

export function DashboardSidebar() {
  const router = useRouter()
  const pathname = usePathname()
  const isMobile = useIsMobile()
  const { user, logout, status, login, userLocations, wallets } = useApp()

  const isNonAdminNonMerchant = user?.isAdmin !== true && user?.isMerchant !== true
  const shouldShortcutToWallet = status === "authenticated" && isMobile && isNonAdminNonMerchant
  const primarySmartWallet =
    wallets.find((wallet) => wallet.type === "smartwallet" && wallet.index === 0n) ??
    wallets.find((wallet) => wallet.type === "smartwallet")
  const hasWalletShortcutTarget = shouldShortcutToWallet && Boolean(primarySmartWallet?.address)
  const walletNavTitle = hasWalletShortcutTarget ? "Wallet" : "Connected Wallets"
  const walletNavPath =
    hasWalletShortcutTarget && primarySmartWallet?.address
      ? `/wallets/${primarySmartWallet.address}?fromWalletMenu=1`
      : "/wallets"

  const isActive = (path: string) => {
    const [pathWithoutQuery] = path.split("?")
    return pathname.startsWith(pathWithoutQuery)
  }

  const handleLogout = () => {
    logout()
    router.push("/map")
  }

  interface NavItem {
    title: string;
    icon: ForwardRefExoticComponent<any>;
    path: string;
  }

  // Define navigation items based on user role
  const getNavItems = () => {
    const baseItems: NavItem[] = [
      // {
      //   title: "Dashboard",
      //   icon: Home,
      //   path: "/dashboard",
      // },
      // {
      //   title: "Opportunities",
      //   icon: CalendarClock,
      //   path: "/opportunities",
      // },
      // {
      //   title: "Calendar",
      //   icon: Calendar,
      //   path: "/calendar",
      // },
      {
        title: "Merchant Map",
        icon: Map,
        path: "/map",
      }
    ]

    const authedItems: NavItem[] = [
      {
        title: walletNavTitle,
        icon: Wallet,
        path: walletNavPath,
      },
      {
        title: "Contacts",
        icon: SquareUserIcon,
        path: "/contacts"
      },
    ]

    const merchantItems: NavItem[] = [
    //   {
    //     title: "Transactions",
    //     icon: ShoppingBag,
    //     path: "/transactions",
    //   },
    //   {
    //     title: "Unwrap Currency",
    //     icon: Wallet,
    //     path: "/unwrap",
    //   },
    ]

    const organizerItems: NavItem[] = [
    //   {
    //     title: "Your Opportunities",
    //     icon: CalendarClock,
    //     path: "/your-opportunities",
    //   },
    ]

    const affiliateItems: NavItem[] = [
      {
        title: "Affiliates Panel",
        icon: Handshake,
        path: "/affiliates",
      },
    ]

    const proposerItems: NavItem[] = [
      {
        title: "Proposer Panel",
        icon: ClipboardList,
        path: "/proposer",
      },
    ]

    const improverItems: NavItem[] = [
      {
        title: "Improver Panel",
        icon: Wrench,
        path: "/improver",
      },
    ]

    const voterItems: NavItem[] = [
      {
        title: "Voter Panel",
        icon: Vote,
        path: "/voter",
      },
    ]

    const issuerItems: NavItem[] = [
      {
        title: "Issuer Panel",
        icon: ShieldCheck,
        path: "/issuer",
      },
    ]

    const supervisorItems: NavItem[] = [
      {
        title: "Supervisor Panel",
        icon: ClipboardCheck,
        path: "/supervisor",
      },
    ]

    const adminItems: NavItem[] = [
      {
        title: "Admin Panel",
        icon: Shield,
        path: "/admin"
      }
    //   {
    //     title: "Users",
    //     icon: Users,
    //     path: "/users",
    //   },
    //   {
    //     title: "Role Management",
    //     icon: FileCheck,
    //     path: "/role-management",
    //   },
    //   {
    //     title: "Metrics",
    //     icon: BarChart3,
    //     path: "/metrics",
    //   },
    ]

    let items = [...baseItems]

    if (status === "authenticated") {
      items = [...items, ...authedItems]
    }

    // Only show merchant items if user is a merchant with approved status
    if (user?.isMerchant) {
      items = [...items, ...merchantItems]
    }

    // Add merchant status link for users with any merchant status
    if (userLocations.length !== 0) {
      items.push({
        title: "Merchant Status",
        icon: FileCheck,
        path: "/merchant-status",
      })
    }

    if (user?.isOrganizer) {
      items = [...items, ...organizerItems]
    }

    if (user?.isAffiliate) {
      items = [...items, ...affiliateItems]
    }

    if (user?.isProposer || user?.isAdmin) {
      items = [...items, ...proposerItems]
    }

    if (user?.isImprover) {
      items = [...items, ...improverItems]
    }

    if (user?.isVoter || user?.isAdmin) {
      items = [...items, ...voterItems]
    }

    if (user?.isIssuer || user?.isAdmin) {
      items = [...items, ...issuerItems]
    }

    if (user?.isSupervisor || user?.isAdmin) {
      items = [...items, ...supervisorItems]
    }

    if (user?.isAdmin) {
      items = [...items, ...merchantItems, ...adminItems]
    }

    return items
  }

  return (
    <Sidebar className="bg-secondary dark:bg-secondary">
      <SidebarHeader className="border-b bg-secondary dark:bg-secondary">
        <div
          className="flex items-center p-2 cursor-pointer hover:bg-secondary/80 transition-colors"
          onClick={() => router.push("/map")}
        >
          <div className="flex-1 overflow-hidden">
            <h2 className="text-lg font-semibold text-black dark:text-white truncate">SFLuv Dashboard</h2>
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
                    "w-full justify-start transition-colors hover:bg-primary/60 rounded-none",
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
          {status === "authenticated" ? <>
          {!isActive("/settings") &&
          <Button
              variant="outline"
              className="bg-secondary text-[#eb6c6c] border-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
              onClick={() => router.push("/settings/merchant-approval")}>
              {userLocations.length === 0 ?
              "Apply to Become a Merchant" :
              "Submit Another Application"
              }
          </Button>
          }
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Settings" isActive={isActive("/settings")}>
                <Button
                  variant="ghost"
                  className={cn(
                    "w-full justify-start transition-colors hover:bg-primary",
                    isActive("/settings")
                      ? "bg-[#eb6c6c] text-white hover:bg-[#d55c5c] rounded-md"
                      : "text-black dark:text-white",
                  )}
                  onClick={() => {router.push("/settings");
                  }
                  }
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
          </> : <>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Connect">
                <Button
                  variant="default"
                  size="lg"
                  className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                  onClick={() => login()}
                >
                  Connect
                </Button>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </>}
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
