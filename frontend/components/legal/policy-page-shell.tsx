import Link from "next/link"
import { ReactNode } from "react"

export function PolicyPageShell({
  title,
  lastUpdated,
  children,
}: {
  title: string
  lastUpdated: string
  children: ReactNode
}) {
  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(235,108,108,0.16),_transparent_40%),linear-gradient(180deg,_hsl(var(--background))_0%,_hsl(var(--background))_100%)] px-4 py-8 sm:px-6 sm:py-10">
      <div className="mx-auto max-w-4xl">
        <div className="rounded-3xl border border-border/70 bg-card/95 p-6 shadow-[0_1px_3px_hsl(var(--foreground)/0.08),0_24px_60px_hsl(var(--foreground)/0.16)] sm:p-10">
          <div className="space-y-3 border-b border-border/70 pb-6">
            <p className="text-sm font-semibold uppercase tracking-[0.24em] text-[#eb6c6c]">
              SFLuv Legal
            </p>
            <h1 className="text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">
              {title}
            </h1>
            <p className="text-sm text-muted-foreground">Last updated: {lastUpdated}</p>
            <Link
              href="/map"
              className="inline-flex text-sm font-semibold text-[#eb6c6c] underline underline-offset-4"
            >
              Return to the app
            </Link>
          </div>

          <article className="space-y-6 pt-6 text-sm leading-7 text-foreground sm:text-base">
            {children}
          </article>
        </div>
      </div>
    </div>
  )
}
