package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/banner"
)

// BannerService generates the printed banners of a schedule's kabupaten.
type BannerService interface {
	Overview(context.Context, string, auth.RegencyScope) (banner.Overview, error)
	SaveSettings(context.Context, auth.Principal, string, banner.Settings, auth.RegencyScope, auth.ClientMeta) (banner.Overview, error)
	SaveLogo(context.Context, auth.Principal, string, string, []byte, auth.RegencyScope, auth.ClientMeta) (banner.Overview, error)
	Logo(context.Context, string, auth.RegencyScope) ([]byte, string, error)
	Banner(context.Context, string, banner.Kind, auth.RegencyScope) ([]byte, string, error)
	BannerImage(context.Context, string, banner.Kind, auth.RegencyScope) ([]byte, string, error)
}

// handleBanners serves /api/v1/banners/... for one schedule (?schedule_id=):
//
//	GET  (empty)            overview: kabupaten, logo, service center, typed text
//	PUT  settings           save the typed text (banner.manage)
//	GET  logo, PUT logo     the kabupaten logo (PUT multipart "file", banner.manage)
//	GET  {kind}.pdf         the banner PDF (backdrop, pelatihan, seremoni, plakat)
//	GET  {kind}.png         the same banner as a PNG image
func (h *Handler) handleBanners(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Banner == nil {
		writeUnavailable(w)
		return
	}
	manage := r.Method != http.MethodGet
	if !h.authorize(w, r, rc.principal, map[bool]string{true: "banner.manage", false: "banner.view"}[manage]) {
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	scheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if scheduleID == "" {
		writeError(w, http.StatusBadRequest, "schedule_required", "Pilih jadwal dulu")
		return
	}
	service := h.deps.Banner
	switch {
	case path == "" && r.Method == http.MethodGet:
		result, err := service.Overview(r.Context(), scheduleID, scope)
		writeBannerResult(w, result, err)
	case path == "settings" && r.Method == http.MethodPut:
		var input banner.Settings
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SaveSettings(r.Context(), rc.principal, scheduleID, input, scope, clientMeta(r))
		writeBannerResult(w, result, err)
	case path == "logo" && r.Method == http.MethodGet:
		data, mime, err := service.Logo(r.Context(), scheduleID, scope)
		if err != nil {
			writeBannerError(w, err)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "private, no-cache")
		_, _ = w.Write(data)
	case path == "logo" && r.Method == http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, banner.MaxLogoBytes+1<<20)
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file_required", "Pilih file logo PNG atau JPG")
			return
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(io.LimitReader(file, banner.MaxLogoBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "file_unreadable", "Logo tidak dapat dibaca")
			return
		}
		result, err := service.SaveLogo(r.Context(), rc.principal, scheduleID, header.Filename, data, scope, clientMeta(r))
		writeBannerResult(w, result, err)
	case strings.HasSuffix(path, ".pdf") && r.Method == http.MethodGet:
		data, filename, err := service.Banner(r.Context(), scheduleID, banner.Kind(strings.TrimSuffix(path, ".pdf")), scope)
		if err != nil {
			writeBannerError(w, err)
			return
		}
		disposition := "attachment"
		if r.URL.Query().Get("inline") == "1" {
			disposition = "inline"
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(data)
	case strings.HasSuffix(path, ".png") && r.Method == http.MethodGet:
		data, filename, err := service.BannerImage(r.Context(), scheduleID, banner.Kind(strings.TrimSuffix(path, ".png")), scope)
		if err != nil {
			writeBannerError(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(data)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func writeBannerResult(w http.ResponseWriter, result banner.Overview, err error) {
	if err != nil {
		writeBannerError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func writeBannerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, banner.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	case errors.Is(err, banner.ErrUnknownKind):
		writeError(w, http.StatusNotFound, "not_found", "Jenis banner tidak dikenal")
	case errors.Is(err, banner.ErrImageUnavailable):
		writeError(w, http.StatusServiceUnavailable, "banner_image_unavailable", "Unduhan PNG belum tersedia di server; gunakan PDF")
	case errors.Is(err, banner.ErrNotSupported):
		writeError(w, http.StatusUnprocessableEntity, "banner_not_supported", "Banner baru tersedia untuk program Petani")
	case errors.Is(err, banner.ErrInvalidLogo):
		writeError(w, http.StatusUnprocessableEntity, "banner_logo_invalid", "Logo harus berupa gambar PNG atau JPG")
	case errors.Is(err, banner.ErrLogoTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "banner_logo_too_large", "Ukuran logo maksimal 10 MB")
	case errors.Is(err, banner.ErrInvalidStyle):
		writeError(w, http.StatusUnprocessableEntity, "banner_style_invalid", "Ukuran huruf 60-140% dan warna berformat #RRGGBB")
	case errors.Is(err, banner.ErrInvalidDate):
		writeFieldError(w, http.StatusUnprocessableEntity, "validation_failed", "Tanggal plakat tidak valid", map[string]string{"plakat_date": "Tanggal tidak valid"})
	default:
		writeServiceError(w, err)
	}
}
