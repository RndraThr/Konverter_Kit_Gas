package api

import (
	"encoding/json"
	"net/http"
)

type response struct {
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func NewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			writeJSON(w, http.StatusNotFound, response{Error: "not_found"})
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, response{Error: "method_not_allowed"})
			return
		}
		writeJSON(w, http.StatusOK, response{Status: "ok"})
	})
}

func writeJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
