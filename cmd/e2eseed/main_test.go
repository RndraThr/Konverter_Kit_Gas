package main

import "testing"

func TestValidateSeedTarget(t *testing.T) {
	if err := validateSeedTarget("test", "postgres://localhost/konkit_test?sslmode=disable"); err != nil {
		t.Fatalf("valid test target rejected: %v", err)
	}
	for _, test := range []struct{ env, url string }{
		{env: "local", url: "postgres://localhost/konkit_test"},
		{env: "test", url: "postgres://localhost/konkit"},
	} {
		if err := validateSeedTarget(test.env, test.url); err == nil {
			t.Fatalf("unsafe target accepted: env=%s url=%s", test.env, test.url)
		}
	}
}
