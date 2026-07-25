"use client"

import { useEffect, useMemo } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { ArrowDownToLine, Landmark, Wallet } from "lucide-react"

// Cash out now lives on each wallet's page (withdrawTo → USDC to the
// location's Bridge liquidation address). This page routes merchants there.
export default function UnwrapPage() {
  const router = useRouter()
  const { user, wallets, walletsStatus } = useApp()

  useEffect(() => {
    if (user && user.isMerchant !== true && user.isAdmin !== true) {
      router.push("/dashboard")
    }
  }, [user, router])

  const cashOutWallets = useMemo(
    () => wallets.filter((wallet) => wallet.type === "smartwallet" && !wallet.isHidden),
    [wallets],
  )

  return (
    <div className="mx-auto w-full max-w-lg space-y-4 p-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Landmark className="h-5 w-5" />
            Cash Out to Bank
          </CardTitle>
          <CardDescription>
            Cashing out unwraps your SFLUV into USDC and sends it to your saved Bridge liquidation
            address, which settles to your bank account. Choose a wallet to get started — the cash
            out form is on the wallet page.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {walletsStatus !== "available" ? (
            <p className="text-sm text-muted-foreground">Loading wallets…</p>
          ) : cashOutWallets.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No smart wallets available. Connect a wallet first.
            </p>
          ) : (
            cashOutWallets.map((wallet) => (
              <Button
                key={wallet.address}
                variant="outline"
                className="w-full justify-between"
                asChild
              >
                <Link href={`/wallets/${wallet.address}`}>
                  <span className="flex items-center gap-2">
                    <Wallet className="h-4 w-4" />
                    {wallet.name}
                  </span>
                  <ArrowDownToLine className="h-4 w-4" />
                </Link>
              </Button>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}
