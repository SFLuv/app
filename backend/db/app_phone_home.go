package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/structs"
)

// RecordClientPhoneHome increments the per-UTC-day aggregate hit counter for an
// anonymous app phone-home (a /config or /client-version fetch). It is safe to
// call on every request: a single upsert into a small, bounded-cardinality
// table keyed by (day, endpoint, platform, version, build).
func (a *AppDB) RecordClientPhoneHome(ctx context.Context, metric structs.ClientPhoneHome) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO client_phone_home_metrics(
			day,
			endpoint,
			platform,
			version,
			build,
			hits,
			first_seen_at,
			last_seen_at
		)
		VALUES ((NOW() AT TIME ZONE 'UTC')::date, $1, $2, $3, $4, 1, NOW(), NOW())
		ON CONFLICT (day, endpoint, platform, version, build)
		DO UPDATE SET
			hits = client_phone_home_metrics.hits + 1,
			last_seen_at = NOW();
	`,
		metric.Endpoint,
		metric.Platform,
		metric.Version,
		metric.Build,
	)
	if err != nil {
		return fmt.Errorf("error recording client phone home: %s", err)
	}

	return nil
}
