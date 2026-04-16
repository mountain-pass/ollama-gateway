package main

import (
	"context"
	"net/http"
	"strings"
)

type contextKey int

const ctxKeyToken contextKey = iota

// authMiddleware rejects requests that do not carry a valid Bearer token.
// Valid requests have the token stored in the request context for downstream use.
func authMiddleware(validTokens map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if _, valid := validTokens[token]; !valid {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyToken, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
