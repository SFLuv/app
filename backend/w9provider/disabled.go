package w9provider

import (
	"context"
	"errors"
	"net/http"
)

// ErrProviderDisabled is returned by every call when no vendor is configured.
//
// Callers must treat it as "cannot offer the form right now", never as "this
// person does not need to file". The distinction matters: escrow still happens
// without a provider, because holding money we are not yet allowed to pay is
// the safe failure. Only the route out is missing.
var ErrProviderDisabled = errors.New("no w9 provider is configured")

type disabled struct{}

func (disabled) Name() string { return "" }

func (disabled) EnsurePayee(context.Context, PayeeInput) (PayeeResult, error) {
	return PayeeResult{}, ErrProviderDisabled
}

func (disabled) CreateW9Request(context.Context, W9RequestInput) (W9Request, error) {
	return W9Request{}, ErrProviderDisabled
}

func (disabled) HostedFormURL(context.Context, string, string) (W9Request, error) {
	return W9Request{}, ErrProviderDisabled
}

func (disabled) GetW9Status(context.Context, string) (W9Status, error) {
	return W9Status{}, ErrProviderDisabled
}

func (disabled) VerifyWebhook(http.Header, []byte) (WebhookEvent, error) {
	return WebhookEvent{}, ErrProviderDisabled
}
