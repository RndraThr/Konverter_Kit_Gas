package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/appreleases"
	"konkit/internal/auth"
)

// AppReleaseService manages mobile app releases (APK updates outside the
// Play Store).
type AppReleaseService interface {
	List(context.Context) ([]appreleases.Release, error)
	Latest(ctx context.Context, platform string, currentCode int) (*appreleases.Update, error)
	Upload(context.Context, auth.Principal, appreleases.UploadInput, io.Reader, auth.ClientMeta) (appreleases.Release, error)
	SetMandatory(ctx context.Context, actor auth.Principal, id string, mandatory bool, meta auth.ClientMeta) (appreleases.Release, error)
	Delete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta) error
	Open(ctx context.Context, id string) (appreleases.Content, error)
}

const maxAppReleaseRequestBody = appreleases.MaxAPKBytes + (1 << 20)

// handleAppReleases: GET (settings.view) lists releases; POST
// (settings.manage) uploads one as multipart: version_name, version_code,
// notes, mandatory, then the file part "apk" last.
func (h *Handler) handleAppReleases(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.AppReleases == nil {
		writeUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "settings.view") {
			return
		}
		result, err := h.deps.AppReleases.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case http.MethodPost:
		if !h.authorize(w, r, rc.principal, "settings.manage") {
			return
		}
		h.uploadAppRelease(w, r, rc)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (h *Handler) uploadAppRelease(w http.ResponseWriter, r *http.Request, rc requestContext) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAppReleaseRequestBody)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
		return
	}
	var input appreleases.UploadInput
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			writeError(w, http.StatusBadRequest, "apk_required", "Berkas APK wajib diunggah")
			return
		}
		if nextErr != nil {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			return
		}
		if part.FormName() == "apk" {
			release, err := h.deps.AppReleases.Upload(r.Context(), rc.principal, input, part, clientMeta(r))
			if err != nil {
				writeAppReleaseError(w, err)
				return
			}
			writeData(w, http.StatusCreated, release)
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(part, 4097))
		if readErr != nil || len(data) > 4096 {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			return
		}
		value := strings.TrimSpace(string(data))
		switch part.FormName() {
		case "version_name":
			input.VersionName = value
		case "version_code":
			input.VersionCode, _ = strconv.Atoi(value)
		case "notes":
			input.Notes = value
		case "mandatory":
			input.Mandatory = value == "true" || value == "1"
		}
	}
}

// handleAppReleaseLatest: any signed-in account; the app asks with its own
// version code and gets the update to offer, or null.
func (h *Handler) handleAppReleaseLatest(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.AppReleases == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	current, _ := strconv.Atoi(r.URL.Query().Get("current_code"))
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = "android"
	}
	update, err := h.deps.AppReleases.Latest(r.Context(), platform, current)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, update)
}

// handleAppRelease: {id}/apk downloads (any signed-in account); {id} PATCH
// {"mandatory": bool} and DELETE (settings.manage).
func (h *Handler) handleAppRelease(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.AppReleases == nil {
		writeUnavailable(w)
		return
	}
	id, rest, _ := strings.Cut(suffix, "/")
	if id == "" || (rest != "" && rest != "apk") {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	if rest == "apk" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		content, err := h.deps.AppReleases.Open(r.Context(), id)
		if err != nil {
			writeAppReleaseError(w, err)
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		w.Header().Set("Content-Length", strconv.FormatInt(content.Size, 10))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": content.Filename}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content.Reader)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		if !h.authorize(w, r, rc.principal, "settings.manage") {
			return
		}
		var input struct {
			Mandatory bool `json:"mandatory"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		release, err := h.deps.AppReleases.SetMandatory(r.Context(), rc.principal, id, input.Mandatory, clientMeta(r))
		if err != nil {
			writeAppReleaseError(w, err)
			return
		}
		writeData(w, http.StatusOK, release)
	case http.MethodDelete:
		if !h.authorize(w, r, rc.principal, "settings.manage") {
			return
		}
		if err := h.deps.AppReleases.Delete(r.Context(), rc.principal, id, clientMeta(r)); err != nil {
			writeAppReleaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "PATCH, DELETE")
	}
}

func writeAppReleaseError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, appreleases.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Rilis aplikasi tidak ditemukan")
	case errors.Is(err, appreleases.ErrNotAnAPK):
		writeError(w, http.StatusBadRequest, "apk_invalid", "Berkas bukan APK Android")
	case errors.Is(err, appreleases.ErrVersionTooLow):
		writeError(w, http.StatusConflict, "version_too_low", "Kode versi harus lebih tinggi dari rilis terakhir")
	case errors.Is(err, appreleases.ErrVersionExists):
		writeError(w, http.StatusConflict, "version_exists", "Rilis dengan kode versi ini sudah ada")
	case errors.Is(err, appreleases.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "validation_failed", "Nama versi dan kode versi wajib diisi")
	case errors.Is(err, appreleases.ErrFileTooLarge), errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "apk_too_large", "Berkas APK melebihi 200 MB")
	default:
		writeServiceError(w, err)
	}
}
