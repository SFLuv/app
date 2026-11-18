# The notification hub

Every way somebody reaches SFLuv, and everything the platform needs a person for, posts one notification to
the backend's hub. The hub decides who hears about it. Nothing else does.

```
touchpoint ──► NotificationService.Post(topic) ──► teams routed to that topic ──► channels ──► people
(form, app,      stored in internal_notifications     (or the catch-all, if the      in the app (always)
 email, sweep)   with where it went                    topic has no team)            push, email members,
                                                                                     a shared address, a chat
```

- **Topic.** What a notification is about. Thirty-seven, listed in `structs.NotificationTopicCatalog()` and in
  the settings screen, grouped as Support and email, Requests and decisions, Infrastructure, and Activity.
- **Touchpoint.** A way somebody reaches us, or something the platform raises on its own, and the topic ids it
  posts under: `structs.NotificationTouchpointCatalog()`, 31 of them. Every topic is reached by at least one.
- **Team.** A named group of house-organization members (SFLuv's own organization). A person can be on any
  number of teams. A team is bounded by the organization at delivery time: somebody who leaves it stops
  receiving, whatever team rows still name them.
- **Route.** Topic → one or more teams. The `unrouted` route is the catch-all: its teams receive every topic
  that has no team of its own, or whose teams reach nobody (no members, no address, no chat).
- **Channels, per team.** Members always get the notification in the app's notification screen. On top of
  that a team can push to its members' phones, email its members, email a shared address, and post into a
  group chat as the support bot.

Settings live on the web app's Organization page (`/organization`), in its notifications section (house organization admins edit, members
read). The same screen lists recent notifications and where each one went, in red when it reached nobody. Its
**Visualizer** tab draws the hub live: every touchpoint wired to its topic ids, each topic to the teams that
would get it right now (its own, the catch-all, or nobody), waiting counts, and topics that just fired. It
reloads the hub every 10 seconds while open and the page is visible.

## Rules that keep anything from falling through

- A notification is stored whether or not anybody receives it. One that reaches no person, address or chat
  is marked `reached_nobody`, logged, counted in the settings screen, and listed there.
- At boot the backend logs every topic that has no team, unless a catch-all exists.
- Email and chat go out once, when a notification is raised. In-app delivery and push also reach people added
  to a team while the notification is still live, the next time it is posted.
- Conditions ("this request is waiting on a decision") stay live until decided; deciding in the admin panel
  retires them everywhere. Activity (signups, signatures, filtered mail) goes straight to history.
- Only somebody the notification was delivered to, and who is still in the organization, may press its
  buttons. Approve and reject still need an administrator.

## Receiving email

Mail to our receiving domain is forwarded by a Mailgun route to `POST /mailgun/inbound`, stored as a support
request (source `email`, with a reference), and posted under a topic chosen by the mailbox:

| Mailbox | Topic |
|---|---|
| `support@` | Email to support@ |
| `admin@` | Email to admin@ |
| `no_reply@`, `noreply@`, `reply@`, `replies@` | Replies to our emails |
| anything else | Email to other addresses |
| spam, auto-replies, bounces, our own mail | Filtered email (kept, archived, history only) |

A reply whose subject carries one of our references (`SUP-XXXXXX`) is marked with the request it answers.
Mailgun retries a forward it thinks failed; the message id is kept, so a retry is not stored twice.
Attachments are not kept: the notification says how many there were, and the original is in Mailgun's
stored messages for three days if the route stores it.

### Setup outside the app

1. **DNS (Mailgun receiving on `mail.sfluv.org`).** `mail.sfluv.org` already has `MX 10 mxa.mailgun.org`.
   Add the second record Mailgun asks for: `MX 10 mxb.mailgun.org`.
2. **Mailgun route.** Receiving → Routes → Create route:
   - Expression: *Match recipient* `.*@mail.sfluv.org`
   - Actions: *Forward* to `https://api.sfluv.org/mailgun/inbound`, *Store and notify* optional (keeps the
     original, attachments included, for three days), *Stop*.
3. **Signing key.** Copy the HTTP webhook signing key (Mailgun → Sending → Webhooks) into the backend's
   `MAILGUN_WEBHOOK_SIGNING_KEY`. Until it is set the endpoint answers 503 and Mailgun holds and retries
   the mail, so turning it on late loses nothing recent.
4. **support@ and admin@ (Google Workspace).** `sfluv.org` mail is Google's. Forward a copy of each to the
   receiving domain: `support@sfluv.org` → `support@mail.sfluv.org`, `admin@sfluv.org` →
   `admin@mail.sfluv.org`. A Workspace routing rule (Admin console → Apps → Google Workspace → Gmail →
   Routing → *Also deliver to*) needs no confirmation. Gmail's own forwarding sends a confirmation code
   to the forwarding address, which arrives in the hub under "Email to support@".

## Environment

- **New:** `MAILGUN_WEBHOOK_SIGNING_KEY` (see above).
- **No longer read** (remove them from the deployment; the hub's teams replace each one):
  `AFFILIATE_ADMIN_EMAIL`, `PROPOSER_ADMIN_EMAIL`, `IMPROVER_ADMIN_EMAIL`, `SUPERVISOR_ADMIN_EMAIL`,
  `ISSUER_ADMIN_EMAIL`, `MERCHANT_ADMIN_EMAIL`, `WORKFLOW_ADMIN_EMAIL`, `BULLETIN_ALERT_EMAIL`,
  `SITE_WITHDRAWAL_ALERT_EMAIL`, `W9_ALERT_EMAIL`, `FUNDING_ALERT_EMAILS`, `SUPPORT_CHAT_TECHNICAL_ID`,
  `SUPPORT_CHAT_ADMIN_ID`.
- **Unchanged:** `SUPPORT_INBOX_EMAIL` is still the Reply-To on our acknowledgement and W-9 emails, so
  replies to those reach support@ and, with step 4, the hub. `APP_BASE_URL` gives notifications their Open
  links.

## Adding a touchpoint

1. Add a topic constant and its catalog entry in `structs/notification_hub.go`, and list the topic on the
   touchpoint that posts it in `NotificationTouchpointCatalog()` (a new way in gets its own entry), so the
   visualizer draws it.
2. Post with `a.notify(ctx, Alert{Topic: ..., Key: ..., Kind: ...})`. Pick a key that is stable for the
   condition (one live alert per thing waiting), or unique per event with `Informational: true` for activity.
3. Retire it with `a.notifications.Resolve(ctx, key, outcome)` wherever it gets decided.

## Data

Migration 1.85 adds `notification_teams`, `notification_team_members`, `notification_routes`,
`internal_notifications.topic` and `.delivery`, and `support_requests.email_message_id`. The tables from the
old per-member sets, `organization_notification_subscriptions` and `support_category_subscriptions`, are left
in place and no longer read; nothing was copied from them.
