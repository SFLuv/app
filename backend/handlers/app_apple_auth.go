package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

const appleOAuthProvider = "apple"
const appleLinkedAccountType = "apple_oauth"

type privyAppleAccount struct {
	Subject string
	Email   string
}

func isApplePrivateRelayEmail(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return strings.HasSuffix(normalized, "@privaterelay.appleid.com")
}

func extractLinkedAppleAccount(record *privyUserRecord) *privyAppleAccount {
	if record == nil {
		return nil
	}

	for _, account := range record.LinkedAccounts {
		accountType, _ := account["type"].(string)
		if strings.ToLower(strings.TrimSpace(accountType)) != appleLinkedAccountType {
			continue
		}

		subject, _ := account["subject"].(string)
		email, _ := account["email"].(string)
		return &privyAppleAccount{
			Subject: strings.TrimSpace(subject),
			Email:   strings.TrimSpace(email),
		}
	}

	return nil
}

func (a *AppService) StoreAppleOAuthCredential(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var request structs.AppleOAuthCredentialUpsertRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(request.AccessToken) == "" && strings.TrimSpace(request.RefreshToken) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	appleAccount := (*privyAppleAccount)(nil)
	appID, appSecret, ok := a.privyManagementCredentials()
	if ok {
		record, err := a.fetchPrivyUser(r.Context(), *userDid, appID, appSecret)
		if err != nil {
			a.logger.Logf("error fetching privy user for apple oauth credential sync %s: %s", *userDid, err)
		} else {
			appleAccount = extractLinkedAppleAccount(record)
		}
	}

	providerSubject := strings.TrimSpace(request.ProviderSubject)
	providerEmail := strings.TrimSpace(request.ProviderEmail)
	if appleAccount != nil {
		if providerSubject == "" {
			providerSubject = appleAccount.Subject
		}
		if providerEmail == "" {
			providerEmail = appleAccount.Email
		}
	}

	request.IsPrivateRelay = request.IsPrivateRelay || isApplePrivateRelayEmail(providerEmail)

	accessTokenEncrypted, err := encryptSensitiveValue(strings.TrimSpace(request.AccessToken))
	if err != nil {
		a.logger.Logf("error encrypting apple access token for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	refreshTokenEncrypted, err := encryptSensitiveValue(strings.TrimSpace(request.RefreshToken))
	if err != nil {
		a.logger.Logf("error encrypting apple refresh token for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	var accessTokenExpiresAt *time.Time
	if request.AccessTokenExpiresInSeconds > 0 {
		expiresAt := now.Add(time.Duration(request.AccessTokenExpiresInSeconds) * time.Second)
		accessTokenExpiresAt = &expiresAt
	}

	var refreshTokenExpiresAt *time.Time
	if request.RefreshTokenExpiresInSeconds > 0 {
		expiresAt := now.Add(time.Duration(request.RefreshTokenExpiresInSeconds) * time.Second)
		refreshTokenExpiresAt = &expiresAt
	}

	credential := &structs.UserOAuthCredential{
		UserID:                *userDid,
		Provider:              appleOAuthProvider,
		ProviderSubject:       providerSubject,
		ProviderEmail:         providerEmail,
		IsPrivateRelay:        request.IsPrivateRelay,
		AccessTokenEncrypted:  accessTokenEncrypted,
		RefreshTokenEncrypted: refreshTokenEncrypted,
		AccessTokenExpiresAt:  accessTokenExpiresAt,
		RefreshTokenExpiresAt: refreshTokenExpiresAt,
		Scopes:                request.Scopes,
	}

	if err := a.db.UpsertUserOAuthCredential(r.Context(), credential); err != nil {
		a.logger.Logf("error storing apple oauth credential for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if exists, err := a.db.UserExistsAndActive(r.Context(), *userDid); err == nil && exists {
		if _, err := a.SyncPrivyLinkedEmailsForUser(r.Context(), *userDid); err != nil {
			a.logger.Logf("error syncing Privy linked emails after apple oauth store for user %s: %s", *userDid, err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) ResolveAppleRecovery(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	request := structs.AppleRecoveryRequest{}
	if r.Body != nil {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			if err := json.Unmarshal(body, &request); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}

	response := &structs.AppleRecoveryResponse{
		CurrentUserID: *userDid,
		Resolution:    structs.AppleRecoveryResolutionNoAppleAccount,
	}

	exists, err := a.db.UserExistsAndActive(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error checking active backend user for apple recovery %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.CurrentUserExists = exists
	if exists {
		response.Resolution = structs.AppleRecoveryResolutionCurrentAccountExists
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	providerSubject := strings.TrimSpace(request.ProviderSubject)
	providerEmail := strings.TrimSpace(request.ProviderEmail)
	isPrivateRelay := request.IsPrivateRelay || isApplePrivateRelayEmail(providerEmail)

	appID, appSecret, ok := a.privyManagementCredentials()
	if ok {
		record, err := a.fetchPrivyUser(r.Context(), *userDid, appID, appSecret)
		if err != nil {
			a.logger.Logf("error fetching Privy user for apple recovery %s: %s", *userDid, err)
		} else if appleAccount := extractLinkedAppleAccount(record); appleAccount != nil {
			if providerSubject == "" {
				providerSubject = appleAccount.Subject
			}
			if providerEmail == "" {
				providerEmail = appleAccount.Email
			}
			isPrivateRelay = isPrivateRelay || isApplePrivateRelayEmail(providerEmail)
		}
	}

	response.AppleLinked = providerSubject != "" || providerEmail != ""
	response.AppleEmail = providerEmail
	response.IsPrivateRelay = isPrivateRelay
	if !response.AppleLinked {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	if providerEmail == "" || isPrivateRelay {
		response.Resolution = structs.AppleRecoveryResolutionNoMatch
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	matches, err := a.db.FindActiveUsersByVerifiedEmail(r.Context(), providerEmail)
	if err != nil {
		a.logger.Logf("error finding active users by apple verified email for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	candidates := make([]*structs.User, 0, len(matches))
	for _, candidate := range matches {
		if candidate == nil || candidate.Id == *userDid {
			continue
		}
		candidates = append(candidates, candidate)
	}

	switch len(candidates) {
	case 0:
		response.Resolution = structs.AppleRecoveryResolutionNoMatch
	case 1:
		response.Resolution = structs.AppleRecoveryResolutionRecoverySuggested
		response.SuggestedExistingAccount = &structs.AppleRecoverySuggestedAccount{
			UserID:               candidates[0].Id,
			VerifiedEmail:        providerEmail,
			PrimaryWalletAddress: candidates[0].PrimaryWalletAddress,
		}
		if candidates[0].Name != nil {
			response.SuggestedExistingAccount.ContactName = strings.TrimSpace(*candidates[0].Name)
		}
	default:
		response.Resolution = structs.AppleRecoveryResolutionAmbiguousMatch
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (a *AppService) revokeStoredAppleCredential(ctx context.Context, userID string) error {
	credential, err := a.db.GetUserOAuthCredential(ctx, userID, appleOAuthProvider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	tokenTypeHint := "access_token"
	revocationToken := credential.AccessTokenEncrypted
	if strings.TrimSpace(credential.RefreshTokenEncrypted) != "" {
		tokenTypeHint = "refresh_token"
		revocationToken = credential.RefreshTokenEncrypted
	}
	if strings.TrimSpace(revocationToken) == "" {
		return a.db.DeleteUserOAuthCredential(ctx, userID, appleOAuthProvider)
	}

	token, err := decryptSensitiveValue(revocationToken)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return a.db.DeleteUserOAuthCredential(ctx, userID, appleOAuthProvider)
	}

	clientSecret, err := buildAppleClientSecret()
	if err != nil {
		return err
	}

	if err := revokeAppleToken(ctx, clientSecret, token, tokenTypeHint); err != nil {
		return err
	}

	if err := a.db.MarkUserOAuthCredentialRevoked(ctx, userID, appleOAuthProvider, time.Now().UTC()); err != nil {
		return err
	}

	return a.db.DeleteUserOAuthCredential(ctx, userID, appleOAuthProvider)
}
