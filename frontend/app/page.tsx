"use client"

import { useApp } from "@/context/AppProvider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { BarChart3, CalendarClock, Map, ShoppingBag, Store, Users, Wallet, ArrowDownToLine } from "lucide-react"
import { useRouter } from "next/navigation"
import { TransactionModal } from "@/components/transactions/transaction-modal"
import { mockMerchantTransactions, mockUserTransactions } from "@/data/mock-transactions"
import { useEffect, useState } from "react"
import { Transaction } from "@/types/transaction"

export default function Dashboard() {
  const { user, status } = useApp()
  const router = useRouter()

  //CHANGE ONCE DASHBOARD IMPLEMENTED
  useEffect(() => {
    router.replace("/map")
  }, [])

  return (
    <div className="min-h-screen flex items-center justify-center">
      <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
    </div>
  )
  // Mock data for dashboard
  const mockData = {
    balance: 1250,
    volunteeredHours: 25,
    transactionsCount: 47,
    merchantCount: 128,
    userCount: 1543,
    pendingApplications: 12,
    totalUnwrapped: 3250,
  }

  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null)
  const [isModalOpen, setIsModalOpen] = useState<boolean>(false)

  // Get transactions based on user role
  const transactions = user?.isMerchant ? mockMerchantTransactions : mockUserTransactions

  const handleTransactionClick = (index: number) => {
    setSelectedTransaction(transactions[index])
    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedTransaction(null)
  }

  // Dashboard cards for different user roles
  const userCards = [
    {
      title: "Your Balance",
      description: "Current SFLuv balance",
      value: `${mockData.balance} SFLuv`,
      icon: Wallet,
      color: "text-green-500",
      action: {
        label: "View History",
        onClick: () => router.push("/history"),
      },
    },
    {
      title: "Volunteered Hours",
      description: "Total hours volunteered",
      value: `${mockData.volunteeredHours} hours`,
      icon: CalendarClock,
      color: "text-blue-500",
      action: {
        label: "Find Opportunities",
        onClick: () => router.push("/opportunities"),
      },
    },
    {
      title: "Participating Merchants",
      description: "Places to spend SFLuv",
      value: `${mockData.merchantCount} merchants`,
      icon: Store,
      color: "text-purple-500",
      action: {
        label: "View Map",
        onClick: () => router.push("/map"),
      },
    },
  ]

  const merchantCards = [
    {
      title: "Your Balance",
      description: "Current SFLuv balance",
      value: `${mockData.balance} SFLuv`,
      icon: Wallet,
      color: "text-green-500",
      action: {
        label: "Unwrap Currency",
        onClick: () => router.push("/unwrap"),
      },
    },
    {
      title: "Transactions",
      description: "Total transactions received",
      value: `${mockData.transactionsCount} transactions`,
      icon: ShoppingBag,
      color: "text-orange-500",
      action: {
        label: "View Transactions",
        onClick: () => router.push("/transactions"),
      },
    },
    {
      title: "Total Unwrapped",
      description: "SFLuv converted to USD",
      value: `${mockData.totalUnwrapped} SFLuv`,
      icon: ArrowDownToLine,
      color: "text-blue-500",
      action: {
        label: "Unwrap More",
        onClick: () => router.push("/unwrap"),
      },
    },
  ]

  const adminCards = [
    {
      title: "Total Users",
      description: "Registered community members",
      value: `${mockData.userCount} users`,
      icon: Users,
      color: "text-blue-500",
      action: {
        label: "Manage Users",
        onClick: () => router.push("/users"),
      },
    },
    {
      title: "Total Merchants",
      description: "Registered businesses",
      value: `${mockData.merchantCount} merchants`,
      icon: Store,
      color: "text-purple-500",
      action: {
        label: "Manage Merchants",
        onClick: () => router.push("/merchants"),
      },
    },
    {
      title: "Pending Applications",
      description: "Merchant applications awaiting approval",
      value: `${mockData.pendingApplications} applications`,
      icon: ShoppingBag,
      color: "text-yellow-500",
      action: {
        label: "Review Applications",
        onClick: () => router.push("/applications"),
      },
    },
    {
      title: "System Metrics",
      description: "Overall platform performance",
      value: "View Details",
      icon: BarChart3,
      color: "text-indigo-500",
      action: {
        label: "View Metrics",
        onClick: () => router.push("/metrics"),
      },
    },
  ]

  // Determine which cards to show based on user role
  const getCards = () => {
    return user?.isAdmin ?
      adminCards
    : user?.isMerchant ?
      merchantCards
    :
      userCards
  }

  const cards = getCards()

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }


  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Welcome, {user?.name || "User"}</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">
          {user?.isAdmin
            ? "Here's an overview of the SFLuv platform"
            : user?.isMerchant
              ? "Here's an overview of your merchant account"
              : "Here's an overview of your SFLuv account"}
        </p>
      </div>

      <div className="grid gap-6 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {cards.map((card, index) => (
          <Card key={index} className="flex flex-col">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <div className="space-y-1">
                <CardTitle className="text-sm font-medium text-black dark:text-white">{card.title}</CardTitle>
                <CardDescription>{card.description}</CardDescription>
              </div>
              <card.icon className={`h-5 w-5 ${card.color}`} />
            </CardHeader>
            <CardContent className="flex flex-col flex-1 justify-between">
              <div className="text-2xl font-bold text-black dark:text-white">{card.value}</div>
              <Button
                variant="ghost"
                className="mt-4 w-full justify-center hover:bg-[#eb6c6c] hover:text-white"
                onClick={card.action.onClick}
              >
                {card.action.label}
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Role-specific content sections */}

      {user?.isAdmin ? (
        <div className="grid gap-6 grid-cols-1 md:grid-cols-2">
          <Card className="flex flex-col">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">System Metrics</CardTitle>
              <CardDescription>Platform performance overview</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col flex-1">
              <div className="rounded-lg border bg-gray-100 dark:bg-gray-800 h-[200px] flex items-center justify-center flex-1">
                <BarChart3 className="h-12 w-12 text-gray-400" />
              </div>
              <Button
                variant="ghost"
                className="mt-4 w-full justify-center hover:bg-[#eb6c6c] hover:text-white"
                onClick={() => router.push("/metrics")}
              >
                View Detailed Metrics
              </Button>
            </CardContent>
          </Card>
          <Card className="flex flex-col">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">Pending Applications</CardTitle>
              <CardDescription>Merchant applications awaiting approval</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col flex-1">
              <div className="space-y-4 flex-1">
                <div className="border rounded-lg p-4">
                  <h3 className="font-medium text-black dark:text-white">Local Bakery</h3>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Applied: Apr 15, 2023</p>
                  <div className="flex justify-end mt-2">
                    <Button size="sm" className="bg-[#eb6c6c] hover:bg-[#d55c5c]">
                      Review
                    </Button>
                  </div>
                </div>
                <div className="border rounded-lg p-4">
                  <h3 className="font-medium text-black dark:text-white">Community Bookstore</h3>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Applied: Apr 14, 2023</p>
                  <div className="flex justify-end mt-2">
                    <Button size="sm" className="bg-[#eb6c6c] hover:bg-[#d55c5c]">
                      Review
                    </Button>
                  </div>
                </div>
              </div>
              <Button
                variant="ghost"
                className="mt-4 w-full justify-center hover:bg-[#eb6c6c] hover:text-white"
                onClick={() => router.push("/applications")}
              >
                View All Applications
              </Button>
            </CardContent>
          </Card>
        </div>
      ) : user?.isMerchant ? (
        <div className="grid gap-6 grid-cols-1 md:grid-cols-2">
          <Card className="flex flex-col">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">Recent Transactions</CardTitle>
              <CardDescription>Your latest SFLuv transactions</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col flex-1">
              <div className="space-y-4 flex-1">
                <div
                  className="border rounded-lg p-4 cursor-pointer hover:bg-secondary/50 transition-colors"
                  onClick={() => handleTransactionClick(0)}
                >
                  <div className="flex justify-between">
                    <h3 className="font-medium text-black dark:text-white">Customer Purchase</h3>
                    <span className="text-green-500 font-medium">+45 SFLuv</span>
                  </div>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Apr 15, 2023 - 2:30 PM</p>
                </div>
                <div
                  className="border rounded-lg p-4 cursor-pointer hover:bg-secondary/50 transition-colors"
                  onClick={() => handleTransactionClick(1)}
                >
                  <div className="flex justify-between">
                    <h3 className="font-medium text-black dark:text-white">Currency Unwrap</h3>
                    <span className="text-red-500 font-medium">-200 SFLuv</span>
                  </div>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Apr 12, 2023 - 10:15 AM</p>
                </div>
                <div
                  className="border rounded-lg p-4 cursor-pointer hover:bg-secondary/50 transition-colors"
                  onClick={() => handleTransactionClick(2)}
                >
                  <div className="flex justify-between">
                    <h3 className="font-medium text-black dark:text-white">Customer Purchase</h3>
                    <span className="text-green-500 font-medium">+30 SFLuv</span>
                  </div>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Apr 10, 2023 - 4:45 PM</p>
                </div>
              </div>
              <Button
                variant="ghost"
                className="mt-4 w-full justify-center hover:bg-[#eb6c6c] hover:text-white"
                onClick={() => router.push("/transactions")}
              >
                View All Transactions
              </Button>
            </CardContent>
          </Card>
          <Card className="flex flex-col">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">Transaction Analytics</CardTitle>
              <CardDescription>Overview of your business activity</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col flex-1">
              <div className="rounded-lg border bg-gray-100 dark:bg-gray-800 h-[200px] flex items-center justify-center flex-1">
                <BarChart3 className="h-12 w-12 text-gray-400" />
              </div>
              <Button
                variant="ghost"
                className="mt-4 w-full justify-center hover:bg-[#eb6c6c] hover:text-white"
                onClick={() => router.push("/transactions?tab=analytics")}
              >
                View Detailed Analytics
              </Button>
            </CardContent>
          </Card>
        </div>
      ) : (
        <div className="grid gap-6 grid-cols-1 md:grid-cols-2">
          <Card className="flex flex-col">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">Volunteer Opportunities</CardTitle>
              <CardDescription>Find ways to earn SFLuv by volunteering</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col flex-1">
              <div className="space-y-4 flex-1">
                <div className="border rounded-lg p-4">
                  <h3 className="font-medium text-black dark:text-white">Community Garden Help</h3>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Help maintain local community gardens</p>
                  <div className="flex justify-between items-center mt-2">
                    <span className="text-sm text-green-500">Earn: 50 SFLuv/hr</span>
                    <Button
                      size="sm"
                      className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                      onClick={() => router.push(`/opportunities?id=opp-1`)}
                    >
                      View Details
                    </Button>
                  </div>
                </div>
                <div className="border rounded-lg p-4">
                  <h3 className="font-medium text-black dark:text-white">Food Bank Volunteer</h3>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Help sort and distribute food</p>
                  <div className="flex justify-between items-center mt-2">
                    <span className="text-sm text-green-500">Earn: 45 SFLuv/hr</span>
                    <Button
                      size="sm"
                      className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                      onClick={() => router.push(`/opportunities?id=opp-1`)}
                    >
                      View Details
                    </Button>
                  </div>
                </div>
              </div>
              <Button
                variant="ghost"
                className="mt-4 w-full justify-center hover:bg-[#eb6c6c] hover:text-white"
                onClick={() => router.push("/opportunities")}
              >
                View All Opportunities
              </Button>
            </CardContent>
          </Card>
          <Card className="flex flex-col">
            <CardHeader>
              <CardTitle className="text-black dark:text-white">Merchant Map</CardTitle>
              <CardDescription>Find places to spend your SFLuv</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col flex-1">
              <div className="rounded-lg border bg-gray-100 dark:bg-gray-800 h-[200px] flex items-center justify-center flex-1">
                <Map className="h-12 w-12 text-gray-400" />
              </div>
              <Button
                variant="ghost"
                className="mt-4 w-full justify-center hover:bg-[#eb6c6c] hover:text-white"
                onClick={() => router.push("/map")}
              >
                Open Full Map
              </Button>
            </CardContent>
          </Card>
        </div>
      )}


      <TransactionModal transaction={selectedTransaction} isOpen={isModalOpen} onClose={handleCloseModal} />
    </div>
  )
}
