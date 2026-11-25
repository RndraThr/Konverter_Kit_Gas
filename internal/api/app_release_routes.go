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
	LatestFor(ctx context.Context, platform string, currentCode int, abis []string) (*appreleases.Update, error)
	Upload(context.Context, auth.Principal, appreleases.UploadInput, io.Reader, auth.ClientMeta) (appreleases.Release, error)
	AddVariant(ctx context.Context, actor auth.Principal, id string, apk io.Reader, meta auth.ClientMeta) (appreleases.Variant, error)
	SetMandatory(ctx context.Context, actor auth.Principal, id string, mandatory bool, meta auth.ClientMeta) (appreleases.Release, error)
	Delete(ctx context.Context, actor auth.Principal, id string, meta auth.ClientMeta) error
	OpenVariant(ctx context.Context, id, abi string) (appreleases.Content, error)
	OpenPatch(ctx context.Context, id string, fromCode int) (appreleases.Content, error)
}

const maxAppReleaseRequestBody = appreleases.MaxAPKBytes + (1 << 20)

// handleAppReleases: GET (settings.view) lists releases; POST
// (settings.manage) uploads one as multipart: version_name, version_code,
// notes, mandatory, then the file part "apk", optionally followed by
// "apk_variant" parts (the 32-bit APK of the same version).
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
			for {
				next, err := reader.NextPart()
				if err != nil {
					break
				}
				if next.FormName() != "apk_variant" {
					continue
				}
				variant, err := h.deps.AppReleases.AddVariant(r.Context(), rc.principal, release.ID, next, clientMeta(r))
				if err != nil {
					writeAppReleaseError(w, err)
					return
				}
				release.Variants = append(release.Variants, variant)
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
	// abis: the phone's supported ABIs, preferred first (32-bit phones get
	// their own APK).
	var abis []string
	for _, abi := range strings.Split(r.URL.Query().Get("abis"), ",") {
		if abi = strings.TrimSpace(abi); abi != "" {
			abis = append(abis, abi)
		}
	}
	update, err := h.deps.AppReleases.LatestFor(r.Context(), platform, current, abis)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, update)
}

// handleAppRelease: {id}/apk (?abi= for another phone type) and
// {id}/patch/{from_code} download (any signed-in account); {id} PATCH
// {"mandatory": bool}, DELETE and POST {id}/variants (multipart "apk", the
// APK for another phone type) (settings.manage).
func (h *Handler) handleAppRelease(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	if h.deps.AppReleases == nil {
		writeUnavailable(w)
		return
	}
	id, rest, _ := strings.Cut(suffix, "/")
	fromText, isPatch := strings.CutPrefix(rest, "patch/")
	fromCode, fromErr := strconv.Atoi(fromText)
	if id == "" || (rest != "" && rest != "apk" && rest != "variants" && !isPatch) || (isPatch && (fromErr != nil || fromCode <= 0)) {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	if rest == "apk" || isPatch {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		var content appreleases.Content
		var err error
		contentType := "application/vnd.android.package-archive"
		if isPatch {
			content, err = h.deps.AppReleases.OpenPatch(r.Context(), id, fromCode)
			contentType = "application/octet-stream"
		} else {
			content, err = h.deps.AppReleases.OpenVariant(r.Context(), id, r.URL.Query().Get("abi"))
		}
		if err != nil {
			writeAppReleaseError(w, err)
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.FormatInt(content.Size, 10))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": content.Filename}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content.Reader)
		return
	}
	if rest == "variants" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		if !h.authorize(w, r, rc.principal, "settings.manage") {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxAppReleaseRequestBody)
		reader, err := r.MultipartReader()
		if err != nil {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
			return
		}
		for {
			part, err := reader.NextPart()
			if err != nil {
				writeError(w, http.StatusBadRequest, "apk_required", "Berkas APK wajib diunggah")
				return
			}
			if part.FormName() != "apk" {
				continue
			}
			variant, err := h.deps.AppReleases.AddVariant(r.Context(), rc.principal, id, part, clientMeta(r))
			if err != nil {
				writeAppReleaseError(w, err)
				return
			}
			writeData(w, http.StatusCreated, variant)
			return
		}
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
	case errors.Is(err, appreleases.ErrVariantABI):
		writeError(w, http.StatusBadRequest, "apk_abi_invalid", "APK tambahan harus versi 32-bit (app-armeabi-v7a-release.apk)")
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
