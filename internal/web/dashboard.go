package web

import (
	"net/http"

	"konkit/internal/auth"
)

type DashboardPageData struct {
	Title     string
	Username  string
	CSRFToken string
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	principal, _ := r.Context().Value(principalContextKey).(auth.Principal)
	rawToken, _ := r.Context().Value(sessionTokenContextKey).(string)
	data := DashboardPageData{
		Title:     "Dashboard Konkit",
		Username:  principal.Username,
		CSRFToken: auth.CSRFToken(s.deps.SessionSecret, rawToken),
	}
	if err := s.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, "Dashboard tidak dapat ditampilkan", http.StatusInternalServerError)
	}
}
