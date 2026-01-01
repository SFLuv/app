package utils

import (
	"net/http"
	"os"
)

func WebhookAuth(r *http.Request, body []byte) bool {
	sig := r.Header.Get("X-Alchemy-Signature")
	key := os.Getenv("ALCHEMY_WEBHOOK_AUTH_KEY")
	return HmacVerify(body, sig, key)
}
