# Improver/Proposer Touchups - Current Work

## Scope
- Proposer template flow and template management touchups.
- Workflow item notification-email UX/validation improvements.
- Photo requirement enhancements (required count, any-count option, aspect ratio).
- Improver/admin submitted-photo preview UX and modal viewing.
- Success/error messaging improvements in proposer/improver flows.

## Implementation Notes
- Completed: proposer template picker now auto-applies on selection; explicit apply button removed.
- Completed: admin can delete default templates from the proposer template library.
- Completed: non-admin template save CTA now reads `Save Template`.
- Completed: pending notification email input auto-includes on save/submit when valid; malformed emails now raise contextual errors (step/item/option).
- Completed: workflow photo requirements extended with required photo count, allow-any-count toggle, and aspect ratio (`vertical`, `square`, `horizontal`) across frontend/backend.
- Completed: improver capture/upload flow now enforces configured aspect ratio and backend photo-count rules.
- Completed: photo thumbnails + click-to-preview modal added for improver local uploads, and submitted response photos in workflow detail views.
- Completed: proposer success banners added for template saves, workflow proposal creation, and template deletion.
- Completed: proposer success toasts now pop up for template saves, workflow proposal creation, and template deletion.
- Completed: improver step submission validation errors now render just above the `Complete Step` button.
- Completed: improver modal `Unclaim series` action now moves beneath the modal title on mobile to avoid overlap/closeness with the close button.
- Completed: modal series arrows now match card behavior (no wrap-around, disabled at bounds, hidden for non-repeatable workflows).
- Completed: proposer `Your Workflows` now renders as series-based cards with per-series card navigation.
- Completed: proposer workflow details modal can now run in a non-tabbable step view (all steps shown, no step pager) and can hide all submission/response data.
- Completed: proposer workflow details modal now supports a bottom action area used for `Save as Template`.
- Completed: proposers can now save a template from an approved workflow via `Save as Template` in the details modal, with a follow-up modal for template title/description.
- Completed: backend now redacts workflow step submission data unless requester is one of: admin, workflow supervisor, an improver assigned to any step in that workflow, or an improver who submitted data on that workflow.
- Completed: workflow photo fetch authorization now matches submission-data policy (removed proposer/voter read access unless they otherwise qualify).
- Verified: current workflow summary/list endpoints (`/workflows/active`, admin workflow listings) do not carry step submission payloads, and all workflow-detail responses are sanitized with the same submission visibility rules.
- Completed: proposer `Save As Template` modal now shows template-save errors directly under the save action area, instead of only in the page-level error banner.
- Completed: proposer `Your Workflows` cards no longer include series paging arrow controls.
- Completed: proposer form errors now clear automatically when switching tabs or when any create-workflow form value is changed.
