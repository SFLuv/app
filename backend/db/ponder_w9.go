package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *PonderDB) GetPaidTotalForWalletYear(ctx context.Context, wallet string, year int, adminAddresses []string) (string, error) {
	if len(adminAddresses) == 0 {
		return "0", nil
	}

	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()

	row := p.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(t.amount), 0)::text
		FROM
			transfer_event t
		WHERE
			t.to = LOWER($1)
		AND
			LOWER(t.from) = ANY($2)
		AND
			t.timestamp >= $3
		AND
			t.timestamp < $4;
	`, wallet, adminAddresses, start, end)

	var total string
	err := row.Scan(&total)
	if err == pgx.ErrNoRows {
		return "0", nil
	}
	if err != nil {
		return "", fmt.Errorf("error getting ponder total for wallet %s: %s", wallet, err)
	}

	return total, nil
}
