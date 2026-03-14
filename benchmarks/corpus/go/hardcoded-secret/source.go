package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

const apiSecret = "my-super-secret-production-api-key-do-not-share-12345" // nolint:gosec

func signRequest(payload string) string {
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Signature")
	expected := signRequest("test-payload")
	if signature != expected {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}
