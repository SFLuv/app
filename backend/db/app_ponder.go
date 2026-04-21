package db

import (
	"context"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppDB) AddPonderSubscription(ctx context.Context, s *structs.PonderSubscription) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO ponder_subscriptions(
			id,
			address,
			type,
			owner,
			data
		) VALUES (
			$1,
			LOWER($2),
			$3,
			$4,
			$5
		)
		ON CONFLICT (id) DO UPDATE
		SET
			address = EXCLUDED.address,
			type = EXCLUDED.type,
			owner = EXCLUDED.owner,
			data = EXCLUDED.data,
			active = TRUE,
			delete_date = NULL,
			delete_reason = NULL;
	`, s.Id, s.Address, s.Type, s.Owner, s.Data)

	return err
}

func (a *AppDB) GetPonderSubscriptions(ctx context.Context, address string) ([]*structs.PonderSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			address,
			type,
			owner,
			data
		FROM
			ponder_subscriptions
		WHERE
			address = LOWER($1)
		AND
			active = TRUE
	`, address)
	if err != nil {
		return nil, fmt.Errorf("error querying for ponder subscriptions for address %s: %s", address, err)
	}

	var subscriptions []*structs.PonderSubscription
	for rows.Next() {
		var subscription structs.PonderSubscription
		err = rows.Scan(
			&subscription.Id,
			&subscription.Address,
			&subscription.Type,
			&subscription.Owner,
			&subscription.Data,
		)
		if err != nil {
			a.logger.Logf("error scanning row into subscription struct: %s", err)
			continue
		}

		subscriptions = append(subscriptions, &subscription)
	}

	return subscriptions, nil
}

func (a *AppDB) GetPonderSubscriptionsByUser(ctx context.Context, userDid string) ([]*structs.PonderSubscription, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			address,
			type,
			owner,
			data
		FROM
			ponder_subscriptions
		WHERE
			owner = $1
		AND
			active = TRUE
	`, userDid)
	if err != nil {
		return nil, fmt.Errorf("error querying for ponder subscriptions for userDid %s: %s", userDid, err)
	}

	var subscriptions []*structs.PonderSubscription
	for rows.Next() {
		var subscription structs.PonderSubscription
		err = rows.Scan(
			&subscription.Id,
			&subscription.Address,
			&subscription.Type,
			&subscription.Owner,
			&subscription.Data,
		)
		if err != nil {
			a.logger.Logf("error scanning row into subscription struct: %s", err)
			continue
		}

		subscriptions = append(subscriptions, &subscription)
	}

	return subscriptions, nil
}

func (a *AppDB) GetPonderSubscription(ctx context.Context, id int) (*structs.PonderSubscription, error) {
	rows := a.db.QueryRow(ctx, `
		SELECT
			id,
			address,
			type,
			owner,
			data
		FROM
			ponder_subscriptions
		WHERE
			id = $1
		AND
			active = TRUE
	`, id)

	var subscription structs.PonderSubscription
	err := rows.Scan(
		&subscription.Id,
		&subscription.Address,
		&subscription.Type,
		&subscription.Owner,
		&subscription.Data,
	)
	if err != nil {
		return nil, err
	}

	return &subscription, nil
}

func (a *AppDB) DeletePonderSubscription(ctx context.Context, id int, owner string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE
			ponder_subscriptions
		SET
			active = FALSE,
			delete_date = $3,
			delete_reason = $4
		WHERE
			id = $1
		AND
			owner = $2
		AND
			active = TRUE;
	`, id, owner, time.Now().UTC().Add(accountDeletionGracePeriod), deleteReasonPonderDelete)

	return err
}
