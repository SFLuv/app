package db

import (
	"context"
	"fmt"
)

// PonderHookReference is one place our database claims a Ponder webhook exists.
//
// Two tables hold hook ids and they hold them differently: a merchant
// subscription's own primary key IS its hook id, while a push subscription
// stores the id in a column. Both are represented here so a single pass can
// compare everything we believe against what Ponder actually has.
type PonderHookReference struct {
	HookID  int
	Address string
	Owner   string
	Token   string
	Source  string
	Active  bool
}

const (
	PonderHookSourceMerchant = "merchant"
	PonderHookSourcePush     = "push"
)

// GetPonderHookReferences lists every hook id our database expects to exist.
func (a *AppDB) GetPonderHookReferences(ctx context.Context) ([]PonderHookReference, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id AS hook_id,
			address,
			'' AS owner,
			'' AS token,
			'merchant' AS source,
			active
		FROM
			ponder_subscriptions
		UNION ALL
		SELECT
			ponder_hook_id AS hook_id,
			address,
			owner,
			token,
			'push' AS source,
			active
		FROM
			mobile_push_subscriptions
		WHERE
			ponder_hook_id IS NOT NULL
		ORDER BY
			hook_id ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying ponder hook references: %w", err)
	}
	defer rows.Close()

	references := make([]PonderHookReference, 0)
	for rows.Next() {
		var reference PonderHookReference
		if err := rows.Scan(
			&reference.HookID,
			&reference.Address,
			&reference.Owner,
			&reference.Token,
			&reference.Source,
			&reference.Active,
		); err != nil {
			return nil, fmt.Errorf("error scanning ponder hook reference: %w", err)
		}
		references = append(references, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ponder hook references: %w", err)
	}

	return references, nil
}

// ClearMobilePushSubscriptionPonderHookIDs drops hook ids that point at hooks
// Ponder no longer has.
//
// Clearing matters beyond tidiness: the sync path skips creating a hook when a
// subscription already carries an id, so an id left pointing at a deleted hook
// permanently suppresses hook creation for that address — the subscription looks
// healthy and no notification ever arrives.
func (a *AppDB) ClearMobilePushSubscriptionPonderHookIDs(ctx context.Context, hookIDs []int) (int64, error) {
	if len(hookIDs) == 0 {
		return 0, nil
	}

	tag, err := a.db.Exec(ctx, `
		UPDATE mobile_push_subscriptions
		SET
			ponder_hook_id = NULL,
			updated_at = NOW()
		WHERE
			ponder_hook_id = ANY($1);
	`, hookIDs)
	if err != nil {
		return 0, fmt.Errorf("error clearing dangling ponder hook ids: %w", err)
	}

	return tag.RowsAffected(), nil
}
