package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("verify failed: ok=%v err=%v", ok, err)
	}

	wrong, err := VerifyPassword("wrong password", hash)
	if err != nil || wrong {
		t.Fatalf("wrong password accepted: ok=%v err=%v", wrong, err)
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("password", "not-a-phc-hash"); err == nil {
		t.Fatal("expected malformed hash error")
	}
}
