package utils

import (
	"context"
	"net/http"
)

type ContextSpoofer struct {
	key   any
	value any
}

func NewContextSpoofer(key any, value any) *ContextSpoofer {
	return &ContextSpoofer{
		key,
		value,
	}
}

func (c *ContextSpoofer) SetValue(key any, value any) {
	c.key = key
	c.value = value
}

func (c *ContextSpoofer) Middleware(next http.Handler) http.Handler {
	return (http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), c.key, c.value)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	}))
}
