"use client"

import { useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Plus, Wallet, ArrowRight, CheckCircle2, RefreshCw } from "lucide-react"
import { useApp } from "@/context/AppProvider"
import { AppWallet } from "@/lib/wallets/wallets"
import { ConnectWalletModal, } from "@/components/wallets/connect-wallet-modal"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { NewWalletModal } from "@/components/wallets/new-wallet-modal"
import { UpdatePayPalAccountModal } from "@/components/wallets/add-paypal-account-modal"
import { getAddress, isAddress } from "viem"

export default function WalletsPage() {
  const router = useRouter()

  const {
    wallets,
    user,
    userLocations,
    error,
    status,
    walletsStatus,
    importWallet,
    addWallet,
    refreshWallets,
    updatePayPalAddress
  } = useApp()

  const [showEoas, setShowEoas] = useState<boolean>(false)
  const [addWalletModalOpen, setAddWalletModalOpen] = useState<boolean>(false)
  const [connectWalletModalOpen, setConnectWalletModalOpen] = useState(false)
  const [addPayPalModalOpen, setAddPayPalModalOpen] = useState<boolean>(false)
  const [unwrapEnabledByAddress, setUnwrapEnabledByAddress] = useState<Record<string, boolean>>({})

  const hasRedeemerWallet = useMemo(
    () =>
      wallets.some((wallet) => {
        if (!wallet.address) return false
        return unwrapEnabledByAddress[wallet.address.toLowerCase()] === true
      }),
    [wallets, unwrapEnabledByAddress]
  )
  const walletsErrorMessage = useMemo(() => {
    if (typeof error === "string" && error.trim() !== "") {
      return error
    }
    if (error instanceof Error && error.message.trim() !== "") {
      return error.message
    }
    return "We couldn't load your connected wallets right now."
  }, [error])

  const normalizedPrimaryWalletAddress = useMemo(() => {
    const current = (user?.primaryWalletAddress || "").trim()
    if (!current || !isAddress(current)) return ""
    return getAddress(current).toLowerCase()
  }, [user?.primaryWalletAddress])

  const visibleWallets = useMemo(
    () =>
      wallets.filter((wallet) => {
        if (wallet.isHidden) return false
        if (wallet.type === "eoa" && !showEoas) return false
        return true
      }),
    [showEoas, wallets]
  )


  useEffect(() => {
    if (!wallets.length) {
      refreshWallets()
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    const syncUnwrapStatuses = async () => {
      const results = await Promise.all(
        wallets.map(async (wallet) => {
          if (!wallet.address) return { address: "", enabled: false }
          const enabled = await wallet.hasRedeemerRole()
          return { address: wallet.address.toLowerCase(), enabled }
        })
      )

      if (cancelled) return

      const nextMap: Record<string, boolean> = {}
      for (const result of results) {
        if (!result.address) continue
        nextMap[result.address] = result.enabled
      }
      setUnwrapEnabledByAddress(nextMap)
    }

    syncUnwrapStatuses()

    return () => {
      cancelled = true
    }
  }, [wallets])

  const toggleAddPayPalModal = () => {
    setAddPayPalModalOpen(!addPayPalModalOpen)
  }

  const toggleAddWalletModal = () => {
    setAddWalletModalOpen(!addWalletModalOpen)
  }

  const getWalletDisplayName = (walletType: string) => {
    switch (walletType) {
      case "metamask":
        return "MetaMask"
      case "coinbase_wallet":
        return "Coinbase Wallet"
      case "wallet_connect":
        return "WalletConnect"
      case "rainbow":
        return "Rainbow"
      case "trust":
        return "Trust Wallet"
      default:
        return walletType.charAt(0).toUpperCase() + walletType.slice(1)
    }
  }

  const formatAddress = (address: string) => {
    if (address.length <= 12) return address
    return `${address.slice(0, 6)}...${address.slice(-4)}`
  }

  const handleSelectWallet = (address: string) => {
    router.push(`/wallets/${address}`)
  }

  const handleDisconnectWallet = (wallet: AppWallet) => {
    console.log("Disconnecting wallet:", wallet.address)
  }

  if (status === "loading" || walletsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]" />
      </div>
    )
  }

  return (
    <div className="mx-auto w-full max-w-4xl space-y-4 px-3 pb-6 pt-2 sm:space-y-6 sm:px-6">
      <ConnectWalletModal
        open={connectWalletModalOpen}
        onOpenChange={() => setConnectWalletModalOpen(!connectWalletModalOpen)}
        importWalletFunction={importWallet}
      />

      <NewWalletModal
        open={addWalletModalOpen}
        onOpenChange={toggleAddWalletModal}
        addWalletFunction={addWallet}
      />

      <UpdatePayPalAccountModal
        open={addPayPalModalOpen}
        onOpenChange={toggleAddPayPalModal}
        updatePayPalAddressFunction={updatePayPalAddress}
      />


      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-black dark:text-white sm:text-3xl">
            Connected Wallets
          </h1>
          <p className="mt-1 text-sm text-gray-600 dark:text-gray-400 sm:text-base">
            Manage wallets connected to your SFLuv account
          </p>
        </div>

        {(user?.isMerchant === true || user?.isAdmin === true) && hasRedeemerWallet && (
          <Button
            variant="outline"
            onClick={toggleAddPayPalModal}
            className="h-10 w-full whitespace-nowrap sm:w-auto"
          >
            <Wallet className="h-4 w-4" />
            Connect PayPal Account
          </Button>
        )}
      </div>

      <div className="flex items-center space-x-2 rounded-lg border bg-muted/25 px-3 py-2">
        <Checkbox
          id="show-eoas"
          checked={showEoas}
          onCheckedChange={() => setShowEoas(!showEoas)}
        />
        <Label
          htmlFor="show-eoas"
          className="text-sm text-black dark:text-white cursor-pointer"
        >
          Show EOA Accounts
        </Label>
      </div>

      <div className="space-y-3 sm:space-y-4">
        {wallets.length === 0 ? (
          <Card className="border-rose-200/70 bg-rose-50/70 dark:border-rose-900/40 dark:bg-rose-950/20">
            <CardContent className="px-4 py-8 sm:px-6 sm:py-10">
              <div className="mx-auto flex max-w-md flex-col items-center text-center">
                <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-rose-100 dark:bg-rose-900/40">
                  <Wallet className="h-6 w-6 text-rose-600 dark:text-rose-300" />
                </div>
                <h3 className="text-lg font-semibold text-black dark:text-white sm:text-xl">
                  Unable to load connected wallets
                </h3>
                <p className="mt-2 text-sm text-gray-700 dark:text-gray-300 sm:text-base">
                  {walletsErrorMessage}
                </p>
                <Button
                  variant="outline"
                  className="mt-5 bg-background/80"
                  onClick={() => void refreshWallets()}
                >
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Try Again
                </Button>
              </div>
            </CardContent>
          </Card>
        ) : visibleWallets.length === 0 ? (
          <Card className="border-border/70 bg-card/90 shadow-sm">
            <CardContent className="px-4 py-8 sm:px-6 sm:py-10">
              <div className="mx-auto flex max-w-md flex-col items-center text-center">
                <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                  <Wallet className="h-6 w-6 text-muted-foreground" />
                </div>
                <h3 className="text-lg font-semibold text-black dark:text-white sm:text-xl">
                  No wallets are currently visible
                </h3>
                <p className="mt-2 text-sm text-gray-700 dark:text-gray-300 sm:text-base">
                  Unhide wallets from your account settings to show them here.
                </p>
              </div>
            </CardContent>
          </Card>
        ) : (
          visibleWallets.map((wallet) => {
            const walletUnwrapEnabled = wallet.address
              ? unwrapEnabledByAddress[wallet.address.toLowerCase()] === true
              : false
            const isPrimaryWallet = wallet.address
              ? wallet.address.toLowerCase() === normalizedPrimaryWalletAddress
              : false

            return (
              <Card
                key={wallet.address}
                onClick={() => handleSelectWallet(wallet.address || "0x")}
                className="cursor-pointer overflow-hidden border border-border/70 bg-card/90 shadow-sm transition-all hover:shadow-md"
              >
                <CardContent className="p-3 sm:p-4">
                  <div className="flex flex-col justify-between gap-3 sm:gap-4 md:flex-row md:items-center">
                    <div className="flex items-center gap-3">
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="text-sm font-medium text-black dark:text-white sm:text-base">
                            {getWalletDisplayName(wallet.name)}
                          </h3>
                          {isPrimaryWallet && (
                            <Badge variant="outline" className="border-[#eb6c6c]/40 bg-[#eb6c6c]/10 text-[10px] text-[#eb6c6c] sm:text-xs">
                              Primary Wallet
                            </Badge>
                          )}
                        </div>

                        {wallet.address && (
                          <p className="mt-1 inline-flex rounded-md bg-muted/50 px-2 py-0.5 font-mono text-xs text-gray-600 dark:text-gray-400">
                            {formatAddress(wallet.address)}
                          </p>
                        )}
                        {walletUnwrapEnabled && (
                          <div className="mt-2">
                            <Badge variant="success" className="gap-1 text-[10px] sm:text-xs">
                              <CheckCircle2 className="h-3.5 w-3.5 mr-1" />
                              Unwrap Enabled
                            </Badge>
                          </div>
                        )}
                      </div>
                    </div>

                    <Button
                      onClick={() => handleSelectWallet(wallet.address || "0x")}
                      className="h-9 w-full bg-[#eb6c6c] text-sm hover:bg-[#d55c5c] sm:w-auto"
                    >
                      Open Wallet
                      <ArrowRight className="h-4 w-4 ml-2" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )
          })
        )}
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:gap-4">
        <Button
          className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c] sm:w-auto"
          onClick={toggleAddWalletModal}
        >
          <Plus className="h-4 w-4 mr-2" />
          New Wallet
        </Button>
      </div>

      <div className="text-xs text-gray-500 dark:text-gray-400 sm:text-sm">
        Showing {visibleWallets.length} connected {showEoas ? "" : "smart"} wallet
        {visibleWallets.length !==
        1
          ? "s"
          : ""}
      </div>
    </div>
  )
}
