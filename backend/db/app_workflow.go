package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (a *AppDB) IsProposer(ctx context.Context, id string) (bool, error) {
	return a.getBoolUserRole(ctx, id, "is_proposer")
}

func (a *AppDB) IsImprover(ctx context.Context, id string) (bool, error) {
	return a.getBoolUserRole(ctx, id, "is_improver")
}

func (a *AppDB) IsVoter(ctx context.Context, id string) (bool, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			(is_voter OR is_admin)
		FROM
			users
		WHERE
			id = $1;
	`, id)
	var value bool
	err := row.Scan(&value)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value, nil
}

func (a *AppDB) IsIssuer(ctx context.Context, id string) (bool, error) {
	return a.getBoolUserRole(ctx, id, "is_issuer")
}

func (a *AppDB) getBoolUserRole(ctx context.Context, id string, column string) (bool, error) {
	query := fmt.Sprintf(`
		SELECT
			%s
		FROM
			users
		WHERE
			id = $1;
	`, column)

	row := a.db.QueryRow(ctx, query, id)
	var value bool
	err := row.Scan(&value)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value, nil
}

func (a *AppDB) UpsertProposerRequest(ctx context.Context, userId string, organization string, email string) (*structs.Proposer, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	organization = strings.TrimSpace(organization)
	email = strings.ToLower(strings.TrimSpace(email))
	if organization == "" {
		return nil, fmt.Errorf("organization is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			proposers
		WHERE
			user_id = $1;
	`, userId).Scan(&status)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
			INSERT INTO proposers
				(user_id, organization, email, status)
			VALUES
				($1, $2, $3, 'pending');
		`, userId, organization, email)
		if err != nil {
			return nil, fmt.Errorf("error inserting proposer request: %s", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		if status == "approved" {
			return nil, fmt.Errorf("proposer already approved")
		}

		_, err = tx.Exec(ctx, `
			UPDATE
				proposers
			SET
				organization = $2,
				email = $3,
				status = 'pending',
				updated_at = NOW()
			WHERE
				user_id = $1;
		`, userId, organization, email)
		if err != nil {
			return nil, fmt.Errorf("error updating proposer request: %s", err)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_proposer = false,
			contact_email = COALESCE(NULLIF($2, ''), contact_email)
		WHERE
			id = $1;
	`, userId, email)
	if err != nil {
		return nil, fmt.Errorf("error resetting proposer status: %s", err)
	}

	proposer, err := getProposerByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return proposer, nil
}

func (a *AppDB) GetProposerByUser(ctx context.Context, userId string) (*structs.Proposer, error) {
	return getProposerByUser(ctx, a.db, userId)
}

func (a *AppDB) GetProposers(ctx context.Context) ([]*structs.Proposer, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			user_id,
			organization,
			email,
			nickname,
			status,
			created_at,
			updated_at
		FROM
			proposers
		ORDER BY
			created_at DESC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying proposers: %s", err)
	}
	defer rows.Close()

	results := []*structs.Proposer{}
	for rows.Next() {
		proposer := structs.Proposer{}
		err = rows.Scan(
			&proposer.UserId,
			&proposer.Organization,
			&proposer.Email,
			&proposer.Nickname,
			&proposer.Status,
			&proposer.CreatedAt,
			&proposer.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning proposer: %s", err)
		}
		results = append(results, &proposer)
	}

	return results, nil
}

func (a *AppDB) UpdateProposer(ctx context.Context, req *structs.ProposerUpdateRequest) (*structs.Proposer, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	if req.Status != nil {
		switch *req.Status {
		case "pending", "approved", "rejected":
		default:
			return nil, fmt.Errorf("invalid proposer status")
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	cmd, err := tx.Exec(ctx, `
		UPDATE
			proposers
		SET
			nickname = COALESCE($2, nickname),
			status = COALESCE($3, status),
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, req.UserId, req.Nickname, req.Status)
	if err != nil {
		return nil, fmt.Errorf("error updating proposer: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("proposer not found")
	}

	if req.Status != nil {
		isProposer := *req.Status == "approved"
		_, err = tx.Exec(ctx, `
			UPDATE
				users
			SET
				is_proposer = $1
			WHERE
				id = $2;
		`, isProposer, req.UserId)
		if err != nil {
			return nil, fmt.Errorf("error updating user proposer flag: %s", err)
		}
	}

	proposer, err := getProposerByUser(ctx, tx, req.UserId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return proposer, nil
}

func getProposerByUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userId string) (*structs.Proposer, error) {
	row := querier.QueryRow(ctx, `
		SELECT
			user_id,
			organization,
			email,
			nickname,
			status,
			created_at,
			updated_at
		FROM
			proposers
		WHERE
			user_id = $1;
	`, userId)

	proposer := structs.Proposer{}
	err := row.Scan(
		&proposer.UserId,
		&proposer.Organization,
		&proposer.Email,
		&proposer.Nickname,
		&proposer.Status,
		&proposer.CreatedAt,
		&proposer.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &proposer, nil
}

func (a *AppDB) UpsertImproverRequest(ctx context.Context, userId string, req *structs.ImproverRequest) (*structs.Improver, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	first := strings.TrimSpace(req.FirstName)
	last := strings.TrimSpace(req.LastName)
	email := strings.TrimSpace(req.Email)
	if first == "" || last == "" || email == "" {
		return nil, fmt.Errorf("first name, last name, and email are required")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			improvers
		WHERE
			user_id = $1;
	`, userId).Scan(&status)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
			INSERT INTO improvers
				(user_id, first_name, last_name, email, status)
			VALUES
				($1, $2, $3, $4, 'pending');
		`, userId, first, last, email)
		if err != nil {
			return nil, fmt.Errorf("error inserting improver request: %s", err)
		}
	} else if err != nil {
		return nil, err
	} else {
		if status == "approved" {
			return nil, fmt.Errorf("improver already approved")
		}

		_, err = tx.Exec(ctx, `
			UPDATE
				improvers
			SET
				first_name = $2,
				last_name = $3,
				email = $4,
				status = 'pending',
				updated_at = NOW()
			WHERE
				user_id = $1;
		`, userId, first, last, email)
		if err != nil {
			return nil, fmt.Errorf("error updating improver request: %s", err)
		}
	}

	fullName := strings.TrimSpace(first + " " + last)
	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_improver = false,
			contact_name = COALESCE(NULLIF($2, ''), contact_name),
			contact_email = COALESCE(NULLIF($3, ''), contact_email)
		WHERE
			id = $1;
	`, userId, fullName, email)
	if err != nil {
		return nil, fmt.Errorf("error updating user improver profile: %s", err)
	}

	improver, err := getImproverByUser(ctx, tx, userId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return improver, nil
}

func (a *AppDB) GetImproverByUser(ctx context.Context, userId string) (*structs.Improver, error) {
	return getImproverByUser(ctx, a.db, userId)
}

func (a *AppDB) GetImprovers(ctx context.Context) ([]*structs.Improver, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			user_id,
			first_name,
			last_name,
			email,
			status,
			created_at,
			updated_at
		FROM
			improvers
		ORDER BY
			created_at DESC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying improvers: %s", err)
	}
	defer rows.Close()

	results := []*structs.Improver{}
	for rows.Next() {
		improver := structs.Improver{}
		err = rows.Scan(
			&improver.UserId,
			&improver.FirstName,
			&improver.LastName,
			&improver.Email,
			&improver.Status,
			&improver.CreatedAt,
			&improver.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning improver: %s", err)
		}
		results = append(results, &improver)
	}

	return results, nil
}

func (a *AppDB) UpdateImprover(ctx context.Context, req *structs.ImproverUpdateRequest) (*structs.Improver, error) {
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	if req.Status != nil {
		switch *req.Status {
		case "pending", "approved", "rejected":
		default:
			return nil, fmt.Errorf("invalid improver status")
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	cmd, err := tx.Exec(ctx, `
		UPDATE
			improvers
		SET
			status = COALESCE($2, status),
			updated_at = NOW()
		WHERE
			user_id = $1;
	`, req.UserId, req.Status)
	if err != nil {
		return nil, fmt.Errorf("error updating improver: %s", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, fmt.Errorf("improver not found")
	}

	if req.Status != nil {
		isImprover := *req.Status == "approved"
		_, err = tx.Exec(ctx, `
			UPDATE
				users
			SET
				is_improver = $1
			WHERE
				id = $2;
		`, isImprover, req.UserId)
		if err != nil {
			return nil, fmt.Errorf("error updating user improver flag: %s", err)
		}
	}

	improver, err := getImproverByUser(ctx, tx, req.UserId)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return improver, nil
}

func getImproverByUser(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userId string) (*structs.Improver, error) {
	row := querier.QueryRow(ctx, `
		SELECT
			user_id,
			first_name,
			last_name,
			email,
			status,
			created_at,
			updated_at
		FROM
			improvers
		WHERE
			user_id = $1;
	`, userId)

	improver := structs.Improver{}
	err := row.Scan(
		&improver.UserId,
		&improver.FirstName,
		&improver.LastName,
		&improver.Email,
		&improver.Status,
		&improver.CreatedAt,
		&improver.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &improver, nil
}

type normalizedWorkflowTemplateData struct {
	SeriesId    *string
	Recurrence  string
	StartAt     time.Time
	Roles       []structs.WorkflowRoleCreateInput
	Steps       []structs.WorkflowStepCreateInput
	TotalBounty uint64
}

func normalizeWorkflowTemplateData(req *structs.WorkflowTemplateCreateRequest, startAt time.Time) (*normalizedWorkflowTemplateData, error) {
	if req == nil {
		return nil, fmt.Errorf("template request is required")
	}

	recurrence := strings.TrimSpace(req.Recurrence)
	switch recurrence {
	case "one_time", "daily", "weekly", "monthly":
	default:
		return nil, fmt.Errorf("invalid recurrence")
	}

	if len(req.Roles) == 0 {
		return nil, fmt.Errorf("at least one workflow role is required")
	}
	if len(req.Steps) == 0 {
		return nil, fmt.Errorf("at least one workflow step is required")
	}

	roleIds := map[string]struct{}{}
	normalizedRoles := make([]structs.WorkflowRoleCreateInput, 0, len(req.Roles))
	for idx, roleInput := range req.Roles {
		roleTitle := strings.TrimSpace(roleInput.Title)
		if roleTitle == "" {
			return nil, fmt.Errorf("workflow role title is required")
		}

		roleClientId := strings.TrimSpace(roleInput.ClientId)
		if roleClientId == "" {
			roleClientId = fmt.Sprintf("role-%d", idx+1)
		}
		if _, exists := roleIds[roleClientId]; exists {
			return nil, fmt.Errorf("duplicate workflow role client_id: %s", roleClientId)
		}
		roleIds[roleClientId] = struct{}{}

		normalizedCredentials := make([]string, 0, len(roleInput.RequiredCredentials))
		seenCredentials := map[string]struct{}{}
		for _, credential := range roleInput.RequiredCredentials {
			credential = strings.TrimSpace(credential)
			if !structs.IsValidCredentialType(credential) {
				return nil, fmt.Errorf("invalid workflow role credential: %s", credential)
			}
			if _, exists := seenCredentials[credential]; exists {
				continue
			}
			seenCredentials[credential] = struct{}{}
			normalizedCredentials = append(normalizedCredentials, credential)
		}
		if len(normalizedCredentials) == 0 {
			return nil, fmt.Errorf("workflow role requires at least one credential")
		}

		normalizedRoles = append(normalizedRoles, structs.WorkflowRoleCreateInput{
			ClientId:            roleClientId,
			Title:               roleTitle,
			RequiredCredentials: normalizedCredentials,
		})
	}

	totalBounty := uint64(0)
	normalizedSteps := make([]structs.WorkflowStepCreateInput, 0, len(req.Steps))
	for _, stepInput := range req.Steps {
		stepTitle := strings.TrimSpace(stepInput.Title)
		if stepTitle == "" {
			return nil, fmt.Errorf("workflow step title is required")
		}

		roleClientId := strings.TrimSpace(stepInput.RoleClientId)
		if roleClientId == "" {
			return nil, fmt.Errorf("workflow step requires a role assignment")
		}
		if _, exists := roleIds[roleClientId]; !exists {
			return nil, fmt.Errorf("workflow step references unknown role client_id: %s", roleClientId)
		}

		totalBounty += stepInput.Bounty
		normalizedItems := make([]structs.WorkflowWorkItemCreateInput, 0, len(stepInput.WorkItems))
		for _, itemInput := range stepInput.WorkItems {
			itemTitle := strings.TrimSpace(itemInput.Title)
			if itemTitle == "" {
				return nil, fmt.Errorf("workflow work item title is required")
			}
			if !itemInput.RequiresPhoto && !itemInput.RequiresWritten && !itemInput.RequiresDropdown {
				return nil, fmt.Errorf("workflow work item must require photo, written response, or dropdown")
			}

			normalizedDropdownOptions := []structs.WorkflowDropdownOptionCreateInput{}
			if itemInput.RequiresDropdown {
				if len(itemInput.DropdownOptions) == 0 {
					return nil, fmt.Errorf("dropdown work item requires at least one option")
				}
				seenValues := map[string]struct{}{}
				for _, option := range itemInput.DropdownOptions {
					label := strings.TrimSpace(option.Label)
					if label == "" {
						return nil, fmt.Errorf("dropdown option label is required")
					}

					value := deriveDropdownValueFromLabel(label)
					if value == "" {
						return nil, fmt.Errorf("dropdown option label must include letters or numbers")
					}
					if _, exists := seenValues[value]; exists {
						return nil, fmt.Errorf("duplicate dropdown option label value: %s", value)
					}
					seenValues[value] = struct{}{}

					normalizedDropdownOptions = append(normalizedDropdownOptions, structs.WorkflowDropdownOptionCreateInput{
						Label:                   label,
						RequiresWrittenResponse: option.RequiresWrittenResponse,
						NotifyEmails:            normalizeEmailList(option.NotifyEmails),
					})
				}
			}

			normalizedItems = append(normalizedItems, structs.WorkflowWorkItemCreateInput{
				Title:            itemTitle,
				Description:      strings.TrimSpace(itemInput.Description),
				Optional:         itemInput.Optional,
				RequiresPhoto:    itemInput.RequiresPhoto,
				RequiresWritten:  itemInput.RequiresWritten,
				RequiresDropdown: itemInput.RequiresDropdown,
				DropdownOptions:  normalizedDropdownOptions,
			})
		}

		normalizedSteps = append(normalizedSteps, structs.WorkflowStepCreateInput{
			Title:        stepTitle,
			Description:  strings.TrimSpace(stepInput.Description),
			Bounty:       stepInput.Bounty,
			RoleClientId: roleClientId,
			WorkItems:    normalizedItems,
		})
	}

	if totalBounty == 0 {
		return nil, fmt.Errorf("workflow total bounty must be greater than zero")
	}

	var seriesId *string
	if req.SeriesId != nil {
		trimmed := strings.TrimSpace(*req.SeriesId)
		if trimmed != "" {
			seriesId = &trimmed
		}
	}

	return &normalizedWorkflowTemplateData{
		SeriesId:    seriesId,
		Recurrence:  recurrence,
		StartAt:     startAt,
		Roles:       normalizedRoles,
		Steps:       normalizedSteps,
		TotalBounty: totalBounty,
	}, nil
}

func (a *AppDB) CreateWorkflowTemplate(
	ctx context.Context,
	creatorUserId string,
	req *structs.WorkflowTemplateCreateRequest,
	startAt time.Time,
	isDefault bool,
) (*structs.WorkflowTemplate, error) {
	normalized, err := normalizeWorkflowTemplateData(req, startAt)
	if err != nil {
		return nil, err
	}

	templateTitle := strings.TrimSpace(req.TemplateTitle)
	templateDescription := strings.TrimSpace(req.TemplateDescription)
	if templateTitle == "" {
		return nil, fmt.Errorf("template_title is required")
	}
	if templateDescription == "" {
		return nil, fmt.Errorf("template_description is required")
	}

	var ownerUserId *string
	if !isDefault {
		ownerUserId = &creatorUserId
	}

	rolesJSON, err := json.Marshal(normalized.Roles)
	if err != nil {
		return nil, fmt.Errorf("error marshalling template roles: %s", err)
	}
	stepsJSON, err := json.Marshal(normalized.Steps)
	if err != nil {
		return nil, fmt.Errorf("error marshalling template steps: %s", err)
	}

	templateId := uuid.NewString()
	_, err = a.db.Exec(ctx, `
		INSERT INTO workflow_templates
			(
				id,
				template_title,
				template_description,
				owner_user_id,
				created_by_user_id,
				is_default,
				recurrence,
				start_at,
				series_id,
				roles_json,
				steps_json
			)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb);
	`, templateId, templateTitle, templateDescription, ownerUserId, creatorUserId, isDefault, normalized.Recurrence, normalized.StartAt, normalized.SeriesId, string(rolesJSON), string(stepsJSON))
	if err != nil {
		return nil, fmt.Errorf("error creating workflow template: %s", err)
	}

	return a.GetWorkflowTemplateByID(ctx, templateId)
}

func (a *AppDB) GetWorkflowTemplateByID(ctx context.Context, templateId string) (*structs.WorkflowTemplate, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			template_title,
			template_description,
			owner_user_id,
			created_by_user_id,
			is_default,
			recurrence,
			start_at,
			series_id,
			roles_json,
			steps_json,
			created_at,
			updated_at
		FROM
			workflow_templates
		WHERE
			id = $1;
	`, templateId)

	template := &structs.WorkflowTemplate{}
	var rolesBytes []byte
	var stepsBytes []byte
	if err := row.Scan(
		&template.Id,
		&template.TemplateTitle,
		&template.TemplateDescription,
		&template.OwnerUserId,
		&template.CreatedByUserId,
		&template.IsDefault,
		&template.Recurrence,
		&template.StartAt,
		&template.SeriesId,
		&rolesBytes,
		&stepsBytes,
		&template.CreatedAt,
		&template.UpdatedAt,
	); err != nil {
		return nil, err
	}

	template.Roles = []structs.WorkflowRoleCreateInput{}
	if len(rolesBytes) > 0 {
		if err := json.Unmarshal(rolesBytes, &template.Roles); err != nil {
			return nil, fmt.Errorf("error unmarshalling template roles: %s", err)
		}
	}

	template.Steps = []structs.WorkflowStepCreateInput{}
	if len(stepsBytes) > 0 {
		if err := json.Unmarshal(stepsBytes, &template.Steps); err != nil {
			return nil, fmt.Errorf("error unmarshalling template steps: %s", err)
		}
	}

	return template, nil
}

func (a *AppDB) GetWorkflowTemplatesForProposer(ctx context.Context, proposerId string) ([]*structs.WorkflowTemplate, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			template_title,
			template_description,
			owner_user_id,
			created_by_user_id,
			is_default,
			recurrence,
			start_at,
			series_id,
			roles_json,
			steps_json,
			created_at,
			updated_at
		FROM
			workflow_templates
		WHERE
			is_default = true
		OR
			owner_user_id = $1
		ORDER BY
			is_default DESC,
			created_at DESC;
	`, proposerId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow templates: %s", err)
	}
	defer rows.Close()

	templates := []*structs.WorkflowTemplate{}
	for rows.Next() {
		template := &structs.WorkflowTemplate{}
		var rolesBytes []byte
		var stepsBytes []byte
		if err := rows.Scan(
			&template.Id,
			&template.TemplateTitle,
			&template.TemplateDescription,
			&template.OwnerUserId,
			&template.CreatedByUserId,
			&template.IsDefault,
			&template.Recurrence,
			&template.StartAt,
			&template.SeriesId,
			&rolesBytes,
			&stepsBytes,
			&template.CreatedAt,
			&template.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow template: %s", err)
		}

		template.Roles = []structs.WorkflowRoleCreateInput{}
		if len(rolesBytes) > 0 {
			if err := json.Unmarshal(rolesBytes, &template.Roles); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow template roles: %s", err)
			}
		}

		template.Steps = []structs.WorkflowStepCreateInput{}
		if len(stepsBytes) > 0 {
			if err := json.Unmarshal(stepsBytes, &template.Steps); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow template steps: %s", err)
			}
		}

		templates = append(templates, template)
	}

	return templates, nil
}

func (a *AppDB) CreateWorkflow(ctx context.Context, proposerId string, req *structs.WorkflowCreateRequest, startAt time.Time) (*structs.Workflow, error) {
	if req == nil {
		return nil, fmt.Errorf("workflow request is required")
	}

	if len(req.Roles) == 0 {
		return nil, fmt.Errorf("at least one workflow role is required")
	}
	if len(req.Steps) == 0 {
		return nil, fmt.Errorf("at least one workflow step is required")
	}

	totalBounty := uint64(0)
	for _, step := range req.Steps {
		totalBounty += step.Bounty
	}
	if totalBounty == 0 {
		return nil, fmt.Errorf("workflow total bounty must be greater than zero")
	}

	seriesId := ""
	if req.SeriesId != nil {
		seriesId = strings.TrimSpace(*req.SeriesId)
	}
	if seriesId == "" {
		seriesId = uuid.NewString()
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var proposerStatus string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			proposers
		WHERE
			user_id = $1
		FOR UPDATE;
	`, proposerId).Scan(&proposerStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("proposer not found")
		}
		return nil, err
	}
	if proposerStatus != "approved" {
		return nil, fmt.Errorf("proposer is not approved")
	}

	isStartBlocked := false
	var blockedById *string
	var previousWorkflowId string
	var previousStatus string
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			status
		FROM
			workflows
		WHERE
			series_id = $1
		ORDER BY
			start_at DESC,
			created_at DESC
		LIMIT 1;
	`, seriesId).Scan(&previousWorkflowId, &previousStatus)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	if err == nil && previousStatus != "paid_out" && previousStatus != "deleted" {
		isStartBlocked = true
		blockedById = &previousWorkflowId
	}

	workflowId := uuid.NewString()
	weeklyRequirement := weeklyBountyRequirement(totalBounty, req.Recurrence)
	status := "pending"

	_, err = tx.Exec(ctx, `
		INSERT INTO workflows
			(
				id,
				series_id,
				proposer_id,
				title,
				description,
				recurrence,
				start_at,
				status,
				is_start_blocked,
				blocked_by_workflow_id,
				total_bounty,
				weekly_bounty_requirement,
				budget_weekly_deducted,
				budget_one_time_deducted
			)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);
	`, workflowId, seriesId, proposerId, strings.TrimSpace(req.Title), strings.TrimSpace(req.Description), req.Recurrence, startAt, status, isStartBlocked, blockedById, totalBounty, weeklyRequirement, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("error inserting workflow: %s", err)
	}

	roleIds := map[string]string{}
	for idx, roleInput := range req.Roles {
		title := strings.TrimSpace(roleInput.Title)
		if title == "" {
			return nil, fmt.Errorf("workflow role title is required")
		}
		if len(roleInput.RequiredCredentials) == 0 {
			return nil, fmt.Errorf("workflow role requires at least one credential")
		}

		roleId := uuid.NewString()
		roleClientId := strings.TrimSpace(roleInput.ClientId)
		if roleClientId == "" {
			roleClientId = fmt.Sprintf("role-%d", idx+1)
		}
		if _, exists := roleIds[roleClientId]; exists {
			return nil, fmt.Errorf("duplicate workflow role client_id: %s", roleClientId)
		}
		roleIds[roleClientId] = roleId

		_, err = tx.Exec(ctx, `
			INSERT INTO workflow_roles
				(id, workflow_id, title)
			VALUES
				($1, $2, $3);
		`, roleId, workflowId, title)
		if err != nil {
			return nil, fmt.Errorf("error inserting workflow role: %s", err)
		}

		for _, credential := range roleInput.RequiredCredentials {
			credential = strings.TrimSpace(credential)
			if !structs.IsValidCredentialType(credential) {
				return nil, fmt.Errorf("invalid workflow role credential: %s", credential)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO workflow_role_credentials
					(role_id, credential_type)
				VALUES
					($1, $2);
			`, roleId, credential)
			if err != nil {
				return nil, fmt.Errorf("error inserting workflow role credential: %s", err)
			}
		}
	}

	now := time.Now().UTC()
	for stepIndex, stepInput := range req.Steps {
		stepTitle := strings.TrimSpace(stepInput.Title)
		if stepTitle == "" {
			return nil, fmt.Errorf("workflow step title is required")
		}

		stepId := uuid.NewString()
		stepStatus := "locked"
		if stepIndex == 0 && !startAt.After(now) {
			stepStatus = "available"
		}

		var roleId *string
		roleClientId := strings.TrimSpace(stepInput.RoleClientId)
		if roleClientId == "" {
			return nil, fmt.Errorf("workflow step requires a role assignment")
		}
		mappedRoleId, ok := roleIds[roleClientId]
		if !ok {
			return nil, fmt.Errorf("workflow step references unknown role client_id: %s", roleClientId)
		}
		roleId = &mappedRoleId

		_, err = tx.Exec(ctx, `
			INSERT INTO workflow_steps
				(id, workflow_id, step_order, title, description, bounty, role_id, status)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8);
		`, stepId, workflowId, stepIndex+1, stepTitle, strings.TrimSpace(stepInput.Description), stepInput.Bounty, roleId, stepStatus)
		if err != nil {
			return nil, fmt.Errorf("error inserting workflow step: %s", err)
		}

		for itemIndex, itemInput := range stepInput.WorkItems {
			itemTitle := strings.TrimSpace(itemInput.Title)
			if itemTitle == "" {
				return nil, fmt.Errorf("workflow work item title is required")
			}
			if !itemInput.RequiresPhoto && !itemInput.RequiresWritten && !itemInput.RequiresDropdown {
				return nil, fmt.Errorf("workflow work item must require photo, written response, or dropdown")
			}

			dropdownOptions := []structs.WorkflowDropdownOption{}
			dropdownRequiresWritten := map[string]bool{}
			if itemInput.RequiresDropdown {
				if len(itemInput.DropdownOptions) == 0 {
					return nil, fmt.Errorf("dropdown work item requires at least one option")
				}
				seenValues := map[string]struct{}{}
				for _, option := range itemInput.DropdownOptions {
					label := strings.TrimSpace(option.Label)
					if label == "" {
						return nil, fmt.Errorf("dropdown option label is required")
					}

					value := deriveDropdownValueFromLabel(label)
					if value == "" {
						return nil, fmt.Errorf("dropdown option label must include letters or numbers")
					}

					if _, exists := seenValues[value]; exists {
						return nil, fmt.Errorf("duplicate dropdown option label value: %s", value)
					}
					seenValues[value] = struct{}{}

					notifyEmails := normalizeEmailList(option.NotifyEmails)
					dropdownOptions = append(dropdownOptions, structs.WorkflowDropdownOption{
						Value:                   value,
						Label:                   label,
						RequiresWrittenResponse: option.RequiresWrittenResponse,
						NotifyEmails:            notifyEmails,
					})
					dropdownRequiresWritten[value] = option.RequiresWrittenResponse
				}
			}

			dropdownOptionsJSON, err := json.Marshal(dropdownOptions)
			if err != nil {
				return nil, fmt.Errorf("error marshalling dropdown options: %s", err)
			}
			dropdownRequiresJSON, err := json.Marshal(dropdownRequiresWritten)
			if err != nil {
				return nil, fmt.Errorf("error marshalling dropdown requirement map: %s", err)
			}

			legacyNotifyEmailsJSON, err := json.Marshal([]string{})
			if err != nil {
				return nil, fmt.Errorf("error marshalling legacy notify emails: %s", err)
			}

			legacyNotifyValuesJSON, err := json.Marshal([]string{})
			if err != nil {
				return nil, fmt.Errorf("error marshalling legacy notify dropdown values: %s", err)
			}

			itemId := uuid.NewString()
			_, err = tx.Exec(ctx, `
				INSERT INTO workflow_step_items
					(
						id,
						step_id,
						item_order,
						title,
						description,
						is_optional,
						requires_photo,
						requires_written_response,
						requires_dropdown,
						dropdown_options,
						dropdown_requires_written_response,
						notify_emails,
						notify_on_dropdown_values
					)
				VALUES
					($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12::jsonb, $13::jsonb);
			`, itemId, stepId, itemIndex+1, itemTitle, strings.TrimSpace(itemInput.Description), itemInput.Optional, itemInput.RequiresPhoto, itemInput.RequiresWritten, itemInput.RequiresDropdown, string(dropdownOptionsJSON), string(dropdownRequiresJSON), string(legacyNotifyEmailsJSON), string(legacyNotifyValuesJSON))
			if err != nil {
				return nil, fmt.Errorf("error inserting workflow work item: %s", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) GetWorkflowsByProposer(ctx context.Context, proposerId string) ([]*structs.Workflow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			proposer_id = $1
		ORDER BY
			created_at DESC;
	`, proposerId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflows: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning workflow id: %s", err)
		}
		workflowIDs = append(workflowIDs, id)
	}

	results := make([]*structs.Workflow, 0, len(workflowIDs))
	for _, id := range workflowIDs {
		wf, err := a.GetWorkflowByID(ctx, id)
		if err != nil {
			return nil, err
		}
		results = append(results, wf)
	}
	return results, nil
}

func (a *AppDB) GetWorkflowByID(ctx context.Context, workflowId string) (*structs.Workflow, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id,
			series_id,
			proposer_id,
			title,
			description,
			recurrence,
			start_at,
			status,
			is_start_blocked,
			blocked_by_workflow_id,
			total_bounty,
			weekly_bounty_requirement,
			budget_weekly_deducted,
			budget_one_time_deducted,
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at,
			vote_finalized_by_user_id,
			vote_decision,
			created_at,
			updated_at
		FROM
			workflows
		WHERE
			id = $1;
	`, workflowId)

	workflow := &structs.Workflow{}
	err := row.Scan(
		&workflow.Id,
		&workflow.SeriesId,
		&workflow.ProposerId,
		&workflow.Title,
		&workflow.Description,
		&workflow.Recurrence,
		&workflow.StartAt,
		&workflow.Status,
		&workflow.IsStartBlocked,
		&workflow.BlockedByWorkflowId,
		&workflow.TotalBounty,
		&workflow.WeeklyBountyRequirement,
		&workflow.BudgetWeeklyDeducted,
		&workflow.BudgetOneTimeDeducted,
		&workflow.VoteQuorumReachedAt,
		&workflow.VoteFinalizeAt,
		&workflow.VoteFinalizedAt,
		&workflow.VoteFinalizedByUserId,
		&workflow.VoteDecision,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	roles, err := a.getWorkflowRoles(ctx, workflowId)
	if err != nil {
		return nil, err
	}
	workflow.Roles = roles

	steps, err := a.getWorkflowSteps(ctx, workflowId)
	if err != nil {
		return nil, err
	}
	workflow.Steps = steps

	votes, err := a.GetWorkflowVotes(ctx, workflowId)
	if err != nil {
		return nil, err
	}
	workflow.Votes = *votes

	return workflow, nil
}

func (a *AppDB) getWorkflowRoles(ctx context.Context, workflowId string) ([]structs.WorkflowRole, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			workflow_id,
			title
		FROM
			workflow_roles
		WHERE
			workflow_id = $1;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow roles: %s", err)
	}
	defer rows.Close()

	roles := []structs.WorkflowRole{}
	roleIndex := map[string]int{}
	for rows.Next() {
		role := structs.WorkflowRole{}
		if err := rows.Scan(&role.Id, &role.WorkflowId, &role.Title); err != nil {
			return nil, fmt.Errorf("error scanning workflow role: %s", err)
		}
		role.RequiredCredentials = []string{}
		roleIndex[role.Id] = len(roles)
		roles = append(roles, role)
	}

	credRows, err := a.db.Query(ctx, `
		SELECT
			role_id,
			credential_type
		FROM
			workflow_role_credentials
		WHERE
			role_id IN (
				SELECT id FROM workflow_roles WHERE workflow_id = $1
			);
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow role credentials: %s", err)
	}
	defer credRows.Close()

	for credRows.Next() {
		var roleId string
		var credential string
		if err := credRows.Scan(&roleId, &credential); err != nil {
			return nil, fmt.Errorf("error scanning workflow role credential: %s", err)
		}
		if idx, ok := roleIndex[roleId]; ok {
			roles[idx].RequiredCredentials = append(roles[idx].RequiredCredentials, credential)
		}
	}

	return roles, nil
}

func (a *AppDB) getWorkflowSteps(ctx context.Context, workflowId string) ([]structs.WorkflowStep, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			workflow_id,
			step_order,
			title,
			description,
			bounty,
			role_id,
			assigned_improver_id,
			status,
			started_at,
			completed_at
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		ORDER BY
			step_order ASC;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow steps: %s", err)
	}
	defer rows.Close()

	steps := []structs.WorkflowStep{}
	stepIndex := map[string]int{}
	for rows.Next() {
		step := structs.WorkflowStep{}
		if err := rows.Scan(
			&step.Id,
			&step.WorkflowId,
			&step.StepOrder,
			&step.Title,
			&step.Description,
			&step.Bounty,
			&step.RoleId,
			&step.AssignedImproverId,
			&step.Status,
			&step.StartedAt,
			&step.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow step: %s", err)
		}
		step.WorkItems = []structs.WorkflowWorkItem{}
		step.Submission = nil
		stepIndex[step.Id] = len(steps)
		steps = append(steps, step)
	}

	itemRows, err := a.db.Query(ctx, `
		SELECT
			id,
			step_id,
			item_order,
			title,
			description,
			is_optional,
			requires_photo,
			requires_written_response,
			requires_dropdown,
			dropdown_options,
			dropdown_requires_written_response,
			notify_emails,
			notify_on_dropdown_values
		FROM
			workflow_step_items
		WHERE
			step_id IN (
				SELECT id FROM workflow_steps WHERE workflow_id = $1
			)
		ORDER BY
			item_order ASC;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow work items: %s", err)
	}
	defer itemRows.Close()

	for itemRows.Next() {
		item := structs.WorkflowWorkItem{}
		var dropdownOptionsBytes []byte
		var dropdownRequiresBytes []byte
		var notifyEmailsBytes []byte
		var notifyValuesBytes []byte
		if err := itemRows.Scan(
			&item.Id,
			&item.StepId,
			&item.ItemOrder,
			&item.Title,
			&item.Description,
			&item.Optional,
			&item.RequiresPhoto,
			&item.RequiresWrittenResponse,
			&item.RequiresDropdown,
			&dropdownOptionsBytes,
			&dropdownRequiresBytes,
			&notifyEmailsBytes,
			&notifyValuesBytes,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow work item: %s", err)
		}

		item.DropdownOptions = []structs.WorkflowDropdownOption{}
		if len(dropdownOptionsBytes) > 0 {
			if err := json.Unmarshal(dropdownOptionsBytes, &item.DropdownOptions); err != nil {
				return nil, fmt.Errorf("error unmarshalling dropdown options: %s", err)
			}
		}
		for idx := range item.DropdownOptions {
			item.DropdownOptions[idx].NotifyEmails = normalizeEmailList(item.DropdownOptions[idx].NotifyEmails)
		}

		item.DropdownRequiresWrittenMap = map[string]bool{}
		if len(dropdownRequiresBytes) > 0 {
			if err := json.Unmarshal(dropdownRequiresBytes, &item.DropdownRequiresWrittenMap); err != nil {
				return nil, fmt.Errorf("error unmarshalling dropdown requirement map: %s", err)
			}
		}

		legacyNotifyEmails := []string{}
		if len(notifyEmailsBytes) > 0 {
			if err := json.Unmarshal(notifyEmailsBytes, &legacyNotifyEmails); err != nil {
				return nil, fmt.Errorf("error unmarshalling notify emails: %s", err)
			}
		}

		legacyNotifyValues := []string{}
		if len(notifyValuesBytes) > 0 {
			if err := json.Unmarshal(notifyValuesBytes, &legacyNotifyValues); err != nil {
				return nil, fmt.Errorf("error unmarshalling notify dropdown values: %s", err)
			}
		}
		legacyNotifyEmails = normalizeEmailList(legacyNotifyEmails)
		if len(legacyNotifyEmails) > 0 && len(legacyNotifyValues) > 0 {
			legacyWatchValues := map[string]struct{}{}
			for _, value := range legacyNotifyValues {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				legacyWatchValues[value] = struct{}{}
			}
			if len(legacyWatchValues) > 0 {
				for idx := range item.DropdownOptions {
					if len(item.DropdownOptions[idx].NotifyEmails) > 0 {
						continue
					}
					if _, ok := legacyWatchValues[item.DropdownOptions[idx].Value]; !ok {
						continue
					}
					item.DropdownOptions[idx].NotifyEmails = append([]string{}, legacyNotifyEmails...)
				}
			}
		}

		if idx, ok := stepIndex[item.StepId]; ok {
			steps[idx].WorkItems = append(steps[idx].WorkItems, item)
		}
	}

	submissionRows, err := a.db.Query(ctx, `
		SELECT
			id,
			workflow_id,
			step_id,
			improver_id,
			item_responses,
			submitted_at,
			updated_at
		FROM
			workflow_step_submissions
		WHERE
			workflow_id = $1;
	`, workflowId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow step submissions: %s", err)
	}
	defer submissionRows.Close()

	for submissionRows.Next() {
		submission := structs.WorkflowStepSubmission{}
		var itemResponsesBytes []byte
		if err := submissionRows.Scan(
			&submission.Id,
			&submission.WorkflowId,
			&submission.StepId,
			&submission.ImproverId,
			&itemResponsesBytes,
			&submission.SubmittedAt,
			&submission.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow step submission: %s", err)
		}

		submission.ItemResponses = []structs.WorkflowStepItemResponse{}
		if len(itemResponsesBytes) > 0 {
			if err := json.Unmarshal(itemResponsesBytes, &submission.ItemResponses); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow step submission item responses: %s", err)
			}
		}

		if idx, ok := stepIndex[submission.StepId]; ok {
			steps[idx].Submission = &submission
		}
	}

	return steps, nil
}

func (a *AppDB) DeleteWorkflowByProposer(ctx context.Context, workflowId string, proposerId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			workflows
		WHERE
			id = $1
		AND
			proposer_id = $2
		FOR UPDATE;
	`, workflowId, proposerId).Scan(&status)
	if err != nil {
		return err
	}

	if status != "pending" && status != "rejected" && status != "expired" {
		return fmt.Errorf("workflow cannot be deleted in current status")
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM workflows WHERE id = $1;
	`, workflowId)
	if err != nil {
		return fmt.Errorf("error deleting workflow: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func weeklyBountyRequirement(total uint64, recurrence string) uint64 {
	switch recurrence {
	case "daily":
		return total * 7
	case "weekly":
		return total
	case "monthly":
		return (total + 3) / 4
	default:
		return total
	}
}

func (a *AppDB) AllocatedWorkflowBalance(ctx context.Context) (uint64, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(ws.bounty), 0)
		FROM
			workflow_steps ws
		JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		WHERE
			w.status IN ('approved', 'blocked', 'in_progress', 'completed')
		AND
			ws.status != 'paid_out';
	`)
	var allocated uint64
	if err := row.Scan(&allocated); err != nil {
		return 0, err
	}
	return allocated, nil
}

func (a *AppDB) AllocatedWorkflowBalanceByProposer(ctx context.Context, proposerId string) (uint64, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(ws.bounty), 0)
		FROM
			workflow_steps ws
		JOIN
			workflows w
		ON
			w.id = ws.workflow_id
		WHERE
			w.proposer_id = $1
		AND
			w.status IN ('pending', 'approved', 'blocked', 'in_progress', 'completed')
		AND
			ws.status != 'paid_out';
	`, proposerId)
	var allocated uint64
	if err := row.Scan(&allocated); err != nil {
		return 0, err
	}
	return allocated, nil
}

func (a *AppDB) GetActiveCredentialTypesForUser(ctx context.Context, userId string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			credential_type
		FROM
			user_credentials
		WHERE
			user_id = $1
		AND
			is_revoked = false
		ORDER BY
			credential_type ASC;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying active credentials: %s", err)
	}
	defer rows.Close()

	credentials := []string{}
	for rows.Next() {
		var credential string
		if err := rows.Scan(&credential); err != nil {
			return nil, fmt.Errorf("error scanning active credential: %s", err)
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func getActiveCredentialTypesTx(ctx context.Context, tx pgx.Tx, userId string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			credential_type
		FROM
			user_credentials
		WHERE
			user_id = $1
		AND
			is_revoked = false;
	`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	credentials := []string{}
	for rows.Next() {
		var credential string
		if err := rows.Scan(&credential); err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func (a *AppDB) RefreshWorkflowStartAvailability(ctx context.Context) (*structs.WorkflowStartRefreshResult, error) {
	rows, err := a.db.Query(ctx, `
		WITH updated_steps AS (
			UPDATE workflow_steps ws
			SET
				status = 'available',
				updated_at = NOW()
			FROM workflows w
			WHERE
				ws.workflow_id = w.id
			AND
				ws.step_order = 1
			AND
				ws.status = 'locked'
			AND
				w.status IN ('approved', 'in_progress')
			AND
					w.start_at <= NOW()
				RETURNING
					ws.id AS step_id,
					ws.workflow_id,
					ws.title AS step_title,
					ws.assigned_improver_id
			),
			updated_workflows AS (
				SELECT
					u.step_id,
					u.workflow_id,
					u.step_title,
					u.assigned_improver_id,
					w.title AS workflow_title,
					w.series_id,
					w.recurrence,
					w.start_at,
					w.total_bounty,
					w.weekly_bounty_requirement
				FROM
					updated_steps u
				JOIN
					workflows w
				ON
					w.id = u.workflow_id
			),
			inserted_notifications AS (
				INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
				SELECT
					step_id,
					assigned_improver_id,
					'step_available'
				FROM
					updated_workflows
				WHERE
					assigned_improver_id IS NOT NULL
				ON CONFLICT DO NOTHING
			RETURNING
				step_id,
				user_id
		)
			SELECT
				u.workflow_id,
				u.workflow_title,
				u.step_id,
				u.step_title,
				u.assigned_improver_id,
				COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(us.contact_name, '')),
				COALESCE(i.email, us.contact_email, ''),
				(n.step_id IS NOT NULL),
				u.series_id,
				u.recurrence,
				u.start_at,
				u.total_bounty,
				u.weekly_bounty_requirement
			FROM
				updated_workflows u
			LEFT JOIN
				inserted_notifications n
			ON
				n.step_id = u.step_id
		AND
			n.user_id = u.assigned_improver_id
			LEFT JOIN
				users us
			ON
				us.id = u.assigned_improver_id
		LEFT JOIN
			improvers i
		ON
			i.user_id = u.assigned_improver_id;
	`)
	if err != nil {
		return nil, fmt.Errorf("error refreshing workflow step start availability: %s", err)
	}
	defer rows.Close()

	result := &structs.WorkflowStartRefreshResult{
		AvailabilityNotifications: []structs.WorkflowStepAvailabilityNotification{},
		SeriesFundingChecks:       []structs.WorkflowSeriesStartFundingCheck{},
	}
	seriesCheckSeen := map[string]struct{}{}
	for rows.Next() {
		var workflowId string
		var workflowTitle string
		var stepId string
		var stepTitle string
		var assignedImproverId *string
		var name string
		var email string
		var shouldNotify bool
		var seriesId string
		var recurrence string
		var startAt time.Time
		var totalBounty uint64
		var weeklyRequirement uint64
		if err := rows.Scan(
			&workflowId,
			&workflowTitle,
			&stepId,
			&stepTitle,
			&assignedImproverId,
			&name,
			&email,
			&shouldNotify,
			&seriesId,
			&recurrence,
			&startAt,
			&totalBounty,
			&weeklyRequirement,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow start availability update: %s", err)
		}

		if shouldNotify && assignedImproverId != nil {
			result.AvailabilityNotifications = append(result.AvailabilityNotifications, structs.WorkflowStepAvailabilityNotification{
				WorkflowId:    workflowId,
				WorkflowTitle: workflowTitle,
				StepId:        stepId,
				StepTitle:     stepTitle,
				UserId:        *assignedImproverId,
				Name:          name,
				Email:         email,
			})
		}

		if recurrence != "one_time" {
			if _, exists := seriesCheckSeen[workflowId]; !exists {
				seriesCheckSeen[workflowId] = struct{}{}
				result.SeriesFundingChecks = append(result.SeriesFundingChecks, structs.WorkflowSeriesStartFundingCheck{
					WorkflowId:              workflowId,
					WorkflowTitle:           workflowTitle,
					SeriesId:                seriesId,
					Recurrence:              recurrence,
					StartAt:                 startAt,
					TotalBounty:             totalBounty,
					WeeklyBountyRequirement: weeklyRequirement,
				})
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workflow start availability updates: %s", err)
	}

	return result, nil
}

func (a *AppDB) GetImproverWorkflows(ctx context.Context) ([]*structs.Workflow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			status IN ('approved', 'in_progress')
		ORDER BY
			start_at ASC,
			created_at ASC
		LIMIT 300;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying improver workflows: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning improver workflow id: %s", err)
		}
		workflowIDs = append(workflowIDs, id)
	}

	workflows := make([]*structs.Workflow, 0, len(workflowIDs))
	for _, workflowId := range workflowIDs {
		workflow, err := a.GetWorkflowByID(ctx, workflowId)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	return workflows, nil
}

func (a *AppDB) ClaimWorkflowStep(
	ctx context.Context,
	workflowId string,
	stepId string,
	improverId string,
) (*structs.Workflow, *structs.WorkflowStepAvailabilityNotification, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	var workflowStatus string
	var workflowStartAt time.Time
	var workflowTitle string
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			start_at,
			title
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&workflowStatus, &workflowStartAt, &workflowTitle)
	if err != nil {
		return nil, nil, err
	}
	if workflowStatus != "approved" && workflowStatus != "in_progress" {
		return nil, nil, fmt.Errorf("workflow is not available for claiming")
	}

	var claimedAssignments int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			assigned_improver_id = $2;
	`, workflowId, improverId).Scan(&claimedAssignments)
	if err != nil {
		return nil, nil, err
	}
	if claimedAssignments > 0 {
		return nil, nil, fmt.Errorf("improver already assigned within this workflow")
	}

	var stepWorkflowId string
	var stepStatus string
	var stepTitle string
	var stepOrder int
	var roleId *string
	var assignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			workflow_id,
			status,
			title,
			step_order,
			role_id,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			id = $1
		FOR UPDATE;
	`, stepId).Scan(&stepWorkflowId, &stepStatus, &stepTitle, &stepOrder, &roleId, &assignedImproverId)
	if err != nil {
		return nil, nil, err
	}
	if stepWorkflowId != workflowId {
		return nil, nil, fmt.Errorf("step does not belong to workflow")
	}
	if assignedImproverId != nil {
		return nil, nil, fmt.Errorf("workflow step is already claimed")
	}
	if roleId == nil {
		return nil, nil, fmt.Errorf("workflow step is missing a role")
	}
	if stepStatus != "locked" && stepStatus != "available" {
		return nil, nil, fmt.Errorf("workflow step is not claimable")
	}

	requiredRows, err := tx.Query(ctx, `
		SELECT
			credential_type
		FROM
			workflow_role_credentials
		WHERE
			role_id = $1;
	`, *roleId)
	if err != nil {
		return nil, nil, err
	}
	defer requiredRows.Close()

	requiredCredentials := []string{}
	for requiredRows.Next() {
		var credential string
		if err := requiredRows.Scan(&credential); err != nil {
			return nil, nil, err
		}
		requiredCredentials = append(requiredCredentials, credential)
	}

	activeCredentials, err := getActiveCredentialTypesTx(ctx, tx, improverId)
	if err != nil {
		return nil, nil, err
	}
	activeSet := map[string]struct{}{}
	for _, credential := range activeCredentials {
		activeSet[credential] = struct{}{}
	}
	for _, required := range requiredCredentials {
		if _, ok := activeSet[required]; !ok {
			return nil, nil, fmt.Errorf("missing required credentials for workflow role")
		}
	}

	var postClaimStatus string
	err = tx.QueryRow(ctx, `
		UPDATE
			workflow_steps
		SET
			assigned_improver_id = $2,
			status = CASE
				WHEN status = 'locked' AND step_order = 1 AND $3 <= NOW() THEN 'available'
				ELSE status
			END,
			updated_at = NOW()
		WHERE
			id = $1
		RETURNING
			status;
	`, stepId, improverId, workflowStartAt).Scan(&postClaimStatus)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "workflow_single_assignment_per_improver_idx" {
			return nil, nil, fmt.Errorf("improver already assigned within this workflow")
		}
		return nil, nil, fmt.Errorf("error assigning workflow step: %s", err)
	}

	var availabilityNotification *structs.WorkflowStepAvailabilityNotification
	if postClaimStatus == "available" {
		cmd, err := tx.Exec(ctx, `
			INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
			VALUES
				($1, $2, 'step_available')
			ON CONFLICT DO NOTHING;
		`, stepId, improverId)
		if err != nil {
			return nil, nil, fmt.Errorf("error recording workflow step notification after claim: %s", err)
		}
		if cmd.RowsAffected() > 0 {
			notification := structs.WorkflowStepAvailabilityNotification{
				WorkflowId:    workflowId,
				WorkflowTitle: workflowTitle,
				StepId:        stepId,
				StepTitle:     stepTitle,
				UserId:        improverId,
			}
			err = tx.QueryRow(ctx, `
				SELECT
					COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, '')),
					COALESCE(i.email, u.contact_email, '')
				FROM
					users u
				LEFT JOIN
					improvers i
				ON
					i.user_id = u.id
				WHERE
					u.id = $1;
			`, improverId).Scan(&notification.Name, &notification.Email)
			if err != nil {
				return nil, nil, err
			}
			availabilityNotification = &notification
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	workflow, err := a.GetWorkflowByID(ctx, workflowId)
	if err != nil {
		return nil, nil, err
	}
	return workflow, availabilityNotification, nil
}

func canStepTransitionToAvailableTx(ctx context.Context, tx pgx.Tx, workflowId string, stepOrder int, workflowStartAt time.Time) (bool, error) {
	if stepOrder <= 1 {
		return !workflowStartAt.After(time.Now().UTC()), nil
	}

	var previousStatus string
	err := tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			step_order = $2;
	`, workflowId, stepOrder-1).Scan(&previousStatus)
	if err != nil {
		return false, err
	}

	return previousStatus == "completed" || previousStatus == "paid_out", nil
}

func (a *AppDB) StartWorkflowStep(ctx context.Context, workflowId string, stepId string, improverId string) (*structs.Workflow, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var workflowStatus string
	var workflowStartAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			start_at
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&workflowStatus, &workflowStartAt)
	if err != nil {
		return nil, err
	}
	if workflowStatus != "approved" && workflowStatus != "in_progress" {
		return nil, fmt.Errorf("workflow is not active")
	}

	var stepWorkflowId string
	var stepOrder int
	var stepStatus string
	var assignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			workflow_id,
			step_order,
			status,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			id = $1
		FOR UPDATE;
	`, stepId).Scan(&stepWorkflowId, &stepOrder, &stepStatus, &assignedImproverId)
	if err != nil {
		return nil, err
	}
	if stepWorkflowId != workflowId {
		return nil, fmt.Errorf("step does not belong to workflow")
	}
	if assignedImproverId == nil || *assignedImproverId != improverId {
		return nil, fmt.Errorf("step is not assigned to this improver")
	}

	if stepStatus == "completed" || stepStatus == "paid_out" {
		return nil, fmt.Errorf("step has already been completed")
	}

	if stepStatus == "locked" {
		canUnlock, err := canStepTransitionToAvailableTx(ctx, tx, workflowId, stepOrder, workflowStartAt)
		if err != nil {
			return nil, err
		}
		if !canUnlock {
			return nil, fmt.Errorf("step is not available yet")
		}
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_steps
			SET
				status = 'available',
				updated_at = NOW()
			WHERE
				id = $1;
		`, stepId)
		if err != nil {
			return nil, fmt.Errorf("error unlocking workflow step: %s", err)
		}
		stepStatus = "available"
	}

	if stepStatus == "available" {
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_steps
			SET
				status = 'in_progress',
				started_at = COALESCE(started_at, NOW()),
				updated_at = NOW()
			WHERE
				id = $1;
		`, stepId)
		if err != nil {
			return nil, fmt.Errorf("error starting workflow step: %s", err)
		}
	}

	if workflowStatus == "approved" {
		_, err = tx.Exec(ctx, `
			UPDATE
				workflows
			SET
				status = 'in_progress',
				updated_at = NOW()
			WHERE
				id = $1;
		`, workflowId)
		if err != nil {
			return nil, fmt.Errorf("error updating workflow status to in_progress: %s", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a.GetWorkflowByID(ctx, workflowId)
}

var dropdownValueSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func deriveDropdownValueFromLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	label = dropdownValueSanitizer.ReplaceAllString(label, "_")
	return strings.Trim(label, "_")
}

func normalizeEmailList(emails []string) []string {
	normalized := make([]string, 0, len(emails))
	seen := map[string]struct{}{}
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		normalized = append(normalized, email)
	}
	return normalized
}

func (a *AppDB) CompleteWorkflowStep(
	ctx context.Context,
	workflowId string,
	stepId string,
	improverId string,
	itemResponses []structs.WorkflowStepItemResponse,
) (*structs.WorkflowStepCompletionResult, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	result := &structs.WorkflowStepCompletionResult{
		AvailabilityNotifications: []structs.WorkflowStepAvailabilityNotification{},
		DropdownNotifications:     []structs.WorkflowDropdownNotification{},
	}

	var workflowStatus string
	var workflowStartAt time.Time
	var workflowTitle string
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			start_at,
			title
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&workflowStatus, &workflowStartAt, &workflowTitle)
	if err != nil {
		return nil, err
	}
	if workflowStatus != "approved" && workflowStatus != "in_progress" {
		return nil, fmt.Errorf("workflow is not active")
	}

	var stepWorkflowId string
	var stepOrder int
	var stepStatus string
	var stepTitle string
	var assignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			workflow_id,
			step_order,
			status,
			title,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			id = $1
		FOR UPDATE;
	`, stepId).Scan(&stepWorkflowId, &stepOrder, &stepStatus, &stepTitle, &assignedImproverId)
	if err != nil {
		return nil, err
	}
	if stepWorkflowId != workflowId {
		return nil, fmt.Errorf("step does not belong to workflow")
	}
	if assignedImproverId == nil || *assignedImproverId != improverId {
		return nil, fmt.Errorf("step is not assigned to this improver")
	}
	if stepStatus == "completed" || stepStatus == "paid_out" {
		return nil, fmt.Errorf("step has already been completed")
	}

	canUnlock, err := canStepTransitionToAvailableTx(ctx, tx, workflowId, stepOrder, workflowStartAt)
	if err != nil {
		return nil, err
	}
	if stepStatus == "locked" && !canUnlock {
		return nil, fmt.Errorf("step is not available yet")
	}

	itemRows, err := tx.Query(ctx, `
		SELECT
			id,
			title,
			is_optional,
			requires_photo,
			requires_written_response,
			requires_dropdown,
			dropdown_options,
			dropdown_requires_written_response,
			notify_emails,
			notify_on_dropdown_values
		FROM
			workflow_step_items
		WHERE
			step_id = $1
		ORDER BY
			item_order ASC;
	`, stepId)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow step items for completion: %s", err)
	}
	defer itemRows.Close()

	type stepItemMeta struct {
		Id                         string
		Title                      string
		Optional                   bool
		RequiresPhoto              bool
		RequiresWrittenResponse    bool
		RequiresDropdown           bool
		DropdownOptions            []structs.WorkflowDropdownOption
		DropdownRequiresWrittenMap map[string]bool
	}

	items := []stepItemMeta{}
	itemByID := map[string]stepItemMeta{}

	for itemRows.Next() {
		item := stepItemMeta{}
		var dropdownOptionsBytes []byte
		var dropdownRequiresBytes []byte
		var notifyEmailsBytes []byte
		var notifyValuesBytes []byte
		if err := itemRows.Scan(
			&item.Id,
			&item.Title,
			&item.Optional,
			&item.RequiresPhoto,
			&item.RequiresWrittenResponse,
			&item.RequiresDropdown,
			&dropdownOptionsBytes,
			&dropdownRequiresBytes,
			&notifyEmailsBytes,
			&notifyValuesBytes,
		); err != nil {
			return nil, fmt.Errorf("error scanning workflow step item metadata: %s", err)
		}

		item.DropdownOptions = []structs.WorkflowDropdownOption{}
		if len(dropdownOptionsBytes) > 0 {
			if err := json.Unmarshal(dropdownOptionsBytes, &item.DropdownOptions); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow step item dropdown options: %s", err)
			}
		}
		for idx := range item.DropdownOptions {
			item.DropdownOptions[idx].NotifyEmails = normalizeEmailList(item.DropdownOptions[idx].NotifyEmails)
		}
		item.DropdownRequiresWrittenMap = map[string]bool{}
		if len(dropdownRequiresBytes) > 0 {
			if err := json.Unmarshal(dropdownRequiresBytes, &item.DropdownRequiresWrittenMap); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow step item dropdown requirement map: %s", err)
			}
		}

		legacyNotifyEmails := []string{}
		if len(notifyEmailsBytes) > 0 {
			if err := json.Unmarshal(notifyEmailsBytes, &legacyNotifyEmails); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow step item notification emails: %s", err)
			}
		}

		legacyNotifyValues := []string{}
		if len(notifyValuesBytes) > 0 {
			if err := json.Unmarshal(notifyValuesBytes, &legacyNotifyValues); err != nil {
				return nil, fmt.Errorf("error unmarshalling workflow step item notification values: %s", err)
			}
		}
		legacyNotifyEmails = normalizeEmailList(legacyNotifyEmails)
		if len(legacyNotifyEmails) > 0 && len(legacyNotifyValues) > 0 {
			legacyWatchValues := map[string]struct{}{}
			for _, value := range legacyNotifyValues {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				legacyWatchValues[value] = struct{}{}
			}
			if len(legacyWatchValues) > 0 {
				for idx := range item.DropdownOptions {
					if len(item.DropdownOptions[idx].NotifyEmails) > 0 {
						continue
					}
					if _, ok := legacyWatchValues[item.DropdownOptions[idx].Value]; !ok {
						continue
					}
					item.DropdownOptions[idx].NotifyEmails = append([]string{}, legacyNotifyEmails...)
				}
			}
		}

		items = append(items, item)
		itemByID[item.Id] = item
	}

	responseMap := map[string]structs.WorkflowStepItemResponse{}
	for _, response := range itemResponses {
		itemId := strings.TrimSpace(response.ItemId)
		if itemId == "" {
			return nil, fmt.Errorf("item_id is required for step completion")
		}
		if _, exists := itemByID[itemId]; !exists {
			return nil, fmt.Errorf("workflow step response references unknown item_id: %s", itemId)
		}
		if _, exists := responseMap[itemId]; exists {
			return nil, fmt.Errorf("duplicate workflow step response item_id: %s", itemId)
		}

		cleanPhotoURLs := []string{}
		for _, value := range response.PhotoURLs {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			cleanPhotoURLs = append(cleanPhotoURLs, value)
		}
		response.PhotoURLs = cleanPhotoURLs

		if response.WrittenResponse != nil {
			trimmed := strings.TrimSpace(*response.WrittenResponse)
			if trimmed == "" {
				response.WrittenResponse = nil
			} else {
				response.WrittenResponse = &trimmed
			}
		}
		if response.DropdownValue != nil {
			trimmed := strings.TrimSpace(*response.DropdownValue)
			if trimmed == "" {
				response.DropdownValue = nil
			} else {
				response.DropdownValue = &trimmed
			}
		}

		response.ItemId = itemId
		responseMap[itemId] = response
	}

	serializedResponses := []structs.WorkflowStepItemResponse{}
	for _, item := range items {
		response, hasResponse := responseMap[item.Id]
		if !hasResponse {
			if item.Optional {
				continue
			}
			return nil, fmt.Errorf("required step item missing response: %s", item.Title)
		}

		if item.RequiresPhoto && len(response.PhotoURLs) == 0 {
			return nil, fmt.Errorf("step item requires photo evidence: %s", item.Title)
		}
		if item.RequiresWrittenResponse && response.WrittenResponse == nil {
			return nil, fmt.Errorf("step item requires written response: %s", item.Title)
		}
		if item.RequiresDropdown {
			if response.DropdownValue == nil {
				return nil, fmt.Errorf("step item requires dropdown selection: %s", item.Title)
			}

			dropdownAllowed := map[string]struct{}{}
			var selectedOption *structs.WorkflowDropdownOption
			for _, option := range item.DropdownOptions {
				dropdownAllowed[option.Value] = struct{}{}
				if option.Value == *response.DropdownValue {
					opt := option
					selectedOption = &opt
				}
			}
			if _, ok := dropdownAllowed[*response.DropdownValue]; !ok {
				return nil, fmt.Errorf("invalid dropdown value for step item: %s", item.Title)
			}

			if requiredWritten, ok := item.DropdownRequiresWrittenMap[*response.DropdownValue]; ok && requiredWritten && response.WrittenResponse == nil {
				return nil, fmt.Errorf("dropdown selection requires written response for step item: %s", item.Title)
			}

			if selectedOption != nil {
				emails := normalizeEmailList(selectedOption.NotifyEmails)
				if len(emails) > 0 {
					result.DropdownNotifications = append(result.DropdownNotifications, structs.WorkflowDropdownNotification{
						WorkflowId:    workflowId,
						WorkflowTitle: workflowTitle,
						StepId:        stepId,
						StepTitle:     stepTitle,
						ItemId:        item.Id,
						ItemTitle:     item.Title,
						DropdownValue: *response.DropdownValue,
						Emails:        emails,
					})
				}
			}
		}

		serializedResponses = append(serializedResponses, response)
	}

	responsesJSON, err := json.Marshal(serializedResponses)
	if err != nil {
		return nil, fmt.Errorf("error marshalling workflow step responses: %s", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_step_submissions
			(id, workflow_id, step_id, improver_id, item_responses, submitted_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5::jsonb, NOW(), NOW())
		ON CONFLICT (step_id)
		DO UPDATE SET
			improver_id = EXCLUDED.improver_id,
			item_responses = EXCLUDED.item_responses,
			submitted_at = NOW(),
			updated_at = NOW();
	`, uuid.NewString(), workflowId, stepId, improverId, string(responsesJSON))
	if err != nil {
		return nil, fmt.Errorf("error upserting workflow step submission: %s", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			workflow_steps
		SET
			status = 'completed',
			started_at = COALESCE(started_at, NOW()),
			completed_at = NOW(),
			updated_at = NOW()
		WHERE
			id = $1;
	`, stepId)
	if err != nil {
		return nil, fmt.Errorf("error marking workflow step completed: %s", err)
	}

	var nextStepId string
	var nextStepTitle string
	var nextStepStatus string
	var nextAssignedImproverId *string
	err = tx.QueryRow(ctx, `
		SELECT
			id,
			title,
			status,
			assigned_improver_id
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			step_order = $2
		FOR UPDATE;
	`, workflowId, stepOrder+1).Scan(&nextStepId, &nextStepTitle, &nextStepStatus, &nextAssignedImproverId)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	if err == nil && nextStepStatus == "locked" {
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_steps
			SET
				status = 'available',
				updated_at = NOW()
			WHERE
				id = $1;
		`, nextStepId)
		if err != nil {
			return nil, fmt.Errorf("error unlocking next workflow step: %s", err)
		}

		if nextAssignedImproverId != nil {
			cmd, err := tx.Exec(ctx, `
				INSERT INTO workflow_step_notifications(step_id, user_id, notification_type)
				VALUES
					($1, $2, 'step_available')
				ON CONFLICT DO NOTHING;
			`, nextStepId, *nextAssignedImproverId)
			if err != nil {
				return nil, fmt.Errorf("error recording step availability notification: %s", err)
			}
			if cmd.RowsAffected() > 0 {
				notification := structs.WorkflowStepAvailabilityNotification{
					WorkflowId:    workflowId,
					WorkflowTitle: workflowTitle,
					StepId:        nextStepId,
					StepTitle:     nextStepTitle,
					UserId:        *nextAssignedImproverId,
				}
				err = tx.QueryRow(ctx, `
					SELECT
						COALESCE(NULLIF(TRIM(COALESCE(i.first_name, '') || ' ' || COALESCE(i.last_name, '')), ''), COALESCE(u.contact_name, '')),
						COALESCE(i.email, u.contact_email, '')
					FROM
						users u
					LEFT JOIN
						improvers i
					ON
						i.user_id = u.id
					WHERE
						u.id = $1;
				`, *nextAssignedImproverId).Scan(&notification.Name, &notification.Email)
				if err != nil {
					return nil, err
				}
				result.AvailabilityNotifications = append(result.AvailabilityNotifications, notification)
			}
		}
	}

	var incompleteSteps int
	err = tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			workflow_steps
		WHERE
			workflow_id = $1
		AND
			status NOT IN ('completed', 'paid_out');
	`, workflowId).Scan(&incompleteSteps)
	if err != nil {
		return nil, err
	}

	if incompleteSteps == 0 {
		result.WorkflowStatus = "completed"
		_, err = tx.Exec(ctx, `
			UPDATE
				workflows
			SET
				status = 'completed',
				updated_at = NOW()
			WHERE
				id = $1;
		`, workflowId)
		if err != nil {
			return nil, fmt.Errorf("error marking workflow completed: %s", err)
		}
	} else {
		result.WorkflowStatus = "in_progress"
		if workflowStatus == "approved" {
			_, err = tx.Exec(ctx, `
				UPDATE
					workflows
				SET
					status = 'in_progress',
					updated_at = NOW()
				WHERE
					id = $1;
			`, workflowId)
			if err != nil {
				return nil, fmt.Errorf("error marking workflow in progress: %s", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *AppDB) CountEligibleVoters(ctx context.Context) (int, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			users
		WHERE
			is_voter = true
		OR
			is_admin = true;
	`)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func quorumVotesRequired(totalVoters int) int {
	if totalVoters <= 0 {
		return 0
	}
	return (totalVoters + 1) / 2
}

func possibleBodyMajority(totalVoters int) int {
	if totalVoters <= 0 {
		return 0
	}
	return (totalVoters / 2) + 1
}

func (a *AppDB) GetWorkflowVotes(ctx context.Context, workflowId string) (*structs.WorkflowVotes, error) {
	return a.getWorkflowVotesInternal(ctx, workflowId, nil)
}

func (a *AppDB) GetWorkflowVotesForUser(ctx context.Context, workflowId string, userId string) (*structs.WorkflowVotes, error) {
	return a.getWorkflowVotesInternal(ctx, workflowId, &userId)
}

func (a *AppDB) getWorkflowVotesInternal(ctx context.Context, workflowId string, userId *string) (*structs.WorkflowVotes, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_votes
		WHERE
			workflow_id = $1;
	`, workflowId)

	votes := &structs.WorkflowVotes{}
	if err := row.Scan(&votes.Approve, &votes.Deny); err != nil {
		return nil, err
	}

	totalVoters, err := a.CountEligibleVoters(ctx)
	if err != nil {
		return nil, err
	}
	votes.TotalVoters = totalVoters
	votes.VotesCast = votes.Approve + votes.Deny
	votes.QuorumThreshold = quorumVotesRequired(totalVoters)
	votes.QuorumReached = votes.VotesCast >= votes.QuorumThreshold && totalVoters > 0

	row = a.db.QueryRow(ctx, `
		SELECT
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at,
			vote_decision
		FROM
			workflows
		WHERE
			id = $1;
	`, workflowId)
	if err := row.Scan(&votes.QuorumReachedAt, &votes.FinalizeAt, &votes.FinalizedAt, &votes.Decision); err != nil {
		return nil, err
	}

	if userId != nil {
		voteRow := a.db.QueryRow(ctx, `
			SELECT
				decision
			FROM
				workflow_votes
			WHERE
				workflow_id = $1
			AND
				voter_id = $2;
		`, workflowId, *userId)
		var decision string
		err := voteRow.Scan(&decision)
		if err == nil {
			votes.MyDecision = &decision
		} else if err != pgx.ErrNoRows {
			return nil, err
		}
	}

	return votes, nil
}

func (a *AppDB) RecordWorkflowVote(ctx context.Context, workflowId string, voterId string, decision string, comment string) (*structs.WorkflowVotes, error) {
	_, err := a.db.Exec(ctx, `
		INSERT INTO workflow_votes
			(workflow_id, voter_id, decision, comment)
		VALUES
			($1, $2, $3, $4)
		ON CONFLICT (workflow_id, voter_id)
		DO UPDATE SET
			decision = EXCLUDED.decision,
			comment = EXCLUDED.comment,
			updated_at = NOW();
	`, workflowId, voterId, decision, comment)
	if err != nil {
		return nil, fmt.Errorf("error recording workflow vote: %s", err)
	}
	return a.GetWorkflowVotesForUser(ctx, workflowId, voterId)
}

func (a *AppDB) GetWorkflowForApproval(ctx context.Context, workflowId string) (*structs.Workflow, error) {
	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) ExpireStaleWorkflowProposals(ctx context.Context) ([]structs.WorkflowProposalExpiryNotice, error) {
	rows, err := a.db.Query(ctx, `
		WITH expired AS (
			UPDATE
				workflows w
			SET
				status = 'expired',
				vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, NOW()),
				vote_finalize_at = COALESCE(vote_finalize_at, NOW()),
				vote_finalized_at = COALESCE(vote_finalized_at, NOW()),
				updated_at = NOW()
			WHERE
				w.status = 'pending'
			AND
				w.created_at <= NOW() - INTERVAL '14 days'
			RETURNING
				w.id,
				w.title,
				w.proposer_id
		)
		SELECT
			e.id,
			e.title,
			e.proposer_id,
			COALESCE(NULLIF(TRIM(p.email), ''), COALESCE(u.contact_email, ''))
		FROM
			expired e
		LEFT JOIN
			proposers p
		ON
			p.user_id = e.proposer_id
		LEFT JOIN
			users u
		ON
			u.id = e.proposer_id;
	`)
	if err != nil {
		return nil, fmt.Errorf("error expiring stale workflow proposals: %s", err)
	}
	defer rows.Close()

	notifications := []structs.WorkflowProposalExpiryNotice{}
	for rows.Next() {
		notice := structs.WorkflowProposalExpiryNotice{}
		if err := rows.Scan(&notice.WorkflowId, &notice.WorkflowTitle, &notice.ProposerUserId, &notice.ProposerEmail); err != nil {
			return nil, fmt.Errorf("error scanning expired workflow notice: %s", err)
		}
		notice.ProposerEmail = strings.TrimSpace(notice.ProposerEmail)
		notifications = append(notifications, notice)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating expired workflow notices: %s", err)
	}

	return notifications, nil
}

func (a *AppDB) GetWorkflowProposalOutcomeNotification(ctx context.Context, workflowId string) (*structs.WorkflowProposalOutcomeNotification, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			w.id,
			w.title,
			CASE
				WHEN w.status IN ('approved', 'blocked') THEN 'approved'
				WHEN w.status = 'rejected' THEN 'rejected'
				ELSE ''
			END,
			w.proposer_id,
			COALESCE(NULLIF(TRIM(p.email), ''), COALESCE(u.contact_email, ''))
		FROM
			workflows w
		LEFT JOIN
			proposers p
		ON
			p.user_id = w.proposer_id
		LEFT JOIN
			users u
		ON
			u.id = w.proposer_id
		WHERE
			w.id = $1;
	`, workflowId)

	notification := structs.WorkflowProposalOutcomeNotification{}
	if err := row.Scan(
		&notification.WorkflowId,
		&notification.WorkflowTitle,
		&notification.Decision,
		&notification.ProposerUserId,
		&notification.ProposerEmail,
	); err != nil {
		return nil, err
	}

	notification.ProposerEmail = strings.TrimSpace(notification.ProposerEmail)
	if notification.Decision == "" {
		return nil, fmt.Errorf("workflow outcome is not finalized")
	}
	return &notification, nil
}

func (a *AppDB) EvaluateWorkflowVoteState(ctx context.Context, workflowId string) (*structs.Workflow, error) {
	return a.EvaluateWorkflowVoteStateWithApproval(ctx, workflowId, true)
}

func (a *AppDB) EvaluateWorkflowVoteStateWithApproval(ctx context.Context, workflowId string, allowApproval bool) (*structs.Workflow, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	type workflowVoteState struct {
		Status          string
		IsStartBlocked  bool
		QuorumReachedAt *time.Time
		FinalizeAt      *time.Time
		FinalizedAt     *time.Time
	}

	state := workflowVoteState{}
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			is_start_blocked,
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(
		&state.Status,
		&state.IsStartBlocked,
		&state.QuorumReachedAt,
		&state.FinalizeAt,
		&state.FinalizedAt,
	)
	if err != nil {
		return nil, err
	}

	if state.Status != "pending" {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return a.GetWorkflowByID(ctx, workflowId)
	}

	totalVoters, err := countEligibleVotersTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	approveCount, denyCount, err := countWorkflowVotesTx(ctx, tx, workflowId)
	if err != nil {
		return nil, err
	}
	votesCast := approveCount + denyCount
	quorumThreshold := quorumVotesRequired(totalVoters)
	quorumReached := totalVoters > 0 && votesCast >= quorumThreshold
	now := time.Now().UTC()

	if quorumReached && state.QuorumReachedAt == nil {
		quorumReachedAt := now
		finalizeAt := now.Add(24 * time.Hour)
		_, err = tx.Exec(ctx, `
			UPDATE
				workflows
			SET
				vote_quorum_reached_at = $2,
				vote_finalize_at = $3,
				updated_at = NOW()
			WHERE
				id = $1;
		`, workflowId, quorumReachedAt, finalizeAt)
		if err != nil {
			return nil, fmt.Errorf("error setting vote quorum countdown: %s", err)
		}
		state.QuorumReachedAt = &quorumReachedAt
		state.FinalizeAt = &finalizeAt
	}

	majorityThreshold := possibleBodyMajority(totalVoters)
	outcome := ""
	if totalVoters > 0 && approveCount >= majorityThreshold {
		outcome = "approve"
	} else if totalVoters > 0 && denyCount >= majorityThreshold {
		outcome = "deny"
	} else if quorumReached && state.FinalizeAt != nil && !now.Before(*state.FinalizeAt) {
		if approveCount > denyCount {
			outcome = "approve"
		} else {
			outcome = "deny"
		}
	}

	if outcome == "approve" && !allowApproval {
		outcome = ""
	}

	if outcome == "approve" {
		if err := finalizeWorkflowApprovalTx(ctx, tx, workflowId, state.IsStartBlocked, nil, "approve"); err != nil {
			return nil, err
		}
	}
	if outcome == "deny" {
		if err := finalizeWorkflowRejectionTx(ctx, tx, workflowId, "deny", nil); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) ApproveWorkflow(ctx context.Context, workflowId string, approverId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var isStartBlocked bool
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			is_start_blocked
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&status, &isStartBlocked)
	if err != nil {
		return err
	}

	if status == "approved" || status == "blocked" || status == "in_progress" || status == "completed" || status == "paid_out" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("workflow is not pending")
	}

	if err := finalizeWorkflowApprovalTx(ctx, tx, workflowId, isStartBlocked, &approverId, "approve"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) ForceApproveWorkflowAsAdmin(ctx context.Context, workflowId string, adminId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var isStartBlocked bool
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			is_start_blocked
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&status, &isStartBlocked)
	if err != nil {
		return err
	}

	if status == "approved" || status == "blocked" || status == "in_progress" || status == "completed" || status == "paid_out" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("workflow is not pending")
	}

	if err := finalizeWorkflowApprovalTx(ctx, tx, workflowId, isStartBlocked, &adminId, "admin_approve"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) RejectWorkflow(ctx context.Context, workflowId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&status)
	if err != nil {
		return err
	}

	if status == "rejected" {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("approved or active workflows cannot be rejected")
	}

	if err := finalizeWorkflowRejectionTx(ctx, tx, workflowId, "deny", nil); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func finalizeWorkflowApprovalTx(
	ctx context.Context,
	tx pgx.Tx,
	workflowId string,
	isStartBlocked bool,
	actorUserId *string,
	decision string,
) error {
	nextStatus := "approved"
	if isStartBlocked {
		nextStatus = "blocked"
	}

	_, err := tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			status = $2,
			approved_at = COALESCE(approved_at, NOW()),
			approved_by_user_id = COALESCE($3, approved_by_user_id),
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, NOW()),
			vote_finalize_at = COALESCE(vote_finalize_at, NOW()),
			vote_finalized_at = COALESCE(vote_finalized_at, NOW()),
			vote_finalized_by_user_id = COALESCE($4, vote_finalized_by_user_id),
			vote_decision = $5,
			updated_at = NOW()
		WHERE
			id = $1;
	`, workflowId, nextStatus, actorUserId, actorUserId, decision)
	if err != nil {
		return fmt.Errorf("error approving workflow: %s", err)
	}
	return nil
}

func finalizeWorkflowRejectionTx(
	ctx context.Context,
	tx pgx.Tx,
	workflowId string,
	decision string,
	actorUserId *string,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE
			workflows
		SET
			status = 'rejected',
			budget_weekly_deducted = 0,
			budget_one_time_deducted = 0,
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, NOW()),
			vote_finalize_at = COALESCE(vote_finalize_at, NOW()),
			vote_finalized_at = COALESCE(vote_finalized_at, NOW()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = NOW()
		WHERE
			id = $1;
	`, workflowId, decision, actorUserId)
	if err != nil {
		return fmt.Errorf("error updating rejected workflow: %s", err)
	}

	return nil
}

func countEligibleVotersTx(ctx context.Context, tx pgx.Tx) (int, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			COUNT(*)
		FROM
			users
		WHERE
			is_voter = true
		OR
			is_admin = true;
	`)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func countWorkflowVotesTx(ctx context.Context, tx pgx.Tx, workflowId string) (int, int, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_votes
		WHERE
			workflow_id = $1;
	`, workflowId)
	var approve int
	var deny int
	if err := row.Scan(&approve, &deny); err != nil {
		return 0, 0, err
	}
	return approve, deny, nil
}

func (a *AppDB) GetWorkflowByIDAndProposer(ctx context.Context, workflowId string, proposerId string) (*structs.Workflow, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			id = $1
		AND
			proposer_id = $2;
	`, workflowId, proposerId)

	var id string
	err := row.Scan(&id)
	if err != nil {
		return nil, err
	}

	return a.GetWorkflowByID(ctx, workflowId)
}

func (a *AppDB) GetVoterWorkflows(ctx context.Context, voterId string) ([]*structs.Workflow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflows
		WHERE
			status IN ('pending', 'approved', 'blocked', 'rejected')
		ORDER BY
			CASE WHEN status = 'pending' THEN 0 ELSE 1 END,
			created_at DESC
		LIMIT 200;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying voter workflows: %s", err)
	}
	defer rows.Close()

	workflowIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning voter workflow id: %s", err)
		}
		workflowIDs = append(workflowIDs, id)
	}

	workflows := make([]*structs.Workflow, 0, len(workflowIDs))
	for _, workflowId := range workflowIDs {
		workflow, err := a.GetWorkflowByID(ctx, workflowId)
		if err != nil {
			return nil, err
		}
		votes, err := a.GetWorkflowVotesForUser(ctx, workflowId, voterId)
		if err != nil {
			return nil, err
		}
		workflow.Votes = *votes
		workflows = append(workflows, workflow)
	}

	return workflows, nil
}

func (a *AppDB) GetActiveWorkflows(ctx context.Context) ([]*structs.ActiveWorkflowListItem, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			series_id,
			proposer_id,
			title,
			description,
			recurrence,
			start_at,
			status,
			is_start_blocked,
			blocked_by_workflow_id,
			total_bounty,
			weekly_bounty_requirement,
			created_at,
			updated_at,
			vote_decision,
			approved_at
		FROM
			workflows
		WHERE
			status IN ('approved', 'blocked', 'in_progress', 'completed')
		ORDER BY
			start_at ASC,
			created_at DESC
		LIMIT 500;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying active workflows: %s", err)
	}
	defer rows.Close()

	results := []*structs.ActiveWorkflowListItem{}
	for rows.Next() {
		workflow := &structs.ActiveWorkflowListItem{}
		if err := rows.Scan(
			&workflow.Id,
			&workflow.SeriesId,
			&workflow.ProposerId,
			&workflow.Title,
			&workflow.Description,
			&workflow.Recurrence,
			&workflow.StartAt,
			&workflow.Status,
			&workflow.IsStartBlocked,
			&workflow.BlockedByWorkflowId,
			&workflow.TotalBounty,
			&workflow.WeeklyBountyRequirement,
			&workflow.CreatedAt,
			&workflow.UpdatedAt,
			&workflow.VoteDecision,
			&workflow.ApprovedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning active workflow: %s", err)
		}
		results = append(results, workflow)
	}

	return results, nil
}

func (a *AppDB) CreateWorkflowDeletionProposal(
	ctx context.Context,
	proposerId string,
	req *structs.WorkflowDeletionProposalCreateRequest,
) (*structs.WorkflowDeletionProposal, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	workflowId := strings.TrimSpace(req.WorkflowId)
	if workflowId == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	targetType := strings.TrimSpace(req.TargetType)
	if targetType == "" {
		targetType = "workflow"
	}
	if targetType != "workflow" && targetType != "series" {
		return nil, fmt.Errorf("invalid target_type")
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 2000 {
		return nil, fmt.Errorf("reason is too long")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var proposerStatus string
	err = tx.QueryRow(ctx, `
		SELECT
			status
		FROM
			proposers
		WHERE
			user_id = $1;
	`, proposerId).Scan(&proposerStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("proposer not found")
		}
		return nil, err
	}
	if proposerStatus != "approved" {
		return nil, fmt.Errorf("proposer is not approved")
	}

	var seriesId string
	var workflowStatus string
	err = tx.QueryRow(ctx, `
		SELECT
			series_id,
			status
		FROM
			workflows
		WHERE
			id = $1
		FOR UPDATE;
	`, workflowId).Scan(&seriesId, &workflowStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, err
	}

	switch workflowStatus {
	case "approved", "blocked", "in_progress", "completed":
	default:
		return nil, fmt.Errorf("workflow is not active")
	}

	if targetType == "workflow" {
		var pendingCount int
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				workflow_deletion_proposals
			WHERE
				target_type = 'workflow'
			AND
				target_workflow_id = $1
			AND
				status = 'pending';
		`, workflowId).Scan(&pendingCount)
		if err != nil {
			return nil, err
		}
		if pendingCount > 0 {
			return nil, fmt.Errorf("a pending deletion vote already exists for this workflow")
		}
	} else {
		var pendingCount int
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				workflow_deletion_proposals
			WHERE
				target_type = 'series'
			AND
				target_series_id = $1
			AND
				status = 'pending';
		`, seriesId).Scan(&pendingCount)
		if err != nil {
			return nil, err
		}
		if pendingCount > 0 {
			return nil, fmt.Errorf("a pending deletion vote already exists for this series")
		}
	}

	proposalId := uuid.NewString()
	var targetWorkflowID *string
	var targetSeriesID *string
	if targetType == "workflow" {
		targetWorkflowID = &workflowId
	} else {
		targetSeriesID = &seriesId
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO workflow_deletion_proposals
			(
				id,
				target_type,
				target_workflow_id,
				target_series_id,
				requested_by_user_id,
				reason
			)
		VALUES
			($1, $2, $3, $4, $5, $6);
	`, proposalId, targetType, targetWorkflowID, targetSeriesID, proposerId, reason)
	if err != nil {
		return nil, fmt.Errorf("error creating workflow deletion proposal: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalId, nil)
}

func (a *AppDB) GetWorkflowDeletionProposalByIDForUser(ctx context.Context, proposalId string, voterId *string) (*structs.WorkflowDeletionProposal, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			p.id,
			p.target_type,
			p.target_workflow_id,
			CASE
				WHEN p.target_type = 'workflow' THEN w.title
				ELSE NULL
			END,
			p.target_series_id,
			p.reason,
			p.status,
			p.requested_by_user_id,
			p.vote_quorum_reached_at,
			p.vote_finalize_at,
			p.vote_finalized_at,
			p.vote_finalized_by_user_id,
			p.vote_decision,
			p.created_at,
			p.updated_at
		FROM
			workflow_deletion_proposals p
		LEFT JOIN
			workflows w
		ON
			w.id = p.target_workflow_id
		WHERE
			p.id = $1;
	`, proposalId)

	proposal := &structs.WorkflowDeletionProposal{}
	if err := row.Scan(
		&proposal.Id,
		&proposal.TargetType,
		&proposal.TargetWorkflowId,
		&proposal.TargetWorkflowTitle,
		&proposal.TargetSeriesId,
		&proposal.Reason,
		&proposal.Status,
		&proposal.RequestedByUserId,
		&proposal.VoteQuorumReachedAt,
		&proposal.VoteFinalizeAt,
		&proposal.VoteFinalizedAt,
		&proposal.VoteFinalizedBy,
		&proposal.VoteDecision,
		&proposal.CreatedAt,
		&proposal.UpdatedAt,
	); err != nil {
		return nil, err
	}

	votes, err := a.getWorkflowDeletionVotesInternal(ctx, proposalId, voterId)
	if err != nil {
		return nil, err
	}
	proposal.Votes = *votes

	return proposal, nil
}

func (a *AppDB) GetWorkflowDeletionProposalsForVoter(ctx context.Context, voterId string) ([]*structs.WorkflowDeletionProposal, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id
		FROM
			workflow_deletion_proposals
		WHERE
			status IN ('pending', 'approved', 'denied')
		ORDER BY
			CASE WHEN status = 'pending' THEN 0 ELSE 1 END,
			created_at DESC
		LIMIT 300;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying workflow deletion proposals: %s", err)
	}
	defer rows.Close()

	proposalIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning workflow deletion proposal id: %s", err)
		}
		proposalIDs = append(proposalIDs, id)
	}

	proposals := make([]*structs.WorkflowDeletionProposal, 0, len(proposalIDs))
	for _, proposalID := range proposalIDs {
		proposal, err := a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalID, &voterId)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func (a *AppDB) getWorkflowDeletionVotesInternal(ctx context.Context, proposalId string, voterId *string) (*structs.WorkflowVotes, error) {
	row := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_deletion_votes
		WHERE
			proposal_id = $1;
	`, proposalId)

	votes := &structs.WorkflowVotes{}
	if err := row.Scan(&votes.Approve, &votes.Deny); err != nil {
		return nil, err
	}

	totalVoters, err := a.CountEligibleVoters(ctx)
	if err != nil {
		return nil, err
	}
	votes.TotalVoters = totalVoters
	votes.VotesCast = votes.Approve + votes.Deny
	votes.QuorumThreshold = quorumVotesRequired(totalVoters)
	votes.QuorumReached = votes.VotesCast >= votes.QuorumThreshold && totalVoters > 0

	row = a.db.QueryRow(ctx, `
		SELECT
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at,
			vote_decision
		FROM
			workflow_deletion_proposals
		WHERE
			id = $1;
	`, proposalId)
	if err := row.Scan(&votes.QuorumReachedAt, &votes.FinalizeAt, &votes.FinalizedAt, &votes.Decision); err != nil {
		return nil, err
	}

	if voterId != nil {
		voteRow := a.db.QueryRow(ctx, `
			SELECT
				decision
			FROM
				workflow_deletion_votes
			WHERE
				proposal_id = $1
			AND
				voter_id = $2;
		`, proposalId, *voterId)
		var decision string
		err := voteRow.Scan(&decision)
		if err == nil {
			votes.MyDecision = &decision
		} else if err != pgx.ErrNoRows {
			return nil, err
		}
	}

	return votes, nil
}

func (a *AppDB) RecordWorkflowDeletionVote(ctx context.Context, proposalId string, voterId string, decision string, comment string) (*structs.WorkflowVotes, error) {
	_, err := a.db.Exec(ctx, `
		INSERT INTO workflow_deletion_votes
			(proposal_id, voter_id, decision, comment)
		VALUES
			($1, $2, $3, $4)
		ON CONFLICT (proposal_id, voter_id)
		DO UPDATE SET
			decision = EXCLUDED.decision,
			comment = EXCLUDED.comment,
			updated_at = NOW();
	`, proposalId, voterId, decision, comment)
	if err != nil {
		return nil, fmt.Errorf("error recording workflow deletion vote: %s", err)
	}
	return a.getWorkflowDeletionVotesInternal(ctx, proposalId, &voterId)
}

func (a *AppDB) EvaluateWorkflowDeletionVoteState(ctx context.Context, proposalId string) (*structs.WorkflowDeletionProposal, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	type deletionVoteState struct {
		Status          string
		TargetType      string
		TargetWorkflow  *string
		TargetSeries    *string
		QuorumReachedAt *time.Time
		FinalizeAt      *time.Time
		FinalizedAt     *time.Time
	}

	state := deletionVoteState{}
	err = tx.QueryRow(ctx, `
		SELECT
			status,
			target_type,
			target_workflow_id,
			target_series_id,
			vote_quorum_reached_at,
			vote_finalize_at,
			vote_finalized_at
		FROM
			workflow_deletion_proposals
		WHERE
			id = $1
		FOR UPDATE;
	`, proposalId).Scan(
		&state.Status,
		&state.TargetType,
		&state.TargetWorkflow,
		&state.TargetSeries,
		&state.QuorumReachedAt,
		&state.FinalizeAt,
		&state.FinalizedAt,
	)
	if err != nil {
		return nil, err
	}

	if state.Status != "pending" {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalId, nil)
	}

	totalVoters, err := countEligibleVotersTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	approveCount, denyCount, err := countWorkflowDeletionVotesTx(ctx, tx, proposalId)
	if err != nil {
		return nil, err
	}
	votesCast := approveCount + denyCount
	quorumThreshold := quorumVotesRequired(totalVoters)
	quorumReached := totalVoters > 0 && votesCast >= quorumThreshold
	now := time.Now().UTC()

	if quorumReached && state.QuorumReachedAt == nil {
		quorumReachedAt := now
		finalizeAt := now.Add(24 * time.Hour)
		_, err = tx.Exec(ctx, `
			UPDATE
				workflow_deletion_proposals
			SET
				vote_quorum_reached_at = $2,
				vote_finalize_at = $3,
				updated_at = NOW()
			WHERE
				id = $1;
		`, proposalId, quorumReachedAt, finalizeAt)
		if err != nil {
			return nil, fmt.Errorf("error setting deletion vote quorum countdown: %s", err)
		}
		state.QuorumReachedAt = &quorumReachedAt
		state.FinalizeAt = &finalizeAt
	}

	majorityThreshold := possibleBodyMajority(totalVoters)
	outcome := ""
	if totalVoters > 0 && approveCount >= majorityThreshold {
		outcome = "approve"
	} else if totalVoters > 0 && denyCount >= majorityThreshold {
		outcome = "deny"
	} else if quorumReached && state.FinalizeAt != nil && !now.Before(*state.FinalizeAt) {
		if approveCount > denyCount {
			outcome = "approve"
		} else {
			outcome = "deny"
		}
	}

	if outcome == "approve" {
		if err := finalizeWorkflowDeletionApprovalTx(ctx, tx, proposalId, state.TargetType, state.TargetWorkflow, state.TargetSeries, nil, "approve"); err != nil {
			return nil, err
		}
	}
	if outcome == "deny" {
		if err := finalizeWorkflowDeletionDenialTx(ctx, tx, proposalId, nil, "deny"); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return a.GetWorkflowDeletionProposalByIDForUser(ctx, proposalId, nil)
}

func countWorkflowDeletionVotesTx(ctx context.Context, tx pgx.Tx, proposalId string) (int, int, error) {
	row := tx.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE decision = 'approve'),
			COUNT(*) FILTER (WHERE decision = 'deny')
		FROM
			workflow_deletion_votes
		WHERE
			proposal_id = $1;
	`, proposalId)
	var approve int
	var deny int
	if err := row.Scan(&approve, &deny); err != nil {
		return 0, 0, err
	}
	return approve, deny, nil
}

func finalizeWorkflowDeletionApprovalTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalId string,
	targetType string,
	targetWorkflowId *string,
	targetSeriesId *string,
	actorUserId *string,
	decision string,
) error {
	if targetType == "workflow" && targetWorkflowId != nil {
		_, err := tx.Exec(ctx, `
			WITH deleted AS (
				UPDATE
					workflows
				SET
					status = 'deleted',
					updated_at = NOW()
				WHERE
					id = $1
				AND
					status IN ('approved', 'blocked', 'in_progress', 'completed')
				RETURNING
					id
			)
			UPDATE
				workflows
			SET
				is_start_blocked = false,
				blocked_by_workflow_id = NULL,
				status = CASE WHEN status = 'blocked' THEN 'approved' ELSE status END,
				updated_at = NOW()
			WHERE
				status = 'blocked'
			AND
				blocked_by_workflow_id IN (SELECT id FROM deleted);
		`, *targetWorkflowId)
		if err != nil {
			return fmt.Errorf("error deleting workflow from approved deletion vote: %s", err)
		}
	}

	if targetType == "series" && targetSeriesId != nil {
		_, err := tx.Exec(ctx, `
			WITH deleted AS (
				UPDATE
					workflows
				SET
					status = 'deleted',
					updated_at = NOW()
				WHERE
					series_id = $1
				AND
					status IN ('approved', 'blocked', 'in_progress', 'completed')
				RETURNING
					id
			)
			UPDATE
				workflows
			SET
				is_start_blocked = false,
				blocked_by_workflow_id = NULL,
				status = CASE WHEN status = 'blocked' THEN 'approved' ELSE status END,
				updated_at = NOW()
			WHERE
				status = 'blocked'
			AND
				blocked_by_workflow_id IN (SELECT id FROM deleted);
		`, *targetSeriesId)
		if err != nil {
			return fmt.Errorf("error deleting workflow series from approved deletion vote: %s", err)
		}
	}

	_, err := tx.Exec(ctx, `
		UPDATE
			workflow_deletion_proposals
		SET
			status = 'approved',
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, NOW()),
			vote_finalize_at = COALESCE(vote_finalize_at, NOW()),
			vote_finalized_at = COALESCE(vote_finalized_at, NOW()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = NOW()
		WHERE
			id = $1;
	`, proposalId, decision, actorUserId)
	if err != nil {
		return fmt.Errorf("error finalizing approved deletion vote: %s", err)
	}

	return nil
}

func finalizeWorkflowDeletionDenialTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalId string,
	actorUserId *string,
	decision string,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE
			workflow_deletion_proposals
		SET
			status = 'denied',
			vote_quorum_reached_at = COALESCE(vote_quorum_reached_at, NOW()),
			vote_finalize_at = COALESCE(vote_finalize_at, NOW()),
			vote_finalized_at = COALESCE(vote_finalized_at, NOW()),
			vote_finalized_by_user_id = COALESCE($3, vote_finalized_by_user_id),
			vote_decision = $2,
			updated_at = NOW()
		WHERE
			id = $1;
	`, proposalId, decision, actorUserId)
	if err != nil {
		return fmt.Errorf("error finalizing denied deletion vote: %s", err)
	}
	return nil
}

func (a *AppDB) GetIssuersWithScopes(ctx context.Context) ([]*structs.IssuerWithScopes, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			u.id,
			u.is_issuer
		FROM
			users u
		WHERE
			u.is_issuer = true
		OR
			EXISTS (
				SELECT 1
				FROM issuer_credential_scopes s
				WHERE s.issuer_id = u.id
			)
		ORDER BY
			u.id ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying issuers: %s", err)
	}
	defer rows.Close()

	results := []*structs.IssuerWithScopes{}
	for rows.Next() {
		issuer := structs.IssuerWithScopes{}
		if err := rows.Scan(&issuer.UserId, &issuer.IsIssuer); err != nil {
			return nil, fmt.Errorf("error scanning issuer: %s", err)
		}
		issuer.AllowedCredentials = []string{}
		results = append(results, &issuer)
	}

	for _, issuer := range results {
		scopes, err := a.GetIssuerScopeCredentials(ctx, issuer.UserId)
		if err != nil {
			return nil, err
		}
		issuer.AllowedCredentials = scopes
	}

	return results, nil
}

func (a *AppDB) GetIssuerScopeCredentials(ctx context.Context, issuerId string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			credential_type
		FROM
			issuer_credential_scopes
		WHERE
			issuer_id = $1
		ORDER BY
			credential_type ASC;
	`, issuerId)
	if err != nil {
		return nil, fmt.Errorf("error querying issuer scopes: %s", err)
	}
	defer rows.Close()

	credentials := []string{}
	for rows.Next() {
		var credential string
		if err := rows.Scan(&credential); err != nil {
			return nil, fmt.Errorf("error scanning issuer scope credential: %s", err)
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func (a *AppDB) SetIssuerScopes(ctx context.Context, adminId string, req *structs.IssuerScopeUpdateRequest) (*structs.IssuerWithScopes, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	req.UserId = strings.TrimSpace(req.UserId)
	if req.UserId == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	normalized := make([]string, 0, len(req.AllowedCredentials))
	seen := map[string]struct{}{}
	for _, credential := range req.AllowedCredentials {
		credential = strings.TrimSpace(credential)
		if credential == "" {
			continue
		}
		if !structs.IsValidCredentialType(credential) {
			return nil, fmt.Errorf("invalid credential type: %s", credential)
		}
		if _, exists := seen[credential]; exists {
			continue
		}
		seen[credential] = struct{}{}
		normalized = append(normalized, credential)
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			id = $1;
	`, req.UserId)
	var userId string
	if err := row.Scan(&userId); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("issuer user not found")
		}
		return nil, err
	}

	makeIssuer := true
	if req.MakeIssuer != nil {
		makeIssuer = *req.MakeIssuer
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			users
		SET
			is_issuer = $2
		WHERE
			id = $1;
	`, req.UserId, makeIssuer)
	if err != nil {
		return nil, fmt.Errorf("error updating issuer role: %s", err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM issuer_credential_scopes WHERE issuer_id = $1;
	`, req.UserId)
	if err != nil {
		return nil, fmt.Errorf("error resetting issuer scopes: %s", err)
	}

	for _, credential := range normalized {
		_, err = tx.Exec(ctx, `
			INSERT INTO issuer_credential_scopes
				(issuer_id, credential_type, created_by)
			VALUES
				($1, $2, $3);
		`, req.UserId, credential, adminId)
		if err != nil {
			return nil, fmt.Errorf("error inserting issuer scope: %s", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	scope := &structs.IssuerWithScopes{
		UserId:             req.UserId,
		IsIssuer:           makeIssuer,
		AllowedCredentials: normalized,
	}
	return scope, nil
}

func (a *AppDB) IssueCredential(ctx context.Context, issuerId string, req *structs.CredentialIssueRequest) (*structs.UserCredential, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	req.UserId = strings.TrimSpace(req.UserId)
	req.CredentialType = strings.TrimSpace(req.CredentialType)

	if req.UserId == "" || req.CredentialType == "" {
		return nil, fmt.Errorf("user_id and credential_type are required")
	}
	if !structs.IsValidCredentialType(req.CredentialType) {
		return nil, fmt.Errorf("invalid credential type")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var issuerIsAdmin bool
	var issuerIsIssuer bool
	err = tx.QueryRow(ctx, `
		SELECT
			is_admin,
			is_issuer
		FROM
			users
		WHERE
			id = $1;
	`, issuerId).Scan(&issuerIsAdmin, &issuerIsIssuer)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("issuer user not found")
		}
		return nil, err
	}

	if !issuerIsAdmin {
		if !issuerIsIssuer {
			return nil, fmt.Errorf("issuer role required")
		}

		scopeRow := tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				issuer_credential_scopes
			WHERE
				issuer_id = $1
			AND
				credential_type = $2;
		`, issuerId, req.CredentialType)
		var scopeCount int
		if err := scopeRow.Scan(&scopeCount); err != nil {
			return nil, err
		}
		if scopeCount == 0 {
			return nil, fmt.Errorf("issuer is not allowed to grant this credential")
		}
	}

	userRow := tx.QueryRow(ctx, `
		SELECT
			id
		FROM
			users
		WHERE
			id = $1;
	`, req.UserId)
	var targetUserId string
	if err := userRow.Scan(&targetUserId); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("target user not found")
		}
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			user_credentials
		SET
			is_revoked = false,
			revoked_at = NULL,
			issued_by = $3,
			issued_at = NOW()
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = true;
	`, req.UserId, req.CredentialType, issuerId)
	if err != nil {
		return nil, fmt.Errorf("error reactivating credential: %s", err)
	}

	row := tx.QueryRow(ctx, `
		SELECT
			id,
			user_id,
			credential_type,
			issued_by,
			issued_at,
			is_revoked,
			revoked_at
		FROM
			user_credentials
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = false
		LIMIT 1;
	`, req.UserId, req.CredentialType)

	credential := &structs.UserCredential{}
	err = row.Scan(
		&credential.Id,
		&credential.UserId,
		&credential.CredentialType,
		&credential.IssuedBy,
		&credential.IssuedAt,
		&credential.IsRevoked,
		&credential.RevokedAt,
	)
	if err == pgx.ErrNoRows {
		row = tx.QueryRow(ctx, `
			INSERT INTO user_credentials
				(user_id, credential_type, issued_by)
			VALUES
				($1, $2, $3)
			RETURNING
				id,
				user_id,
				credential_type,
				issued_by,
				issued_at,
				is_revoked,
				revoked_at;
		`, req.UserId, req.CredentialType, issuerId)
		err = row.Scan(
			&credential.Id,
			&credential.UserId,
			&credential.CredentialType,
			&credential.IssuedBy,
			&credential.IssuedAt,
			&credential.IsRevoked,
			&credential.RevokedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error issuing credential: %s", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error checking existing credential: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return credential, nil
}

func (a *AppDB) RevokeCredential(ctx context.Context, issuerId string, req *structs.CredentialIssueRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	req.UserId = strings.TrimSpace(req.UserId)
	req.CredentialType = strings.TrimSpace(req.CredentialType)

	if req.UserId == "" || req.CredentialType == "" {
		return fmt.Errorf("user_id and credential_type are required")
	}
	if !structs.IsValidCredentialType(req.CredentialType) {
		return fmt.Errorf("invalid credential type")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var issuerIsAdmin bool
	var issuerIsIssuer bool
	err = tx.QueryRow(ctx, `
		SELECT
			is_admin,
			is_issuer
		FROM
			users
		WHERE
			id = $1;
	`, issuerId).Scan(&issuerIsAdmin, &issuerIsIssuer)
	if err != nil {
		return err
	}

	if !issuerIsAdmin {
		if !issuerIsIssuer {
			return fmt.Errorf("issuer role required")
		}
		scopeRow := tx.QueryRow(ctx, `
			SELECT
				COUNT(*)
			FROM
				issuer_credential_scopes
			WHERE
				issuer_id = $1
			AND
				credential_type = $2;
		`, issuerId, req.CredentialType)
		var scopeCount int
		if err := scopeRow.Scan(&scopeCount); err != nil {
			return err
		}
		if scopeCount == 0 {
			return fmt.Errorf("issuer is not allowed to revoke this credential")
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE
			user_credentials
		SET
			is_revoked = true,
			revoked_at = NOW()
		WHERE
			user_id = $1
		AND
			credential_type = $2
		AND
			is_revoked = false;
	`, req.UserId, req.CredentialType)
	if err != nil {
		return fmt.Errorf("error revoking credential: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (a *AppDB) GetUserCredentials(ctx context.Context, userId string) ([]*structs.UserCredential, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			user_id,
			credential_type,
			issued_by,
			issued_at,
			is_revoked,
			revoked_at
		FROM
			user_credentials
		WHERE
			user_id = $1
		ORDER BY
			issued_at DESC;
	`, userId)
	if err != nil {
		return nil, fmt.Errorf("error querying user credentials: %s", err)
	}
	defer rows.Close()

	credentials := []*structs.UserCredential{}
	for rows.Next() {
		credential := structs.UserCredential{}
		if err := rows.Scan(
			&credential.Id,
			&credential.UserId,
			&credential.CredentialType,
			&credential.IssuedBy,
			&credential.IssuedAt,
			&credential.IsRevoked,
			&credential.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning user credential: %s", err)
		}
		credentials = append(credentials, &credential)
	}

	return credentials, nil
}
