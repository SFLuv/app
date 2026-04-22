import { PolicyPageShell } from "@/components/legal/policy-page-shell"
import { EMAIL_OPT_IN_POLICY_LAST_UPDATED } from "@/lib/policies"
import { ReactNode } from "react"

function Section({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="space-y-3">
      <h2 className="text-xl font-semibold tracking-tight text-foreground">{title}</h2>
      {children}
    </section>
  )
}

export default function EmailOptInPolicyPage() {
  return (
    <PolicyPageShell title="Email Opt-In Policy" lastUpdated={EMAIL_OPT_IN_POLICY_LAST_UPDATED}>
      <p>
        This Email Opt-In Policy explains how SFLuv collects and uses Your consent to send
        newsletters, product updates, event announcements, service notices, and other marketing or
        community email communications.
      </p>
      <p>
        By opting in, You agree that SFLuv may send You emails that promote the Service, share
        updates about SFLuv programs, highlight local events or merchant opportunities, and provide
        related community information.
      </p>

      <Section title="What You Are Opting Into">
        <ul className="list-disc space-y-2 pl-6 text-muted-foreground">
          <li>Newsletters and community updates.</li>
          <li>Announcements about features, promotions, offers, or events.</li>
          <li>Educational content and invitations related to SFLuv participation.</li>
          <li>
            Email messages sent through trusted service providers that help Us manage and deliver
            communications.
          </li>
        </ul>
      </Section>

      <Section title="Consent Standard">
        <p>
          We only treat You as opted in when You take an affirmative action to indicate consent.
          When You submit the opt-in choice in the app, We record that preference alongside the
          then-current policy version and timestamp.
        </p>
        <p>
          Your opt-in is voluntary and is not required to create an account or use non-marketing
          portions of the Service.
        </p>
      </Section>

      <Section title="How We Use Your Email Address">
        <p>
          If You opt in, We may use Your email address to send marketing and community emails. We
          may also use engagement signals such as opens, clicks, unsubscribe actions, and delivery
          diagnostics to understand campaign performance, maintain list hygiene, prevent abuse, and
          improve the relevance of future communications.
        </p>
      </Section>

      <Section title="Service Providers">
        <p>
          We may use third-party email delivery and customer engagement providers, including
          Mailgun and Hubspot, to store mailing preferences, deliver campaigns, handle unsubscribe
          requests, and measure aggregate engagement. These providers process personal data on Our
          behalf in accordance with their own privacy commitments and applicable agreements with Us.
        </p>
      </Section>

      <Section title="How to Unsubscribe">
        <p>
          You may opt out of marketing emails at any time by clicking the unsubscribe link in a
          marketing email or by contacting Us at admin@sfluv.org. Please note that it may take a
          short period of time for preference changes to take effect across all systems.
        </p>
        <p>
          Even if You unsubscribe from marketing messages, We may still send You transactional or
          service-related communications when needed to operate the Service, protect account
          security, or respond to Your requests.
        </p>
      </Section>

      <Section title="Retention of Opt-In Records">
        <p>
          We keep records of Your opt-in status, the policy version presented at the time of
          consent, and related timestamps for as long as reasonably necessary to demonstrate
          consent, honor Your preferences, comply with legal obligations, resolve disputes, and
          enforce Our agreements.
        </p>
      </Section>

      <Section title="Your Choices">
        <ul className="list-disc space-y-2 pl-6 text-muted-foreground">
          <li>You can choose not to opt in.</li>
          <li>You can unsubscribe later at any time.</li>
          <li>You can contact Us to ask questions about how Your preferences are handled.</li>
        </ul>
      </Section>

      <Section title="Relationship to the Privacy Policy">
        <p>
          This Email Opt-In Policy should be read together with the SFLuv Privacy Policy, which
          describes how We collect, use, disclose, retain, and protect personal information more
          broadly.
        </p>
      </Section>

      <Section title="Changes to this Email Opt-In Policy">
        <p>
          We may update this Email Opt-In Policy from time to time. When We do, We will post the
          revised version on this page and update the "Last updated" date above. Material changes
          will apply prospectively from the time they are posted unless otherwise required by law.
        </p>
      </Section>

      <Section title="Contact Us">
        <ul className="list-disc space-y-2 pl-6 text-muted-foreground">
          <li>By email: admin@sfluv.org</li>
          <li>By visiting this page on our website: www.sfluv.org</li>
        </ul>
      </Section>
    </PolicyPageShell>
  )
}
