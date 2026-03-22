import QRCode from "react-qr-code";
import { buildEventRedeemQrValue } from "@/lib/redeem-link";

export const AffiliateQRCodeCard = ({
  code,
  logoUrl,
  organization,
}: {
  code: string
  logoUrl: string
  organization: string
}) => {
  const url = buildEventRedeemQrValue(code)

  return (
    <div style={{textAlign: "center", justifyContent: "center", color: "black", height: "550px", width: "425px", margin: "auto", overflowY: "hidden", paddingTop: "10px"}}>
      <div style={{display: "flex", alignItems: "center", justifyContent: "center", gap: "12px", marginTop: "24px"}}>
        <img src="../icon.png" alt="SFLuv logo" style={{
          height: "56px",
          width: "56px",
          objectFit: "contain"
        }} />
        <span style={{fontWeight: "bold", fontSize: "20px"}}>X</span>
        <img src={logoUrl} alt="Affiliate logo" style={{
          height: "56px",
          width: "56px",
          objectFit: "contain"
        }} />
      </div>
      <div style={{textAlign: "center"}}>
        <h1 style={{
          margin: "12px 0 0",
          fontWeight: "bold",
          fontSize: "18px"
        }}>Thank you from SFLuv and {organization}!</h1>
        <h3 style={{margin: "10px"}}>To redeem your tokens:</h3>
        <ol style={{textAlign: "center", width: "70%", margin: "auto", fontSize: "12px"}}>
          <li>1. Scan the QR code.</li>
          <li>2. Select "Continue with Web Wallet".</li>
          <li>3. Sign up with Google or your preferred email account.</li>
          <li>4. Wait to receive your SFLuv!</li>
        </ol>
      </div>

      <div style={{margin: "auto", marginTop: "20px", marginBottom: "15px", height: "auto", width: "40%", textAlign: "center"}}>
        <QRCode
          size={256}
          style={{ height: "auto", maxWidth: "100%", width: "100%" }}
          value={url}
          viewBox={`0 0 256 256`}
        />
      </div>
      <div style={{textAlign: "center", fontSize: "10px"}}>
        <p>Interested in more SFLuv supported events?<br/>
          Visit <a>www.sfluv.org/volunteers</a>
        </p>
      </div>
    </div>

  )
}
