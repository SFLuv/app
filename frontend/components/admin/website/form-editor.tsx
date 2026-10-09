"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { ArrowLeft, History, Loader2, Plus, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAuthFetch } from "./use-auth-fetch"
import { useNotice } from "./notice"
import { emptyFormConfig, formatWhen, formPublicUrl, jsonBody, siteFetch, slugify } from "@/lib/site-admin"
import type { SiteFormConfig, SiteFormDetail, SiteFormRequest } from "@/types/site"

interface Props {
  /** The form being edited, or null to create one. */
  formId: string | null
  onBack: () => void
  /** Called after a successful save with the saved form. */
  onSaved: (form: SiteFormDetail) => void
}

const CHOICES_TOKEN = "{{choices}}"

export function FormEditor({ formId, onBack, onSaved }: Props) {
  const authFetch = useAuthFetch()
  const { toast } = useNotice()
  const bodyRef = useRef<HTMLTextAreaElement>(null)

  const [loading, setLoading] = useState(formId !== null)
  const [error, setError] = useState("")
  const [saving, setSaving] = useState(false)
  const [detail, setDetail] = useState<SiteFormDetail | null>(null)

  const [kind, setKind] = useState<"waiver" | "withdrawal">("waiver")
  const [slug, setSlug] = useState("")
  const [slugTouched, setSlugTouched] = useState(false)
  const [title, setTitle] = useState("")
  const [summary, setSummary] = useState("")
  const [body, setBody] = useState("")
  const [config, setConfig] = useState<SiteFormConfig>(emptyFormConfig)
  const [baseline, setBaseline] = useState("")

  const load = (form: SiteFormDetail) => {
    setDetail(form)
    setKind(form.kind)
    setSlug(form.slug)
    setTitle(form.current.title)
    setSummary(form.current.summary)
    setBody(form.current.body)
    setConfig(form.current.config)
    setBaseline(JSON.stringify([form.current.title, form.current.summary, form.current.body, form.current.config]))
  }

  useEffect(() => {
    if (formId === null) {
      setBaseline(JSON.stringify(["", "", "", emptyFormConfig()]))
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const form = await siteFetch<SiteFormDetail>(authFetch, `/admin/site/forms/${formId}`)
        if (!cancelled) load(form)
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : "Unable to load the form.")
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [formId, authFetch])

  const dirty = JSON.stringify([title, summary, body, config]) !== baseline
  const patchConfig = (changes: Partial<SiteFormConfig>) => setConfig((c) => ({ ...c, ...changes }))

  const hasChoices = config.choices.length > 0
  const bodyHasToken = body.includes(CHOICES_TOKEN)
  const addressPreview = useMemo(() => (slug ? formPublicUrl(slug) : ""), [slug])

  const insertToken = () => {
    const el = bodyRef.current
    const at = el ? el.selectionStart : body.length
    const next = `${body.slice(0, at)}${at > 0 ? "\n\n" : ""}${CHOICES_TOKEN}\n\n${body.slice(at)}`.replace(/\n{3,}/g, "\n\n")
    setBody(next)
  }

  const save = async () => {
    const request: SiteFormRequest = { slug, kind, title, summary, body, config }
    setSaving(true)
    try {
      const saved =
        formId === null
          ? await siteFetch<SiteFormDetail>(authFetch, "/admin/site/forms", { method: "POST", ...jsonBody(request) })
          : await siteFetch<SiteFormDetail>(authFetch, `/admin/site/forms/${formId}`, { method: "PUT", ...jsonBody(request) })
      load(saved)
      const earlierSignatures = formId !== null && saved.signature_count > 0
      toast({
        title: formId === null ? "Form created" : `Saved as version ${saved.current_version}`,
        description:
          formId === null
            ? "It is closed until you open it for signing."
            : earlierSignatures
              ? "People who already signed keep the earlier wording they agreed to."
              : undefined,
      })
      onSaved(saved)
    } catch (err) {
      toast({ title: "Could not save the form", description: err instanceof Error ? err.message : "", variant: "destructive" })
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading the form…
      </div>
    )
  }
  if (error) return <p className="text-sm text-destructive">{error}</p>

  return (
    <div className="space-y-6">
      <Button variant="ghost" size="sm" onClick={() => (!dirty || window.confirm("Leave without saving your changes?")) && onBack()}>
        <ArrowLeft className="mr-2 h-4 w-4" /> All forms
      </Button>

      <Card>
        <CardHeader>
          <CardTitle>{formId === null ? "New form" : `Edit “${detail?.current.title ?? ""}”`}</CardTitle>
          <CardDescription>
            {formId === null
              ? "Write the form people will sign. It stays closed until you open it for signing."
              : "Saving adds a new version. People who already signed are not affected: each signature is kept with the exact wording that person agreed to."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="form-title">Title</Label>
              <Input
                id="form-title"
                value={title}
                maxLength={150}
                placeholder="Media Consent and Release"
                onChange={(e) => {
                  setTitle(e.target.value)
                  if (formId === null && !slugTouched) setSlug(slugify(e.target.value))
                }}
              />
            </div>

            {formId === null ? (
              <>
                <div className="space-y-1">
                  <Label htmlFor="form-slug">Web address</Label>
                  <Input
                    id="form-slug"
                    value={slug}
                    maxLength={60}
                    onChange={(e) => {
                      setSlugTouched(true)
                      setSlug(slugify(e.target.value))
                    }}
                  />
                  <p className="text-xs text-muted-foreground">
                    {addressPreview ? `People will visit ${addressPreview}. ` : ""}This can’t be changed later, because printed QR codes point to it.
                  </p>
                </div>
                <div className="space-y-1">
                  <Label>Type</Label>
                  <Select value={kind} onValueChange={(v) => setKind(v as "waiver" | "withdrawal")}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="waiver">Waiver or release (people agree to something)</SelectItem>
                      <SelectItem value="withdrawal">Withdrawal (people take back their consent)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </>
            ) : (
              <p className="text-sm text-muted-foreground sm:col-span-2">
                Web address: <span className="font-mono">{addressPreview}</span>
              </p>
            )}

            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="form-summary">One-line summary (shown on the Forms and Waivers page)</Label>
              <Input
                id="form-summary"
                value={summary}
                maxLength={300}
                placeholder="Photography, video, audio, and statements."
                onChange={(e) => setSummary(e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Label htmlFor="form-body">The form text</Label>
              {hasChoices && (
                <Button type="button" variant="outline" size="sm" onClick={insertToken} disabled={bodyHasToken}>
                  {bodyHasToken ? "Options are placed in the text" : "Place the options here"}
                </Button>
              )}
            </div>
            <Textarea
              ref={bodyRef}
              id="form-body"
              value={body}
              rows={16}
              className="font-mono text-sm leading-relaxed"
              onChange={(e) => setBody(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Start a line with <span className="font-mono">## </span>to make a heading. Leave a blank line between
              paragraphs. Start lines with <span className="font-mono">- </span>for a bulleted list.
              {hasChoices && (
                <>
                  {" "}
                  Write <span className="font-mono">{CHOICES_TOKEN}</span> on its own line to choose where the options go
                  {bodyHasToken ? "." : " — otherwise they appear after the text."}
                </>
              )}
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">What people are asked</CardTitle>
          <CardDescription>
            Everyone is always asked for their full name, a signature, and to agree to sign electronically. Choose what else to ask.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1">
              <Label>Email or phone</Label>
              <Select value={config.contact} onValueChange={(v) => patchConfig({ contact: v as SiteFormConfig["contact"] })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="off">Don’t ask</SelectItem>
                  <SelectItem value="optional">Ask, but optional</SelectItem>
                  <SelectItem value="required">Required</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">If they give an email address, they get a copy of what they signed.</p>
            </div>

            <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
              <Label htmlFor="cfg-preferred">Ask for a preferred name</Label>
              <Switch id="cfg-preferred" checked={config.preferred_name} onCheckedChange={(v) => patchConfig({ preferred_name: v })} />
            </div>

            <div className="space-y-1">
              <Label>Event or project</Label>
              <Select value={config.event} onValueChange={(v) => patchConfig({ event: v as SiteFormConfig["event"] })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="off">Not part of this form</SelectItem>
                  <SelectItem value="ask">Let each person type it (and a date)</SelectItem>
                  <SelectItem value="fixed">It’s always the same one (I’ll name it)</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {config.event === "fixed" && (
              <div className="space-y-1">
                <Label htmlFor="cfg-event">Event or project name</Label>
                <Input
                  id="cfg-event"
                  value={config.event_name}
                  maxLength={200}
                  placeholder="Boundless Grant video"
                  onChange={(e) => patchConfig({ event_name: e.target.value })}
                />
              </div>
            )}

            <div className="flex items-center justify-between gap-3 rounded-lg border p-3 sm:col-span-2">
              <div>
                <Label htmlFor="cfg-guardian">Include a parent or guardian section</Label>
                <p className="text-xs text-muted-foreground">
                  Lets someone under 18 be signed for by a parent or guardian, who also signs.
                </p>
              </div>
              <Switch id="cfg-guardian" checked={config.guardian_section} onCheckedChange={(v) => patchConfig({ guardian_section: v })} />
            </div>
          </div>

          <div className="space-y-3 rounded-lg border p-4">
            <div>
              <p className="font-medium">Choices (optional)</p>
              <p className="text-xs text-muted-foreground">
                If you add choices, each person picks exactly one — for example “this event only” or “ongoing”.
              </p>
            </div>
            {hasChoices && (
              <div className="space-y-1">
                <Label htmlFor="cfg-prompt">Question above the choices (optional)</Label>
                <Input
                  id="cfg-prompt"
                  value={config.choices_prompt}
                  maxLength={200}
                  placeholder="Choose the scope of your consent"
                  onChange={(e) => patchConfig({ choices_prompt: e.target.value })}
                />
              </div>
            )}
            {config.choices.map((choice, index) => (
              <div key={index} className="grid gap-2 rounded-md bg-muted/40 p-3 sm:grid-cols-[1fr_2fr_auto] sm:items-start">
                <Input
                  aria-label={`Choice ${index + 1} label`}
                  value={choice.label}
                  maxLength={200}
                  placeholder="Short label"
                  onChange={(e) =>
                    patchConfig({ choices: config.choices.map((c, i) => (i === index ? { ...c, label: e.target.value } : c)) })
                  }
                />
                <Input
                  aria-label={`Choice ${index + 1} explanation`}
                  value={choice.description}
                  maxLength={500}
                  placeholder="What it means (optional)"
                  onChange={(e) =>
                    patchConfig({ choices: config.choices.map((c, i) => (i === index ? { ...c, description: e.target.value } : c)) })
                  }
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`Remove choice ${index + 1}`}
                  onClick={() => patchConfig({ choices: config.choices.filter((_, i) => i !== index) })}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
            {config.choices.length < 8 && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => patchConfig({ choices: [...config.choices, { id: "", label: "", description: "" }] })}
              >
                <Plus className="mr-2 h-4 w-4" /> Add a choice
              </Button>
            )}
          </div>

          <div className="space-y-1">
            <Label htmlFor="cfg-confirm">Message shown after someone signs</Label>
            <Input
              id="cfg-confirm"
              value={config.confirmation_message}
              maxLength={500}
              onChange={(e) => patchConfig({ confirmation_message: e.target.value })}
            />
          </div>
        </CardContent>
      </Card>

      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={save} disabled={saving || (!dirty && formId !== null) || title.trim() === "" || body.trim() === ""}>
          {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {formId === null ? "Create form" : "Save as a new version"}
        </Button>
        {formId !== null &&
          (dirty ? <span className="text-sm text-amber-600">You have unsaved changes.</span> : <span className="text-sm text-muted-foreground">Everything is saved.</span>)}
      </div>

      {detail && detail.versions.length > 1 && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg">
              <History className="h-4 w-4" /> Earlier versions
            </CardTitle>
            <CardDescription>
              “Use this version” loads its wording into the editor above. Nothing changes until you save.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="divide-y rounded-lg border">
              {detail.versions.map((v) => (
                <div key={v.version} className="flex flex-wrap items-center justify-between gap-3 p-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">
                      Version {v.version}
                      {v.version === detail.current_version && " (current)"}
                    </p>
                    <p className="truncate text-xs text-muted-foreground">
                      {v.saved_by || "Unknown"} · {formatWhen(v.saved_at)}
                    </p>
                  </div>
                  {v.version !== detail.current_version && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setTitle(v.title)
                        setSummary(v.summary)
                        setBody(v.body)
                        setConfig(v.config)
                        window.scrollTo({ top: 0, behavior: "smooth" })
                      }}
                    >
                      Use this version
                    </Button>
                  )}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
