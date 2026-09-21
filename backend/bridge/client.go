// Package bridge is the SFLUV-side client for Bridge (bridge.xyz), the
// stablecoin orchestration provider behind merchant bank payouts.
//
// The money path is deliberately thin. A merchant's SFLUV is unwrapped on-chain
// into USDC and sent to a Bridge "liquidation address" — a per-location Celo
// address Bridge owns, which drains automatically to the merchant's linked bank
// account. This package only ever creates and reads the objects around that
// (customers, bank accounts, liquidation addresses, drains); it never moves
// money itself and it never sees a bank account number. Bank linking goes
// through Plaid Link, so account numbers travel merchant → Plaid → Bridge and
// nothing here is ever asked to hold one.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	sandboxAPI    = "https://api.sandbox.bridge.xyz"
	productionAPI = "https://api.bridge.xyz"

	// Everything we route is USDC on Celo out to USD over ACH. Keeping the
	// route as constants means a liquidation address cannot be created for a
	// chain the token does not live on by a typo in a request body.
	Chain               = "celo"
	Currency            = "usdc"
	DestinationRail     = "ach"
	DestinationCurrency = "usd"
)

// Config is read from the environment by bootstrap. An empty APIKey yields a
// disabled client: the routes exist, they answer "payouts are not configured",
// and nothing else in the server changes. Money already in wallets is
// unaffected by the off-ramp being absent.
type Config struct {
	// Environment is "production" or anything else, which means sandbox.
	// Sandbox is the default so a deploy that forgot to set it can only ever
	// create test objects.
	Environment string
	APIKey      string
	// BaseURL overrides the environment default. Tests point it at httptest.
	BaseURL string
	// WebhookPublicKeyPEM is the per-webhook RSA public key Bridge returns when
	// the webhook endpoint is registered. Deliveries are refused without it.
	WebhookPublicKeyPEM string
	// RedirectURL is where Bridge sends a merchant back after the hosted KYB
	// flow. It is the SFLUV settings page, so they land where the next step
	// (connecting a bank) lives.
	RedirectURL string
}

type Client struct {
	apiKey     string
	baseURL    string
	production bool
	redirect   string
	http       *http.Client

	// webhookKey is the PEM the signature check verifies against. It is
	// guarded because it is resolved from Bridge after the server is already
	// serving: the request goroutine reads it while the resolver writes it.
	webhookMu  sync.RWMutex
	webhookKey string
}

// WebhookPublicKey is the PEM currently in force, or "" when none has been
// resolved yet.
func (c *Client) WebhookPublicKey() string {
	if c == nil {
		return ""
	}
	c.webhookMu.RLock()
	defer c.webhookMu.RUnlock()
	return c.webhookKey
}

// SetWebhookPublicKey installs a PEM resolved from Bridge (or from a cache).
// An empty PEM is ignored so a failed lookup can never disarm verification.
func (c *Client) SetWebhookPublicKey(pem string) {
	pem = strings.TrimSpace(pem)
	if c == nil || pem == "" {
		return
	}
	c.webhookMu.Lock()
	defer c.webhookMu.Unlock()
	c.webhookKey = pem
}

func New(cfg Config) *Client {
	production := strings.EqualFold(strings.TrimSpace(cfg.Environment), "production")
	base := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		if production {
			base = productionAPI
		} else {
			base = sandboxAPI
		}
	}
	return &Client{
		apiKey:     strings.TrimSpace(cfg.APIKey),
		baseURL:    base,
		production: production,
		redirect:   strings.TrimSpace(cfg.RedirectURL),
		http:       &http.Client{Timeout: 20 * time.Second},
		webhookKey: strings.TrimSpace(cfg.WebhookPublicKeyPEM),
	}
}

// Enabled reports whether an API key was configured. Every handler checks it
// first and turns a false into a 503 the frontend renders as "not available
// yet", which is the honest state of a deploy without credentials.
func (c *Client) Enabled() bool { return c != nil && c.apiKey != "" }

func (c *Client) Production() bool { return c != nil && c.production }

func (c *Client) RedirectURL() string {
	if c == nil {
		return ""
	}
	return c.redirect
}

// ErrDisabled is returned by every call on a client without an API key.
var ErrDisabled = errors.New("bridge: client is not configured")

// APIError is a non-2xx from Bridge with whatever they said about it. The
// status is kept so handlers can tell a 404 (object gone) from a 422
// (rejected) from a 5xx (retry later).
type APIError struct {
	Status int
	Body   string
	Path   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bridge: %s returned %d: %s", e.Path, e.Status, e.Body)
}

// IsNotFound is the one status handlers branch on by name.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if !c.Enabled() {
		return ErrDisabled
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("bridge: encoding %s body: %w", path, err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return fmt.Errorf("bridge: building %s request: %w", path, err)
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Bridge requires an idempotency key on every POST and rejects one on PUT
	// (verified against sandbox: a PUT with the header is a 422). A fresh UUID
	// per call is the honest choice: our own idempotency is enforced by what
	// we store (one profile per owner, one address per location), not by
	// replaying a key, and reusing one across different bodies is rejected.
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", uuid.NewString())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("bridge: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("bridge: reading %s response: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw)), Path: path}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("bridge: decoding %s response: %w", path, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Customers and KYB
// ---------------------------------------------------------------------------

// KYCLink is Bridge's onboarding handle for a business we have not verified
// yet. Creating one creates the customer shell; the merchant fills in
// everything else (EIN, ownership, control persons, selfie) on Bridge's hosted
// page, and none of it comes back to us.
type KYCLink struct {
	ID         string `json:"id"`
	FullName   string `json:"full_name"`
	Email      string `json:"email"`
	Type       string `json:"type"`
	KYCLink    string `json:"kyc_link"`
	TOSLink    string `json:"tos_link"`
	KYCStatus  string `json:"kyc_status"`
	TOSStatus  string `json:"tos_status"`
	CustomerID string `json:"customer_id"`
}

// KYB status values Bridge reports, as documented. Anything else is stored
// verbatim and treated as "in progress" by the UI.
const (
	KYCNotStarted            = "not_started"
	KYCUnderReview           = "under_review"
	KYCIncomplete            = "incomplete"
	KYCAwaitingQuestionnaire = "awaiting_questionnaire"
	KYCAwaitingUBO           = "awaiting_ubo"
	KYCApproved              = "approved"
	KYCRejected              = "rejected"
	KYCPaused                = "paused"
	KYCOffboarded            = "offboarded"
)

type CreateKYCLinkInput struct {
	BusinessLegalName string
	Email             string
}

func (c *Client) CreateKYCLink(ctx context.Context, in CreateKYCLinkInput) (*KYCLink, error) {
	body := map[string]any{
		"full_name": strings.TrimSpace(in.BusinessLegalName),
		"email":     strings.TrimSpace(in.Email),
		"type":      "business",
	}
	if c.redirect != "" {
		body["redirect_uri"] = c.redirect
	}
	var out KYCLink
	if err := c.do(ctx, http.MethodPost, "/v0/kyc_links", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetKYCLink(ctx context.Context, id string) (*KYCLink, error) {
	var out KYCLink
	if err := c.do(ctx, http.MethodGet, "/v0/kyc_links/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Customer is the slice of Bridge's customer object we act on.
type Customer struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	Status            string `json:"status"`
	BusinessLegalName string `json:"business_legal_name"`
	Email             string `json:"email"`
	// RequirementsDue names what Bridge is still waiting on — "external_account"
	// is the one that matters here, meaning KYB passed but no bank is linked.
	RequirementsDue []string `json:"requirements_due"`
}

func (c *Client) GetCustomer(ctx context.Context, id string) (*Customer, error) {
	var out Customer
	if err := c.do(ctx, http.MethodGet, "/v0/customers/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HostedKYCLinkForCustomer re-issues a hosted link for a customer that already
// exists — the "resend verification" path. Links expire, so this is fetched
// when needed rather than stored as truth.
func (c *Client) HostedKYCLinkForCustomer(ctx context.Context, customerID string) (string, error) {
	path := "/v0/customers/" + url.PathEscape(customerID) + "/kyc_link"
	if c.redirect != "" {
		path += "?redirect_uri=" + url.QueryEscape(c.redirect)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// ---------------------------------------------------------------------------
// Bank accounts (Bridge "external accounts") via Plaid Link
// ---------------------------------------------------------------------------

type ExternalAccount struct {
	ID               string `json:"id"`
	Currency         string `json:"currency"`
	BankName         string `json:"bank_name"`
	Last4            string `json:"last_4"`
	AccountOwnerName string `json:"account_owner_name"`
	Active           bool   `json:"active"`
	AccountType      string `json:"account_type"`
}

func (c *Client) ListExternalAccounts(ctx context.Context, customerID string) ([]ExternalAccount, error) {
	var out struct {
		Data []ExternalAccount `json:"data"`
	}
	path := "/v0/customers/" + url.PathEscape(customerID) + "/external_accounts?limit=100"
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// PlaidLinkRequest is what the frontend needs to open Plaid Link. The token is
// short-lived; the callback URL is Bridge's, and we call it server-side so the
// API key never reaches a browser.
type PlaidLinkRequest struct {
	LinkToken          string `json:"link_token"`
	LinkTokenExpiresAt string `json:"link_token_expires_at"`
	CallbackURL        string `json:"callback_url"`
}

func (c *Client) CreatePlaidLinkRequest(ctx context.Context, customerID string) (*PlaidLinkRequest, error) {
	var out PlaidLinkRequest
	path := "/v0/customers/" + url.PathEscape(customerID) + "/plaid_link_requests"
	if err := c.do(ctx, http.MethodPost, path, map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExchangePlaidPublicToken hands Plaid's public token to Bridge, which then
// creates the external account(s) on its side, asynchronously. Nothing about
// the account comes back here; ListExternalAccounts a moment later is how it
// is discovered.
func (c *Client) ExchangePlaidPublicToken(ctx context.Context, linkToken, publicToken string) error {
	path := "/v0/plaid_exchange_public_token/" + url.PathEscape(linkToken)
	return c.do(ctx, http.MethodPost, path, map[string]any{"public_token": publicToken}, nil)
}

// ---------------------------------------------------------------------------
// Liquidation addresses and their drains
// ---------------------------------------------------------------------------

type LiquidationAddress struct {
	ID                     string `json:"id"`
	Address                string `json:"address"`
	Chain                  string `json:"chain"`
	Currency               string `json:"currency"`
	ExternalAccountID      string `json:"external_account_id"`
	DestinationPaymentRail string `json:"destination_payment_rail"`
	DestinationCurrency    string `json:"destination_currency"`
	State                  string `json:"state"`
}

func (c *Client) ListLiquidationAddresses(ctx context.Context, customerID string) ([]LiquidationAddress, error) {
	var out struct {
		Data []LiquidationAddress `json:"data"`
	}
	path := "/v0/customers/" + url.PathEscape(customerID) + "/liquidation_addresses?limit=100"
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

type CreateLiquidationAddressInput struct {
	CustomerID        string
	ExternalAccountID string
	// ACHReference is the memo that shows on the merchant's bank statement.
	ACHReference string
}

// CreateLiquidationAddress mints a Celo USDC address that drains to the given
// bank account over ACH. No developer fee is set, so the merchant receives the
// gross amount; Bridge's own fee is billed to us separately.
func (c *Client) CreateLiquidationAddress(ctx context.Context, in CreateLiquidationAddressInput) (*LiquidationAddress, error) {
	body := map[string]any{
		"chain":                    Chain,
		"currency":                 Currency,
		"external_account_id":      in.ExternalAccountID,
		"destination_payment_rail": DestinationRail,
		"destination_currency":     DestinationCurrency,
	}
	if ref := strings.TrimSpace(in.ACHReference); ref != "" {
		body["destination_ach_reference"] = ref
	}
	var out LiquidationAddress
	path := "/v0/customers/" + url.PathEscape(in.CustomerID) + "/liquidation_addresses"
	if err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Drain is one deposit into a liquidation address and its journey to the bank.
// DepositTxHash is the on-chain hash of the merchant's withdrawTo, which is
// how a drain is matched back to a row in our unwrap ledger.
type Drain struct {
	ID            string `json:"id"`
	State         string `json:"state"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	DepositTxHash string `json:"deposit_tx_hash"`
	// DepositTxTimestamp is when the deposit landed on-chain (RFC 3339). It is
	// the fallback key when the hash cannot be matched: the web app submits
	// through an ERC-4337 bundler, so the hash it sees can be the user
	// operation's, not the transaction's.
	DepositTxTimestamp string `json:"deposit_tx_timestamp"`
	// ACH trace number, the reference a merchant can quote to their bank.
	TraceNumber string `json:"trace_number"`
	CreatedAt   string `json:"created_at"`
}

// Drain states as documented. Terminal ones end the status sweep for a row.
const (
	DrainInReview            = "in_review"
	DrainFundsReceived       = "funds_received"
	DrainPaymentSubmitted    = "payment_submitted"
	DrainPaymentProcessed    = "payment_processed"
	DrainUndeliverable       = "undeliverable"
	DrainReturned            = "returned"
	DrainMissingReturnPolicy = "missing_return_policy"
	DrainRefunded            = "refunded"
	DrainRefundInFlight      = "refund_in_flight"
	DrainRefundFailed        = "refund_failed"
	DrainError               = "error"
	DrainCanceled            = "canceled"
)

func DrainIsTerminal(state string) bool {
	switch state {
	case DrainPaymentProcessed, DrainUndeliverable, DrainReturned, DrainRefunded, DrainRefundFailed, DrainError, DrainCanceled:
		return true
	}
	return false
}

func (c *Client) ListDrains(ctx context.Context, customerID, liquidationAddressID string) ([]Drain, error) {
	var out struct {
		Data []Drain `json:"data"`
	}
	path := "/v0/customers/" + url.PathEscape(customerID) + "/liquidation_addresses/" + url.PathEscape(liquidationAddressID) + "/drains?limit=100"
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// ---------------------------------------------------------------------------
// Webhook endpoints
// ---------------------------------------------------------------------------

// EventCategories are the deliveries this backend acts on. Anything outside
// this list is signed, parsed and dropped, so subscribing to it only costs
// work — see handleBridgeEvent, which branches on exactly these.
var EventCategories = []string{
	"customer",
	"kyc_link",
	"external_account",
	"liquidation_address.drain",
}

// Webhook is one registered endpoint. public_key is the point of this type:
// Bridge mints a per-endpoint RSA key and hands it back on create AND on list,
// so the signing key never has to be copied out of the dashboard by hand.
type Webhook struct {
	ID              string   `json:"id"`
	URL             string   `json:"url"`
	Status          string   `json:"status"`
	PublicKey       string   `json:"public_key"`
	EventCategories []string `json:"event_categories"`
}

func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var out struct {
		Data []Webhook `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v0/webhooks", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// CreateWebhook registers an endpoint.
//
// event_epoch is pinned to "webhook_creation" deliberately. The other option,
// "beginning_of_time", replays every event the account has ever produced — and
// every drain delivery kicks off a sweep, so a replay would be a self-inflicted
// stampede against both Bridge and our own database.
func (c *Client) CreateWebhook(ctx context.Context, url string, categories []string) (*Webhook, error) {
	body := map[string]any{
		"url":              url,
		"event_epoch":      "webhook_creation",
		"event_categories": categories,
	}
	var out Webhook
	if err := c.do(ctx, http.MethodPost, "/v0/webhooks", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
