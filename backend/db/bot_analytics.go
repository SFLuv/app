package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
)

func (s *BotDB) GetAnalyticsVolunteerEvents(ctx context.Context) ([]*structs.AnalyticsVolunteerEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			e.id,
			COALESCE(e.title, ''),
			COALESCE(e.amount, 0),
			COALESCE(e.start_at, 0),
			COALESCE(e.expiration, 0),
			COUNT(c.id)::int AS code_count,
			COUNT(c.id) FILTER (WHERE COALESCE(c.redeemed, FALSE) = TRUE)::int AS redeemed_count,
			COUNT(DISTINCT LOWER(r.address)) FILTER (WHERE r.address IS NOT NULL AND TRIM(r.address) <> '')::int AS unique_earners,
			COALESCE(STRING_AGG(DISTINCT LOWER(TRIM(r.address)), ','), '') AS earner_addresses
		FROM
			events e
		LEFT JOIN
			codes c
		ON
			c.event = e.id
		LEFT JOIN
			redemptions r
		ON
			r.event = e.id
		GROUP BY
			e.id
		ORDER BY
			COALESCE(e.start_at, e.expiration, 0) ASC,
			e.id ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics volunteer events: %w", err)
	}
	defer rows.Close()

	events := make([]*structs.AnalyticsVolunteerEvent, 0)
	for rows.Next() {
		var event structs.AnalyticsVolunteerEvent
		var earnerAddresses string
		if err := rows.Scan(
			&event.ID,
			&event.Title,
			&event.Amount,
			&event.StartAt,
			&event.Expiration,
			&event.CodeCount,
			&event.RedeemedCount,
			&event.UniqueEarners,
			&earnerAddresses,
		); err != nil {
			return nil, fmt.Errorf("error scanning analytics volunteer event: %w", err)
		}
		if strings.TrimSpace(earnerAddresses) != "" {
			event.EarnerAddresses = strings.Split(earnerAddresses, ",")
		}
		events = append(events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics volunteer events: %w", err)
	}

	return events, nil
}

func (s *BotDB) GetAnalyticsVolunteerParticipationCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			LOWER(TRIM(address)) AS address,
			COUNT(*)::int AS participation_count
		FROM
			redemptions
		WHERE
			address IS NOT NULL
		AND
			TRIM(address) <> ''
		GROUP BY
			LOWER(TRIM(address));
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying analytics volunteer participation counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var address string
		var count int
		if err := rows.Scan(&address, &count); err != nil {
			return nil, fmt.Errorf("error scanning analytics volunteer participation count: %w", err)
		}
		counts[address] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analytics volunteer participation counts: %w", err)
	}

	return counts, nil
}
