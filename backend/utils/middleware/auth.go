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

		token, err := jwt.ParseWithClaims(accessToken, &jwt.MapClaims{}, keyFunc)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		err = Valid(token.Claims)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		userDid, err := token.Claims.GetSubject()
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), "userDid", userDid)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	}))
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
