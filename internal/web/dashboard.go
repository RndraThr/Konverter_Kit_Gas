package web

import "net/http"

type DashboardPageData struct {
	Title  string
	Script string
	Styles []string
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	script, styles := loadViteEntry(projectRoot())
	data := DashboardPageData{
		Title: "Dashboard Konkit", Script: script, Styles: styles,
	}
	if err := s.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, "Dashboard tidak dapat ditampilkan", http.StatusInternalServerError)
	}
}
