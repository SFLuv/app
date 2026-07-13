package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return (http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		accessToken := r.Header.Get("Access-Token")
		userDid, err := ValidatePrivyToken(accessToken)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), "userDid", userDid)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	}))
}

// ValidatePrivyToken parses and validates a Privy access token (ES256) using the
// same verification key, audience, issuer, and expiry checks the app uses for
// its normal auth, and returns the authenticated user's DID (the JWT subject).
//
// It is the single source of truth for Privy token validation so that any new
// surface (e.g. the admin MCP endpoint) authenticates identically to the rest
// of the backend rather than re-implementing token checks.
func ValidatePrivyToken(accessToken string) (string, error) {
	if accessToken == "" {
		return "", errors.New("missing access token")
	}

	token, err := jwt.ParseWithClaims(accessToken, &jwt.MapClaims{}, keyFunc)
	if err != nil {
		return "", err
	}

	if err := Valid(token.Claims); err != nil {
		return "", err
	}

	userDid, err := token.Claims.GetSubject()
	if err != nil {
		return "", err
	}
	if userDid == "" {
		return "", errors.New("token subject is empty")
	}

	return userDid, nil
}

func Valid(c jwt.Claims) error {
	appId := os.Getenv("PRIVY_APP_ID")

	aud, err := c.GetAudience()
	if err != nil {
		return err
	}
	if aud[0] != appId {
		return errors.New("aud claim must be your Privy App ID")
	}

	iss, err := c.GetIssuer()
	if err != nil {
		return err
	}
	if iss != "privy.io" {
		return errors.New("iss claim must be 'privy.io'")
	}

	exp, err := c.GetExpirationTime()
	if err != nil {
		return err
	}
	if exp.Before(time.Now()) {
		return errors.New("token is expired")
	}

	return nil
}

func keyFunc(token *jwt.Token) (interface{}, error) {
	verificationKey := os.Getenv("PRIVY_VKEY")
	if token.Method.Alg() != "ES256" {
		return nil, fmt.Errorf("unexpected JWT signing method=%v", token.Header["alg"])
	}
	return jwt.ParseECPublicKeyFromPEM([]byte(verificationKey))
}
