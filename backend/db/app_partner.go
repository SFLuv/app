package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
)

// PartnerRow is the stored partner, without logo bytes. Handlers turn it into
// structs.Partner by attaching a logo URL.
type PartnerRow struct {
	Id         string
	Name       string
	LinkURL    string
	HasLogo    bool
	LogoWidth  int
	LogoHeight int
	Position   int
	Active     bool
	CreatedAt  int64
	UpdatedAt  int64
}

const partnerColumns = `
	id,
	name,
	link_url,
	(logo_data IS NOT NULL) AS has_logo,
	logo_width,
	logo_height,
	position,
	active,
	created_at,
	updated_at
`

func scanPartnerRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*PartnerRow, error) {
	partners := []*PartnerRow{}
	for rows.Next() {
		partner := &PartnerRow{}
		if err := rows.Scan(
			&partner.Id,
			&partner.Name,
			&partner.LinkURL,
			&partner.HasLogo,
			&partner.LogoWidth,
			&partner.LogoHeight,
			&partner.Position,
			&partner.Active,
			&partner.CreatedAt,
			&partner.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning partner row: %s", err)
		}
		partners = append(partners, partner)
	}
	return partners, rows.Err()
}

// GetPartners lists partners in display order. activeOnly is what the public
// carousel endpoint uses; the admin panel lists everything so a deactivated
// partner can be edited and brought back.
func (a *AppDB) GetPartners(ctx context.Context, activeOnly bool) ([]*PartnerRow, error) {
	query := `
		SELECT ` + partnerColumns + `
		FROM partners
	`
	if activeOnly {
		// A partner with no logo would render as a hole in the strip, so the
		// public list requires one.
		query += ` WHERE active = TRUE AND logo_data IS NOT NULL `
	}
	query += ` ORDER BY position ASC, created_at ASC;`

	rows, err := a.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error querying partners: %s", err)
	}
	defer rows.Close()

	return scanPartnerRows(rows)
}

func (a *AppDB) GetPartner(ctx context.Context, id string) (*PartnerRow, error) {
	partner := &PartnerRow{}
	err := a.db.QueryRow(ctx, `
		SELECT `+partnerColumns+`
		FROM partners
		WHERE id = $1;
	`, id).Scan(
		&partner.Id,
		&partner.Name,
		&partner.LinkURL,
		&partner.HasLogo,
		&partner.LogoWidth,
		&partner.LogoHeight,
		&partner.Position,
		&partner.Active,
		&partner.CreatedAt,
		&partner.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return partner, nil
}

func (a *AppDB) CreatePartner(ctx context.Context, req *structs.PartnerRequest) (*PartnerRow, error) {
	id := uuid.NewString()
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	// New partners land at the end of the strip unless placed explicitly.
	position := 0
	if req.Position != nil {
		position = *req.Position
	} else {
		if err := a.db.QueryRow(ctx, `
			SELECT COALESCE(MAX(position), -1) + 1 FROM partners;
		`).Scan(&position); err != nil {
			return nil, fmt.Errorf("error computing partner position: %s", err)
		}
	}

	if _, err := a.db.Exec(ctx, `
		INSERT INTO partners (id, name, link_url, position, active)
		VALUES ($1, $2, $3, $4, $5);
	`, id, strings.TrimSpace(req.Name), strings.TrimSpace(req.LinkURL), position, active); err != nil {
		return nil, fmt.Errorf("error inserting partner: %s", err)
	}

	return a.GetPartner(ctx, id)
}

func (a *AppDB) UpdatePartner(ctx context.Context, id string, req *structs.PartnerRequest) (*PartnerRow, error) {
	if _, err := a.db.Exec(ctx, `
		UPDATE partners
		SET
			name = $2,
			link_url = $3,
			active = COALESCE($4, active),
			position = COALESCE($5, position),
			updated_at = unix_now()
		WHERE id = $1;
	`, id, strings.TrimSpace(req.Name), strings.TrimSpace(req.LinkURL), req.Active, req.Position); err != nil {
		return nil, fmt.Errorf("error updating partner: %s", err)
	}

	return a.GetPartner(ctx, id)
}

func (a *AppDB) DeletePartner(ctx context.Context, id string) error {
	_, err := a.db.Exec(ctx, `DELETE FROM partners WHERE id = $1;`, id)
	if err != nil {
		return fmt.Errorf("error deleting partner: %s", err)
	}
	return nil
}

// SetPartnerLogo replaces a partner's logo. Dimensions are supplied by the
// handler, which decodes them from the uploaded bytes.
func (a *AppDB) SetPartnerLogo(ctx context.Context, id string, data []byte, contentType string, width int, height int) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE partners
		SET
			logo_data = $2,
			logo_content_type = $3,
			logo_width = $4,
			logo_height = $5,
			logo_updated_at = unix_now(),
			updated_at = unix_now()
		WHERE id = $1;
	`, id, data, contentType, width, height)
	if err != nil {
		return fmt.Errorf("error storing partner logo: %s", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("partner not found")
	}
	return nil
}

func (a *AppDB) GetPartnerLogo(ctx context.Context, id string) (*StoredPhoto, error) {
	logo := &StoredPhoto{}
	var data []byte
	err := a.db.QueryRow(ctx, `
		SELECT logo_data, logo_content_type
		FROM partners
		WHERE id = $1;
	`, id).Scan(&data, &logo.ContentType)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("partner has no logo")
	}
	logo.Data = data
	return logo, nil
}

// ReorderPartners applies a full ordering in one statement, so a drag that
// moves several rows cannot interleave with another admin's reorder.
func (a *AppDB) ReorderPartners(ctx context.Context, orderedIds []string) error {
	if len(orderedIds) == 0 {
		return nil
	}

	_, err := a.db.Exec(ctx, `
		UPDATE partners p
		SET position = ordering.index, updated_at = unix_now()
		FROM (
			SELECT id, (ROW_NUMBER() OVER ())::int - 1 AS index
			FROM UNNEST($1::text[]) AS id
		) AS ordering
		WHERE p.id = ordering.id;
	`, orderedIds)
	if err != nil {
		return fmt.Errorf("error reordering partners: %s", err)
	}
	return nil
}
