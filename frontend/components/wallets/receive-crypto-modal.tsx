"use client"

import { useMemo, useRef, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { QRCode } from "react-qrcode-logo"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent } from "@/components/ui/card"
import { Copy, CheckCircle, ChevronLeft, ChevronDown, ChevronRight, AlertTriangle } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import type { ConnectedWallet } from "@/types/privy-wallet"
import { AppWallet } from "@/lib/wallets/wallets"
import { CHAIN, COMMUNITY, CW_APP_BASE_URL, SYMBOL } from "@/lib/constants"
import { TabsTrigger, Tabs, TabsList } from "../ui/tabs"
import { Address } from "viem"
import { generateReceiveLink } from "@citizenwallet/sdk"
import config from "@/app.config"
import { Collapsible, CollapsibleTrigger } from "../ui/collapsible"
import { CollapsibleContent } from "@radix-ui/react-collapsible"
import ContactOrAddressInput from "../contacts/contact-or-address-input"

interface ReceiveCryptoModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
}

export function ReceiveCryptoModal({ open, onOpenChange, wallet }: ReceiveCryptoModalProps) {
  const [amount, setAmount] = useState("")
  const [memo, setMemo] = useState("")
  const [error, setError] = useState("")
  const [copied, setCopied] = useState(false)
  const [activeTab, setActiveTab] = useState<string>("cw")
  const [tipAddress, setTipAddress] = useState<string>("")
  const [moreOptions, setMoreOptions] = useState<boolean>(false)
  const { toast } = useToast()

  const QRRef = useRef<QRCode>(null)

  const handleDownload = () => {
    const tipEnabled = tipAddress !== "" && tipAddress.startsWith("0x") && tipAddress.length === 42 && activeTab === "cw"

    const qrName = "SFLuv_"
      + wallet.name.replaceAll(" ", "_")
      + (tipEnabled ? "_TIP_TO_" + tipAddress + "_" : "")
      + "_QR"

    QRRef.current?.download("png", qrName)
  }

  const copyReceive = async () => {
    try {
      await navigator.clipboard.writeText((activeTab === "cw" ? cwLinkValue : wallet.address) || "0x")
      setCopied(true)
      toast({
        title: "Address Copied",
        description: "Wallet address has been copied to clipboard",
      })
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      toast({
        title: "Copy Failed",
        description: "Failed to copy address to clipboard",
        variant: "destructive",
      })
    }
  }

  const cwLinkValue = useMemo(() => {
    const link = generateReceiveLink(
      CW_APP_BASE_URL,
      COMMUNITY,
      wallet.address as Address,
      undefined,
      memo
    )
    if(tipAddress === "") {
      setError("")
      return link
    }
    if (!tipAddress.startsWith("0x") || tipAddress.length !== 42) {
      setError("Please enter a valid tip address")
      return link
    }

    return link + "&tipTo=" + tipAddress
  }, [tipAddress])

  const generatePaymentRequest = () => {
    const params = new URLSearchParams()
    if (amount) params.append("amount", amount)
    if (memo) params.append("message", memo)

    const currencySymbol = SYMBOL
    const paymentUrl = `${currencySymbol.toLowerCase()}:${wallet.address}${params.toString() ? `?${params.toString()}` : ""}`

    toast({
      title: "Payment Request Generated",
      description: "Share this address or QR code to receive payments",
    })

    return paymentUrl
  }

  const currencySymbol = SYMBOL
  const networkName = CHAIN.name

  return (
    <Dialog open={open} onOpenChange={(open) => {
      setMoreOptions(false)
      onOpenChange(open)
    }}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">Receive Cryptocurrency</DialogTitle>
          <DialogDescription className="text-sm">
            Share your wallet address to receive {currencySymbol} on {networkName}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 sm:space-y-6">
          {/* QR Code Section */}
          <Card>
            <CardContent className="p-4 sm:p-6">
              <div className="text-center space-y-3 sm:space-y-4">
                <div className=" mx-auto bg-muted rounded-lg flex items-center justify-center">
                  <div className="text-center">
                    <QRCode
                      ref={QRRef}
                      value={activeTab === "cw" ? cwLinkValue : wallet.address}
                      style={{
                        // display: "flex",
                        height: "30vh",
                        width: "30vh",
                        borderRadius: "10px"
                      }}
                      logoImage={"/icon.png"}
                      removeQrCodeBehindLogo={true}
                      logoPadding={1}
                      logoPaddingStyle="circle"
                      logoWidth={40}
                      qrStyle="dots"
                      eyeRadius={6}
                      eyeColor={"#eb6c6c"}
                      ecLevel="H"
                      quietZone={5}
                    />
                    {/* <p className="text-sm text-muted-foreground">QR Code</p>
                    <p className="text-xs text-muted-foreground">{wallet?.address?.slice(0, 8) || "0x"}...</p> */}
                  </div>
                </div>
                <p className="text-sm text-muted-foreground">
                  Scan this QR code to send {currencySymbol} to this wallet
                </p>
              </div>
            </CardContent>
          </Card>

          {/* Choose CitizenWallet or External QR*/}
          <Tabs defaultValue="cw" value={activeTab} onValueChange={setActiveTab} className="w-full">
            <TabsList className="grid grid-cols-2 w-full mb-6 bg-secondary">
              <TabsTrigger value="cw" className="text-black dark:text-white">
                Citizen Wallet
              </TabsTrigger>
              <TabsTrigger value="external" className="text-black dark:text-white">
                External Wallet
              </TabsTrigger>
            </TabsList>
          </Tabs>

          {/* Details */}
          <div className="space-y-2">

            {/* Wallet Address */}
            <Label className="text-sm font-medium">Wallet {activeTab === "cw" ? "Link" : "Address"}</Label>
            <div className="flex gap-2">
              <Input value={activeTab === "cw" ? cwLinkValue : wallet.address} readOnly className="font-mono text-xs sm:text-sm h-11" />
              <Button
                variant="outline"
                size="sm"
                onClick={copyReceive}
                className="px-3 bg-transparent h-11 flex-shrink-0"
              >
                {copied ? <CheckCircle className="h-4 w-4 text-green-600" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>

            {/* More Options */}
            <div className="space-y-2">
              <Collapsible
                open={moreOptions}
                onOpenChange={() => {
                  setMoreOptions(!moreOptions)
                  setTipAddress("")
                }}
                className="mx-auto text-center"
              >
                <CollapsibleTrigger>
                  <div className="flex items-center gap-0.5 lg:gap-0.2 text-xs text-muted-foreground">
                    More Options {moreOptions
                      ? <ChevronDown className="h-3 w-3 sm:h-3.5 sm:w-3.5 p-0 text-muted-foreground" />
                      : <ChevronRight className="h-3 w-3 sm:h-3.5 sm:w-3.5 p-0 text-muted-foreground" />
                    }
                  </div>
                </CollapsibleTrigger>
                <CollapsibleContent
                  style={{
                    transition: "height 300ms ease-in-out"
                  }}
                  className="space-y-2 text-left"
                >
                  {/* TipTo Address */}
                  {activeTab === "cw" && <>
                    <Label className="text-sm font-medium">Tip To</Label>
                    <div className="flex gap-2">
                      <ContactOrAddressInput
                        className="font-mono text-xs sm:text-sm h-11"
                        onChange={(value) => {
                          setTipAddress(value)
                        }}
                        id="tip-address"
                      />
                    </div>
                    {error && (
                      <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                        <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                        <span>{error}</span>
                      </div>
                    )}
                  </>}
                  {/* Download QR */}
                  <div className="pt-2 text-center">
                    <Button onClick={handleDownload}>
                      Download QR Code
                    </Button>
                  </div>
                </CollapsibleContent>
              </Collapsible>
            </div>

          </div>

        </div>
      </DialogContent>
    </Dialog>
  )
}
