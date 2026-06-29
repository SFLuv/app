package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) GetAdminMCPAllowedEmails(w http.ResponseWriter, r *http.Request) {
	emails, err := a.db.GetAdminMCPAllowedEmails(r.Context())
	if err != nil {
		a.logger.Logf("error getting admin mcp allowed emails: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(emails)
}

func (a *AppService) AddAdminMCPAllowedEmail(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	defer r.Body.Close()

	var req structs.AdminMCPAllowedEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	email, err := a.db.UpsertAdminMCPAllowedEmail(r.Context(), req.Email, *userDid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(email)
}

func (a *AppService) RevokeAdminMCPAllowedEmail(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	email := strings.TrimSpace(r.PathValue("email"))
	if email == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.RevokeAdminMCPAllowedEmail(r.Context(), email, *userDid); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
