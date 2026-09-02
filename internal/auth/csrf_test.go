package auth

import "testing"

func TestCSRFTokenIsBoundToSession(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	token := CSRFToken(secret, "session-one")
	if token == "" {
		t.Fatal("expected CSRF token")
	}
	if !VerifyCSRF(secret, "session-one", token) {
		t.Fatal("expected valid token for original session")
	}
	if VerifyCSRF(secret, "session-two", token) {
		t.Fatal("token must not validate for another session")
	}
	if VerifyCSRF(secret, "session-one", token+"changed") {
		t.Fatal("changed token must be rejected")
	}
	if VerifyCSRF(secret, "", token) || VerifyCSRF(secret, "session-one", "") {
		t.Fatal("empty CSRF values must be rejected")
	}
}
