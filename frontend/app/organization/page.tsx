"use client"

import { OrganizationPanel } from "@/components/organization/organization-panel"

/**
 * My Organization.
 *
 * Lifted out of /settings, where it had grown into the largest tab on a page
 * about the account: an admin managing a team was doing it next to their own
 * notification preferences and a delete-account button. Organization
 * membership is a thing you administer, not a preference you set, and it now
 * has a place in the sidebar beside the other role panels.
 *
 * The panel itself is unchanged by the move — it is mounted here instead of
 * there — so somebody following an old link to /settings finds everything else
 * exactly where it was.
 */
export default function OrganizationPage() {
  return (
    <div className="mx-auto w-full max-w-5xl px-4 py-6 sm:px-6 sm:py-8">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold text-black dark:text-white">My Organization</h1>
        <p className="text-sm text-muted-foreground">
          Your organization, its roles, and who is in it.
        </p>
      </div>
      <OrganizationPanel />
    </div>
  )
}
