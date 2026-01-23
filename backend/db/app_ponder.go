package db

import (
	"context"
	"fmt"

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
		);
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
		DELETE FROM
			ponder_subscriptions
		WHERE
			id = $1 AND owner = $2;
	`, id, owner)

	return err
}
