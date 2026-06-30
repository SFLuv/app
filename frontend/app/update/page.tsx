import { SFLUV_GOOGLE_PLAY_URL, SFLUV_IOS_APP_STORE_URL } from "@/lib/app-download-links"

export const metadata = {
  title: "Update the SFLUV app",
  description: "Download the latest version of the SFLUV Wallet app.",
}

export default function UpdatePage() {
  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-10">
      <div className="mx-auto w-full max-w-md text-center">
        <div className="rounded-lg border bg-card/95 p-6 shadow-sm">
          <img
            src="/icon.png"
            alt="SFLUV"
            className="mx-auto h-16 w-16 object-contain"
          />
          <h1 className="mt-4 text-2xl font-bold text-black dark:text-white">
            Update the SFLUV app
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Get the latest version of the SFLUV Wallet from your app store to keep using the app.
          </p>
          <div className="mt-5 grid gap-3 sm:grid-cols-2">
            <a
              href={SFLUV_IOS_APP_STORE_URL}
              target="_blank"
              rel="noreferrer"
              className="flex min-h-14 items-center gap-3 rounded-lg border border-border bg-white px-3 py-2 text-left shadow-sm transition hover:border-[#eb6c6c]/50 hover:bg-[#fff7f7] dark:bg-black"
            >
              <img
                src="/appstore.svg"
                alt=""
                className="h-9 w-9 shrink-0 object-contain"
              />
              <span>
                <span className="block text-xs text-muted-foreground">Download on</span>
                <span className="block text-sm font-semibold text-foreground">App Store</span>
              </span>
            </a>
            <a
              href={SFLUV_GOOGLE_PLAY_URL}
              target="_blank"
              rel="noreferrer"
              className="flex min-h-14 items-center gap-3 rounded-lg border border-border bg-white px-3 py-2 text-left shadow-sm transition hover:border-[#eb6c6c]/50 hover:bg-[#fff7f7] dark:bg-black"
            >
              <img
                src="/googleplaystore.svg"
                alt=""
                className="h-9 w-9 shrink-0 object-contain"
              />
              <span>
                <span className="block text-xs text-muted-foreground">Get it on</span>
                <span className="block text-sm font-semibold text-foreground">Google Play</span>
              </span>
            </a>
          </div>
        </div>
      </div>
    </main>
  )
}
