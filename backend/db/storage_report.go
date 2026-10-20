package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// imageStore names one place the app keeps image bytes in Postgres. The
// storage report sizes each of them so the move to object storage can start
// with the table that matters most. Identifiers here are constants, never
// request input.
type imageStore struct {
	// Database is "app" or "bot"; volunteer events live in the bot database.
	Database string
	Table    string
	Columns  []string
	// DatedBy is a SQL expression giving each row's timestamp, used to bucket
	// bytes by month. Most tables store unix seconds (unix_now()).
	DatedBy string
	Note    string
}

var imageStores = []imageStore{
	{Database: "app", Table: "workflow_submission_photos", Columns: []string{"photo_data"}, DatedBy: "to_timestamp(created_at)"},
	{Database: "app", Table: "workflow_step_photo_uploads", Columns: []string{"photo_data"}, DatedBy: "to_timestamp(created_at)"},
	{Database: "app", Table: "workflow_step_photo_upload_chunks", Columns: []string{"chunk_data"}, DatedBy: "to_timestamp(created_at)", Note: "temporary chunks of in-progress uploads"},
	{Database: "bot", Table: "event_photos", Columns: []string{"photo_data"}, DatedBy: "to_timestamp(created_at)"},
	{Database: "bot", Table: "event_blast_images", Columns: []string{"image_data"}, DatedBy: "to_timestamp(created_at)"},
	{Database: "app", Table: "site_files", Columns: []string{"data"}, DatedBy: "to_timestamp(created_at)", Note: "website CMS files, including past-event galleries"},
	{Database: "app", Table: "partners", Columns: []string{"logo_data"}, DatedBy: "to_timestamp(created_at)"},
	{Database: "app", Table: "location_icons", Columns: []string{"image_data"}, DatedBy: "updated_at", Note: "dated by last update, not first upload"},
	{Database: "app", Table: "location_photos", Columns: []string{"image_data"}, DatedBy: "updated_at", Note: "dated by last update, not first upload"},
	{Database: "app", Table: "credential_type_definitions", Columns: []string{"badge_data"}, DatedBy: "created_at"},
	{Database: "app", Table: "organizations", Columns: []string{"logo"}, DatedBy: "created_at", Note: "base64 data URL stored as text"},
	{Database: "app", Table: "affiliates", Columns: []string{"affiliate_logo"}, DatedBy: "created_at", Note: "base64 data URL stored as text"},
	{Database: "app", Table: "site_form_signatures", Columns: []string{"signature_png", "guardian_signature_png"}, DatedBy: "to_timestamp(signed_at)", Note: "legal records; planned to stay in Postgres"},
}

type StorageMonth struct {
	Month string `json:"month"`
	Rows  int64  `json:"rows"`
	Bytes int64  `json:"bytes"`
}

type ImageStoreUsage struct {
	Database        string         `json:"database"`
	Table           string         `json:"table"`
	Columns         []string       `json:"columns"`
	Note            string         `json:"note,omitempty"`
	Rows            int64          `json:"rows"`
	ImageBytes      int64          `json:"image_bytes"`
	ImageSize       string         `json:"image_size"`
	TableTotalBytes int64          `json:"table_total_bytes"`
	ByMonth         []StorageMonth `json:"by_month"`
	Missing         bool           `json:"missing,omitempty"`
}

type TableSize struct {
	Table      string `json:"table"`
	TotalBytes int64  `json:"total_bytes"`
	TotalSize  string `json:"total_size"`
}

type DatabaseSize struct {
	Database   string `json:"database"`
	TotalBytes int64  `json:"total_bytes"`
	TotalSize  string `json:"total_size"`
}

type StorageReport struct {
	GeneratedAt     string            `json:"generated_at"`
	Databases       []DatabaseSize    `json:"databases"`
	ImageBytesTotal int64             `json:"image_bytes_total"`
	ImageSizeTotal  string            `json:"image_size_total"`
	ImageStores     []ImageStoreUsage `json:"image_stores"`
	// LargestTables covers the app database, where nearly all bytes live.
	LargestTables []TableSize `json:"largest_tables"`
}

// StorageReport sizes the app and bot databases and every image column in
// them, with bytes added per month. Read-only. octet_length on bytea reads the
// stored length without fetching the bytes, so this stays cheap even on large
// tables.
func (a *AppDB) StorageReport(ctx context.Context, bot *BotDB) (*StorageReport, error) {
	var botPool *pgxpool.Pool
	if bot != nil {
		botPool = bot.db
	}
	return BuildStorageReport(ctx, a.db, botPool)
}

func BuildStorageReport(ctx context.Context, appPool, botPool *pgxpool.Pool) (*StorageReport, error) {
	report := &StorageReport{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Databases:     []DatabaseSize{},
		ImageStores:   []ImageStoreUsage{},
		LargestTables: []TableSize{},
	}

	appTx, err := readOnlyReportTx(ctx, appPool)
	if err != nil {
		return nil, err
	}
	defer appTx.Rollback(ctx)
	txs := map[string]pgx.Tx{"app": appTx}
	if botPool != nil {
		botTx, err := readOnlyReportTx(ctx, botPool)
		if err != nil {
			return nil, err
		}
		defer botTx.Rollback(ctx)
		txs["bot"] = botTx
	}

	for _, name := range []string{"app", "bot"} {
		tx, ok := txs[name]
		if !ok {
			continue
		}
		size := DatabaseSize{Database: name}
		if err := tx.QueryRow(ctx, `SELECT pg_database_size(current_database()), pg_size_pretty(pg_database_size(current_database()))`).
			Scan(&size.TotalBytes, &size.TotalSize); err != nil {
			return nil, fmt.Errorf("%s database size: %w", name, err)
		}
		report.Databases = append(report.Databases, size)
	}

	for _, store := range imageStores {
		tx, ok := txs[store.Database]
		if !ok {
			report.ImageStores = append(report.ImageStores, ImageStoreUsage{Database: store.Database, Table: store.Table, Columns: store.Columns, Note: store.Note, ByMonth: []StorageMonth{}, Missing: true})
			continue
		}
		usage, err := imageStoreUsage(ctx, tx, store)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", store.Database, store.Table, err)
		}
		report.ImageBytesTotal += usage.ImageBytes
		report.ImageStores = append(report.ImageStores, usage)
	}
	report.ImageSizeTotal = prettyBytes(report.ImageBytesTotal)

	rows, err := appTx.Query(ctx, `
		SELECT relname, pg_total_relation_size(relid), pg_size_pretty(pg_total_relation_size(relid))
		FROM pg_catalog.pg_statio_user_tables
		ORDER BY pg_total_relation_size(relid) DESC
		LIMIT 15
	`)
	if err != nil {
		return nil, fmt.Errorf("largest tables: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t TableSize
		if err := rows.Scan(&t.Table, &t.TotalBytes, &t.TotalSize); err != nil {
			return nil, err
		}
		report.LargestTables = append(report.LargestTables, t)
	}
	return report, rows.Err()
}

func readOnlyReportTx(ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '60s'"); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func imageStoreUsage(ctx context.Context, tx pgx.Tx, store imageStore) (ImageStoreUsage, error) {
	usage := ImageStoreUsage{Database: store.Database, Table: store.Table, Columns: store.Columns, Note: store.Note, ByMonth: []StorageMonth{}}

	// Skip stores whose table or column does not exist in this database
	// (older schemas, fresh dev databases) instead of failing the report.
	var present int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = ANY($2)
	`, store.Table, store.Columns).Scan(&present); err != nil {
		return usage, err
	}
	if present != len(store.Columns) {
		usage.Missing = true
		return usage, nil
	}

	if err := tx.QueryRow(ctx, `SELECT pg_total_relation_size($1::regclass)`, store.Table).Scan(&usage.TableTotalBytes); err != nil {
		return usage, err
	}

	lengths := make([]string, len(store.Columns))
	nonNull := make([]string, len(store.Columns))
	for i, col := range store.Columns {
		lengths[i] = fmt.Sprintf("COALESCE(octet_length(%s), 0)", pgx.Identifier{col}.Sanitize())
		nonNull[i] = fmt.Sprintf("%s IS NOT NULL", pgx.Identifier{col}.Sanitize())
	}
	query := fmt.Sprintf(`
		SELECT COALESCE(to_char(date_trunc('month', %s), 'YYYY-MM'), 'unknown') AS month,
		       COUNT(*), COALESCE(SUM(%s), 0)::bigint
		FROM %s
		WHERE %s
		GROUP BY 1
		ORDER BY 1
	`, store.DatedBy, strings.Join(lengths, " + "), pgx.Identifier{store.Table}.Sanitize(), strings.Join(nonNull, " OR "))

	rows, err := tx.Query(ctx, query)
	if err != nil {
		return usage, err
	}
	defer rows.Close()
	for rows.Next() {
		var m StorageMonth
		if err := rows.Scan(&m.Month, &m.Rows, &m.Bytes); err != nil {
			return usage, err
		}
		usage.Rows += m.Rows
		usage.ImageBytes += m.Bytes
		usage.ByMonth = append(usage.ByMonth, m)
	}
	if err := rows.Err(); err != nil {
		return usage, err
	}
	usage.ImageSize = prettyBytes(usage.ImageBytes)
	return usage, nil
}

func prettyBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
