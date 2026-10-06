package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Queries behind the admin-editable site and Forms and Waivers. See
// docs/features/website-editing-and-forms.md.

var (
	ErrSiteNotFound  = errors.New("not found")
	ErrSiteSlugTaken = errors.New("that web address is already used by another form")
	// ErrSiteVersionConflict means someone else saved since the editor loaded.
	ErrSiteVersionConflict = errors.New("site content changed since it was loaded")
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// displayUser is the SQL for "who": a person's name, falling back to email.
const displayUser = `COALESCE(NULLIF(u.contact_name, ''), NULLIF(u.contact_email, ''), '')`

// ── Capabilities ─────────────────────────────────────────────────────────────

// UserHasActiveCredential reports whether a user holds a credential right now.
func (a *AppDB) UserHasActiveCredential(ctx context.Context, userId string, credentialType string) (bool, error) {
	var held bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_credentials
			WHERE user_id = $1 AND credential_type = $2 AND is_revoked = false
		);
	`, userId, credentialType).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("error checking credential: %s", err)
	}
	return held, nil
}

// ── Files ────────────────────────────────────────────────────────────────────

func (a *AppDB) CreateSiteFile(ctx context.Context, filename string, contentType string, width int, height int, data []byte, uploadedBy *string) (*structs.SiteFile, error) {
	file := &structs.SiteFile{
		Id:          uuid.NewString(),
		Filename:    filename,
		ContentType: contentType,
		SizeBytes:   len(data),
		Width:       width,
		Height:      height,
	}
	err := a.db.QueryRow(ctx, `
		INSERT INTO site_files (id, filename, content_type, size_bytes, width, height, data, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at;
	`, file.Id, filename, contentType, len(data), width, height, data, uploadedBy).Scan(&file.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("error storing site file: %s", err)
	}
	return file, nil
}

func (a *AppDB) GetSiteFileData(ctx context.Context, id string) (*structs.SiteFileData, error) {
	file := &structs.SiteFileData{}
	err := a.db.QueryRow(ctx, `
		SELECT id, filename, content_type, width, height, data FROM site_files WHERE id = $1;
	`, id).Scan(&file.Id, &file.Filename, &file.ContentType, &file.Width, &file.Height, &file.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading site file: %s", err)
	}
	return file, nil
}

// DeleteUnreferencedSiteFiles removes uploads older than the cutoff that nothing
// uses: no financial document, and no version of any site content (so a
// picture stays as long as a restorable version refers to it). This is what
// clears out abandoned uploads and replaced photos.
func (a *AppDB) DeleteUnreferencedSiteFiles(ctx context.Context, createdBefore int64) (int64, error) {
	tag, err := a.db.Exec(ctx, `
		DELETE FROM site_files f
		WHERE f.created_at < $1
			AND NOT EXISTS (SELECT 1 FROM financial_documents d WHERE d.file_id = f.id)
			AND NOT EXISTS (SELECT 1 FROM site_content_versions v WHERE strpos(v.value::text, f.id) > 0);
	`, createdBefore)
	if err != nil {
		return 0, fmt.Errorf("error deleting unused site files: %s", err)
	}
	return tag.RowsAffected(), nil
}

// GetSiteFile returns a file's metadata without its bytes.
func (a *AppDB) GetSiteFile(ctx context.Context, id string) (*structs.SiteFile, error) {
	file := &structs.SiteFile{}
	err := a.db.QueryRow(ctx, `
		SELECT id, filename, content_type, size_bytes, width, height, created_at FROM site_files WHERE id = $1;
	`, id).Scan(&file.Id, &file.Filename, &file.ContentType, &file.SizeBytes, &file.Width, &file.Height, &file.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading site file: %s", err)
	}
	return file, nil
}

// ── Singleton content (the banner) ───────────────────────────────────────────

type SiteContentRow struct {
	Value     []byte
	Version   int
	UpdatedAt int64
}

type SiteContentVersionRow struct {
	Version int
	Value   []byte
	SavedAt int64
	SavedBy string
	Note    string
}

// GetSiteContent returns nil, nil when the key has never been saved.
func (a *AppDB) GetSiteContent(ctx context.Context, key string) (*SiteContentRow, error) {
	row := &SiteContentRow{}
	err := a.db.QueryRow(ctx, `
		SELECT value, version, updated_at FROM site_content WHERE key = $1;
	`, key).Scan(&row.Value, &row.Version, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error loading site content: %s", err)
	}
	return row, nil
}

// SaveSiteContent stores a new version and makes it current. The upsert and
// the version number are one statement, so two simultaneous saves cannot claim
// the same version.
//
// expectedVersion is the version the editor started from; when it is not the
// current one, nothing is written and ErrSiteVersionConflict is returned, so
// two people editing at once cannot silently overwrite each other. A negative
// value skips the check (a restore is a deliberate overwrite).
func (a *AppDB) SaveSiteContent(ctx context.Context, key string, value []byte, userId *string, note string, expectedVersion int) (int, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("error starting transaction: %s", err)
	}
	defer tx.Rollback(ctx)

	if expectedVersion >= 0 {
		current := 0
		err := tx.QueryRow(ctx, `SELECT version FROM site_content WHERE key = $1 FOR UPDATE;`, key).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("error locking site content: %s", err)
		}
		if current != expectedVersion {
			return 0, ErrSiteVersionConflict
		}
	}

	var version int
	err = tx.QueryRow(ctx, `
		INSERT INTO site_content (key, value, version, updated_by, updated_at)
		VALUES ($1, $2::jsonb, 1, $3, unix_now())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			version = site_content.version + 1,
			updated_by = EXCLUDED.updated_by,
			updated_at = unix_now()
		RETURNING version;
	`, key, value, userId).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("error saving site content: %s", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO site_content_versions (key, version, value, saved_by, note)
		VALUES ($1, $2, $3::jsonb, $4, $5);
	`, key, version, value, userId, note); err != nil {
		return 0, fmt.Errorf("error recording site content version: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("error committing site content: %s", err)
	}
	return version, nil
}

func (a *AppDB) ListSiteContentVersions(ctx context.Context, key string, limit int) ([]*SiteContentVersionRow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT v.version, v.value, v.saved_at, `+displayUser+`, v.note
		FROM site_content_versions v
		LEFT JOIN users u ON u.id = v.saved_by
		WHERE v.key = $1
		ORDER BY v.version DESC
		LIMIT $2;
	`, key, limit)
	if err != nil {
		return nil, fmt.Errorf("error listing site content versions: %s", err)
	}
	defer rows.Close()

	versions := []*SiteContentVersionRow{}
	for rows.Next() {
		v := &SiteContentVersionRow{}
		if err := rows.Scan(&v.Version, &v.Value, &v.SavedAt, &v.SavedBy, &v.Note); err != nil {
			return nil, fmt.Errorf("error scanning site content version: %s", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (a *AppDB) GetSiteContentVersion(ctx context.Context, key string, version int) ([]byte, error) {
	var value []byte
	err := a.db.QueryRow(ctx, `
		SELECT value FROM site_content_versions WHERE key = $1 AND version = $2;
	`, key, version).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading site content version: %s", err)
	}
	return value, nil
}

// ── Financial documents ──────────────────────────────────────────────────────

const financialDocumentColumns = `
	d.id, d.kind, d.fiscal_year, d.period, d.as_of, d.label,
	d.file_id, d.external_url, f.filename, d.removed_at, d.created_at, d.updated_at
`

func scanFinancialDocument(row pgx.Row) (*structs.FinancialDocument, error) {
	doc := &structs.FinancialDocument{}
	var externalURL *string
	if err := row.Scan(
		&doc.Id, &doc.Kind, &doc.FiscalYear, &doc.Period, &doc.AsOf, &doc.Label,
		&doc.FileId, &externalURL, &doc.Filename, &doc.RemovedAt, &doc.CreatedAt, &doc.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if externalURL != nil {
		doc.Source = "legacy"
		doc.Href = *externalURL
	} else {
		doc.Source = "upload"
	}
	return doc, nil
}

// ListFinancialDocuments returns documents newest fiscal year first. Within a
// period the order is by kind, so a quarter always reads Activity, Cash Flows,
// Financial Position, then the rest.
func (a *AppDB) ListFinancialDocuments(ctx context.Context, includeRemoved bool) ([]*structs.FinancialDocument, error) {
	rows, err := a.db.Query(ctx, `
		SELECT `+financialDocumentColumns+`
		FROM financial_documents d
		LEFT JOIN site_files f ON f.id = d.file_id
		WHERE $1 OR d.removed_at IS NULL
		ORDER BY
			d.fiscal_year DESC,
			CASE d.period WHEN 'Q4' THEN 0 WHEN 'Q3' THEN 1 WHEN 'Q2' THEN 2 WHEN 'Q1' THEN 3 ELSE 4 END,
			CASE d.kind
				WHEN 'activity' THEN 0 WHEN 'cash_flows' THEN 1 WHEN 'financial_position' THEN 2
				WHEN 'activity_comparison' THEN 3 WHEN 'form_199n' THEN 4 WHEN 'form_990n' THEN 5
				WHEN 'impact_report' THEN 6 ELSE 7 END,
			d.created_at, d.id;
	`, includeRemoved)
	if err != nil {
		return nil, fmt.Errorf("error listing financial documents: %s", err)
	}
	defer rows.Close()

	docs := []*structs.FinancialDocument{}
	for rows.Next() {
		doc, err := scanFinancialDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning financial document: %s", err)
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (a *AppDB) GetFinancialDocument(ctx context.Context, id string) (*structs.FinancialDocument, error) {
	doc, err := scanFinancialDocument(a.db.QueryRow(ctx, `
		SELECT `+financialDocumentColumns+`
		FROM financial_documents d
		LEFT JOIN site_files f ON f.id = d.file_id
		WHERE d.id = $1;
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading financial document: %s", err)
	}
	return doc, nil
}

func (a *AppDB) CreateFinancialDocument(ctx context.Context, req *structs.FinancialDocumentRequest, label string, userId *string) (string, error) {
	id := uuid.NewString()
	_, err := a.db.Exec(ctx, `
		INSERT INTO financial_documents (id, kind, fiscal_year, period, as_of, label, file_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`, id, req.Kind, req.FiscalYear, req.Period, req.AsOf, label, req.FileId, userId)
	if err != nil {
		return "", fmt.Errorf("error creating financial document: %s", err)
	}
	return id, nil
}

// UpdateFinancialDocument edits a document's placement and label. A new file
// replaces the old one (and turns a legacy document into an upload); without
// one the existing document is kept.
func (a *AppDB) UpdateFinancialDocument(ctx context.Context, id string, req *structs.FinancialDocumentRequest, label string) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE financial_documents SET
			kind = $2, fiscal_year = $3, period = $4, as_of = $5, label = $6,
			file_id = CASE WHEN $7 <> '' THEN $7 ELSE file_id END,
			external_url = CASE WHEN $7 <> '' THEN NULL ELSE external_url END,
			updated_at = unix_now()
		WHERE id = $1;
	`, id, req.Kind, req.FiscalYear, req.Period, req.AsOf, label, req.FileId)
	if err != nil {
		return fmt.Errorf("error updating financial document: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}

func (a *AppDB) SetFinancialDocumentRemoved(ctx context.Context, id string, removed bool) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE financial_documents
		SET removed_at = CASE WHEN $2 THEN unix_now() ELSE NULL END, updated_at = unix_now()
		WHERE id = $1;
	`, id, removed)
	if err != nil {
		return fmt.Errorf("error updating financial document: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}

// ── Forms ────────────────────────────────────────────────────────────────────

// SiteFormStatus is the public-facing state of a form. A form is open only
// while its switch is on and any close date is still ahead.
func SiteFormStatus(isOpen bool, openedAt *int64, closesAt *int64, now int64) string {
	if isOpen && (closesAt == nil || *closesAt > now) {
		return "open"
	}
	if openedAt == nil {
		return "draft"
	}
	return "closed"
}

func defaultSiteFormConfig() structs.SiteFormConfig {
	return structs.SiteFormConfig{Contact: "optional", Event: "off", Choices: []structs.SiteFormChoice{}}
}

func parseSiteFormConfig(raw []byte) structs.SiteFormConfig {
	config := defaultSiteFormConfig()
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &config)
	}
	if config.Choices == nil {
		config.Choices = []structs.SiteFormChoice{}
	}
	return config
}

const siteFormSummaryColumns = `
	f.id, f.slug, f.kind, f.is_open, f.opened_at, f.closes_at, f.current_version, f.updated_at,
	v.title, v.summary,
	(SELECT COUNT(*) FROM site_form_signatures s WHERE s.form_id = f.id)
`

func scanSiteFormSummary(row pgx.Row, now int64) (*structs.SiteFormSummary, error) {
	form := &structs.SiteFormSummary{}
	var openedAt *int64
	if err := row.Scan(
		&form.Id, &form.Slug, &form.Kind, &form.IsOpen, &openedAt, &form.ClosesAt,
		&form.CurrentVersion, &form.UpdatedAt, &form.Title, &form.Summary, &form.SignatureCount,
	); err != nil {
		return nil, err
	}
	form.Status = SiteFormStatus(form.IsOpen, openedAt, form.ClosesAt, now)
	return form, nil
}

// ListSiteForms returns forms newest first. openOnly narrows to what the
// public may sign right now.
func (a *AppDB) ListSiteForms(ctx context.Context, openOnly bool) ([]*structs.SiteFormSummary, error) {
	now := time.Now().Unix()
	rows, err := a.db.Query(ctx, `
		SELECT `+siteFormSummaryColumns+`
		FROM site_forms f
		JOIN site_form_versions v ON v.form_id = f.id AND v.version = f.current_version
		WHERE f.archived_at IS NULL
			AND (NOT $1 OR (f.is_open AND (f.closes_at IS NULL OR f.closes_at > $2)))
		ORDER BY f.created_at DESC, f.id;
	`, openOnly, now)
	if err != nil {
		return nil, fmt.Errorf("error listing forms: %s", err)
	}
	defer rows.Close()

	forms := []*structs.SiteFormSummary{}
	for rows.Next() {
		form, err := scanSiteFormSummary(rows, now)
		if err != nil {
			return nil, fmt.Errorf("error scanning form: %s", err)
		}
		forms = append(forms, form)
	}
	return forms, rows.Err()
}

func (a *AppDB) getSiteFormSummary(ctx context.Context, where string, arg string) (*structs.SiteFormSummary, error) {
	now := time.Now().Unix()
	form, err := scanSiteFormSummary(a.db.QueryRow(ctx, `
		SELECT `+siteFormSummaryColumns+`
		FROM site_forms f
		JOIN site_form_versions v ON v.form_id = f.id AND v.version = f.current_version
		WHERE `+where+` AND f.archived_at IS NULL;
	`, arg), now)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading form: %s", err)
	}
	return form, nil
}

func (a *AppDB) GetSiteFormSummaryById(ctx context.Context, id string) (*structs.SiteFormSummary, error) {
	return a.getSiteFormSummary(ctx, "f.id = $1", id)
}

func (a *AppDB) GetSiteFormSummaryBySlug(ctx context.Context, slug string) (*structs.SiteFormSummary, error) {
	return a.getSiteFormSummary(ctx, "f.slug = $1", slug)
}

func scanSiteFormVersion(row pgx.Row) (*structs.SiteFormVersion, error) {
	v := &structs.SiteFormVersion{}
	var config []byte
	if err := row.Scan(&v.Version, &v.Title, &v.Summary, &v.Body, &config, &v.SavedAt, &v.SavedBy); err != nil {
		return nil, err
	}
	v.Config = parseSiteFormConfig(config)
	return v, nil
}

const siteFormVersionColumns = `
	v.version, v.title, v.summary, v.body, v.config, v.saved_at, ` + displayUser

func (a *AppDB) GetSiteFormVersion(ctx context.Context, formId string, version int) (*structs.SiteFormVersion, error) {
	v, err := scanSiteFormVersion(a.db.QueryRow(ctx, `
		SELECT `+siteFormVersionColumns+`
		FROM site_form_versions v
		LEFT JOIN users u ON u.id = v.saved_by
		WHERE v.form_id = $1 AND v.version = $2;
	`, formId, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading form version: %s", err)
	}
	return v, nil
}

func (a *AppDB) ListSiteFormVersions(ctx context.Context, formId string) ([]*structs.SiteFormVersion, error) {
	rows, err := a.db.Query(ctx, `
		SELECT `+siteFormVersionColumns+`
		FROM site_form_versions v
		LEFT JOIN users u ON u.id = v.saved_by
		WHERE v.form_id = $1
		ORDER BY v.version DESC;
	`, formId)
	if err != nil {
		return nil, fmt.Errorf("error listing form versions: %s", err)
	}
	defer rows.Close()

	versions := []*structs.SiteFormVersion{}
	for rows.Next() {
		v, err := scanSiteFormVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning form version: %s", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (a *AppDB) CreateSiteForm(ctx context.Context, req *structs.SiteFormRequest, userId *string) (string, error) {
	config, err := json.Marshal(req.Config)
	if err != nil {
		return "", fmt.Errorf("error encoding form config: %s", err)
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("error starting transaction: %s", err)
	}
	defer tx.Rollback(ctx)

	id := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO site_forms (id, slug, kind, current_version, created_by)
		VALUES ($1, $2, $3, 1, $4);
	`, id, req.Slug, req.Kind, userId); err != nil {
		if isUniqueViolation(err) {
			return "", ErrSiteSlugTaken
		}
		return "", fmt.Errorf("error creating form: %s", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO site_form_versions (form_id, version, title, summary, body, config, saved_by)
		VALUES ($1, 1, $2, $3, $4, $5::jsonb, $6);
	`, id, req.Title, req.Summary, req.Body, config, userId); err != nil {
		return "", fmt.Errorf("error creating form version: %s", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("error committing form: %s", err)
	}
	return id, nil
}

// AddSiteFormVersion saves an edit as a NEW version. It reports changed=false,
// and writes nothing, when the edit is identical to the current version, so an
// accidental double-click or a no-op save does not litter the history.
func (a *AppDB) AddSiteFormVersion(ctx context.Context, formId string, req *structs.SiteFormRequest, userId *string) (int, bool, error) {
	config, err := json.Marshal(req.Config)
	if err != nil {
		return 0, false, fmt.Errorf("error encoding form config: %s", err)
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("error starting transaction: %s", err)
	}
	defer tx.Rollback(ctx)

	var current int
	err = tx.QueryRow(ctx, `SELECT current_version FROM site_forms WHERE id = $1 FOR UPDATE;`, formId).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, ErrSiteNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("error locking form: %s", err)
	}

	var same bool
	if err := tx.QueryRow(ctx, `
		SELECT title = $3 AND summary = $4 AND body = $5 AND config = $6::jsonb
		FROM site_form_versions WHERE form_id = $1 AND version = $2;
	`, formId, current, req.Title, req.Summary, req.Body, config).Scan(&same); err != nil {
		return 0, false, fmt.Errorf("error comparing form versions: %s", err)
	}
	if same {
		return current, false, nil
	}

	next := current + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO site_form_versions (form_id, version, title, summary, body, config, saved_by)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7);
	`, formId, next, req.Title, req.Summary, req.Body, config, userId); err != nil {
		return 0, false, fmt.Errorf("error adding form version: %s", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE site_forms SET current_version = $2, updated_at = unix_now() WHERE id = $1;
	`, formId, next); err != nil {
		return 0, false, fmt.Errorf("error advancing form version: %s", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("error committing form version: %s", err)
	}
	return next, true, nil
}

func (a *AppDB) SetSiteFormOpen(ctx context.Context, id string, open bool, closesAt *int64) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE site_forms SET
			is_open = $2,
			opened_at = CASE WHEN $2 AND opened_at IS NULL THEN unix_now() ELSE opened_at END,
			closes_at = CASE WHEN $2 THEN $3::bigint ELSE closes_at END,
			updated_at = unix_now()
		WHERE id = $1 AND archived_at IS NULL;
	`, id, open, closesAt)
	if err != nil {
		return fmt.Errorf("error updating form: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}

// ── Signatures ───────────────────────────────────────────────────────────────

type SiteSignatureInsert struct {
	FormId               string
	Version              int
	SignerName           string
	Contact              string
	Answers              []byte
	IsMinor              bool
	GuardianName         string
	GuardianRelationship string
	SignaturePNG         []byte
	GuardianSignaturePNG []byte
	EsignConsent         bool
	TextSHA256           string
	ClientIP             string
	UserAgent            string
}

func (a *AppDB) CreateSiteFormSignature(ctx context.Context, s *SiteSignatureInsert) (string, int64, error) {
	id := uuid.NewString()
	var signedAt int64
	err := a.db.QueryRow(ctx, `
		INSERT INTO site_form_signatures (
			id, form_id, version, signer_name, contact, answers, is_minor,
			guardian_name, guardian_relationship, signature_png, guardian_signature_png,
			esign_consent, text_sha256, client_ip, user_agent
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING signed_at;
	`, id, s.FormId, s.Version, s.SignerName, s.Contact, s.Answers, s.IsMinor,
		s.GuardianName, s.GuardianRelationship, s.SignaturePNG, s.GuardianSignaturePNG,
		s.EsignConsent, s.TextSHA256, s.ClientIP, s.UserAgent).Scan(&signedAt)
	if err != nil {
		return "", 0, fmt.Errorf("error recording signature: %s", err)
	}
	return id, signedAt, nil
}

const siteSignatureColumns = `
	s.id, s.form_id, v.title, s.version, s.signer_name, s.contact, s.answers, s.is_minor,
	s.guardian_name, s.guardian_relationship,
	(s.signature_png IS NOT NULL), (s.guardian_signature_png IS NOT NULL),
	s.esign_consent, s.text_sha256, s.signed_at, s.client_ip, s.user_agent, s.withdrawn_at
`

func scanSiteSignature(row pgx.Row) (*structs.SiteSignatureRecord, error) {
	s := &structs.SiteSignatureRecord{}
	var answers []byte
	if err := row.Scan(
		&s.Id, &s.FormId, &s.FormTitle, &s.Version, &s.SignerName, &s.Contact, &answers, &s.IsMinor,
		&s.GuardianName, &s.GuardianRelationship, &s.HasSignature, &s.HasGuardianSignature,
		&s.EsignConsent, &s.TextSHA256, &s.SignedAt, &s.ClientIP, &s.UserAgent, &s.WithdrawnAt,
	); err != nil {
		return nil, err
	}
	s.Answers = json.RawMessage(answers)
	return s, nil
}

// ListSiteFormSignatures never loads signature images; those are fetched one
// record at a time.
func (a *AppDB) ListSiteFormSignatures(ctx context.Context, formId string) ([]*structs.SiteSignatureRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT `+siteSignatureColumns+`
		FROM site_form_signatures s
		JOIN site_form_versions v ON v.form_id = s.form_id AND v.version = s.version
		WHERE s.form_id = $1
		ORDER BY s.signed_at DESC, s.id
		LIMIT 5000;
	`, formId)
	if err != nil {
		return nil, fmt.Errorf("error listing signatures: %s", err)
	}
	defer rows.Close()

	signatures := []*structs.SiteSignatureRecord{}
	for rows.Next() {
		s, err := scanSiteSignature(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning signature: %s", err)
		}
		signatures = append(signatures, s)
	}
	return signatures, rows.Err()
}

// GetSiteFormSignature returns one record including its signature images.
func (a *AppDB) GetSiteFormSignature(ctx context.Context, id string) (*structs.SiteSignatureRecord, error) {
	s, err := scanSiteSignature(a.db.QueryRow(ctx, `
		SELECT `+siteSignatureColumns+`
		FROM site_form_signatures s
		JOIN site_form_versions v ON v.form_id = s.form_id AND v.version = s.version
		WHERE s.id = $1;
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error loading signature: %s", err)
	}

	var signature, guardian []byte
	if err := a.db.QueryRow(ctx, `
		SELECT signature_png, guardian_signature_png FROM site_form_signatures WHERE id = $1;
	`, id).Scan(&signature, &guardian); err != nil {
		return nil, fmt.Errorf("error loading signature images: %s", err)
	}
	if len(signature) > 0 {
		s.SignatureImage = "data:image/png;base64," + base64.StdEncoding.EncodeToString(signature)
	}
	if len(guardian) > 0 {
		s.GuardianSignatureImage = "data:image/png;base64," + base64.StdEncoding.EncodeToString(guardian)
	}
	return s, nil
}

func (a *AppDB) SetSiteSignatureWithdrawn(ctx context.Context, id string, withdrawn bool) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE site_form_signatures
		SET withdrawn_at = CASE WHEN $2 THEN unix_now() ELSE NULL END
		WHERE id = $1;
	`, id, withdrawn)
	if err != nil {
		return fmt.Errorf("error updating signature: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSiteNotFound
	}
	return nil
}

// ── Activity ─────────────────────────────────────────────────────────────────

func (a *AppDB) RecordSiteActivity(ctx context.Context, actor *string, capability string, action string, entity string, entityId string, summary string) {
	if _, err := a.db.Exec(ctx, `
		INSERT INTO site_activity (actor_user_id, capability, action, entity, entity_id, summary)
		VALUES ($1, $2, $3, $4, $5, $6);
	`, actor, capability, action, entity, entityId, strings.TrimSpace(summary)); err != nil {
		// The change itself already succeeded; losing its audit line must not
		// turn that into a failure for the person who made it.
		a.logger.Logf("error recording site activity: %s", err)
	}
}

func (a *AppDB) ListSiteActivity(ctx context.Context, capabilities []string, limit int) ([]*structs.SiteActivity, error) {
	rows, err := a.db.Query(ctx, `
		SELECT x.id, `+displayUser+`, x.capability, x.action, x.entity, x.entity_id, x.summary, x.at
		FROM site_activity x
		LEFT JOIN users u ON u.id = x.actor_user_id
		WHERE x.capability = ANY($1)
		ORDER BY x.at DESC, x.id DESC
		LIMIT $2;
	`, capabilities, limit)
	if err != nil {
		return nil, fmt.Errorf("error listing site activity: %s", err)
	}
	defer rows.Close()

	items := []*structs.SiteActivity{}
	for rows.Next() {
		item := &structs.SiteActivity{}
		if err := rows.Scan(&item.Id, &item.Actor, &item.Capability, &item.Action, &item.Entity, &item.EntityId, &item.Summary, &item.At); err != nil {
			return nil, fmt.Errorf("error scanning site activity: %s", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
