package web

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"konkit/internal/auth"
)

type AuthService interface {
	Login(context.Context, string, string, bool, auth.ClientMeta) (string, error)
	Authenticate(context.Context, string) (auth.Principal, error)
	Logout(context.Context, string) error
	Can(context.Context, auth.Principal, string) (bool, error)
}

type Dependencies struct {
	Auth                AuthService
	API                 http.Handler
	SessionSecret       []byte
	SessionCookieSecure bool
	SessionTTL          time.Duration
	RememberTTL         time.Duration
}

type authContextKey uint8

const (
	principalContextKey authContextKey = iota
	sessionTokenContextKey
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.loginPage(w, r)
	case http.MethodPost:
		s.loginPost(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		if _, err := s.deps.Auth.Authenticate(r.Context(), cookie.Value); err == nil {
			http.Redirect(w, r, "/dashboard", http.StatusFound)
			return
		}
	}

	script, styles := loadViteEntry(projectRoot())
	data := LoginPageData{
		Title:  "Sistem Manajemen Program Konkit Gas",
		Script: script,
		Styles: styles,
	}
	if err := s.templates.ExecuteTemplate(w, "login.html", data); err != nil {
		http.Error(w, "Failed to render login page", http.StatusInternalServerError)
	}
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
		return
	}
	identity := strings.TrimSpace(r.FormValue("identity"))
	password := r.FormValue("password")
	if identity == "" || password == "" {
		http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
		return
	}

	ip := clientIP(r.RemoteAddr)
	limiterKey := ip + "|" + strings.ToLower(identity)
	if !s.limiter.Allow(limiterKey, time.Now()) {
		http.Redirect(w, r, "/login?error=throttled", http.StatusSeeOther)
		return
	}

	remember := r.FormValue("remember") == "on"
	rawToken, err := s.deps.Auth.Login(r.Context(), identity, password, remember, auth.ClientMeta{
		IPAddress: ip,
		UserAgent: r.UserAgent(),
	})
	if errors.Is(err, auth.ErrInvalidCredentials) {
		http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
		return
	}
	if err != nil {
		log.Printf("login failed: %v", err)
		http.Error(w, "Login tidak dapat diproses", http.StatusInternalServerError)
		return
	}
	s.limiter.Reset(limiterKey)

	ttl := s.deps.SessionTTL
	if remember {
		ttl = s.deps.RememberTTL
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.deps.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Permintaan tidak valid", http.StatusBadRequest)
		return
	}
	rawToken, _ := r.Context().Value(sessionTokenContextKey).(string)
	if !auth.VerifyCSRF(s.deps.SessionSecret, rawToken, r.FormValue("csrf_token")) {
		http.Error(w, "Permintaan tidak diizinkan", http.StatusForbidden)
		return
	}
	if err := s.deps.Auth.Logout(r.Context(), rawToken); err != nil {
		log.Printf("logout failed: %v", err)
		http.Error(w, "Logout tidak dapat diproses", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.deps.SessionCookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.SessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		principal, err := s.deps.Auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), principalContextKey, principal)
		ctx = context.WithValue(ctx, sessionTokenContextKey, cookie.Value)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requirePermission(permission string, next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, _ := r.Context().Value(principalContextKey).(auth.Principal)
		allowed, err := s.deps.Auth.Can(r.Context(), principal, permission)
		if err != nil {
			log.Printf("permission check failed: %v", err)
			http.Error(w, "Akses tidak dapat diperiksa", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "Akses ditolak", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}
