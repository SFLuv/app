"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { AlertTriangle, Download, Loader2, Trash2 } from "lucide-react"
import { Event } from "@/types/event"
import { DeleteEventModal } from "./delete-event-modal"
import { QRCodeCard } from "./qr-code-card"
import { AffiliateQRCodeCard } from "./affiliate-qr-code-card"
import { useApp } from "@/context/AppProvider"
import generatePDF, { Margin, Resolution } from "react-to-pdf"

interface EventModalProps {
  open: boolean
  onOpenChange: () => void
  event: Event | undefined
  handleDeleteEvent: (id: string) => Promise<void>
  deleteEventError: unknown
  eventsBasePath?: string
  ownerLabel?: string
}

export function EventModal({
  open,
  onOpenChange,
  event,
  handleDeleteEvent,
  deleteEventError,
  eventsBasePath = "/events",
  ownerLabel,
}: EventModalProps) {
  if(!event) return

  const { authFetch, affiliate, user } = useApp()
  const exportTargetRef = useRef<HTMLDivElement | null>(null)

  const [deleteError, setDeleteError]= useState<string | null>()
  const [deleteModalOpen, setDeleteModalOpen] = useState<boolean>(false)

  const [codes, setCodes] = useState<string[]>([])
  const [codesError, setCodesError] = useState<string | undefined>()
  const [affiliateLogo, setAffiliateLogo] = useState<string | null>(null)
  const [affiliateOrganization, setAffiliateOrganization] = useState<string | null>(null)
  const [exportCodes, setExportCodes] = useState<string[]>([])
  const [downloadingPdf, setDownloadingPdf] = useState<boolean>(false)

  const maxCodesPerPdf = 30
  const eventCodesPageSize = 200
  const eventFilenameBase = useMemo(() => (
    event.title
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "event"
  ), [event.title])
  const codeBatches = useMemo(() => {
    const batches: string[][] = []
    for (let index = 0; index < codes.length; index += maxCodesPerPdf) {
      batches.push(codes.slice(index, index + maxCodesPerPdf))
    }
    return batches
  }, [codes])



  const getCodes = async () => {
    try {
      const loadedCodes: string[] = []
      for (let page = 0; ; page += 1) {
        const url = `${eventsBasePath}/${event.id}?page=${page}&count=${eventCodesPageSize}`
        const res = await authFetch(url)
        if (!res.ok) {
          if (res.status === 404 && page > 0) {
            break
          }
          throw new Error("error fetching event codes")
        }

        const pageCodes = await res.json() as Array<{ id: string }>
        loadedCodes.push(...pageCodes.map(({ id }) => id))

        if (pageCodes.length < eventCodesPageSize) {
          break
        }
      }

      setCodes(loadedCodes)

      setCodesError(undefined)
    }
    catch {
      setCodes([])
      setCodesError("Error fetching codes.")
    }

  }

  const getAffiliateLogo = async () => {
    if (!event?.owner) {
      setAffiliateLogo(null)
      setAffiliateOrganization(null)
      return
    }

    if (user?.id === event.owner) {
      setAffiliateLogo(affiliate?.affiliate_logo || null)
      setAffiliateOrganization(affiliate?.organization || null)
      return
    }

    try {
      const res = await authFetch(`/affiliates/${encodeURIComponent(event.owner)}`)
      if (!res.ok) {
        setAffiliateLogo(null)
        setAffiliateOrganization(null)
        return
      }
      const data = await res.json()
      setAffiliateLogo(data?.affiliate_logo || null)
      setAffiliateOrganization(data?.organization || null)
    } catch {
      setAffiliateLogo(null)
      setAffiliateOrganization(null)
    }
  }


  useEffect(() => {
    setDeleteError(null)
    setCodesError(undefined)
    setExportCodes([])
    if(open) {
      setAffiliateLogo(null)
      setAffiliateOrganization(null)
      getCodes()
      getAffiliateLogo()
    }
  }, [open])

  const toggleDeleteModal = () => {
    setDeleteModalOpen(!deleteModalOpen)
  }

  const handleDelete = async (id: string) => {
    await handleDeleteEvent(event.id)
    if(!deleteEventError) {
      onOpenChange()
    } else {
      setDeleteError("Encountered a server error while deleting event. Please try again later.")
    }
  }

  const waitForExportRender = async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    await new Promise<void>((resolve) => setTimeout(() => resolve(), 50))
  }

  const downloadCodeBatch = async (batchCodes: string[], batchIndex?: number) => {
    setExportCodes(batchCodes)
    await waitForExportRender()
    await generatePDF(() => exportTargetRef.current, {
      method: "save",
      filename: batchIndex === undefined
        ? `${eventFilenameBase}.pdf`
        : `${eventFilenameBase}-batch-${batchIndex + 1}.pdf`,
      resolution: Resolution.NORMAL,
      page: { margin: Margin.NONE, format: [55, 42.5] },
      canvas: {
        mimeType: "image/jpeg",
        qualityRatio: 0.95,
        useCORS: true,
      },
    })
  }

  const handleDownloadPdf = async (specificBatchIndex?: number) => {
    if (codes.length === 0) {
      setCodesError("Error fetching codes.")
      return
    }

    setDownloadingPdf(true)
    setCodesError(undefined)
    try {
      if (specificBatchIndex !== undefined) {
        const batch = codeBatches[specificBatchIndex]
        if (!batch?.length) {
          throw new Error("Unable to prepare this QR code batch.")
        }
        await downloadCodeBatch(batch, specificBatchIndex)
      } else if (codeBatches.length <= 1) {
        await downloadCodeBatch(codes)
      } else {
        for (let batchIndex = 0; batchIndex < codeBatches.length; batchIndex += 1) {
          await downloadCodeBatch(codeBatches[batchIndex], batchIndex)
        }
      }
    } catch {
      setCodesError(
        codeBatches.length > 1
          ? "Unable to generate one or more QR code PDF batches. Try downloading an individual batch."
          : "Unable to generate the QR code PDF right now. Please try again.",
      )
    } finally {
      setDownloadingPdf(false)
      setExportCodes([])
    }
  }


  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DeleteEventModal
        open={deleteModalOpen}
        onOpenChange={toggleDeleteModal}
        handleDeleteEvent={handleDeleteEvent}
        event={event}
        deleteEventError={deleteEventError}
      />
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">{event.title}</DialogTitle>
          <DialogDescription className="text-sm">
            {event.description}
          </DialogDescription>
        </DialogHeader>
          {/* Details */}
          <div className="m-auto space-y-3">

            <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
              <span className="text-muted-foreground text-sm">Codes: </span>
              <span className="font-mono text-sm text-left sm:text-right">
                {event.codes}
              </span>
            </div>

            {ownerLabel && (
              <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                <span className="text-muted-foreground text-sm">Owner: </span>
                <span className="font-mono text-sm break-all text-left sm:text-right">
                  {ownerLabel}
                </span>
              </div>
            )}

            <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
              <span className="text-muted-foreground text-sm">Amount: </span>
              <span className="font-mono text-sm text-left sm:text-right">
                {event.amount}
              </span>
            </div>

            <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
              <span className="text-muted-foreground text-sm">Start At: </span>
              <span className="font-mono text-sm text-left sm:text-right">
                {event.start_at
                  ? (new Date(event.start_at * 1000)).toLocaleDateString() + " " + (new Date(event.start_at * 1000)).toLocaleTimeString().split(" ")[0] + " " + (new Date(event.start_at * 1000)).toLocaleTimeString().split(" ")[1]
                  : "Immediate"}
              </span>
            </div>

            <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
              <span className="text-muted-foreground text-sm">Expiration: </span>
              <span className="font-mono text-sm text-left sm:text-right">
                {(new Date(event.expiration * 1000)).toLocaleDateString() + " " + (new Date(event.expiration * 1000)).toLocaleTimeString().split(" ")[0] + " " + (new Date(event.expiration * 1000)).toLocaleTimeString().split(" ")[1]}
              </span>
            </div>

            {/* Download Button */}
            <div className="pt-2 text-center">
              <div
                aria-hidden="true"
                className="pointer-events-none fixed left-[-10000px] top-0 z-[-1] opacity-0"
              >
                <div ref={exportTargetRef} style={{ width: "425px", padding: 0 }}>
                  {exportCodes.map((code) => (
                    affiliateLogo
                      ? <AffiliateQRCodeCard
                          key={code}
                          code={code}
                          logoUrl={affiliateLogo}
                          organization={affiliateOrganization || "our partner"}
                        />
                      : <QRCodeCard key={code} code={code} />
                  ))}
                </div>
              </div>
              <div className="space-y-3">
                <Button
                  type="button"
                  className="w-full sm:w-auto"
                  onClick={() => void handleDownloadPdf()}
                  disabled={downloadingPdf}
                >
                  {downloadingPdf ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Preparing PDF{codeBatches.length > 1 ? "s" : ""}
                    </>
                  ) : (
                    <>
                      <Download className="mr-2 h-4 w-4" />
                      {codeBatches.length > 1 ? `Download All QR Code PDFs (${codeBatches.length})` : "Download QR Codes"}
                    </>
                  )}
                </Button>
                {codeBatches.length > 1 && (
                  <div className="space-y-2">
                    <p className="text-xs text-muted-foreground">
                      Large events are split into batches of up to {maxCodesPerPdf} QR codes per PDF to avoid blank downloads.
                    </p>
                    <div className="flex flex-wrap justify-center gap-2">
                      {codeBatches.map((batch, batchIndex) => (
                        <Button
                          key={`batch-${batchIndex}`}
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={downloadingPdf}
                          onClick={() => void handleDownloadPdf(batchIndex)}
                        >
                          Batch {batchIndex + 1} ({batch.length})
                        </Button>
                      ))}
                    </div>
                  </div>
                )}
              </div>
              {codesError && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 mt-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{codesError}</span>
                </div>
              )}
            </div>

            {/* Delete Button */}
            <div className="pt-2 text-center">
              <Button
                type="button"
                className="w-full bg-secondary hover:bg-[#333333] sm:w-auto"
                onClick={toggleDeleteModal}
              >
                <Trash2 color="red" />
              </Button>
            </div>

            {deleteError && (
              <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                <span>{deleteError}</span>
              </div>
            )}

          </div>
      </DialogContent>
    </Dialog>
  )
}
