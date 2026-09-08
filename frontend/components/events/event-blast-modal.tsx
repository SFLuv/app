"use client"

import { useEffect, useRef, useState } from "react"
import { Bold, Image as ImageIcon, Italic, Link2, Loader2, Send, Users, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { useApp } from "@/context/AppProvider"
import { useToast } from "@/hooks/use-toast"

const MAX_IMAGE_BYTES = 8 * 1024 * 1024

interface BlastImage {
  id: string
  url: string
  name: string
}

interface EventBlastModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** "/admin/volunteer-events" or "/affiliates/volunteer-events". */
  basePath: string
  eventId: string
  eventTitle: string
  signupCount: number
}

/**
 * Composer for an organizer's message to their volunteers.
 *
 * The preview is rendered by the SAME server template that sends the email,
 * rather than reproduced here — a client-side approximation drifts from the
 * real thing, and the entire value of a preview is that it is truthful.
 */
export function EventBlastModal({
  open,
  onOpenChange,
  basePath,
  eventId,
  eventTitle,
  signupCount,
}: EventBlastModalProps) {
  const { authFetch } = useApp()
  const { toast } = useToast()
  const [subject, setSubject] = useState("")
  const [message, setMessage] = useState("")
  const [images, setImages] = useState<BlastImage[]>([])
  const [previewHtml, setPreviewHtml] = useState("")
  const [recipients, setRecipients] = useState<number | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [sending, setSending] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState("")
  const messageRef = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) return
    setSubject("")
    setMessage("")
    setImages([])
    setPreviewHtml("")
    setRecipients(null)
    setError("")
  }, [open])

  // Wrap the selection in a formatting marker, keeping the caret sensible so
  // the toolbar feels like an editor rather than a text-appender.
  const wrapSelection = (before: string, after: string, placeholder: string) => {
    const field = messageRef.current
    if (!field) return
    const start = field.selectionStart ?? message.length
    const end = field.selectionEnd ?? message.length
    const selected = message.slice(start, end) || placeholder
    const next = message.slice(0, start) + before + selected + after + message.slice(end)
    setMessage(next)
    requestAnimationFrame(() => {
      field.focus()
      field.setSelectionRange(start + before.length, start + before.length + selected.length)
    })
  }

  const uploadImage = async (file: File) => {
    if (file.size > MAX_IMAGE_BYTES) {
      setError(`"${file.name}" is larger than 8 MB.`)
      return
    }
    setUploading(true)
    setError("")
    try {
      const form = new FormData()
      form.append("image", file)
      const res = await authFetch(`${basePath}/${eventId}/blast/images`, { method: "POST", body: form })
      if (!res.ok) throw new Error((await res.text()).trim() || "Could not upload the image.")
      const uploaded = await res.json()
      setImages((current) => [...current, { id: uploaded.id, url: uploaded.url, name: file.name }])
      setPreviewHtml("")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not upload the image.")
    } finally {
      setUploading(false)
    }
  }

  const buildPreview = async () => {
    if (subject.trim() === "" || message.trim() === "") {
      setError("Add a subject and a message first.")
      return
    }
    setPreviewing(true)
    setError("")
    try {
      const res = await authFetch(`${basePath}/${eventId}/blast/preview`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: subject.trim(),
          message: message.trim(),
          image_ids: images.map((image) => image.id),
        }),
      })
      if (!res.ok) throw new Error("Could not build the preview.")
      const data = await res.json()
      setPreviewHtml(data.html || "")
      setRecipients(typeof data.recipients === "number" ? data.recipients : null)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not build the preview.")
    } finally {
      setPreviewing(false)
    }
  }

  const send = async () => {
    setSending(true)
    setError("")
    try {
      const res = await authFetch(`${basePath}/${eventId}/blast`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: subject.trim(),
          message: message.trim(),
          image_ids: images.map((image) => image.id),
        }),
      })
      if (!res.ok) throw new Error((await res.text()).trim() || "Could not send the message.")
      const result = await res.json()
      toast({
        title: "Message sent",
        description: `${result.pushed} push notification${result.pushed === 1 ? "" : "s"}, ${result.emailed} email${result.emailed === 1 ? "" : "s"}.`,
      })
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not send the message.")
    } finally {
      setSending(false)
    }
  }

  const canSend = subject.trim() !== "" && message.trim() !== "" && !sending && !uploading

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90vh] flex-col overflow-hidden sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Message volunteers</DialogTitle>
          <DialogDescription className="flex items-center gap-1.5">
            <Users className="h-3.5 w-3.5" />
            {eventTitle} · {signupCount} signed up. Volunteers with the app get a push notification; everyone
            else gets an email.
          </DialogDescription>
        </DialogHeader>

        <div className="grid min-h-0 flex-1 gap-6 overflow-y-auto lg:grid-cols-2">
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="blast-subject">Subject</Label>
              <Input
                id="blast-subject"
                value={subject}
                onChange={(event) => {
                  setSubject(event.target.value)
                  setPreviewHtml("")
                }}
                placeholder="Parking update for Saturday"
                maxLength={120}
              />
            </div>

            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <Label htmlFor="blast-message">Message</Label>
                <div className="flex items-center gap-1">
                  <Button type="button" variant="ghost" size="icon" className="h-7 w-7"
                    onClick={() => wrapSelection("**", "**", "bold text")} title="Bold">
                    <Bold className="h-3.5 w-3.5" />
                  </Button>
                  <Button type="button" variant="ghost" size="icon" className="h-7 w-7"
                    onClick={() => wrapSelection("_", "_", "italic text")} title="Italic">
                    <Italic className="h-3.5 w-3.5" />
                  </Button>
                  <Button type="button" variant="ghost" size="icon" className="h-7 w-7"
                    onClick={() => wrapSelection("[", "](https://)", "link text")} title="Link">
                    <Link2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
              <Textarea
                id="blast-message"
                ref={messageRef}
                value={message}
                onChange={(event) => {
                  setMessage(event.target.value)
                  setPreviewHtml("")
                }}
                rows={10}
                placeholder={"Hi everyone,\n\nThe car park on 5th is closed, so please use the Great Highway entrance instead.\n\nSee you Saturday!"}
                maxLength={4000}
              />
              <p className="text-xs text-muted-foreground">
                <strong>**bold**</strong>, <em>_italic_</em>, and [link](https://…) are supported. Push
                notifications show plain text.
              </p>
            </div>

            <div className="space-y-2">
              <Label>Attachments</Label>
              <div className="flex flex-wrap items-center gap-2">
                {images.map((image) => (
                  <div key={image.id} className="relative">
                    {/* eslint-disable-next-line @next/next/no-img-element -- API host, arbitrary ratios */}
                    <img src={image.url} alt={image.name} className="h-16 w-24 rounded-md border object-cover" />
                    <button
                      type="button"
                      className="absolute -right-2 -top-2 rounded-full bg-destructive p-0.5 text-white"
                      onClick={() => {
                        setImages((current) => current.filter((entry) => entry.id !== image.id))
                        setPreviewHtml("")
                      }}
                      aria-label={`Remove ${image.name}`}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </div>
                ))}
                <Button variant="outline" size="sm" disabled={uploading} onClick={() => fileRef.current?.click()}>
                  {uploading ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <ImageIcon className="mr-2 h-3.5 w-3.5" />}
                  Add image
                </Button>
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/png,image/jpeg,image/gif,image/webp"
                  className="hidden"
                  onChange={(event) => {
                    const file = event.target.files?.[0]
                    event.target.value = ""
                    if (file) void uploadImage(file)
                  }}
                />
              </div>
              <p className="text-xs text-muted-foreground">Images appear in the email only, not the push.</p>
            </div>

            {error !== "" && <p className="text-sm text-destructive">{error}</p>}
          </div>

          <div className="flex min-h-0 flex-col gap-2">
            <div className="flex items-center justify-between">
              <Label>Email preview</Label>
              <Button variant="outline" size="sm" onClick={buildPreview} disabled={previewing}>
                {previewing && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
                {previewHtml ? "Refresh" : "Build preview"}
              </Button>
            </div>
            <div className="min-h-[320px] flex-1 overflow-hidden rounded-lg border bg-muted/30">
              {previewHtml ? (
                // Sandboxed so the rendered email cannot run scripts or navigate
                // the panel, even though the server escapes everything already.
                <iframe
                  title="Email preview"
                  srcDoc={previewHtml}
                  sandbox=""
                  className="h-full min-h-[320px] w-full border-0 bg-white"
                />
              ) : (
                <div className="flex h-full min-h-[320px] items-center justify-center p-6 text-center text-sm text-muted-foreground">
                  Build a preview to see exactly what volunteers will receive.
                </div>
              )}
            </div>
            {recipients !== null && (
              <p className="text-xs text-muted-foreground">
                This will reach {recipients} confirmed volunteer{recipients === 1 ? "" : "s"}.
              </p>
            )}
          </div>
        </div>

        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={sending}>
            Cancel
          </Button>
          <Button onClick={send} disabled={!canSend}>
            {sending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
            Send message
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
