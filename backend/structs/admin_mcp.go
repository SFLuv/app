package structs

import "time"

type AdminMCPAllowedEmail struct {
	Email           string     `json:"email"`
	CreatedByUserID string     `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	RevokedByUserID string     `json:"revoked_by_user_id,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	Active          bool       `json:"active"`
}

type AdminMCPAllowedEmailRequest struct {
	Email string `json:"email"`
}
