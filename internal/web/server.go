package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"konkit/internal/auth"
)

type Server struct {
	mux       *http.ServeMux
	templates *template.Template
	deps      Dependencies
	limiter   *auth.LoginLimiter
}

func NewHandler(deps Dependencies) http.Handler {
	root := projectRoot()
	server := &Server{
		mux:       http.NewServeMux(),
		templates: template.Must(template.ParseGlob(filepath.Join(root, "web", "templates", "*.html"))),
		deps:      deps,
		limiter:   auth.NewLoginLimiter(5, 15*time.Minute),
	}

	server.routes()
	return server.mux
}

func (s *Server) routes() {
	static := http.FileServer(http.Dir(filepath.Join(projectRoot(), "web", "static")))
	s.mux.Handle("/static/", http.StripPrefix("/static/", static))

	s.mux.HandleFunc("/", s.redirectToLogin)
	s.mux.HandleFunc("/favicon.ico", s.favicon)
	s.mux.HandleFunc("/login", s.login)
	s.mux.Handle("/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	s.mux.Handle("/dashboard", s.requirePermission("dashboard.view", http.HandlerFunc(s.dashboard)))
	if s.deps.API != nil {
		s.mux.Handle("/api/v1/", s.deps.API)
	}
}

func projectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			return "."
		}
		wd = parent
	}
}

func (s *Server) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) favicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	http.ServeFile(w, r, filepath.Join(projectRoot(), "web", "static", "images", "favicon.svg"))
}

type LoginPageData struct {
	Title  string
	Script string
	Styles []string
}

type viteManifestEntry struct {
	File string   `json:"file"`
	CSS  []string `json:"css"`
}

func loadViteEntry(root string) (script string, styles []string) {
	const fallbackScript = "/static/app/assets/index.js"

	path := filepath.Join(root, "web", "static", "app", ".vite", "manifest.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return fallbackScript, nil
	}

	var manifest map[string]viteManifestEntry
	if err := json.Unmarshal(content, &manifest); err != nil {
		return fallbackScript, nil
	}

	entry, ok := manifest["index.html"]
	if !ok || entry.File == "" {
		return fallbackScript, nil
	}

	for _, css := range entry.CSS {
		styles = append(styles, "/static/app/"+css)
	}

	return "/static/app/" + entry.File, styles
}
