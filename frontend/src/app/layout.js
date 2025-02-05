import "./globals.css";
import { Suspense } from "react";

export const metadata = {
  title: "SFLuv Redemption Portal",
  description: "Thank you for volunteering!",
};

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body>
        <Suspense>
          {children}
        </Suspense>
      </body>
    </html>
  );
}
