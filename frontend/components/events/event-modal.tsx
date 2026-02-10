"use client"

import { Dispatch, SetStateAction, useEffect, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { AlertTriangle, Container, Divide, Trash2 } from "lucide-react"
import { Event } from "@/types/event"
import { DeleteEventModal } from "./delete-event-modal"
import { QRCodeCard } from "./qr-code-card"
import { useApp } from "@/context/AppProvider"
import { Margin, Options, usePDF } from "react-to-pdf";
import ReactPDF from '@react-pdf/renderer';

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

  const { authFetch } = useApp()

  const [deleteError, setDeleteError]= useState<string | null>()
  const [deleteModalOpen, setDeleteModalOpen] = useState<boolean>(false)

  const [codes, setCodes] = useState<string[]>([])
  const [codesError, setCodesError] = useState<string | undefined>()

  const { toPDF, targetRef } = usePDF({
    method: "save",
    filename: event.title.toLowerCase().split(" ").join("-") + ".pdf",
    page: { margin: Margin.NONE, format: [55, 42.5] },
  });



  const getCodes = async () => {
    const url = `${eventsBasePath}/${event.id}`
    try {
      const res = await authFetch(url)
      const codes = await res.json()
      setCodes(codes.map(({ id }: { id: string }) => id))

      setCodesError(undefined)
    }
    catch {
      setCodes([])
    }

  }


  useEffect(() => {
    setDeleteError(null)
    setCodesError(undefined)
    if(open) {
      getCodes()
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


  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DeleteEventModal
        open={deleteModalOpen}
        onOpenChange={toggleDeleteModal}
        handleDeleteEvent={handleDeleteEvent}
        event={event}
        deleteEventError={deleteEventError}
      />
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-none">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">{event.title}</DialogTitle>
          <DialogDescription className="text-sm">
            {event.description}
          </DialogDescription>
        </DialogHeader>
          {/* Details */}
          <div className="space-y-2 m-auto">

            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground text-sm">Codes: </span>
              <span className="font-mono text-sm">
                {event.codes}
              </span>
            </div>

            {ownerLabel && (
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground text-sm">Owner: </span>
                <span className="font-mono text-sm">
                  {ownerLabel}
                </span>
              </div>
            )}

            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground text-sm">Amount: </span>
              <span className="font-mono text-sm">
                {event.amount}
              </span>
            </div>

            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground text-sm">Expiration: </span>
              <span className="font-mono text-sm">
                {(new Date(event.expiration * 1000)).toLocaleDateString() + " " + (new Date(event.expiration * 1000)).toLocaleTimeString().split(" ")[0] + " " + (new Date(event.expiration * 1000)).toLocaleTimeString().split(" ")[1]}
              </span>
            </div>

            {/* Download Button */}
            <div className="pt-2 text-center">
              <div style={{top: 100000, position: "relative"}}>
                <div ref={targetRef} style={{zIndex: -1, position: "fixed", width: "425px", padding: 0}}>
                  {codes.map((code) => <QRCodeCard key={code} code={code} />)}
                </div>
              </div>
              <Button
                type="button"
                onClick={() => {
                  if(codes.length == 0) {
                    setCodesError("Error fetching codes.")
                    return
                  }
                  toPDF()
                }}
              >
                Download QR Codes
              </Button>
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
                className="bg-secondary hover:bg-[#333333]"
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
