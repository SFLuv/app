import type { SiteFormConfig } from "@/types/site"

type AuthFetch = (endpoint: string, options?: RequestInit) => Promise<Response>

/** The public site. Only used to preview links and to show visitors' form addresses. */
export const SITE_ORIGIN = (process.env.NEXT_PUBLIC_WEBSITE_URL || "https://sfluv.org").replace(/\/+$/, "")

/** Turns a path on the site ("/assets/…") into something the admin panel can open or preview. */
export function resolveSiteUrl(url: string | null | undefined): string | null {
  if (!url) return null
  return url.startsWith("/") ? `${SITE_ORIGIN}${url}` : url
}

export function formPublicUrl(slug: string): string {
  return `${SITE_ORIGIN}/forms/${slug}`
}

/**
 * Calls a site-editing route and returns its JSON. Failures throw an Error whose
 * message is the plain-language sentence the backend sent, because the person
 * reading it is staff editing a page, not a developer.
 */
export async function siteFetch<T>(authFetch: AuthFetch, path: string, init?: RequestInit): Promise<T> {
  const res = await authFetch(path, init)
  if (!res.ok) {
    let message = ""
    try {
      message = ((await res.json()) as { message?: string }).message ?? ""
    } catch {
      /* body was not JSON */
    }
    if (res.status === 403) message = message || "You don't have permission to do that."
    throw new Error(message || `Something went wrong (${res.status}). Please try again.`)
  }
  return (await res.json()) as T
}

export function jsonBody(value: unknown): RequestInit {
  return { headers: { "Content-Type": "application/json" }, body: JSON.stringify(value) }
}

/** Uploads a PDF or image and returns it. No Content-Type: the browser must set the multipart boundary. */
/** Matches the backend's limit (maxSiteFileBytes). */
export const MAX_SITE_FILE_BYTES = 25 * 1024 * 1024

/**
 * Checks a chosen file before anything is uploaded, so a wrong or oversized
 * file is turned away the moment it is picked rather than at publish time.
 * It reads the first bytes, as the backend does, because a file's name and the
 * type the browser guesses from it can be anything. Returns the problem in
 * plain words, or null. The backend checks again; this is for the person, not
 * the lock.
 */
export async function checkSiteFile(file: File, want: "pdf" | "image"): Promise<string | null> {
  if (file.size > MAX_SITE_FILE_BYTES) return "That file is too large. Files can be up to 25 MB."
  const head = new Uint8Array(await file.slice(0, 12).arrayBuffer())
  const text = (from: number, to: number) => String.fromCharCode(...head.slice(from, to))
  const isPdf = text(0, 5) === "%PDF-"
  const isImage =
    (head[0] === 0x89 && text(1, 4) === "PNG") ||
    (head[0] === 0xff && head[1] === 0xd8 && head[2] === 0xff) ||
    text(0, 4) === "GIF8" ||
    (text(0, 4) === "RIFF" && text(8, 12) === "WEBP")
  if (want === "pdf" && !isPdf) return "That file isn’t a PDF, even if its name ends in .pdf. Choose a PDF."
  if (want === "image" && !isImage) return "That file isn’t a photo the site can show. Choose a PNG, JPEG, WebP or GIF."
  return null
}

export async function uploadSiteFile(authFetch: AuthFetch, file: File) {
  const form = new FormData()
  form.append("file", file)
  return siteFetch<import("@/types/site").SiteFile>(authFetch, "/admin/site/files", { method: "POST", body: form })
}

export function formatWhen(unixSeconds: number | null | undefined): string {
  if (!unixSeconds) return ""
  return new Date(unixSeconds * 1000).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  })
}

export function formatSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function emptyFormConfig(): SiteFormConfig {
  return {
    contact: "optional",
    preferred_name: false,
    event: "off",
    event_name: "",
    choices_prompt: "",
    choices: [],
    guardian_section: true,
    confirmation_message: "Thank you. Your signed form has been recorded.",
  }
}

export function slugify(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60)
}

export const FINANCIAL_KIND_LABELS: Record<string, string> = {
  activity: "Statement of Activity",
  cash_flows: "Statement of Cash Flows",
  financial_position: "Statement of Financial Position",
  activity_comparison: "Statement of Activity Comparison",
  form_990n: "990-N filing",
  form_199n: "199-N confirmation",
  impact_report: "Annual impact report",
  other: "Other document",
}

export const PERIOD_LABELS: Record<string, string> = {
  Q1: "Q1 (Jul–Sep)",
  Q2: "Q2 (Oct–Dec)",
  Q3: "Q3 (Jan–Mar)",
  Q4: "Q4 / year-end (Apr–Jun)",
  FULL: "Full year",
}
