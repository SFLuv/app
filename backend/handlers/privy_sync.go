package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const defaultPrivyAPIBaseURL = "https://auth.privy.io/api/v1"

var privyLinkedEmailAccountTypes = map[string]struct{}{
	"email":        {},
	"google_oauth": {},
	"google":       {},
	"apple_oauth":  {},
}

var privyManagementHTTPClient = &http.Client{
	Timeout: 12 * time.Second,
}

type privyUserRecord struct {
	ID             string           `json:"id"`
	Email          string           `json:"email"`
	LinkedAccounts []map[string]any `json:"linked_accounts"`
}

func privyAPIBaseURL() string {
	base := strings.TrimSpace(os.Getenv("PRIVY_API_BASE_URL"))
	if base == "" {
		base = defaultPrivyAPIBaseURL
	}
	return strings.TrimRight(base, "/")
}

func (a *AppService) privyManagementCredentials() (string, string, bool) {
	appId := strings.TrimSpace(os.Getenv("PRIVY_APP_ID"))
	appSecret := strings.TrimSpace(os.Getenv("PRIVY_APP_SECRET"))
	if appId == "" || appSecret == "" {
		return "", "", false
	}
	return appId, appSecret, true
}

func extractEmailsFromPrivyUser(record *privyUserRecord) []string {
	if record == nil {
		return nil
	}

	collected := map[string]string{}
	addEmail := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		parsed, err := mail.ParseAddress(raw)
		if err != nil || strings.TrimSpace(parsed.Address) == "" {
			return
		}
		normalized := strings.ToLower(strings.TrimSpace(parsed.Address))
		if normalized == "" {
			return
		}
		collected[normalized] = strings.TrimSpace(parsed.Address)
	}

	addEmail(record.Email)

	for _, account := range record.LinkedAccounts {
		accountType, _ := account["type"].(string)
		accountType = strings.ToLower(strings.TrimSpace(accountType))
		if _, ok := privyLinkedEmailAccountTypes[accountType]; !ok {
			continue
		}

		if value, ok := account["address"].(string); ok {
			if accountType == appleLinkedAccountType && isApplePrivateRelayEmail(value) {
				continue
			}
			addEmail(value)
		}
		if value, ok := account["email"].(string); ok {
			if accountType == appleLinkedAccountType && isApplePrivateRelayEmail(value) {
				continue
			}
			addEmail(value)
		}
	}

	emails := make([]string, 0, len(collected))
	for _, email := range collected {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	return emails
}

func (a *AppService) fetchPrivyUser(ctx context.Context, userDid string, appId string, appSecret string) (*privyUserRecord, error) {
	endpoint := fmt.Sprintf("%s/users/%s", privyAPIBaseURL(), url.PathEscape(strings.TrimSpace(userDid)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(appId, appSecret)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("privy-app-id", appId)

	res, err := privyManagementHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("privy get-user failed with status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var record privyUserRecord
	if err := json.NewDecoder(res.Body).Decode(&record); err != nil {
		return nil, fmt.Errorf("error decoding privy user response: %s", err)
	}
	if record.ID == "" {
		record.ID = userDid
	}
	return &record, nil
}

func (a *AppService) SyncPrivyLinkedEmailsForUser(ctx context.Context, userDid string) (int, error) {
	appId, appSecret, ok := a.privyManagementCredentials()
	if !ok {
		return 0, nil
	}

	record, err := a.fetchPrivyUser(ctx, userDid, appId, appSecret)
	if err != nil {
		return 0, err
	}
	if record == nil {
		return 0, nil
	}

	emails := extractEmailsFromPrivyUser(record)
	if len(emails) == 0 {
		return 0, nil
	}

	verifiedCount := 0
	for _, email := range emails {
		if _, err := a.db.UpsertVerifiedUserEmail(ctx, userDid, email); err != nil {
			a.logger.Logf("error upserting Privy-linked verified email for user %s and email %s: %s", userDid, email, err)
			continue
		}
		verifiedCount++
	}

	return verifiedCount, nil
}

func (a *AppService) SyncPrivyLinkedEmailsForAllUsers(ctx context.Context) error {
	_, _, ok := a.privyManagementCredentials()
	if !ok {
		a.logger.Logf("skipping Privy linked-email sync on startup: PRIVY_APP_ID or PRIVY_APP_SECRET not configured")
		return nil
	}

	userIds, err := a.db.GetAllUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("error listing users for Privy linked-email sync: %s", err)
	}

	totalVerified := 0
	totalFailures := 0
	for _, userDid := range userIds {
		count, err := a.SyncPrivyLinkedEmailsForUser(ctx, userDid)
		if err != nil {
			totalFailures++
			a.logger.Logf("error syncing Privy linked emails for user %s: %s", userDid, err)
			continue
		}
		totalVerified += count
	}

	a.logger.Logf(
		"completed Privy linked-email sync on startup: users=%d verified_emails_upserted=%d failures=%d",
		len(userIds),
		totalVerified,
		totalFailures,
	)
	return nil
}
