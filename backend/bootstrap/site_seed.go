package bootstrap

// Seed data for migration 1.62. Kept apart from schema_migrations.go because it
// is long and is content rather than schema.

// siteLegacyFinancialDocuments is every document the public site's financials
// page listed when editing moved into the database. Generated from the site's
// src/content/financials.ts; ids are stable so re-running is a no-op.
const siteLegacyFinancialDocuments = `
				('legacy-fin-01', 'activity', 2026, 'Q4', '2026-06-30', '2026-06-30 Statement of Activity', '/assets/wp-content/uploads/2026/09/2026-06-30-FYE-Statement-of-Activity.pdf'),
				('legacy-fin-02', 'cash_flows', 2026, 'Q4', '2026-06-30', '2026-06-30 Statement of Cash Flows', '/assets/wp-content/uploads/2026/09/2026-06-30-FYE-Statement-of-Cash-Flows.pdf'),
				('legacy-fin-03', 'financial_position', 2026, 'Q4', '2026-06-30', '2026-06-30 Statement of Financial Position', '/assets/wp-content/uploads/2026/09/2026-06-30-FYE-Statement-of-Financial-Position.pdf'),
				('legacy-fin-04', 'activity_comparison', 2026, 'Q4', '2026-06-30', '2026-06-30 Statement of Activity Comparison', '/assets/wp-content/uploads/2026/09/2026-06-30-FYE-Statement-of-Activity-Comparison.pdf'),
				('legacy-fin-05', 'activity', 2026, 'Q3', '2026-03-31', '2026-03-31 Statement of Activity', '/assets/wp-content/uploads/2026/09/2026-03-31-Statement-of-Activity.pdf'),
				('legacy-fin-06', 'cash_flows', 2026, 'Q3', '2026-03-31', '2026-03-31 Statement of Cash Flows', '/assets/wp-content/uploads/2026/09/2026-03-31-Statement-of-Cash-Flows.pdf'),
				('legacy-fin-07', 'financial_position', 2026, 'Q3', '2026-03-31', '2026-03-31 Statement of Financial Position', '/assets/wp-content/uploads/2026/09/2026-03-31-Statement-of-Financial-Position.pdf'),
				('legacy-fin-08', 'activity', 2026, 'Q2', '2025-12-31', '2025-12-31 Statement of Activity', '/assets/wp-content/uploads/2026/02/SFLuv20StatementofActivity202025_12_31-2.pdf'),
				('legacy-fin-09', 'cash_flows', 2026, 'Q2', '2025-12-31', '2025-12-31 Statement of Cash Flows', '/assets/wp-content/uploads/2026/02/SFLuv20StatementofCashFlows202025_12_31.pdf'),
				('legacy-fin-10', 'financial_position', 2026, 'Q2', '2025-12-31', '2025-12-31 Statement of Financial Position', '/assets/wp-content/uploads/2026/02/SFLuv-StatementofFinancialPosition-2025_12_31.pdf-SFLuv-StatementofActivity-2025_12_31.pdf-SFLuv-StatementofCashFlows-2025_12_31.pdf'),
				('legacy-fin-11', 'activity', 2026, 'Q1', '2025-09-30', '2025-09-30 Statement of Activity', '/assets/wp-content/uploads/2026/01/2025-09-30-Statement-of-Activity-.pdf'),
				('legacy-fin-12', 'cash_flows', 2026, 'Q1', '2025-09-30', '2025-09-30 Statement of Cash Flows', '/assets/wp-content/uploads/2026/01/2025-09-30-Statement-of-Cash-Flows.pdf'),
				('legacy-fin-13', 'financial_position', 2026, 'Q1', '2025-09-30', '2025-09-30 Statement of Financial Position', '/assets/wp-content/uploads/2026/01/2025-09-30-Statement-of-Financial-Position.pdf'),
				('legacy-fin-14', 'activity', 2025, 'Q4', '2025-06-30', '2025-06-30 Statement of Activity', '/assets/wp-content/uploads/2025/07/2025_06_30-FYE-StatementofActivity.pdf'),
				('legacy-fin-15', 'cash_flows', 2025, 'Q4', '2025-06-30', '2025-06-30 Statement of Cash Flows', '/assets/wp-content/uploads/2025/07/2025_06_30-FYE_StatementofCashFlows.pdf'),
				('legacy-fin-16', 'financial_position', 2025, 'Q4', '2025-06-30', '2025-06-30 Statement of Financial Position', '/assets/wp-content/uploads/2025/07/2025_06_30-FYE-Statement-of-Financial-Position.pdf'),
				('legacy-fin-17', 'activity_comparison', 2025, 'Q4', '2025-06-30', '2025-06-30 Statement of Activity Comparison', '/assets/wp-content/uploads/2025/07/2025_06_30_Stmnt-of-Activity_Comparison.pdf'),
				('legacy-fin-18', 'form_199n', 2025, 'Q4', NULL, '2025 199N Confirmation', '/assets/wp-content/uploads/2025/09/FYE-2025_199N-Confirmation.pdf'),
				('legacy-fin-19', 'form_990n', 2025, 'Q4', NULL, '2025 990N', '/assets/wp-content/uploads/2025/09/FYE-2025_990N.pdf'),
				('legacy-fin-20', 'activity', 2025, 'Q3', '2025-03-31', '2025-03-31 Statement of Activity', '/assets/wp-content/uploads/2025/06/2025-03-31-Statement-of-Activity.pdf'),
				('legacy-fin-21', 'cash_flows', 2025, 'Q3', '2025-03-31', '2025-03-31 Statement of Cash Flows', '/assets/wp-content/uploads/2025/06/2025-03-31-Statement-of-Cash-Flows.pdf'),
				('legacy-fin-22', 'financial_position', 2025, 'Q3', '2025-03-31', '2025-03-31 Statement of Financial Position', '/assets/wp-content/uploads/2025/06/2025-03-31-Statement-of-Financial-Position.pdf'),
				('legacy-fin-23', 'activity', 2025, 'Q2', '2024-12-31', '2024-12-31 Statement of Activity', '/assets/wp-content/uploads/2025/06/2024-12-31-Statement-of-Activity-.pdf'),
				('legacy-fin-24', 'cash_flows', 2025, 'Q2', '2024-12-31', '2024-12-31 Statement of Cash Flows', '/assets/wp-content/uploads/2025/06/2024-12-31-Statement-of-Cash-Flows.pdf'),
				('legacy-fin-25', 'financial_position', 2025, 'Q2', '2024-12-31', '2024-12-31 Statement of Financial Position', '/assets/wp-content/uploads/2025/06/2024-12-31-Statement-of-Financial-Position.pdf'),
				('legacy-fin-26', 'activity', 2024, 'FULL', '2024-06-30', '2024-06-30 Statement of Activity', '/assets/wp-content/uploads/2024/11/2024-06-30-Statement-of-Activity.pdf'),
				('legacy-fin-27', 'cash_flows', 2024, 'FULL', '2024-06-30', '2024-06-30 Statement of Cash Flows', '/assets/wp-content/uploads/2024/11/2024-06-30-Statement-of-Cash-Flows.pdf'),
				('legacy-fin-28', 'financial_position', 2024, 'FULL', '2024-06-30', '2024-06-30 Statement of Financial Position', '/assets/wp-content/uploads/2024/11/2024-06-30-Statement-of-Financial-Position.pdf'),
				('legacy-fin-29', 'form_199n', 2024, 'FULL', NULL, '2024 199N Confirmation', '/assets/wp-content/uploads/2025/09/FYE-2024_199N-Confirmation.pdf'),
				('legacy-fin-30', 'form_990n', 2024, 'FULL', NULL, '2024 990N', '/assets/wp-content/uploads/2025/09/FYE-2024_990N.pdf'),
				('legacy-fin-31', 'impact_report', 2026, 'FULL', NULL, '2025–2026 Annual Impact Report', '/assets/wp-content/uploads/2026/09/SFLuv-Annual-Impact-Report-2025-2026.pdf')
`

// siteSeedSpotlight is the homepage carousel the site shows today (the slides in
// the site's src/content/spotlight.ts), expressed as stored content, so moving
// the site onto the database changes nothing visitors see.
const siteSeedSpotlight = `{
	"slides": [
		{
			"id": "impact-report-2025-2026",
			"enabled": true,
			"label": "Now published",
			"title": "Our 2025–2026 Annual Impact Report",
			"body": "A look back at our first year of operations in San Francisco's Tenderloin.",
			"image_file_id": null,
			"image_url": "/assets/announcements/impact-report-2025-2026-photo.jpg",
			"image_width": 1050,
			"image_height": 817,
			"image_alt": "Five SFLuv volunteers in safety vests holding litter grabbers on a Tenderloin sidewalk",
			"image_position": "center 8%",
			"action": {
				"label": "Read the report",
				"href": "/financials-and-reports#annual-impact-reports",
				"new_tab": false,
				"also_open_file_id": null,
				"also_open_url": "/assets/wp-content/uploads/2026/09/SFLuv-Annual-Impact-Report-2025-2026.pdf"
			},
			"event_match": ""
		},
		{
			"id": "weekly-cleanup",
			"enabled": true,
			"label": "Every Sunday",
			"title": "Tenderloin Weekly Clean-up",
			"body": "Clean up the neighborhood with us and earn SFLuv to spend at local shops.",
			"image_file_id": null,
			"image_url": "/assets/wp-content/uploads/2026/01/Tenderloin-Weekly-Cleanup-._001_20240703164325983424_20250215031301212105.jpg",
			"image_width": 944,
			"image_height": 494,
			"image_alt": "Volunteers in safety vests at the Tenderloin weekly cleanup",
			"image_position": "",
			"action": {
				"label": "Join the clean-up",
				"href": "/volunteers",
				"new_tab": false,
				"also_open_file_id": null,
				"also_open_url": null
			},
			"event_match": "weekly clean"
		}
	]
}`

type siteSeedForm struct {
	id      string
	slug    string
	kind    string
	title   string
	summary string
	body    string
	config  string
}

// siteSeedForms are the two forms every deployment starts with, closed. The
// media release wording is the paper "SFLuv Media Consent and Release" verbatim.
// {{choices}} marks where the scope choices appear in the body.
var siteSeedForms = []siteSeedForm{
	{
		id:      "seed-media-release",
		slug:    "media-release",
		kind:    "waiver",
		title:   "Media Consent and Release",
		summary: "Photography, video, audio, and statements.",
		body: `## Consent

I authorize SFLuv and its authorized representatives to photograph, film, and record me, including my image, likeness, voice, statements, and participation. I authorize SFLuv to edit, reproduce, publish, display, distribute, and otherwise use those recordings in any media, including websites, social media, Reels, YouTube, presentations, printed materials, and other community outreach, educational, fundraising, or promotional materials.

I understand that SFLuv may edit the recordings for length, clarity, format, or presentation, but will not intentionally use them in a materially misleading way. I understand that I will not receive payment or royalties for these uses unless SFLuv and I agree otherwise in writing.

## Choose the scope of your consent

{{choices}}

If I select ongoing consent, I may withdraw it for future recordings by giving SFLuv written notice through sfluv.org. Withdrawal will not require SFLuv to remove materials already published or distributed before the notice was received.

## Release and acknowledgment

I release SFLuv and its authorized representatives from claims arising from the uses authorized by this form, including claims based on privacy or publicity rights. This release does not authorize unlawful use. I have read this form, understand it, and voluntarily agree to it.`,
		config: `{
			"contact": "optional",
			"preferred_name": true,
			"event": "ask",
			"event_name": "",
			"choices_prompt": "",
			"choices": [
				{"id": "event_only", "label": "This event/project only.", "description": "My consent covers recordings made in connection with the event or project listed above."},
				{"id": "ongoing", "label": "Ongoing SFLuv participation.", "description": "My consent also covers recordings made during my future participation in SFLuv programs and events."}
			],
			"guardian_section": true,
			"confirmation_message": "Thank you. Your signed release has been recorded, and you can close this page."
		}`,
	},
	{
		id:      "seed-withdraw-media-consent",
		slug:    "withdraw-media-consent",
		kind:    "withdrawal",
		title:   "Withdraw Media Consent",
		summary: "Withdraw ongoing consent to be recorded at future SFLuv events.",
		body: `I previously signed the SFLuv Media Consent and Release and chose ongoing SFLuv participation. I withdraw that consent for recordings made from now on.

I understand that withdrawing does not require SFLuv to remove materials that were already published or distributed before this notice was received.`,
		config: `{
			"contact": "required",
			"preferred_name": false,
			"event": "off",
			"event_name": "",
			"choices_prompt": "",
			"choices": [],
			"guardian_section": true,
			"confirmation_message": "Thank you. Your withdrawal has been recorded."
		}`,
	},
}
