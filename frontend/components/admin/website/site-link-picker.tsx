"use client"

import { useEffect, useState } from "react"
import * as Menu from "@radix-ui/react-dropdown-menu"
import { Check, ChevronDown, ChevronRight } from "lucide-react"

import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { SITE_ORIGIN } from "@/lib/site-admin"

/**
 * A place on the site a link can go, as the site lists it at `/api/pages`.
 * `children` are places within it (one form, a fiscal year, a single
 * document); the parent is a place too (the whole page). A document says which
 * file it `opens`.
 */
export type SitePlace = { path: string; title: string; opens?: string; children?: SitePlace[] }
export type SitePage = SitePlace & { group: string }

// One request per visit to the page, shared by every picker on it.
let pagesRequest: Promise<SitePage[] | null> | null = null

function loadSitePages(): Promise<SitePage[] | null> {
  pagesRequest ??= fetch(`${SITE_ORIGIN}/api/pages`)
    .then((res) => (res.ok ? (res.json() as Promise<{ pages: SitePage[] }>) : null))
    .then((data) => data?.pages ?? null)
    .catch(() => null)
  return pagesRequest
}

/** The site's pages, as the site itself lists them. Null while loading or if the site cannot be reached. */
export function useSitePages() {
  const [pages, setPages] = useState<SitePage[] | null>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let cancelled = false
    void loadSitePages().then((result) => {
      if (cancelled) return
      if (result) setPages(result)
      else setFailed(true)
    })
    return () => {
      cancelled = true
    }
  }, [])
  return { pages, failed }
}

/** The place on the menu with this path, at any depth. */
export function findPlace(places: SitePlace[], path: string): SitePlace | null {
  for (const place of places) {
    if (place.path === path) return place
    const below = place.children ? findPlace(place.children, path) : null
    if (below) return below
  }
  return null
}

/** The titles leading to a path, e.g. ["Financials and Reports", "FYE June 30, 2026", "…Cash Flows"]. */
function trail(places: SitePlace[], path: string): string[] | null {
  for (const place of places) {
    if (place.path === path) return [place.title]
    const below = place.children ? trail(place.children, path) : null
    if (below) return [place.title, ...below]
  }
  return null
}

const menuSurface =
  "z-50 max-h-[min(24rem,var(--radix-dropdown-menu-content-available-height))] min-w-[14rem] max-w-[22rem] overflow-y-auto rounded-lg border border-border/70 bg-popover p-1 text-popover-foreground shadow-lg"
const menuRow =
  "relative flex cursor-pointer select-none items-center gap-2 rounded-md py-1.5 pl-7 pr-2 text-sm outline-none data-[highlighted]:bg-primary/12 data-[state=open]:bg-primary/12"

function PlaceItems({ places, value, onPick }: { places: SitePlace[]; value: string; onPick: (place: SitePlace) => void }) {
  return (
    <>
      {places.map((place) =>
        place.children && place.children.length > 0 ? (
          <Menu.Sub key={place.path}>
            <Menu.SubTrigger className={menuRow}>
              {trail([place], value) && <Check className="absolute left-2 h-3.5 w-3.5 opacity-60" />}
              <span className="flex-1 truncate">{place.title}</span>
              <ChevronRight className="h-4 w-4 opacity-60" />
            </Menu.SubTrigger>
            <Menu.Portal>
              <Menu.SubContent className={menuSurface} sideOffset={4} collisionPadding={8}>
                <Menu.Item className={menuRow} onSelect={() => onPick(place)}>
                  {value === place.path && <Check className="absolute left-2 h-3.5 w-3.5" />}
                  <span className="truncate font-medium">All of {place.title}</span>
                </Menu.Item>
                <Menu.Separator className="my-1 h-px bg-border" />
                <PlaceItems places={place.children} value={value} onPick={onPick} />
              </Menu.SubContent>
            </Menu.Portal>
          </Menu.Sub>
        ) : (
          <Menu.Item key={place.path} className={menuRow} onSelect={() => onPick(place)}>
            {value === place.path && <Check className="absolute left-2 h-3.5 w-3.5" />}
            <span className="truncate">{place.title}</span>
          </Menu.Item>
        ),
      )}
    </>
  )
}

/**
 * Chooses where a link goes: a place on sfluv.org from a menu (pages, and
 * within some pages a submenu of their parts: one form, a fiscal year, a single
 * document), or, under "Other link", any address typed by hand. A value that
 * is not on the menu (an older link, or another website) opens on "Other link"
 * so it is shown as it is rather than lost.
 */
export function SiteLinkPicker({
  id,
  value,
  onChange,
}: {
  id: string
  value: string
  /**
   * `picked` is the place chosen from the menu (absent when typed), and
   * `previous` the place the link pointed at before, if it was on the menu, so
   * the caller can attach or detach the file a document opens.
   */
  onChange: (href: string, picked?: SitePlace, previous?: SitePlace) => void
}) {
  const { pages, failed } = useSitePages()
  const current = pages ? trail(pages, value) : null
  const [typing, setTyping] = useState(false)

  // Once the list arrives, decide which way to show what is already there.
  useEffect(() => {
    if (pages) setTyping(value !== "" && !trail(pages, value))
    // Only when the list arrives; after that the person's choice stands.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pages])

  if (failed) {
    return (
      <div className="space-y-1">
        <Input id={id} value={value} placeholder="/volunteers or https://…" onChange={(e) => onChange(e.target.value)} />
        <p className="text-xs text-muted-foreground">Couldn’t load the list of the site’s pages, so type the link instead.</p>
      </div>
    )
  }

  const groups: { label: string; pages: SitePage[] }[] = []
  for (const page of pages ?? []) {
    const group = groups.find((g) => g.label === page.group)
    if (group) group.pages.push(page)
    else groups.push({ label: page.group, pages: [page] })
  }

  const pick = (place: SitePlace) => {
    setTyping(false)
    onChange(place.path, place, (pages && findPlace(pages, value)) || undefined)
  }

  const shown = typing ? "Other link" : current ? current.join(" › ") : ""

  return (
    <div className="space-y-2">
      <Menu.Root modal={false}>
        <Menu.Trigger
          id={id}
          disabled={!pages}
          className={cn(
            "flex h-10 w-full items-center justify-between gap-2 rounded-lg border border-input bg-card/90 px-3 py-2 text-left text-sm shadow-sm focus:border-primary/55 focus:outline-none focus:ring-2 focus:ring-ring/35 disabled:cursor-not-allowed disabled:opacity-50",
            !shown && "text-muted-foreground",
          )}
        >
          <span className="truncate">{shown || (pages ? "Choose a page on the site" : "Loading the site’s pages…")}</span>
          <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className={menuSurface} align="start" sideOffset={4} collisionPadding={8}>
            {groups.map((group, index) => (
              <Menu.Group key={group.label}>
                {index > 0 && <Menu.Separator className="my-1 h-px bg-border" />}
                <Menu.Label className="px-2 py-1 text-xs font-medium text-muted-foreground">{group.label}</Menu.Label>
                <PlaceItems places={group.pages} value={typing ? "" : value} onPick={pick} />
              </Menu.Group>
            ))}
            <Menu.Separator className="my-1 h-px bg-border" />
            <Menu.Item
              className={menuRow}
              onSelect={() => {
                setTyping(true)
                if (current) onChange("", undefined, (pages && findPlace(pages, value)) || undefined)
              }}
            >
              {typing && <Check className="absolute left-2 h-3.5 w-3.5" />}
              Other link (type it in)
            </Menu.Item>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
      {typing && (
        <Input aria-label="Link address" value={value} placeholder="https://… or mailto:…" onChange={(e) => onChange(e.target.value)} />
      )}
    </div>
  )
}
