import { clsx, type ClassValue } from "clsx"
import { extendTailwindMerge } from "tailwind-merge"

// The layout spacing names from tailwind.config.ts. Without them tailwind-merge
// does not know `p-card` is padding, keeps it beside a caller's `p-4`, and the
// stylesheet's order decides which one wins.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      spacing: ["gutter", "section", "card"],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function validateAddress(address: string): boolean {
   return address.startsWith("0x") && address.length === 42

}
