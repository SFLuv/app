// Shared USDC-blue palette, chosen to sit alongside the SFLUV coral (#eb6c6c)
// scheme without clashing. USDC's brand blue is #2775CA.
export const USDC = {
  blue: "#2775ca",
  blueHover: "#1c5fa6",
  blueLight: "#6aa8e0",
  // Tailwind class fragments reused across the USDC widgets/modals.
  button: "bg-[#2775ca] hover:bg-[#1c5fa6] text-white",
  buttonOutline:
    "border-[#2775ca]/40 text-[#2775ca] hover:bg-[#eff5fc] hover:text-[#1c5fa6] dark:text-[#6aa8e0] dark:hover:bg-[#12233a]",
  accentText: "text-[#2775ca] dark:text-[#6aa8e0]",
  cardBorder: "border-[#2775ca]/20",
  cardBg:
    "bg-gradient-to-br from-[#eff5fc] via-background to-muted/20 dark:from-[#12233a]/50 dark:via-background",
  accentBar: "bg-gradient-to-r from-[#2775ca] to-[#6aa8e0]",
} as const

export const USDC_SYMBOL = "USDC"
