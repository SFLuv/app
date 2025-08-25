import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function validateAddress(address: string): boolean {
   return address.startsWith("0x") && address.length === 42

}
