package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// Beta feature access: the server half of it.
//
// The client hides a tab it has not been granted, but hiding is not enforcing —
// a hidden tab is one HTTP call away from being used anyway. So every route
// behind a beta feature goes through requireBetaFeature, and the answer comes
// from the database rather than from anything the caller sends.
//
// Two conditions, both required:
//
//  1. the feature's env switch is on, which is the master control and can take
//     the surface away from everybody at once without touching the grants; and
//  2. this user holds an unrevoked grant for it.
//
// Admins are not exempt. Every other guard in this router lets an admin
// through, and this one deliberately does not: a beta gate that an admin
// cannot feel is a gate nobody can test, and the chat rules below — who may be
// messaged, who is greyed out, who stops getting pushes — are only observable
// from inside the beta. Granting yourself access is one click in the panel.

// betaFeatureEnvEnabled reports the master switch for one feature.
func betaFeatureEnvEnabled(feature string) bool {
	switch feature {
	case structs.BetaFeatureBulletin:
		return envBool(structs.EnvBetaBulletin, false)
	case structs.BetaFeatureChat:
		return envBool(structs.EnvBetaChat, false)
	default:
		return false
	}
}

// BetaFeatureAllowed is the effective answer: switch on and grant held.
//
// Takes a context rather than a request because the callers are not all
// handlers — a push is sent from a background goroutine with its own context,
// and it has to reach the same answer as the gate rather than re-deriving one.
func (a *AppService) BetaFeatureAllowed(ctx context.Context, userId string, feature string) bool {
	if !betaFeatureEnvEnabled(feature) {
		return false
	}
	active, err := a.db.UserHasBetaFeature(ctx, userId, feature)
	if err != nil {
		a.logger.Logf("error checking beta feature %s for %s: %s", feature, userId, err)
		// Closed on error. An unreadable grant is not a grant, and the failure
		// mode of guessing yes is somebody using a surface they were removed
		// from.
		return false
	}
	return active
}

// RequireBetaFeature wraps a handler so it can only be reached by somebody
// holding the feature. Returns 403 with a reason, because a bare 403 on a tab
// the client is showing reads to the user as the app being broken.
func (a *AppService) RequireBetaFeature(feature string, handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userDid := utils.GetDid(r)
		if userDid == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !a.BetaFeatureAllowed(r.Context(), *userDid, feature) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error":   "beta_feature_unavailable",
				"feature": feature,
				"message": "This feature is in beta and is not enabled for your account.",
			})
			return
		}
		handlerFunc(w, r)
	}
}

// ── The client's own view ────────────────────────────────────────────────────

type myBetaResponse struct {
	// Features is the raw grants, for display ("you are in the chat beta").
	Features []string `json:"features"`
	// Available is what the client should actually gate on: the grant and the
	// master switch together. A client that reads Features alone will show a
	// tab for a feature the server has turned off for everyone.
	Available map[string]bool `json:"available"`
}

// GetMyBetaFeatures tells the signed-in client which beta surfaces to show.
func (a *AppService) GetMyBetaFeatures(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	features, err := a.db.BetaFeaturesForUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error listing beta features for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	held := map[string]struct{}{}
	for _, feature := range features {
		held[feature] = struct{}{}
	}

	available := map[string]bool{}
	for _, definition := range structs.BetaFeatureCatalog() {
		_, has := held[definition.Id]
		available[definition.Id] = has && betaFeatureEnvEnabled(definition.Id)
	}

	writeJSON(w, http.StatusOK, myBetaResponse{Features: features, Available: available})
}

// ── Admin panel ─────────────────────────────────────────────────────────────

// AdminListBetaFeatures is the catalog, with the master switch and the current
// headcount for each. The switch is included because an admin who grants
// access while it is off would otherwise have no way to see why nothing
// happened.
func (a *AppService) AdminListBetaFeatures(w http.ResponseWriter, r *http.Request) {
	counts, err := a.db.CountBetaFeatureMembers(r.Context())
	if err != nil {
		a.logger.Logf("error counting beta members: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	catalog := structs.BetaFeatureCatalog()
	for index := range catalog {
		catalog[index].EnvEnabled = betaFeatureEnvEnabled(catalog[index].Id)
		catalog[index].Members = counts[catalog[index].Id]
	}

	writeJSON(w, http.StatusOK, map[string]any{"features": catalog})
}

// AdminListBetaFeatureMembers lists one feature's membership, revoked included.
func (a *AppService) AdminListBetaFeatureMembers(w http.ResponseWriter, r *http.Request) {
	feature := strings.TrimSpace(r.URL.Query().Get("feature"))
	if !structs.IsBetaFeature(feature) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown feature"})
		return
	}

	members, err := a.db.ListBetaFeatureMembers(r.Context(), feature)
	if err != nil {
		a.logger.Logf("error listing beta members for %s: %s", feature, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"feature":     feature,
		"env_enabled": betaFeatureEnvEnabled(feature),
		"members":     members,
	})
}

type betaAccessRequest struct {
	Feature string `json:"feature"`
	UserId  string `json:"user_id"`
}

func decodeBetaAccessRequest(w http.ResponseWriter, r *http.Request) (*betaAccessRequest, bool) {
	var req betaAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return nil, false
	}
	req.Feature = strings.TrimSpace(req.Feature)
	req.UserId = strings.TrimSpace(req.UserId)
	if !structs.IsBetaFeature(req.Feature) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown feature"})
		return nil, false
	}
	if req.UserId == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no user"})
		return nil, false
	}
	return &req, true
}

// AdminGrantBetaFeature lets somebody in, or back in.
func (a *AppService) AdminGrantBetaFeature(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBetaAccessRequest(w, r)
	if !ok {
		return
	}

	actor := ""
	if did := utils.GetDid(r); did != nil {
		actor = *did
	}

	if err := a.db.GrantBetaFeature(r.Context(), req.Feature, req.UserId, actor); err != nil {
		a.logger.Logf("error granting beta feature %s to %s: %s", req.Feature, req.UserId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.announceBetaChange(r.Context(), req.UserId)
	writeJSON(w, http.StatusOK, map[string]any{"feature": req.Feature, "user_id": req.UserId, "active": true})
}

// AdminRevokeBetaFeature closes access and leaves everything the person owns
// alone, so re-granting restores them rather than rebuilding them.
func (a *AppService) AdminRevokeBetaFeature(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBetaAccessRequest(w, r)
	if !ok {
		return
	}

	actor := ""
	if did := utils.GetDid(r); did != nil {
		actor = *did
	}

	if err := a.db.RevokeBetaFeature(r.Context(), req.Feature, req.UserId, actor); err != nil {
		a.logger.Logf("error revoking beta feature %s from %s: %s", req.Feature, req.UserId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.announceBetaChange(r.Context(), req.UserId)
	writeJSON(w, http.StatusOK, map[string]any{"feature": req.Feature, "user_id": req.UserId, "active": false})
}

// announceBetaChange tells a connected client its access changed, so a surface
// appears or disappears without waiting for a relaunch.
//
// This is what turning somebody's messaging on actually looks like from their
// side: the tab shows up while they are holding the phone, rather than the next
// time they happen to cold-start the app. The same event in reverse is how a
// revoke takes effect immediately on a device that is open.
//
// Best-effort and silent. Somebody who is not connected finds out at their next
// launch, which is exactly what happened before this existed.
func (a *AppService) announceBetaChange(ctx context.Context, userId string) {
	features, err := a.db.BetaFeaturesForUser(ctx, userId)
	if err != nil {
		a.logger.Logf("error reading beta features for %s after a change: %s", userId, err)
		return
	}
	a.PublishBetaChanged(userId, features)
}
