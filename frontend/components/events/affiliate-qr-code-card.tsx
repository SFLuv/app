import QRCode from "react-qr-code";

export const AffiliateQRCodeCard = ({
  code,
  logoUrl,
  organization,
}: {
  code: string
  logoUrl: string
  organization: string
}) => {
  const url = process.env.NEXT_PUBLIC_APP_REDEEM_URL_PRE
    + code
    + process.env.NEXT_PUBLIC_APP_REDEEM_URL_POST

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
        <ol style={{textAlign: "center", width: "70%", margin: "auto", fontSize: "10px"}}>
          <li>1. Scan the QR code</li>
          <li>2. Download the app (CitizenWallet)</li>
          <li>3. Select "Browse Communities"</li>
          <li>4. Select "SFLUV Community"</li>
          <li>5. Scan the QR again to claim your $SFLUV!</li>
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
      <div style={{textAlign: "center", fontSize: "8px"}}>
        <p>Interested in more SFLuv supported events?<br/>
          Visit <a>www.sfluv.org/volunteers</a>
        </p>
      </div>
    </div>

  )
}
