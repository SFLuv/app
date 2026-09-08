import { buildEventRedeemQrValue } from "@/lib/redeem-link";
import { CardQRCode } from "./card-qr";
import {
  CardFooter,
  CardHeader,
  CardInstructions,
  CardQRFrame,
  CardHeading,
  Gap,
  cardBodyStyle,
  cardShellStyle,
  rigid,
} from "./card-chrome";

export const QRCodeCard = ({ code, eventTitle, codeNumber, eventStartAt }: { code: string; eventTitle: string; codeNumber: number; eventStartAt?: number }) => {
  const url = buildEventRedeemQrValue(code)

  return (
    <div style={cardShellStyle}>
      <CardHeader codeNumber={codeNumber} eventTitle={eventTitle} eventStartAt={eventStartAt} />

      <div style={cardBodyStyle}>
        <Gap size={4} grow={1} />
        {/* Absolute path, not "../icon.png": the export renders from whatever
            route the modal was opened on, and a relative path resolves against
            that route rather than the public root. */}
        <img src="/icon.png" alt="SFLuv logo" style={{ ...rigid, height: "auto", width: "136px" }} />
        <Gap size={10} />
        <CardHeading fitKey="sfluv">Thank you from SFLuv!</CardHeading>
        <Gap size={10} />
        <CardInstructions />
        <Gap size={10} />
        <CardQRFrame>
          <CardQRCode value={url} />
        </CardQRFrame>
        <Gap size={10} />
      </div>

      <CardFooter />
    </div>
  )
}
