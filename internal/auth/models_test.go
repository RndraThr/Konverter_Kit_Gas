package auth

import "testing"

func TestPrincipalRecognizesSuperAdminRole(t *testing.T) {
	principal := Principal{Roles: []string{"viewer", "super_admin"}}
	if !principal.IsSuperAdmin() {
		t.Fatal("expected principal to have Super Admin access")
	}
}

func TestPrincipalWithoutSuperAdminRoleIsNotElevated(t *testing.T) {
	principal := Principal{Roles: []string{"viewer"}}
	if principal.IsSuperAdmin() {
		t.Fatal("expected viewer not to have Super Admin access")
	}
}
