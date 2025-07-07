"use client"

import { useState, useRef, useEffect } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Copy,
  ExternalLink,
  CheckCircle,
  Trash2,
  Star,
  Calendar,
  Clock,
  Download,
  QrCode,
  Pencil,
  Check,
  X,
} from "lucide-react"
import type { Wallet as WalletType } from "@/types/wallet"
import { walletTypeLabels } from "@/types/wallet"
import { generateProceduralQrData, addressToColor } from "@/utils/wallet-utils"
import { Input } from "@/components/ui/input"
import Image from "next/image"

interface WalletDetailModalProps {
  wallet: WalletType | null
  isOpen: boolean
  onClose: () => void
  onSetDefault: (walletId: string) => void
  onRemove: (walletId: string) => void
  onRename?: (walletId: string, newName: string) => void
}

export function WalletDetailModal({
  wallet,
  isOpen,
  onClose,
  onSetDefault,
  onRemove,
  onRename,
}: WalletDetailModalProps) {
  const [copied, setCopied] = useState<boolean>(false)
  const [isRemoving, setIsRemoving] = useState<boolean>(false)
  const [isSettingDefault, setIsSettingDefault] = useState<boolean>(false)
  const [qrCodeTab, setQrCodeTab] = useState<"citizen" | "external">("citizen")
  const [qrCodeUrl, setQrCodeUrl] = useState<string | null>(null)
  const [isGeneratingQr, setIsGeneratingQr] = useState<boolean>(false)
  const [isDownloading, setIsDownloading] = useState<boolean>(false)
  const [isEditingName, setIsEditingName] = useState<boolean>(false)
  const [newName, setNewName] = useState<string>("")
  const [isRenamingWallet, setIsRenamingWallet] = useState<boolean>(false)
  const qrCodeRef = useRef<HTMLDivElement>(null)
  const nameInputRef = useRef<HTMLInputElement>(null)

  // Set initial name when wallet changes
  useEffect(() => {
    if (wallet) {
      setNewName(wallet.name)
    }
  }, [wallet])

  // Focus input when editing starts
  useEffect(() => {
    if (isEditingName && nameInputRef.current) {
      nameInputRef.current.focus()
    }
  }, [isEditingName])

  // Generate QR code when tab changes or wallet changes
  useEffect(() => {
    if (wallet && isOpen) {
      generateQrCode()
    }
  }, [wallet, qrCodeTab, isOpen])

  // Handle QR code generation
  const generateQrCode = () => {
    if (!wallet) return

    setIsGeneratingQr(true)
    try {
      const qrData = qrCodeTab === "citizen" ? generateProceduralQrData(wallet.address) : wallet.address

      // In a real implementation, we would use QRCode.toDataURL to generate the QR code
      // For now, we'll use a placeholder with a delay to simulate generation
      setTimeout(() => {
        // Simulate QR code generation with different colors based on the wallet address
        const color = wallet ? addressToColor(wallet.address) : "#000000"
        setQrCodeUrl(
          `/placeholder.svg?height=200&width=200&text=${encodeURIComponent(qrData)}&color=${encodeURIComponent(color)}`,
        )
        setIsGeneratingQr(false)
      }, 500)
    } catch (error) {
      console.error("Error generating QR code:", error)
      setIsGeneratingQr(false)
    }
  }

  // All handler functions
  const handleCopy = () => {
    if (!wallet) return
    navigator.clipboard.writeText(wallet.address)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleSetDefault = () => {
    if (!wallet) return
    setIsSettingDefault(true)
    // Simulate API call
    setTimeout(() => {
      onSetDefault(wallet.id)
      setIsSettingDefault(false)
    }, 1000)
  }

  const handleRemove = () => {
    if (!wallet) return
    setIsRemoving(true)
    // Simulate API call
    setTimeout(() => {
      onRemove(wallet.id)
      setIsRemoving(false)
      onClose()
    }, 1000)
  }

  const handleQrCodeTabChange = (value: string) => {
    setQrCodeTab(value as "citizen" | "external")
  }

  const handleDownloadQrCode = () => {
    if (!qrCodeUrl || !wallet) return

    setIsDownloading(true)

    // In a real implementation, we would create a download link for the QR code
    // For now, we'll simulate a download with a delay
    setTimeout(() => {
      // Create a temporary link element
      const link = document.createElement("a")
      link.href = qrCodeUrl
      link.download = `${wallet.name.replace(/\s+/g, "-").toLowerCase()}-${qrCodeTab}-qrcode.png`
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)

      setIsDownloading(false)
    }, 1000)
  }

  const handleEditName = () => {
    setIsEditingName(true)
  }

  const handleCancelEdit = () => {
    setIsEditingName(false)
    if (wallet) {
      setNewName(wallet.name)
    }
  }

  const handleSaveName = () => {
    if (!wallet || !newName.trim()) return

    setIsRenamingWallet(true)

    // Simulate API call to rename wallet
    setTimeout(() => {
      if (onRename) {
        onRename(wallet.id, newName.trim())
      }
      setIsRenamingWallet(false)
      setIsEditingName(false)
    }, 1000)
  }

  const formatDate = (dateString: string | null) => {
    if (!dateString) return "Never"
    const date = new Date(dateString)
    return date.toLocaleDateString("en-US", {
      year: "numeric",
      month: "long",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    })
  }

  // If no wallet, render the dialog but with empty or placeholder content
  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
        {wallet ? (
          <>
            <DialogHeader>
              <div className="flex items-center justify-between">
                {isEditingName ? (
                  <div className="flex items-center gap-2 w-full">
                    <Input
                      ref={nameInputRef}
                      value={newName}
                      onChange={(e) => setNewName(e.target.value)}
                      className="text-xl font-semibold text-black dark:text-white"
                      placeholder="Wallet name"
                      maxLength={30}
                    />
                    <div className="flex gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={handleSaveName}
                        disabled={isRenamingWallet || !newName.trim()}
                        className="h-8 w-8 text-green-500 hover:text-green-600 hover:bg-green-100"
                      >
                        {isRenamingWallet ? (
                          <div className="animate-spin h-4 w-4 border-2 border-green-500 border-t-transparent rounded-full" />
                        ) : (
                          <Check className="h-4 w-4" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={handleCancelEdit}
                        disabled={isRenamingWallet}
                        className="h-8 w-8 text-red-500 hover:text-red-600 hover:bg-red-100"
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ) : (
                  <>
                    <DialogTitle className="text-2xl text-black dark:text-white">{wallet.name}</DialogTitle>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={handleEditName}
                      className="h-8 w-8 text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                  </>
                )}
              </div>
              <DialogDescription className="flex items-center gap-2">
                <Badge variant="outline" className="bg-secondary text-black dark:text-white">
                  {walletTypeLabels[wallet.type]}
                </Badge>
                {wallet.isDefault && (
                  <Badge className="bg-[#eb6c6c] text-white">
                    <Star className="h-3 w-3 mr-1" />
                    Default
                  </Badge>
                )}
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-6">
              <div className="space-y-4">
                {/* QR Code Section */}
                <div className="mt-6">
                  <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400 mb-2">Wallet QR Code</h4>

                  <Tabs defaultValue="citizen" value={qrCodeTab} onValueChange={handleQrCodeTabChange}>
                    <TabsList className="grid grid-cols-2 mb-4 bg-secondary">
                      <TabsTrigger value="citizen" className="text-black dark:text-white">
                        Citizen Wallet
                      </TabsTrigger>
                      <TabsTrigger value="external" className="text-black dark:text-white">
                        External Wallet
                      </TabsTrigger>
                    </TabsList>

                    <TabsContent value="citizen" className="flex flex-col items-center">
                      <div
                        ref={qrCodeRef}
                        className="bg-white p-4 rounded-lg mb-2 relative"
                        style={{
                          borderColor: addressToColor(wallet.address),
                          borderWidth: "2px",
                          borderStyle: "solid",
                        }}
                      >
                        {isGeneratingQr ? (
                          <div className="h-[200px] w-[200px] flex items-center justify-center">
                            <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-[#eb6c6c]"></div>
                          </div>
                        ) : (
                          qrCodeUrl && (
                            <div className="relative">
                              <Image
                                src={qrCodeUrl || "/placeholder.svg"}
                                alt="Citizen Wallet QR Code"
                                width={200}
                                height={200}
                              />
                              <div
                                className="absolute inset-0 flex items-center justify-center opacity-20"
                                style={{ color: addressToColor(wallet.address) }}
                              >
                                <QrCode className="h-16 w-16" />
                              </div>
                            </div>
                          )
                        )}
                      </div>
                      <p className="text-xs text-gray-500 dark:text-gray-400 text-center mb-4">
                        Scan with the Citizen Wallet app for enhanced features
                      </p>
                    </TabsContent>

                    <TabsContent value="external" className="flex flex-col items-center">
                      <div ref={qrCodeRef} className="bg-white p-4 rounded-lg mb-2">
                        {isGeneratingQr ? (
                          <div className="h-[200px] w-[200px] flex items-center justify-center">
                            <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-[#eb6c6c]"></div>
                          </div>
                        ) : (
                          qrCodeUrl && (
                            <Image
                              src={qrCodeUrl || "/placeholder.svg"}
                              alt="External Wallet QR Code"
                              width={200}
                              height={200}
                            />
                          )
                        )}
                      </div>
                      <p className="text-xs text-gray-500 dark:text-gray-400 text-center mb-4">
                        Standard QR code containing only the wallet address
                      </p>
                    </TabsContent>
                  </Tabs>

                  <Button
                    onClick={handleDownloadQrCode}
                    disabled={isDownloading || !qrCodeUrl}
                    className="w-full bg-secondary hover:bg-secondary/80 text-black dark:text-white"
                  >
                    {isDownloading ? (
                      <>
                        <div className="animate-spin mr-2 h-4 w-4 border-2 border-current border-t-transparent rounded-full" />
                        Downloading...
                      </>
                    ) : (
                      <>
                        <Download className="h-4 w-4 mr-2" />
                        Download QR Code
                      </>
                    )}
                  </Button>
                </div>

                <div>
                  <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">Wallet Address</h4>
                  <div className="flex items-center mt-1">
                    <code className="text-xs bg-secondary/50 p-1 rounded text-gray-600 dark:text-gray-300 flex-1 overflow-hidden text-ellipsis">
                      {wallet.address}
                    </code>
                    <Button variant="ghost" size="icon" className="h-6 w-6 ml-1" onClick={handleCopy}>
                      {copied ? <CheckCircle className="h-3 w-3 text-green-500" /> : <Copy className="h-3 w-3" />}
                    </Button>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">Date Added</h4>
                    <div className="flex items-center mt-1 text-black dark:text-white">
                      <Calendar className="h-4 w-4 mr-2 text-[#eb6c6c]" />
                      {formatDate(wallet.dateAdded)}
                    </div>
                  </div>

                  <div>
                    <h4 className="text-sm font-medium text-gray-500 dark:text-gray-400">Last Used</h4>
                    <div className="flex items-center mt-1 text-black dark:text-white">
                      <Clock className="h-4 w-4 mr-2 text-[#eb6c6c]" />
                      {formatDate(wallet.lastUsed)}
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <DialogFooter className="flex flex-col sm:flex-row gap-2 mt-6">
              <Button
                variant="outline"
                className="text-red-500 border-red-500 hover:bg-red-500 hover:text-white"
                onClick={handleRemove}
                disabled={isRemoving || wallet.isDefault}
              >
                {isRemoving ? (
                  <>
                    <div className="animate-spin mr-2 h-4 w-4 border-2 border-red-500 border-t-transparent rounded-full" />
                    Removing...
                  </>
                ) : (
                  <>
                    <Trash2 className="h-4 w-4 mr-2" />
                    Remove Wallet
                  </>
                )}
              </Button>

              {!wallet.isDefault && (
                <Button
                  variant="outline"
                  className="text-black dark:text-white bg-secondary hover:bg-secondary/80"
                  onClick={handleSetDefault}
                  disabled={isSettingDefault}
                >
                  {isSettingDefault ? (
                    <>
                      <div className="animate-spin mr-2 h-4 w-4 border-2 border-current border-t-transparent rounded-full" />
                      Setting as Default...
                    </>
                  ) : (
                    <>
                      <Star className="h-4 w-4 mr-2" />
                      Set as Default
                    </>
                  )}
                </Button>
              )}

              <Button
                className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                onClick={() => window.open(`https://explorer.sfluv.org/address/${wallet.address}`, "_blank")}
              >
                <ExternalLink className="h-4 w-4 mr-2" />
                View on Explorer
              </Button>
            </DialogFooter>
          </>
        ) : (
          <div className="py-8 text-center">
            <p className="text-gray-500 dark:text-gray-400">Loading wallet details...</p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
