package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	apihttp "konkit/internal/api"
	"konkit/internal/auth"
	"konkit/internal/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationLoginDashboardAndLogout(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if poolConfig.ConnConfig.Database != "konkit_test" {
		t.Fatalf("integration tests require database konkit_test, got %q", poolConfig.ConnConfig.Database)
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	const email = "web-flow-admin@konkit.test"
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE lower(email) = lower($1)", email)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE lower(email) = lower($1)", email)
	})
	passwordHash, err := auth.HashPassword("integration-password")
	if err != nil {
		t.Fatal(err)
	}
	repository := auth.NewRepository(pool)
	if err := repository.CreateSuperAdmin(ctx, "web.flow.admin", email, passwordHash); err != nil {
		t.Fatal(err)
	}

	secret := []byte("01234567890123456789012345678901")
	handler := NewHandler(Dependencies{
		Auth:                auth.NewService(repository, 12*time.Hour, 30*24*time.Hour),
		API:                 apihttp.NewHandler(),
		SessionSecret:       secret,
		SessionCookieSecure: false,
		SessionTTL:          12 * time.Hour,
		RememberTTL:         30 * 24 * time.Hour,
	})

	login := formRequest(http.MethodPost, "/login", url.Values{
		"identity": {"WEB.FLOW.ADMIN"},
		"password": {"integration-password"},
	})
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusSeeOther || loginResponse.Header().Get("Location") != "/dashboard" {
		t.Fatalf("login failed: status=%d location=%q body=%s", loginResponse.Code, loginResponse.Header().Get("Location"), loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected session cookie, got %+v", cookies)
	}
	sessionCookie := cookies[0]

	dashboard := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	dashboard.AddCookie(sessionCookie)
	dashboardResponse := httptest.NewRecorder()
	handler.ServeHTTP(dashboardResponse, dashboard)
	if dashboardResponse.Code != http.StatusOK || !strings.Contains(dashboardResponse.Body.String(), "Selamat datang, web.flow.admin") {
		t.Fatalf("dashboard failed: status=%d body=%s", dashboardResponse.Code, dashboardResponse.Body.String())
	}

	logout := formRequest(http.MethodPost, "/logout", url.Values{
		"csrf_token": {auth.CSRFToken(secret, sessionCookie.Value)},
	})
	logout.AddCookie(sessionCookie)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusSeeOther || logoutResponse.Header().Get("Location") != "/login" {
		t.Fatalf("logout failed: status=%d location=%q", logoutResponse.Code, logoutResponse.Header().Get("Location"))
	}

	afterLogout := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	afterLogout.AddCookie(sessionCookie)
	afterLogoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(afterLogoutResponse, afterLogout)
	if afterLogoutResponse.Code != http.StatusFound || afterLogoutResponse.Header().Get("Location") != "/login" {
		t.Fatalf("revoked session still accepted: status=%d location=%q", afterLogoutResponse.Code, afterLogoutResponse.Header().Get("Location"))
	}
}
