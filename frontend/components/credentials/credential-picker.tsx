"use client"

import { useRef, useState } from "react"
import * as Menu from "@radix-ui/react-dropdown-menu"
import { Check, ChevronDown, ChevronRight } from "lucide-react"

import { cn } from "@/lib/utils"
import type { GlobalCredentialType } from "@/types/workflow"

/** A credential type with the ones listed under it (`parent_value`), to any depth. */
export type CredentialNode = { type: GlobalCredentialType; children: CredentialNode[] }

/**
 * Arranges credential types as the menu shows them. A type whose parent is
 * missing (deleted, or filtered out) sits at the top level rather than vanish.
 */
export function credentialTree(types: GlobalCredentialType[]): CredentialNode[] {
  const nodes = new Map(types.map((type) => [type.value, { type, children: [] as CredentialNode[] }]))
  const roots: CredentialNode[] = []
  for (const node of nodes.values()) {
    const parent = node.type.parent_value ? nodes.get(node.type.parent_value) : undefined
    if (parent && parent !== node) parent.children.push(node)
    else roots.push(node)
  }
  return roots
}

/** Every type listed under `value`, at any depth. A type cannot be moved under one of these. */
export function credentialDescendants(types: GlobalCredentialType[], value: string): Set<string> {
  const out = new Set<string>()
  const walk = (parent: string) => {
    for (const type of types) {
      if (type.parent_value === parent && !out.has(type.value)) {
        out.add(type.value)
        walk(type.value)
      }
    }
  }
  walk(value)
  return out
}

const surface =
  "z-50 max-h-[min(20rem,var(--radix-dropdown-menu-content-available-height))] w-[var(--radix-dropdown-menu-trigger-width)] min-w-[14rem] overflow-y-auto rounded-lg border border-border/70 bg-popover p-1 text-popover-foreground shadow-lg"
const row =
  "relative flex cursor-pointer select-none items-center gap-2 rounded-md py-1.5 pr-2 text-sm outline-none data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[highlighted]:bg-primary/12"

/** "Website editor: banner items" under "Website editor" reads as "Banner items". */
function childLabel(label: string, parentLabel: string): string {
  const prefix = `${parentLabel}: `
  if (!label.startsWith(prefix) || label.length === prefix.length) return label
  const rest = label.slice(prefix.length)
  return rest.charAt(0).toUpperCase() + rest.slice(1)
}

/** Indent per level, plus room for the check mark. */
const indent = (depth: number) => ({ paddingLeft: `${1.75 + depth * 1.25}rem` })

function Items({
  nodes,
  depth,
  parentLabel,
  value,
  expanded,
  onToggle,
  isDisabled,
  labelOf,
  onPick,
}: {
  nodes: CredentialNode[]
  depth: number
  parentLabel?: string
  value: string
  expanded: Set<string>
  onToggle: (value: string) => void
  isDisabled: (value: string) => boolean
  labelOf: (type: GlobalCredentialType) => string
  onPick: (value: string) => void
}) {
  return (
    <>
      {nodes.map(({ type, children }) => {
        const label = parentLabel ? childLabel(labelOf(type), parentLabel) : labelOf(type)
        if (children.length === 0) {
          return (
            <Menu.Item
              key={type.value}
              className={row}
              style={indent(depth)}
              disabled={isDisabled(type.value)}
              onSelect={() => onPick(type.value)}
            >
              {value === type.value && <Check className="absolute h-3.5 w-3.5" style={{ left: `${0.5 + depth * 1.25}rem` }} />}
              <span>{label}</span>
            </Menu.Item>
          )
        }
        const open = expanded.has(type.value)
        return (
          <div key={type.value} data-credential-group={type.value}>
            {/* Opens or closes the group in place; it does not pick anything. */}
            <Menu.Item
              className={row}
              style={indent(depth)}
              aria-expanded={open}
              onSelect={(event) => {
                event.preventDefault()
                const group = (event.currentTarget as HTMLElement).closest<HTMLElement>("[data-credential-group]")
                onToggle(type.value)
                // Scroll the menu so the whole group shows, its last option included,
                // rather than opening below the visible part of the list.
                if (!open && group) {
                  requestAnimationFrame(() => {
                    group.lastElementChild?.scrollIntoView({ block: "nearest" })
                    group.scrollIntoView({ block: "nearest" })
                  })
                }
              }}
            >
              <ChevronRight
                className={cn("absolute h-3.5 w-3.5 opacity-60 transition-transform", open && "rotate-90")}
                style={{ left: `${0.5 + depth * 1.25}rem` }}
              />
              <span className="flex-1">{label}</span>
              <span className="text-xs text-muted-foreground">{children.length + 1} options</span>
            </Menu.Item>
            {open && (
              <>
                <Menu.Item
                  className={row}
                  style={indent(depth + 1)}
                  disabled={isDisabled(type.value)}
                  onSelect={() => onPick(type.value)}
                >
                  {value === type.value && <Check className="absolute h-3.5 w-3.5" style={{ left: `${0.5 + (depth + 1) * 1.25}rem` }} />}
                  <span className="font-medium">{labelOf(type)}</span>
                  <span className="text-xs text-muted-foreground">(all of it)</span>
                </Menu.Item>
                <Items
                  nodes={children}
                  depth={depth + 1}
                  parentLabel={labelOf(type)}
                  value={value}
                  expanded={expanded}
                  onToggle={onToggle}
                  isDisabled={isDisabled}
                  labelOf={labelOf}
                  onPick={onPick}
                />
              </>
            )}
          </div>
        )
      })}
    </>
  )
}

/**
 * Picks a credential type from one menu in which types listed under another
 * (`parent_value`) sit in a group that expands in place, like Website editor →
 * Banner items. The parent is itself the first choice in its group. Types for
 * which `isDisabled` is true (already held, say) are greyed out, and a group
 * still opens when its parent is taken.
 */
export function CredentialPicker({
  id,
  types,
  value,
  onChange,
  isDisabled = () => false,
  labelOf = (type) => type.label,
  placeholder = "Choose a credential…",
  disabled,
  className,
}: {
  id?: string
  types: GlobalCredentialType[]
  value: string
  onChange: (value: string) => void
  isDisabled?: (value: string) => boolean
  labelOf?: (type: GlobalCredentialType) => string
  placeholder?: string
  disabled?: boolean
  className?: string
}) {
  const selected = types.find((type) => type.value === value)
  const triggerRef = useRef<HTMLButtonElement>(null)
  // Inside a dialog, the menu is drawn inside it. A dialog blocks clicks on
  // everything outside itself, and this menu (a different Radix release from
  // the app's dialog) is not recognised as one of its layers, so drawn on the
  // page body its items could not be clicked.
  const [container, setContainer] = useState<HTMLElement | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const toggle = (group: string) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(group)) next.delete(group)
      else next.add(group)
      return next
    })
  return (
    <Menu.Root
      modal={false}
      onOpenChange={(open) => {
        if (!open) return
        setContainer(triggerRef.current?.closest<HTMLElement>('[role="dialog"]') ?? null)
        // Open the groups that hold the current choice, so it is visible.
        const groups = new Set<string>()
        for (let parent = selected?.parent_value; parent; parent = types.find((t) => t.value === parent)?.parent_value) {
          groups.add(parent)
        }
        setExpanded(groups)
      }}
    >
      <Menu.Trigger
        ref={triggerRef}
        id={id}
        disabled={disabled}
        className={cn(
          "flex h-10 w-full items-center justify-between gap-2 rounded-lg border border-input bg-card/90 px-3 py-2 text-left text-sm shadow-sm focus:border-primary/55 focus:outline-none focus:ring-2 focus:ring-ring/35 disabled:cursor-not-allowed disabled:opacity-50",
          !selected && "text-muted-foreground",
          className,
        )}
      >
        <span className="truncate">{selected ? labelOf(selected) : placeholder}</span>
        <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
      </Menu.Trigger>
      <Menu.Portal container={container}>
        <Menu.Content
          className={surface}
          align="start"
          sideOffset={4}
          collisionPadding={8}
          // Keep the whole menu inside the dialog it opens in, so nothing is clipped.
          collisionBoundary={container ?? undefined}
        >
          <Items
            nodes={credentialTree(types)}
            depth={0}
            value={value}
            expanded={expanded}
            onToggle={toggle}
            isDisabled={isDisabled}
            labelOf={labelOf}
            onPick={onChange}
          />
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  )
}
