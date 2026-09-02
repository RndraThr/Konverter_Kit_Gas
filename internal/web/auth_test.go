package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	apihttp "konkit/internal/api"
	"konkit/internal/auth"
)

func TestLoginPostSetsSecureSessionCookieAndRedirects(t *testing.T) {
	fake := &fakeAuthService{loginToken: "raw-session-token"}
	handler := NewHandler(testDependencies(fake, true))
	form := url.Values{"identity": {"Admin"}, "password": {"secret-password"}, "remember": {"on"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:4567"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("unexpected response: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != auth.SessionCookieName || cookie.Value != "raw-session-token" || !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("unexpected cookie: %+v", cookie)
	}
	if cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != int((30*24*time.Hour).Seconds()) {
		t.Fatalf("unexpected cookie policy: %+v", cookie)
	}
	if fake.identity != "Admin" || !fake.remember || fake.meta.IPAddress != "127.0.0.1" {
		t.Fatalf("unexpected login input: %+v", fake)
	}
}

func TestLoginPostRedirectsInvalidCredentialsToGenericError(t *testing.T) {
	fake := &fakeAuthService{loginErr: auth.ErrInvalidCredentials}
	req := formRequest(http.MethodPost, "/login", url.Values{
		"identity": {"admin"}, "password": {"wrong-password"},
	})
	rec := httptest.NewRecorder()

	NewHandler(testDependencies(fake, false)).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?error=invalid" {
		t.Fatalf("unexpected response: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLoginPostRejectsMissingFields(t *testing.T) {
	req := formRequest(http.MethodPost, "/login", url.Values{"identity": {"admin"}})
	rec := httptest.NewRecorder()
	NewHandler(testDependencies(&fakeAuthService{}, false)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?error=invalid" {
		t.Fatalf("unexpected response: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLoginPostThrottlesSixthFailedAttempt(t *testing.T) {
	fake := &fakeAuthService{loginErr: auth.ErrInvalidCredentials}
	handler := NewHandler(testDependencies(fake, false))
	for attempt := 1; attempt <= 6; attempt++ {
		req := formRequest(http.MethodPost, "/login", url.Values{
			"identity": {"admin"}, "password": {"wrong-password"},
		})
		req.RemoteAddr = "127.0.0.1:4567"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		want := "/login?error=invalid"
		if attempt == 6 {
			want = "/login?error=throttled"
		}
		if rec.Header().Get("Location") != want {
			t.Fatalf("attempt %d location=%q want=%q", attempt, rec.Header().Get("Location"), want)
		}
	}
}

func TestAuthenticatedUserOpeningLoginRedirectsToDashboard(t *testing.T) {
	fake := &fakeAuthService{principal: auth.Principal{UserID: "user-1", Username: "admin"}}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid-token"})
	rec := httptest.NewRecorder()

	NewHandler(testDependencies(fake, false)).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("unexpected response: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLogoutRejectsInvalidCSRFToken(t *testing.T) {
	fake := &fakeAuthService{principal: auth.Principal{UserID: "user-1", Username: "admin"}}
	req := formRequest(http.MethodPost, "/logout", url.Values{"csrf_token": {"wrong"}})
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid-token"})
	rec := httptest.NewRecorder()

	NewHandler(testDependencies(fake, false)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusForbidden)
	}
	if fake.loggedOutToken != "" {
		t.Fatal("invalid CSRF token must not revoke the session")
	}
}

func TestLogoutRevokesSessionAndExpiresCookie(t *testing.T) {
	fake := &fakeAuthService{principal: auth.Principal{UserID: "user-1", Username: "admin"}}
	deps := testDependencies(fake, false)
	req := formRequest(http.MethodPost, "/logout", url.Values{
		"csrf_token": {auth.CSRFToken(deps.SessionSecret, "valid-token")},
	})
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid-token"})
	rec := httptest.NewRecorder()

	NewHandler(deps).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("unexpected response: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if fake.loggedOutToken != "valid-token" {
		t.Fatalf("logged out token=%q", fake.loggedOutToken)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatalf("expected expired cookie, got %+v", cookies)
	}
}

func TestDashboardRedirectsAnonymousUserToLogin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()

	NewHandler(testDependencies(&fakeAuthService{}, false)).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("unexpected response: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestDashboardRendersAuthenticatedReactShell(t *testing.T) {
	fake := &fakeAuthService{principal: auth.Principal{UserID: "user-1", Username: "admin"}}
	deps := testDependencies(fake, false)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid-token"})
	rec := httptest.NewRecorder()

	NewHandler(deps).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{
		"Dashboard Konkit",
		`id="konkit-root"`,
		`type="module"`,
		`/static/app/assets/`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("dashboard does not contain %q", expected)
		}
	}
}

func TestDashboardDescendantRendersAuthenticatedReactShell(t *testing.T) {
	fake := &fakeAuthService{principal: auth.Principal{UserID: "user-1", Username: "admin"}}
	req := httptest.NewRequest(http.MethodGet, "/dashboard/pengguna", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid-token"})
	rec := httptest.NewRecorder()

	NewHandler(testDependencies(fake, false)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="konkit-root"`) {
		t.Fatalf("expected dashboard descendant shell, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDashboardRejectsUserWithoutPermission(t *testing.T) {
	fake := &fakeAuthService{
		principal: auth.Principal{UserID: "user-1", Username: "viewer"},
		deny:      true,
	}
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "valid-token"})
	rec := httptest.NewRecorder()

	NewHandler(testDependencies(fake, false)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusForbidden)
	}
}

func TestVersionedAPIIsMountedWithoutHTMLRedirect(t *testing.T) {
	deps := testDependencies(&fakeAuthService{}, false)
	deps.API = apihttp.NewHandler(apihttp.Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	NewHandler(deps).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected API response: status=%d content-type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func newTestHandler() http.Handler {
	return NewHandler(testDependencies(&fakeAuthService{}, false))
}

func testDependencies(service *fakeAuthService, secure bool) Dependencies {
	return Dependencies{
		Auth:                service,
		SessionSecret:       []byte("01234567890123456789012345678901"),
		SessionCookieSecure: secure,
		SessionTTL:          12 * time.Hour,
		RememberTTL:         30 * 24 * time.Hour,
	}
}

func formRequest(method, target string, values url.Values) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

type fakeAuthService struct {
	loginToken     string
	loginErr       error
	principal      auth.Principal
	authErr        error
	identity       string
	remember       bool
	meta           auth.ClientMeta
	loggedOutToken string
	deny           bool
}

func (f *fakeAuthService) Login(_ context.Context, identity, _ string, remember bool, meta auth.ClientMeta) (string, error) {
	f.identity = identity
	f.remember = remember
	f.meta = meta
	return f.loginToken, f.loginErr
}

func (f *fakeAuthService) Authenticate(context.Context, string) (auth.Principal, error) {
	if f.authErr != nil {
		return auth.Principal{}, f.authErr
	}
	if f.principal.UserID == "" {
		return auth.Principal{}, auth.ErrSessionNotFound
	}
	return f.principal, nil
}

func (f *fakeAuthService) Logout(_ context.Context, token string) error {
	f.loggedOutToken = token
	return nil
}

func (f *fakeAuthService) Can(context.Context, auth.Principal, string) (bool, error) {
	return !f.deny, nil
}

var _ AuthService = (*fakeAuthService)(nil)
