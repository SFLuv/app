package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/SFLuv/app/backend/protocol"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
)

// The Protocol panel.
//
// Two levels of access, because the two things it does are not the same thing.
// Seeing which Safe pays for what, and which key signs for it, is operational
// information. Minting a key and handing a Safe over to it is the most
// consequential action in the admin panel, and it is gated separately so it can
// be given to someone without giving them everything else, or withheld from
// someone who has the rest.
//
// A platform admin has both without a grant, matching every other admin
// surface. Anyone else needs the capability, which the superadmin hands out
// one at a time from the organization's Administrative section.

// protocolAccess resolves what this caller may do.
func (a *AppService) protocolAccess(w http.ResponseWriter, r *http.Request) (actor string, canRotate bool, ok bool) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return "", false, false
	}
	actor = *userDid

	if a.IsAdmin(r.Context(), actor) {
		return actor, true, true
	}

	canView, err := a.db.HasOrgCapability(r.Context(), actor, structs.CapAdminProtocolView)
	if err != nil {
		a.logger.Logf("error checking protocol view access for %s: %s", actor, err)
		w.WriteHeader(http.StatusInternalServerError)
		return "", false, false
	}
	if !canView {
		w.WriteHeader(http.StatusForbidden)
		return "", false, false
	}

	canRotate, err = a.db.HasOrgCapability(r.Context(), actor, structs.CapAdminProtocolRotate)
	if err != nil {
		a.logger.Logf("error checking protocol rotate access for %s: %s", actor, err)
		w.WriteHeader(http.StatusInternalServerError)
		return "", false, false
	}
	return actor, canRotate, true
}

// GetProtocolPanel is everything the tab renders: the accounts with their
// current signers, the address history behind them, and the cutover ledger.
func (a *AppService) GetProtocolPanel(w http.ResponseWriter, r *http.Request) {
	_, canRotate, ok := a.protocolAccess(w, r)
	if !ok {
		return
	}
	if a.protocolSvc == nil {
		writeJSON(w, http.StatusOK, structs.ProtocolPanelResponse{
			Accounts: []structs.ProtocolAccount{},
			History:  []structs.ProtocolAddressWindow{},
			Steps:    []structs.ProtocolMigrationStep{},
		})
		return
	}

	response := structs.ProtocolPanelResponse{
		ChainID:             a.protocolSvc.ChainID(),
		MasterKeyConfigured: protocol.MasterKeyConfigured(),
		SweepEnabled:        a.protocolSvc.SweepEnabled(),
		CanRotate:           canRotate,
		Accounts:            []structs.ProtocolAccount{},
		History:             []structs.ProtocolAddressWindow{},
		Steps:               []structs.ProtocolMigrationStep{},
	}

	accounts, err := a.protocolSvc.Accounts(r.Context())
	if err != nil {
		a.logger.Logf("error listing protocol accounts: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.Accounts = accounts

	history, err := a.db.ListProtocolAddressHistory(r.Context(), a.protocolSvc.ChainID())
	if err != nil {
		a.logger.Logf("error listing protocol address history: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.History = history

	steps, err := a.db.ListProtocolSteps(r.Context())
	if err != nil {
		a.logger.Logf("error listing protocol cutover steps: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.Steps = steps

	writeJSON(w, http.StatusOK, response)
}

// RotateProtocolAccount rotates one account's key.
func (a *AppService) RotateProtocolAccount(w http.ResponseWriter, r *http.Request) {
	actor, canRotate, ok := a.protocolAccess(w, r)
	if !ok {
		return
	}
	if !canRotate {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Rotating a protocol signing key needs the rotate capability.",
		})
		return
	}
	if a.protocolSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "The protocol service is not configured on this deployment.",
		})
		return
	}

	key := strings.TrimSpace(chi.URLParam(r, "key"))
	if key == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	rotated, err := a.protocolSvc.Rotate(r.Context(), key, actor)
	if err != nil {
		a.logger.Logf("protocol rotation of %s by %s failed: %s", key, actor, err)
		// The message is the operator's only account of what happened to a key,
		// so it is passed through rather than flattened to "rotation failed".
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rotated)
}

// RotateAllProtocolAccounts rotates every account, reporting each separately.
func (a *AppService) RotateAllProtocolAccounts(w http.ResponseWriter, r *http.Request) {
	actor, canRotate, ok := a.protocolAccess(w, r)
	if !ok {
		return
	}
	if !canRotate {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Rotating protocol signing keys needs the rotate capability.",
		})
		return
	}
	if a.protocolSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "The protocol service is not configured on this deployment.",
		})
		return
	}

	result := a.protocolSvc.RotateAll(r.Context(), actor)
	a.logger.Logf("protocol rotate-all by %s: %d rotated, %d failed", actor, len(result.Rotated), len(result.Failed))
	// 200 even with failures: the body distinguishes them per account, and a
	// status code cannot say "two of these four keys are new", which is exactly
	// what the operator has to know.
	writeJSON(w, http.StatusOK, result)
}

// SweepProtocolFunds moves the legacy EOA balances onto the Safes.
//
// Admin-only regardless of capability, and never offered to a capability
// holder: this is the irreversible half of the cutover, it spends with keys
// that are presumed exposed, and it happens once.
func (a *AppService) SweepProtocolFunds(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil || !a.IsAdmin(r.Context(), *userDid) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if a.protocolSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "The protocol service is not configured on this deployment.",
		})
		return
	}

	if err := a.protocolSvc.SweepLegacyFunds(r.Context(), true); err != nil {
		a.logger.Logf("protocol sweep by %s failed: %s", *userDid, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	steps, err := a.db.ListProtocolSteps(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "swept"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "swept", "steps": steps})
}
