package db

// SiteSchemaDDL creates the tables behind the admin-editable parts of the
// public site (banner, financial documents) and the Forms and Waivers section.
// Purely additive. See docs/features/website-editing-and-forms.md.
const SiteSchemaDDL = `
	-- Public uploads: PDFs and images used on the site. Bytes live in Postgres,
	-- the same pattern as partner logos. Signatures are NOT stored here: this
	-- table is served to anyone who has an id.
	CREATE TABLE IF NOT EXISTS site_files(
		id TEXT PRIMARY KEY,
		filename TEXT NOT NULL,
		content_type TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0,
		data BYTEA NOT NULL,
		uploaded_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		created_at BIGINT NOT NULL DEFAULT unix_now()
	);

	-- Singleton documents edited as a whole (today: the homepage banner).
	-- Versions are append-only; restoring writes a NEW version, so history is
	-- never rewritten.
	CREATE TABLE IF NOT EXISTS site_content(
		key TEXT PRIMARY KEY,
		value JSONB NOT NULL,
		version INTEGER NOT NULL,
		updated_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		updated_at BIGINT NOT NULL DEFAULT unix_now()
	);

	CREATE TABLE IF NOT EXISTS site_content_versions(
		id BIGSERIAL PRIMARY KEY,
		key TEXT NOT NULL,
		version INTEGER NOT NULL,
		value JSONB NOT NULL,
		saved_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		saved_at BIGINT NOT NULL DEFAULT unix_now(),
		note TEXT NOT NULL DEFAULT '',
		UNIQUE (key, version)
	);

	-- fiscal_year is the year the fiscal year ENDS (2026 = FYE June 30, 2026).
	-- A document is either an upload (file_id) or a legacy file already hosted
	-- by the site (external_url, a relative path) — never both, never neither.
	-- removed_at is a soft delete, so removing a document is undoable.
	CREATE TABLE IF NOT EXISTS financial_documents(
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL CHECK (kind IN (
			'activity', 'cash_flows', 'financial_position', 'activity_comparison',
			'form_990n', 'form_199n', 'impact_report', 'other'
		)),
		fiscal_year INTEGER NOT NULL,
		period TEXT NOT NULL CHECK (period IN ('Q1', 'Q2', 'Q3', 'Q4', 'FULL')),
		as_of TEXT,
		label TEXT NOT NULL,
		file_id TEXT REFERENCES site_files(id) ON DELETE RESTRICT,
		external_url TEXT,
		removed_at BIGINT,
		created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		created_at BIGINT NOT NULL DEFAULT unix_now(),
		updated_at BIGINT NOT NULL DEFAULT unix_now(),
		CHECK ((file_id IS NOT NULL) <> (external_url IS NOT NULL))
	);

	CREATE INDEX IF NOT EXISTS financial_documents_year_idx
		ON financial_documents(fiscal_year DESC, period, created_at);

	CREATE TABLE IF NOT EXISTS site_forms(
		id TEXT PRIMARY KEY,
		slug TEXT NOT NULL UNIQUE,
		kind TEXT NOT NULL DEFAULT 'waiver' CHECK (kind IN ('waiver', 'withdrawal')),
		is_open BOOLEAN NOT NULL DEFAULT FALSE,
		opened_at BIGINT,
		closes_at BIGINT,
		current_version INTEGER NOT NULL DEFAULT 1,
		archived_at BIGINT,
		created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		created_at BIGINT NOT NULL DEFAULT unix_now(),
		updated_at BIGINT NOT NULL DEFAULT unix_now()
	);

	-- Immutable. Editing a form adds a version; it never changes one in place,
	-- which is what keeps every signature tied to the exact words signed.
	CREATE TABLE IF NOT EXISTS site_form_versions(
		form_id TEXT NOT NULL REFERENCES site_forms(id) ON DELETE CASCADE,
		version INTEGER NOT NULL,
		title TEXT NOT NULL,
		summary TEXT NOT NULL DEFAULT '',
		body TEXT NOT NULL,
		config JSONB NOT NULL DEFAULT '{}'::jsonb,
		saved_by TEXT REFERENCES users(id) ON DELETE SET NULL,
		saved_at BIGINT NOT NULL DEFAULT unix_now(),
		PRIMARY KEY (form_id, version)
	);

	CREATE TABLE IF NOT EXISTS site_form_signatures(
		id TEXT PRIMARY KEY,
		form_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		signer_name TEXT NOT NULL,
		contact TEXT NOT NULL DEFAULT '',
		answers JSONB NOT NULL DEFAULT '{}'::jsonb,
		is_minor BOOLEAN NOT NULL DEFAULT FALSE,
		guardian_name TEXT NOT NULL DEFAULT '',
		guardian_relationship TEXT NOT NULL DEFAULT '',
		signature_png BYTEA,
		guardian_signature_png BYTEA,
		esign_consent BOOLEAN NOT NULL,
		text_sha256 TEXT NOT NULL,
		signed_at BIGINT NOT NULL DEFAULT unix_now(),
		client_ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		withdrawn_at BIGINT,
		FOREIGN KEY (form_id, version) REFERENCES site_form_versions(form_id, version)
	);

	CREATE INDEX IF NOT EXISTS site_form_signatures_form_idx
		ON site_form_signatures(form_id, signed_at DESC);

	-- Append-only trail shown as "Recent changes" in the admin UI.
	CREATE TABLE IF NOT EXISTS site_activity(
		id BIGSERIAL PRIMARY KEY,
		actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
		capability TEXT NOT NULL,
		action TEXT NOT NULL,
		entity TEXT NOT NULL,
		entity_id TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL,
		at BIGINT NOT NULL DEFAULT unix_now()
	);

	CREATE INDEX IF NOT EXISTS site_activity_at_idx ON site_activity(at DESC);
`
