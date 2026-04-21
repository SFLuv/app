# Workflow Security Hardening (Current Scope)

## Scope
- Implement the pending security hardening for workflow payouts and submission-data access controls discussed earlier.

## Implemented
- Added payout in-flight locking at DB level to prevent duplicate sends under concurrency:
  - `workflow_steps.payout_in_progress`
  - `workflows.manager_payout_in_progress`
- Added DB claim methods for payout attempts:
  - `ClaimWorkflowStepPayoutAttempt(...)`
  - `ClaimWorkflowManagerPayoutAttempt(...)`
- Updated payout processor flow to claim lock **before** any transfer is attempted.
- Updated payout failure/success writes to clear in-progress flags safely.
- Updated payout retry requests to reject retries while a payout is in progress.
- Updated workflow completion transitions to reset payout in-progress flags.
- Updated workflow loading queries/structs to include in-progress payout fields (internal-only, not exposed in API JSON).
- Updated payout target selection to skip records already in payout progress.
- Added admin alert path for post-transfer DB state-update failures (manual intervention scenario, duplicate-send-safe).
- Submission data redaction and photo access restrictions remain enforced to relevant parties only.
- Added admin manual payout-lock resolution endpoint with persisted audit records:
  - `POST /admin/workflows/{workflow_id}/payout-lock-resolution`
  - request fields:
    - `target_type`: `step` or `supervisor`
    - `action`: `mark_paid_out` or `mark_failed`
    - `step_id`: required for `target_type=step`
    - `error_message`: optional for `mark_failed`
  - every action is written to `workflow_payout_admin_actions` with admin actor and timestamp.

## Endpoint Examples
- Mark a stuck step payout lock as paid out:
  - `{"target_type":"step","action":"mark_paid_out","step_id":"<step-id>"}`
- Mark a stuck step payout lock as failed:
  - `{"target_type":"step","action":"mark_failed","step_id":"<step-id>","error_message":"manual reconciliation required"}`
- Mark a stuck supervisor payout lock as paid out:
  - `{"target_type":"supervisor","action":"mark_paid_out"}`
- Mark a stuck supervisor payout lock as failed:
  - `{"target_type":"supervisor","action":"mark_failed","error_message":"transfer uncertain, unlocked for retry"}`

## Validation
- `GOCACHE=/tmp/go-build go test -vet=off ./db ./handlers ./router ./structs` passes.

## Follow-Up
- Locked workflow edit proposals so recurrence, recurrence end date, and start date/time can no longer be changed during edit mode. The proposer UI now shows them as read-only, and the backend always reuses the existing series schedule when building an edit proposal.
- Investigated workflow edit behavior before improver claims. Root issue: approved edit proposals update `workflow_series.current_state_id` but do not retarget the existing unclaimed workflow instance or rebuild its copied roles/steps, so the live workflow stays on the old state while the series points at the new one. Secondary frontend footgun: template application clears `editProposalWorkflowId`, which would fall the shared form back to create mode and produce a genuinely new series if used during edit mode.

- Backend now explicitly rejects workflow edit proposal payloads that include `start_at`, `recurrence`, or `recurrence_end_at`, instead of silently ignoring them.
- Backend startup has been split so DB initialization/migrations now live at `backend/cmd/init/main.go`, while the normal server boot path lives at `backend/cmd/server/main.go` and assumes schema setup has already been handled.
- Applying a template during workflow edit mode now preserves `editProposalWorkflowId` and the locked schedule fields, so template use no longer falls the proposer form back into create mode.
- Restyled the proposer edit-mode notice to a neutral card treatment and shortened the copy.
- Moved the proposer workflow modal edit action to the top-right action area and moved the improver live camera capture button below the camera preview.
- Moved the proposer workflow modal `Save as Template` action into the same top action area as `Edit Workflow`.
- Repositioned the proposer workflow modal actions so they sit right-aligned above the roles section instead of beside the title.
- Restored the proposer modal `Edit Workflow` button to its filled styling while keeping the new placement above the roles section.

- Voter panel now supports admin force approval for workflow edit and deletion proposals, and edit/deletion proposal cards are clickable to open redacted detail previews in the workflow modal.
- Moved redeemer role sync, minter role sync, and Privy linked-email sync out of cmd/server startup and into cmd/init, so normal server boot no longer blocks on those startup-wide synchronization passes.
- Updated the voter active-workflow deletion proposal button so pending proposals now show `Workflow Deletion Proposed` or `Series Deletion Proposed` and remain disabled.
- Added proposer-side pending deletion proposal awareness so the deletion button now disables and switches to `Workflow Deletion Proposed` or `Series Deletion Proposed` once a matching deletion proposal already exists.
- Fixed proposer deletion-button matching so pending deletion proposals are detected by either matching series id or workflow id first, then the button label uses the actual pending proposal target type instead of relying only on the current inferred deletion target.
- Enforced vote-view notify-email redaction in the voter panel modal by stripping dropdown option `notify_emails` and preserving only `notify_email_count` for workflow, edit-proposal, and deletion-proposal detail views.
- Moved vote-view notify-email redaction into backend voter routes by adding a voter-specific workflow detail endpoint and explicit vote-view sanitization for voter workflow lists/details and edit proposal payloads.
- Smart wallet initialization now deploys code on the client during sign-in whenever an initialized smart wallet is still undeployed, and newly created smart wallets (signup/manual add) now require deployment to succeed before the wallet row is finalized.
- Diagnosed payout regression risk around the 3/14 merge window: workflow edit/state-version changes do not directly suppress payout execution, but the merge-era payout destination resolver now prefers primary rewards accounts / smart wallet index 0, so payouts may be landing at a different address than before; app logs currently show no workflow payout failure entries for this incident, but do show repeated unpaid-workflow query errors that would hide pending/failed payout visibility in the improver UI.
- Stronger payout diagnosis: the series payout processor stops on any earlier workflow status outside deleted/rejected/expired/paid_out/completed. After the recurrence/blocking changes, stale predecessors can now be left as blocked or converted to skipped, and skipped/failed are treated as terminal elsewhere but not by payout processing. That means an edited or catch-up-adjusted series can strand later completed payouts without logging a transfer failure, because the processor returns before it reaches the payable workflow.
- Workflow series payout processing now refreshes workflow availability/blocked-state repair before walking the series, and now treats skipped/failed predecessor workflows as terminal so they no longer strand payouts for later completed workflows in the same series.
- Proposer panel loading was refactored so create-form data and workflow-list data load independently, tab switches no longer use Next router navigation for query syncing, already-loaded tab data stays visible while background refreshes run, and proposer workflow cards now come from a lightweight summary query instead of full workflow hydration on every list fetch.
- Proposer workflow lookups were further tightened with a composite partial index on `workflows(proposer_id, created_at DESC) WHERE status <> 'deleted'`, and the proposer workflow-list query now counts votes with a per-workflow lateral subquery instead of aggregating the entire `workflow_votes` table on each request.
- Workflow edit approval now treats title/description as retroactive series-level presentation fields: approval still updates `workflow_series.title/description` from the proposed state, and it now also syncs those two fields onto workflow-state rows currently linked to existing workflows in the series so historical workflows immediately reflect the approved rename/re-description without rewriting unrelated proposal-only states.
- Added a user-level primary wallet setting for all users in the account settings tab, backed by `users.primary_wallet_address`; wallet dropdowns now only list smart wallets and use an `Other` manual-address path for EOAs/custom addresses, with frontend/backend EVM address validation.
- Improver and supervisor rewards-account selectors now default to the user’s top-level primary wallet when they do not already have an explicit role-specific rewards account saved, without overwriting any existing role-specific wallet settings.
- Redeem payout address canonicalization now prefers a user’s saved top-level primary wallet and only falls back to smart wallet index `0` when no top-level primary wallet is set, so app-wide “default wallet” behavior no longer hardcodes the nonce-0 smart wallet.
- Added wallet visibility controls in settings so users can hide individual wallets from the wallets page; wallet records now carry `is_hidden`, the settings page manages that flag, and the wallets page filters hidden wallets while still allowing them to remain usable elsewhere in the app.
- Updated the wallets page card presentation to remove the EOA/smart-wallet type badge and instead show a `Primary Wallet` indicator when a listed wallet matches the user’s top-level primary wallet setting.
- Tightened the default primary-wallet fallback so blank users now default to the nonce-0 smart wallet derived from the first managed/primary Privy EOA when available, with backend fallback prioritizing the first EOA-backed nonce-0 smart wallet and sign-in now backfilling `users.primary_wallet_address` best-effort when it is still blank.
- Reordered the settings account tab so `Request Role Access` now appears above the primary-wallet and wallet-visibility controls.
- Added a per-dropdown-option `Send pictures with email` checkbox in workflow/template creation/editing, persisted it through workflow/template payloads, and wired dropdown alert emails to attach that step submission’s uploaded photos when enabled.
- Dropdown-alert photo attachments are now resized to email-friendly JPEGs (long edge capped at 1280px) before sending so notification emails do not include raw full-resolution uploads.
- Diagnosed supervisor photo-download quality complaints to a supervisor-specific export path that was decoding and re-encoding every stored upload as JPEG even though the improver upload pipeline already normalizes photos to JPEG under the 2MB limit, causing avoidable second-generation quality loss and metadata stripping.
- Fixed supervisor photo ZIP downloads to preserve the original stored JPEG bytes whenever the uploaded photo is already JPEG, while still keeping JPEG export fallback for non-JPEG legacy rows, and raised live camera capture requests to ask mobile browsers for a higher-resolution source stream so future phone captures are less likely to start from a low-res frame.
- Added a step-header bounty pill in the proposer workflow form so each step's collapse control now shows the currently specified SFLuv bounty even while the step is collapsed.
- Confirmed recurrence end date was already present in the workflow create form, then re-enabled it for edit mode by allowing proposer edit drafts to send recurrence_end_at changes while keeping recurrence and start_at locked; frontend and backend validation still require any specified end date to be on or after the workflow start.
- Locked workflow edit eligibility once a workflow has ended: recurring series with a passed recurrence end date can no longer be edited or have their end date changed, and one-time workflows can no longer be edited once they reach a terminal ended status; the proposer UI now hides edit access accordingly and the backend enforces the same rule.
- Made workflow recurrence end date controls explicit in the proposer form by replacing the subtle checkbox treatment with a visible 'Schedule End' block and helper text in both create and edit mode, so it is clear where to set an end date for recurring workflows.
- Workflow edit proposals can now change start time while keeping the original local start date locked, and approved time edits are persisted in workflow state/version records, applied to future scheduled workflow rows only, and used by recurring generation logic to skip past slots so new workflows are not created in the past or on overlapping timestamps.
- Workflow templates now preserve only the chosen start time-of-day, not the source workflow’s calendar date: template saves send just the time, template storage keeps that time in a date-free encoded form, and applying a template reuses the saved time against the current date in the proposer form instead of replaying an old date.
- Workflow response photos now preserve more resolution end to end: live camera capture requests higher camera resolution, photo encoding uses higher-quality sizing/quality selection under the 2MB limit, compatible uploads under the size limit are no longer needlessly re-encoded, supervisor exports preserve original bytes where possible, and dropdown notification emails now send direct `/photos/{photo_id}` links instead of low-res attachments via a new public no-login photo page and public photo asset route.
- Dropdown workflow options can now require a photo attachment when selected, with proposer-configurable per-option photo instructions. The proposer form saves those settings into workflow/template definitions, workflow detail views display them, and improver completion enforces the extra photo requirement only for the selected option while clearing hidden stale photos when switching away from a photo-required option.
- Supervisor CSV/photo exports now use the exporting browser’s local timezone for formatted datetime values, include an `export_timezone` CSV column, and stamp the timezone into exported photo filenames.
- Workflow deletion proposals now match the existing zero-payout shortcut behavior used by create/edit proposals: if the workflow series’ proposer and supervisor are the same user, the deletion proposal is immediately approved without entering the voter queue.
- Same proposer/supervisor workflows can now self-apply edit proposals without voter approval when the edit leaves payout amounts unchanged; the comparison keeps supervisor bounty fixed and compares the set of positive step bounty amounts while leaving the existing immediate-delete shortcut in place.
- Improver live camera capture now degrades gracefully: the app tries the higher-resolution camera constraints first, then falls back through smaller resolutions and finally environment-camera-without-size constraints, and capture processing now retries at smaller output sizes if a full-resolution frame is too large to render/process on the device.
- The improver camera fallback still preserves workflow-specified photo aspect ratios: preview framing remains bound to the workflow’s square/vertical/horizontal setting, capture output is cropped to that aspect on save, and camera constraint fallbacks now also carry an aspect-ratio hint so framing stays closer to the requested shape on devices that support it.
- Workflow step photo submission is now transport-aware: the frontend estimates base64 JSON payload size for step completion, selectively recompresses the largest uploads only when the full step payload would exceed a safe threshold, and the backend now caps step-completion request bodies and returns a clear oversized-upload error instead of failing ambiguously.
- Restyled the proposer step bounty indicator from a custom red badge to a neutral compact bounty capsule that matches the current card/border/text styling language more closely.
- Tightened the “save workflow as template” proposer path so supervisor assignment and supervisor data fields are copied from the actual workflow fields present on the detail payload, instead of depending only on the `supervisor_required` flag.
- Recurring improver claims now only allow steps that can unlock immediately: the improver workflow board no longer treats future generated series instances as claimable just because they are locked, and claiming a currently eligible locked step now promotes it straight to `available` on the backend so past-due recurring work can be completed as soon as it is claimed.
- Recurring series continuity now advances one interval at a time, chooses the latest occurrence at or before now when catching up, and prunes accidental future unclaimed workflow rows when an earlier current occurrence is still unclaimed. This prevents recurring series from skipping straight to an upcoming workflow instead of stopping at the current claimable one.
- Recurring catch-up now also collapses older missed unclaimed occurrences to `skipped` while preserving the latest currently-fillable occurrence. That keeps daily/weekly/monthly series compressed to the newest actionable workflow instead of leaving every prior missed occurrence visible.
- Restored recurring overdue-claim skipping: once a claimed recurring workflow has aged past its recurrence window, continuity catch-up now skips it and advances the series again. Only the newest currently-fillable occurrence is preserved; claimed overdue occurrences are no longer held open indefinitely.
- Proposer workflow form reset now also clears the selected template id, so after submitting a workflow or otherwise resetting the draft the template dropdown no longer shows a stale template selection that is no longer actually applied.
- Proposer numeric inputs are now protected against accidental mouse-wheel changes: supervisor payout, step bounty, and other number fields blur on wheel so scrolling the page can no longer silently alter payout amounts.
- Claiming a workflow step now assigns the improver to every other step in that workflow with the same role, and recurring series claim propagation/unclaiming now treats those same-role step orders as a bundle. The old one-step-per-workflow assignment index was replaced with a non-unique lookup index, so `cmd/init` must run to apply the schema/index change before this behavior works against an existing database.
- Workflow start-time edits no longer fail when the new time would make a previously generated future workflow fall into the past. Instead, those invalid future rows are retired, and if that leaves the series without a future row the recurrence engine regenerates the next valid occurrence from the new time anchor without touching currently active workflows.
- Workflow dropdown alert emails now use the title/subject `Workflow Alert`, no longer include the workflow id in the email body, and photo links in those emails now render as `View {item title}` based on the work item the photo came from.
- Workflow step photo submission now uses a two-step upload flow: improver photo bytes are first uploaded to a step-scoped multipart endpoint (`POST /improvers/workflows/{workflow_id}/steps/{step_id}/photos`) that validates the assigned improver, workflow, step, and item before storing a temporary photo record, and final step completion now submits only `photo_ids` in JSON. This avoids mobile `Load failed` issues from large base64 JSON bodies while preventing the endpoint from being abused as generic image storage. `cmd/init` must run to create the new `workflow_step_photo_uploads` table.
- Larger workflow photo uploads are now chunked on the client before they hit that step-scoped upload endpoint. The backend stages chunk sessions/chunks per `workflow_id` + `step_id` + `item_id` + improver and only finalizes the photo once all chunks arrive and reassemble under the same 2MB post-assembly limit. `cmd/init` must also create `workflow_step_photo_upload_sessions` and `workflow_step_photo_upload_chunks`.
- Tightened the mobile completion flow further by uploading step photos sequentially instead of in parallel and by treating post-success feed/detail refreshes as non-fatal once the backend has already confirmed step completion, so transient mobile follow-up fetch failures no longer masquerade as submit failures.
- The improver workflow modal now shows a chunk-upload progress bar during photo transfer and renders the submission success state as a styled banner at the top of the modal, with a larger app-consistent confirmation treatment and a prominent `Done` action after the backend confirms step completion.
- Improver panel tab switching now mirrors the proposer panel behavior: tabs update the URL with `window.history.replaceState` instead of router navigation, each tab loads only the data it needs, already-loaded tab data stays on screen during background refreshes, and first-time tab visits show local tab-level loaders instead of retriggering the page-wide loading state.

- Moved improver chunked-upload progress UI into the step action area so it appears where the submit button normally sits.

- Allowed public workflow photo routes under /photos and /photo to bypass unauthenticated redirect-to-map behavior in AppProvider.

- Workflow dropdown alert email photo links now use the actual source step item title, number repeated photos per item, and HTML-escape those labels before rendering.

- Restyled submitted-step modal view and collapsed submitted details behind a dropdown under the submitted indicator for a cleaner single-screen mobile first view.

- Workflow submission success state now appears both in the modal header and in the step action area where the submit button normally sits.

- Mirrored proposer workflow submit success messages above the bottom submit actions, while keeping the existing top-of-page success banner.

- Switched improver submission-complete and submitted-state cards to dark-mode-safe success styling instead of hardcoded light emerald gradients.

- Added root-level SVG previews for the current light and dark improver submission-complete cards: submission-complete-preview-light.svg and submission-complete-preview-dark.svg.

- Switched improver submission-complete cards and regenerated the root SVG previews to use the SFLuv red accent palette in both light and dark mode.

- Kept submission-complete cards on the SFLuv red palette while switching the success checkmark icon circles back to green in UI and SVG previews.

- Signup no longer hard-fails on smart-wallet deployment; new wallet rows are saved first, then smart-wallet code deployment is attempted best-effort afterward. Wallet creation now ignores client-supplied ids on add.

- Wallet nav shortcut now prefers the saved primary wallet when it matches a stored wallet, and only falls back to smart wallet 0 for custom/manual primary wallet addresses.

- Wallet sidebar tab now points directly to the saved stored primary wallet across the app, not just on mobile; custom/manual primary addresses still fall back to smart wallet 0.

- Restored wallet-tab shortcut behavior to mobile-only; desktop continues to open the wallet list while mobile still targets the saved stored primary wallet when available.
- Workflow step bounties now pay out as soon as the individual step is completed, while workflow-level supervisor payout and full workflow finalization still wait until the overall workflow is complete; the improver unpaid-workflows query now includes in-progress workflows with completed unpaid steps.
- Admin improver modals now show each improver's payout wallet address and active credential list, with credential ids rendered by label on the frontend.
- Authenticated clients now refresh their `/users` record in the background (on focus, when the tab becomes visible, and on a short interval), so newly approved roles appear on the frontend without requiring the user to log out and back in.
- Android workflow photo capture now falls back to a hidden `capture=environment` camera input, camera open/capture failures stay local to the step UI, canceled picker opens no longer clear existing photos, and camera/photo processing errors no longer risk wiping the improver workflow modal state.
- Admins can now use the proposer panel `Your Workflows` tab as an all-workflows view with pagination, title/description search, proposer filtering, admin-wide workflow detail access, and admin edit/save-template actions for any workflow, while deletion/archive controls stay limited to workflows the current proposer actually owns.
- Workflow proposal creation now uses a review-before-submit modal in the proposer panel: clicking the create-mode submit button validates the draft, opens the existing workflow details modal with an in-memory preview of the proposed workflow, and only the modal's Confirm Submission action sends the create request.
- Audited list/query scaling across handlers and DB paths: added shared page/count caps on older admin/public handlers, batched location-hours loading to remove N+1 queries from location lists, batched issuer-scope loading for issuer admin lists, tightened several user-scoped queries with ordering/limits, and added a `v1.1` schema migration batch with supporting indexes for locations, contacts, wallets, workflow templates, issuer scopes, and bot event/code lookups.
- Workflow edit hydration now fetches a fresh workflow detail payload with `include_notify_emails=true` before entering edit mode, so admins editing someone else's workflow and proposers editing their own workflows keep dropdown notification emails intact instead of re-submitting a redacted version that clears them.
- Dropdown-triggered workflow photo requirements can now also require a live photo: the flag is saved through create/edit/template paths, shown in workflow details, and switches improver completion into the existing camera-only capture flow when that option is selected.
- Merchant wallet settings in the settings page now auto-save direct actions: selecting a payment wallet adds it immediately, default/remove actions save immediately, tipping wallet changes save immediately, custom addresses commit on blur/Enter, and the panel layout was simplified for cleaner mobile-friendly reading.
- The merchant payment-wallet add selector now resets to an empty `Select wallet` placeholder after each add instead of holding onto the previous choice, preventing sticky-selection bugs in the auto-add flow.
- The merchant payment-wallet selector now uses a non-empty internal placeholder value for the `Select wallet` state, avoiding Radix empty-value errors while still resetting cleanly after each add.
- Map/list location-type filters now drop empty location types before building Select items, preventing Radix from rendering a `SelectItem` with `value=\"\"` when a location record has a blank type.
- Removed the sidebar `Merchant Status` entry and moved merchant status visibility into the settings merchant tab. Approved locations now render first as default-collapsed location settings cards that combine real location-specific profile editing with payment/tip wallet routing, while pending/rejected location indicators remain below those cards and approved locations no longer show the old green status panels.
- Merchant settings location-profile address editing now mirrors merchant onboarding: approved location cards lazy-load Google Places search, re-selecting a place refreshes canonical Google-backed fields (including address components, lat/lng, maps page, and related place metadata), and the saved payload updates those fields through the normal location update route while keeping street manually editable.
- Improver panel background polling is now silent after the initial tab load, so recurring refreshes no longer inject temporary loading rows that shift the page. The unpaid workflows tab now also highlights payout errors more clearly and adds a workflow-level `Retry Failed Payouts` action alongside the per-step retry button.
- Workflow payout processing now runs on its own bounded background context instead of the request context, stale payout locks are auto-recovered into retryable payout errors, and completed workflows with only payout errors no longer block later workflows in the same series from continuing through payout processing.
- Workflow payouts now persist the token transfer tx hash immediately after submission, retry requests reconcile that hash against on-chain SFLUV `Transfer` events before any resend, already-complete payouts are finalized instead of retried, and still-pending payout transactions are blocked from duplicate resend attempts.

- Added frontend nonce-based CSP/security headers plus env-driven CSP allowlist hooks, enabled Privy captcha/preserved MFA prompts, and locked backend API CORS to explicit origins with localhost defaults when not in production.

- CSP origin allowlist now also accepts NEXT_PUBLIC_FRONTEND_URL so production app-host calls can be pinned explicitly alongside backend origins.

- Removed script-src strict-dynamic from the frontend CSP because Next-managed _next asset loads were being blocked as blocked:csp without a fully propagated nonce pipeline.

- CSP connect-src now explicitly allows vercel.live and *.vercel.live over https and wss so Vercel-hosted live/runtime calls do not get blocked in deployed environments.

- CSP hardening follow-up: vercel.live feedback.js needed to be allowed under script-src, not just connect-src. Also made the App Router root layout request-bound via headers() so middleware-provided nonces can propagate to Next framework scripts in production, avoiding blank-page failures from blocked inline/bootstrap scripts.

- Passed the middleware CSP nonce through RootLayout into next-themes ThemeProvider so its inline theme bootstrap script is permitted under script-src without needing unsafe-inline.

- CSP connect-src now also allows configured chain RPC origins (NEXT_PUBLIC_CHAIN_RPC_URL, NEXT_PUBLIC_ENGINE_URL, NEXT_PUBLIC_ALCHEMY_TRANSFERS_BASE_URL) plus safe Berachain/Alchemy defaults so browser-side viem/paymaster calls are not blocked.

- Merchant location wallet auto-save now reverts the draft back to the persisted DB-backed location state on save failure, so tip/payment wallet selectors cannot continue to look saved when the backend update actually failed.

- Merchant tipping now prompts on the initial send confirmation screen: the optional tip amount is entered there, the old second-step tip prompt screen is removed, and any entered tip is sent automatically as a separate follow-up transfer after the main payment succeeds.

- Proposer panel loading banners are now gone: background refreshes in Create Workflow and Your Workflows run silently, and first-load tab placeholders use a centered spinner without loading text so the UI no longer shifts down.

- Merchant tipping detection is restored in the wallet send flow: tip prompts are now derived reactively from QR link tip targets and merchant wallet lookups, so tipping still appears in all prior cases while staying on the initial confirmation screen.

- Wallet send manual entry now includes approved merchant location payment accounts alongside contacts in the recipient autocomplete, and selecting a merchant location also carries its tip wallet so the tip prompt appears correctly on confirm.

- Merchant tipping prompt now happens only after the main payment succeeds: tip detection is unchanged, but the initial confirm screen no longer asks for a tip and the post-send success screen now offers optional tipping with safe retry behavior that does not resend the main payment.

- Ponder transaction alert emails now format token amounts with integer-safe base-unit conversion instead of low-precision big.Float math, preventing slight balance drift in emailed SFLuv amounts.

- Event QR PDF export now fetches all event code pages before batching, so large events are no longer capped by the first 100-code page when admins or affiliates download QR PDFs.
- Server boot now starts a daily deleted-account purge loop: it runs once on startup, then once per day after that, while still relying on the existing 30-day `delete_date` gate so only eligible soft-deleted accounts are purged.
- Proposer panel polling is now explicit and silent: background 30-second refreshes no longer piggyback on app-level auth refreshes, and workflow-list polling no longer toggles visible loading state or jolt the workflow edit form.
