"use client"

import type React from "react"

import { useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardFooter } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { Pagination } from "@/components/opportunities/pagination"
import { WalletDetailModal } from "@/components/wallets/wallet-detail-modal"
import { mockWallets } from "@/data/mock-wallets"
import { walletTypeLabels } from "@/types/wallet"
import { Search, Plus, ArrowLeft, Star, WalletIcon } from "lucide-react"
import Image from "next/image"
import type { Wallet } from "@/types/wallet"

export default function WalletsPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const walletId = searchParams.get("id")

  // State for wallet list
  const [wallets, setWallets] = useState<Wallet[]>(mockWallets)
  const [searchQuery, setSearchQuery] = useState("")
  const [typeFilter, setTypeFilter] = useState<string>("all")
  const [currentPage, setCurrentPage] = useState(1)
  const [selectedWallet, setSelectedWallet] = useState<Wallet | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)

  // State for QR code view
  const [showQrCode, setShowQrCode] = useState(false)
  const [isGeneratingQr, setIsGeneratingQr] = useState(false)
  const [qrCodeUrl, setQrCodeUrl] = useState<string | null>(null)

  // State for wallet name input
  const [showNameInput, setShowNameInput] = useState(false)
  const [newWalletName, setNewWalletName] = useState("")
  const [walletNameError, setWalletNameError] = useState("")

  // Pagination settings
  const ITEMS_PER_PAGE = 5

  // Find wallet by ID if provided in URL
  useState(() => {
    if (walletId) {
      const wallet = wallets.find((w) => w.id === walletId)
      if (wallet) {
        setSelectedWallet(wallet)
        setIsModalOpen(true)
      }
    }
  })

  // Filter wallets
  const filteredWallets = wallets.filter((wallet) => {
    // Filter by type
    if (typeFilter !== "all" && wallet.type !== typeFilter) {
      return false
    }

    // Filter by search query
    if (searchQuery) {
      const query = searchQuery.toLowerCase()
      return (
        wallet.name.toLowerCase().includes(query) ||
        wallet.address.toLowerCase().includes(query) ||
        walletTypeLabels[wallet.type].toLowerCase().includes(query)
      )
    }

    return true
  })

  // Calculate pagination
  const totalPages = Math.ceil(filteredWallets.length / ITEMS_PER_PAGE)
  const paginatedWallets = filteredWallets.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)

  // Handle wallet selection
  const handleSelectWallet = (wallet: Wallet) => {
    setSelectedWallet(wallet)
    setIsModalOpen(true)
    // Update URL with wallet ID
    router.push(`/dashboard/wallets?id=${wallet.id}`, { scroll: false })
  }

  // Handle modal close
  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedWallet(null)
    // Remove wallet ID from URL
    router.push("/dashboard/wallets", { scroll: false })
  }

  // Handle set default wallet
  const handleSetDefault = (walletId: string) => {
    setWallets(
      wallets.map((wallet) => ({
        ...wallet,
        isDefault: wallet.id === walletId,
      })),
    )
  }

  // Handle remove wallet
  const handleRemoveWallet = (walletId: string) => {
    setWallets(wallets.filter((wallet) => wallet.id !== walletId))
  }

  // Handle rename wallet
  const handleRenameWallet = (walletId: string, newName: string) => {
    setWallets(wallets.map((wallet) => (wallet.id === walletId ? { ...wallet, name: newName } : wallet)))
  }

  // Handle connect new wallet button click
  const handleConnectWallet = () => {
    setNewWalletName("")
    setWalletNameError("")
    setShowNameInput(true)
  }

  // Handle wallet name submission
  const handleWalletNameSubmit = (e: React.FormEvent) => {
    e.preventDefault()

    // Validate wallet name
    if (!newWalletName.trim()) {
      setWalletNameError("Please enter a wallet name")
      return
    }

    // Check if name already exists
    if (wallets.some((wallet) => wallet.name.toLowerCase() === newWalletName.trim().toLowerCase())) {
      setWalletNameError("A wallet with this name already exists")
      return
    }

    // Proceed to QR code generation
    setShowNameInput(false)
    setIsGeneratingQr(true)

    // Simulate API call to generate QR code
    setTimeout(() => {
      setQrCodeUrl("/placeholder.svg?height=300&width=300")
      setIsGeneratingQr(false)
      setShowQrCode(true)
    }, 1500)
  }

  // Handle back from name input
  const handleBackFromNameInput = () => {
    setShowNameInput(false)
  }

  // Handle back from QR code view
  const handleBackFromQrCode = () => {
    setShowQrCode(false)
    setQrCodeUrl(null)
  }

  // Format date
  const formatDate = (dateString: string | null) => {
    if (!dateString) return "Never"
    const date = new Date(dateString)
    return date.toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    })
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Connected Wallets</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Manage wallets connected to your SFLuv account</p>
      </div>

      {showNameInput ? (
        <Card>
          <CardHeader>
            <div className="flex items-center">
              <Button
                variant="ghost"
                size="sm"
                className="mr-2 h-8 w-8 p-0"
                onClick={handleBackFromNameInput}
                aria-label="Back to wallet list"
              >
                <ArrowLeft className="h-4 w-4" />
              </Button>
              <div>
                <CardTitle className="text-black dark:text-white">Connect New Wallet</CardTitle>
                <CardDescription>Name your wallet before connecting</CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleWalletNameSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="wallet-name" className="text-black dark:text-white">
                  Wallet Name
                </Label>
                <Input
                  id="wallet-name"
                  value={newWalletName}
                  onChange={(e) => {
                    setNewWalletName(e.target.value)
                    setWalletNameError("")
                  }}
                  placeholder="Enter a name for this wallet"
                  className="text-black dark:text-white bg-secondary"
                  maxLength={30}
                />
                {walletNameError && <p className="text-sm text-red-500">{walletNameError}</p>}
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  Choose a descriptive name to help you identify this wallet later
                </p>
              </div>
            </form>
          </CardContent>
          <CardFooter className="flex justify-end gap-2">
            <Button
              variant="outline"
              onClick={handleBackFromNameInput}
              className="text-black dark:text-white bg-secondary"
            >
              Cancel
            </Button>
            <Button
              className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
              onClick={handleWalletNameSubmit}
              disabled={!newWalletName.trim()}
            >
              Continue
            </Button>
          </CardFooter>
        </Card>
      ) : showQrCode ? (
        <Card>
          <CardContent className="p-6">
            <div className="flex items-center mb-6">
              <Button
                variant="ghost"
                size="sm"
                className="mr-2 h-8 w-8 p-0"
                onClick={handleBackFromQrCode}
                aria-label="Back to wallet list"
              >
                <ArrowLeft className="h-4 w-4" />
              </Button>
              <div>
                <h2 className="text-xl font-bold text-black dark:text-white">Connect "{newWalletName}"</h2>
                <p className="text-gray-600 dark:text-gray-400">Scan this QR code with your wallet app</p>
              </div>
            </div>

            <div className="flex flex-col items-center justify-center py-8">
              {qrCodeUrl ? (
                <div className="text-center">
                  <div className="bg-white p-4 rounded-lg inline-block mb-4">
                    <Image src={qrCodeUrl || "/placeholder.svg"} alt="QR Code" width={250} height={250} />
                  </div>
                  <div className="mt-6 max-w-md mx-auto">
                    <p className="text-sm text-gray-600 dark:text-gray-400">1. Open your wallet app</p>
                    <p className="text-sm text-gray-600 dark:text-gray-400 mt-2">
                      2. Scan this QR code to connect your wallet
                    </p>
                    <p className="text-sm text-gray-600 dark:text-gray-400 mt-2">
                      3. Approve the connection request in your wallet app
                    </p>
                    <p className="text-sm text-gray-600 dark:text-gray-400 mt-4">
                      This QR code will expire in 15 minutes for security reasons.
                    </p>
                  </div>
                </div>
              ) : (
                <div className="text-center py-12">
                  <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c] mb-4"></div>
                  <p className="text-gray-600 dark:text-gray-400">Generating QR code...</p>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
              <Input
                placeholder="Search wallets..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-10 text-black dark:text-white bg-secondary"
              />
            </div>

            <Select value={typeFilter} onValueChange={setTypeFilter}>
              <SelectTrigger className="w-[180px] text-black dark:text-white bg-secondary">
                <SelectValue placeholder="Filter by type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                <SelectItem value="metamask">MetaMask</SelectItem>
                <SelectItem value="coinbase">Coinbase</SelectItem>
                <SelectItem value="walletconnect">WalletConnect</SelectItem>
                <SelectItem value="trust">Trust Wallet</SelectItem>
                <SelectItem value="ledger">Ledger</SelectItem>
                <SelectItem value="trezor">Trezor</SelectItem>
                <SelectItem value="other">Other</SelectItem>
              </SelectContent>
            </Select>

            <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={handleConnectWallet} disabled={isGeneratingQr}>
              {isGeneratingQr ? (
                <>
                  <div className="animate-spin mr-2 h-4 w-4 border-2 border-white border-t-transparent rounded-full" />
                  Generating...
                </>
              ) : (
                <>
                  <Plus className="h-4 w-4 mr-2" />
                  Connect New Wallet
                </>
              )}
            </Button>
          </div>

          <div className="space-y-4">
            {paginatedWallets.length === 0 ? (
              <div className="text-center py-12">
                <WalletIcon className="h-12 w-12 text-gray-300 dark:text-gray-600 mx-auto mb-4" />
                <h3 className="text-xl font-medium text-black dark:text-white mb-2">No wallets found</h3>
                <p className="text-gray-600 dark:text-gray-400 max-w-md mx-auto mb-6">
                  {wallets.length === 0
                    ? "You don't have any wallets connected to your account yet."
                    : "Try adjusting your search or filters to find wallets."}
                </p>
                {wallets.length === 0 && (
                  <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={handleConnectWallet}>
                    <Plus className="h-4 w-4 mr-2" />
                    Connect Your First Wallet
                  </Button>
                )}
              </div>
            ) : (
              paginatedWallets.map((wallet) => (
                <Card
                  key={wallet.id}
                  className="overflow-hidden cursor-pointer hover:shadow-md transition-shadow"
                  onClick={() => handleSelectWallet(wallet)}
                >
                  <CardContent className="p-4">
                    <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                      <div className="flex items-center gap-3">
                        <div className="flex-shrink-0 w-10 h-10 bg-secondary rounded-full flex items-center justify-center">
                          <WalletIcon className="h-5 w-5 text-[#eb6c6c]" />
                        </div>
                        <div>
                          <div className="flex items-center">
                            <h3 className="font-medium text-black dark:text-white">{wallet.name}</h3>
                            {wallet.isDefault && (
                              <Badge className="ml-2 bg-[#eb6c6c] text-white">
                                <Star className="h-3 w-3 mr-1" />
                                Default
                              </Badge>
                            )}
                          </div>
                          <p className="text-sm text-gray-600 dark:text-gray-400 truncate max-w-[200px] md:max-w-[300px]">
                            {wallet.address}
                          </p>
                        </div>
                      </div>

                      <div className="flex flex-wrap items-center gap-2 md:text-right">
                        <Badge variant="outline" className="bg-secondary text-black dark:text-white">
                          {walletTypeLabels[wallet.type]}
                        </Badge>
                        <span className="font-bold text-black dark:text-white md:ml-4">{wallet.balance} SFLuv</span>
                      </div>
                    </div>

                    <div className="grid grid-cols-2 mt-4 text-sm text-gray-600 dark:text-gray-400">
                      <div>Added: {formatDate(wallet.dateAdded)}</div>
                      <div className="text-right">Last used: {formatDate(wallet.lastUsed)}</div>
                    </div>
                  </CardContent>
                </Card>
              ))
            )}
          </div>

          {totalPages > 1 && (
            <div className="mt-8">
              <Pagination currentPage={currentPage} totalPages={totalPages} onPageChange={setCurrentPage} />
            </div>
          )}

          <div className="text-sm text-gray-500 dark:text-gray-400">
            Showing {paginatedWallets.length} of {filteredWallets.length} wallets
          </div>
        </>
      )}

      <WalletDetailModal
        wallet={selectedWallet}
        isOpen={isModalOpen}
        onClose={handleCloseModal}
        onSetDefault={handleSetDefault}
        onRemove={handleRemoveWallet}
        onRename={handleRenameWallet}
      />
    </div>
  )
}
