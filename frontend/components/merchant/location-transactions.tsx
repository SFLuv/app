"use client"

import { useCallback, useEffect, useState } from "react"
import { formatUnits } from "viem"
import { ArrowDownLeft, ArrowUpRight, Loader2, RefreshCw, Undo2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useApp } from "@/context/AppProvider"
import { useChainConfig } from "@/context/ChainConfigProvider"
import { RefundModal } from "@/components/merchant/refund-modal"
import type { AppWallet } from "@/lib/wallets/wallets"
import type { LocationTransaction, LocationTransactionsResponse } from "@/types/location-transactions"

type LocationTransactionsProps = {
  locationId: number
  locationName?: string
  tillWallet: AppWallet | null
}

const PAGE_SIZE = 25

export function LocationTransactions({ locationId, locationName, tillWallet }: LocationTransactionsProps) {
  const { authFetch } = useApp()
  const chainConfig = useChainConfig()

  const [rows, setRows] = useState<LocationTransaction[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(true)
  const [refundTarget, setRefundTarget] = useState<LocationTransaction | null>(null)

  const load = useCallback(
    async (nextPage: number) => {
      setLoading(true)
      try {
        const res = await authFetch(`/locations/${locationId}/transactions?page=${nextPage}&count=${PAGE_SIZE}`)
        if (!res.ok) throw new Error(`status ${res.status}`)
        const body = (await res.json()) as LocationTransactionsResponse
        setRows(body.transactions ?? [])
        setTotal(body.total ?? 0)
        setPage(body.page ?? nextPage)
      } catch (error) {
        console.error("location transactions", error)
      } finally {
        setLoading(false)
      }
    },
    [authFetch, locationId],
  )

  useEffect(() => {
    void load(0)
  }, [load])

  const fmt = (base: string) => {
    try {
      return `${Number(formatUnits(BigInt(base), chainConfig.tokenDecimals)).toLocaleString(undefined, {
        maximumFractionDigits: 2,
      })} ${chainConfig.tokenSymbol}`
    } catch {
      return base
    }
  }

  const lastPage = Math.max(0, Math.ceil(total / PAGE_SIZE) - 1)

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
        <div>
          <CardTitle className="text-base">Transactions</CardTitle>
          <CardDescription>
            Money in and out of {locationName || "this location"}, newest first.
          </CardDescription>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={() => void load(page)} disabled={loading}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
        </Button>
      </CardHeader>
      <CardContent className="space-y-2">
        {loading && rows.length === 0 && <p className="py-6 text-center text-sm text-muted-foreground">Loading…</p>}
        {!loading && rows.length === 0 && (
          <p className="py-6 text-center text-sm text-muted-foreground">
            No transactions yet. Payments to this location will appear here.
          </p>
        )}

        {rows.map((row) => {
          const incoming = row.direction === "in"
          // Offered only where a refund is possible: money that came into the
          // till, with something still left on it. A fully refunded payment
          // shows its mark and no button.
          const canRefund =
            incoming &&
            row.wallet === "payment" &&
            row.refund.status !== "refunded" &&
            (row.refund.remaining_base ?? "0") !== "0" &&
            Boolean(tillWallet)

          return (
            <div
              key={row.hash}
              className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3 text-sm"
            >
              <div className="flex min-w-0 items-center gap-3">
                <div className={`rounded-full p-2 ${incoming ? "bg-green-100 dark:bg-green-950" : "bg-muted"}`}>
                  {incoming ? (
                    <ArrowDownLeft className="h-4 w-4 text-green-700 dark:text-green-400" />
                  ) : (
                    <ArrowUpRight className="h-4 w-4 text-muted-foreground" />
                  )}
                </div>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">
                      {incoming ? "+" : "−"}
                      {fmt(row.amount_base)}
                    </span>
                    {row.wallet === "tipping" && <Badge variant="outline">Tip</Badge>}
                    <RefundBadge row={row} />
                  </div>
                  <p className="truncate text-xs text-muted-foreground">
                    {new Date(row.timestamp * 1000).toLocaleString()}
                    {row.refund.refunds_original_hash && (
                      <> · refund of {row.refund.refunds_original_hash.slice(0, 10)}…</>
                    )}
                  </p>
                </div>
              </div>

              {canRefund && (
                <Button type="button" variant="outline" size="sm" onClick={() => setRefundTarget(row)}>
                  <Undo2 className="mr-2 h-4 w-4" />
                  Issue refund
                </Button>
              )}
            </div>
          )
        })}

        {total > PAGE_SIZE && (
          <div className="flex items-center justify-between pt-2 text-sm">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page === 0 || loading}
              onClick={() => void load(page - 1)}
            >
              Previous
            </Button>
            <span className="text-muted-foreground">
              Page {page + 1} of {lastPage + 1}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page >= lastPage || loading}
              onClick={() => void load(page + 1)}
            >
              Next
            </Button>
          </div>
        )}
      </CardContent>

      <RefundModal
        open={Boolean(refundTarget)}
        onOpenChange={(open) => !open && setRefundTarget(null)}
        locationId={locationId}
        transaction={refundTarget}
        tillWallet={tillWallet}
        onRefunded={() => load(page)}
      />
    </Card>
  )
}

function RefundBadge({ row }: { row: LocationTransaction }) {
  switch (row.refund.status) {
    case "refunded":
      return <Badge variant="secondary">Refunded</Badge>
    case "partially_refunded":
      return <Badge variant="secondary">Partially refunded</Badge>
    case "refund":
      return <Badge variant="outline">Refund</Badge>
    default:
      return null
  }
}
