package server

import (
	"net/http"
	"strings"
)

// authMiddleware returns an HTTP middleware that enforces Bearer token
// authentication. Requests without a valid "Authorization: Bearer <key>"
// header receive a 401 Unauthorized response and are not forwarded to the
// wrapped handler.
func authMiddleware(validKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != validKey {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
