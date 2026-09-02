package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

func CSRFToken(secret []byte, sessionToken string) string {
	if len(secret) == 0 || sessionToken == "" {
		return ""
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func VerifyCSRF(secret []byte, sessionToken, submitted string) bool {
	if sessionToken == "" || submitted == "" {
		return false
	}
	expected := CSRFToken(secret, sessionToken)
	return hmac.Equal([]byte(expected), []byte(submitted))
}
