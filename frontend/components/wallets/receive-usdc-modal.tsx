"use client"

import { useRef, useState } from "react"
import { SfluvQRCode, type SfluvQRCodeHandle } from "@/components/ui/sfluv-qr-code"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Copy, Check, Download } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { AppWallet } from "@/lib/wallets/wallets"
import { USDC, USDC_SYMBOL } from "./usdc-theme"

interface ReceiveUsdcModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
}

// Receiving USDC is just sharing the wallet address on Celo — same address that
// holds SFLUV. Kept intentionally simple and in the USDC-blue theme.
export function ReceiveUsdcModal({ open, onOpenChange, wallet }: ReceiveUsdcModalProps) {
  const { toast } = useToast()
  const qrRef = useRef<SfluvQRCodeHandle>(null)
  const [copied, setCopied] = useState(false)
  const address = wallet.address || "0x"

  const copyAddress = async () => {
    await navigator.clipboard.writeText(address)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
    toast({ title: "Address copied", description: "Wallet address copied to clipboard" })
  }

  const downloadQr = () => {
    void qrRef.current?.download("png", "SFLUV_" + wallet.name.replaceAll(" ", "_") + "_USDC_QR")
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[420px]">
        <DialogHeader>
          <DialogTitle className={USDC.accentText}>Receive {USDC_SYMBOL}</DialogTitle>
          <DialogDescription>
            Send USDC on Celo to this address. It shares the same address as your SFLUV wallet.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col items-center gap-4">
          <SfluvQRCode ref={qrRef} value={address} size={224} />

          <p className="w-full break-all rounded-md bg-muted/50 px-3 py-2 text-center font-mono text-xs text-muted-foreground">
            {address}
          </p>

          <div className="grid w-full grid-cols-2 gap-2">
            <Button variant="outline" className={USDC.buttonOutline} onClick={copyAddress}>
              {copied ? <Check className="mr-2 h-4 w-4" /> : <Copy className="mr-2 h-4 w-4" />}
              {copied ? "Copied" : "Copy"}
            </Button>
            <Button variant="outline" className={USDC.buttonOutline} onClick={downloadQr}>
              <Download className="mr-2 h-4 w-4" />
              Save QR
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
