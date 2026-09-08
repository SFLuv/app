import { SfluvQRCode } from "@/components/ui/sfluv-qr-code"

/**
 * The QR that goes on a printed card.
 *
 * Unframed: the card supplies its own white, and a plate outline around the
 * code would read as a box on the print. Everything else — module style, brand
 * eyes, centre mark — comes from the shared component, so a volunteer holding
 * a card next to the app sees the same code.
 */
export const CardQRCode = ({ value }: { value: string }) => (
  <SfluvQRCode value={value} framed={false} />
)
