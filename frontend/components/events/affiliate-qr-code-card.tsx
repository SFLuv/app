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

export const AffiliateQRCodeCard = ({
  code,
  logoUrl,
  organization,
  eventTitle,
  codeNumber,
  eventStartAt,
}: {
  code: string
  logoUrl: string
  organization: string
  eventTitle: string
  codeNumber: number
  eventStartAt?: number
}) => {
  const url = buildEventRedeemQrValue(code)

  return (
    <div style={cardShellStyle}>
      <CardHeader codeNumber={codeNumber} eventTitle={eventTitle} eventStartAt={eventStartAt} />

      <div style={cardBodyStyle}>
        <Gap size={4} grow={1} />
        <div
          style={{
            ...rigid,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            gap: "12px",
          }}
        >
          {/* See qr-code-card.tsx on why this path is absolute. */}
          <img src="/icon.png" alt="SFLuv logo" style={{ height: "88px", width: "88px", objectFit: "contain" }} />
          <span style={{ fontWeight: "bold", fontSize: "24px" }}>X</span>
          <img src={logoUrl} alt="Affiliate logo" style={{ height: "88px", width: "88px", objectFit: "contain" }} />
        </div>

        <Gap size={10} />

        {/* Split into two units so a long organization name breaks the line
            after "from" rather than mid-phrase ("...SFLuv and Golden / Gate
            Audubon"). The second span is inline-block, so it sizes against the
            full heading width and drops to its own row whole when it will not
            fit alongside the first; only a name too long for a full row wraps
            internally. The side margins are wide enough that the break happens
            well before the text approaches the card edge.

            line-height is 1.2 rather than the inherited 1.5 because rows here
            are the main thing that eats the body's flexible space. */}
        <CardHeading fitKey={organization}>
          <span style={{ whiteSpace: "nowrap" }}>Thank you from</span>{" "}
          <span style={{ display: "inline-block" }}>SFLuv and {organization}!</span>
        </CardHeading>

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
