import "./globals.css";

export const metadata = {
  title: "SFLuv Redemption Portal",
  description: "Thank you for volunteering!",
};

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body>
        {children}
      </body>
    </html>
  );
}
