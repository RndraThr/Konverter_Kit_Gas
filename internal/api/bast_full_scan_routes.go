package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/bast"
)

type BASTFullScanService interface {
	Status(context.Context, string, string, string, auth.RegencyScope) (bast.FullScanStatus, error)
	Print(context.Context, string, string, string, auth.RegencyScope) (bast.AggregatePreview, error)
	Upload(context.Context, auth.Principal, string, string, string, []byte, auth.RegencyScope, auth.ClientMeta) error
	Delete(context.Context, auth.Principal, string, string, string, auth.RegencyScope, auth.ClientMeta) error
	Open(context.Context, string, string, string, auth.RegencyScope) ([]byte, error)
}

// handleBASTFullScan melayani /api/v1/bast/full-scan
// (?schedule_id=&document=&date=[&location_id=]):
//
//	GET          status pindai lengkap (bast.view)
//	GET  print   PDF BA lengkap untuk dicetak (bast.view)
//	GET  scan    PDF pindaian (bast.view)
//	PUT  scan    unggah multipart: "file" (PDF) atau beberapa "page" (JPG/PNG,
//	             dijilid jadi satu PDF) (bast.manage)
//	DELETE scan  hapus pindaian (bast.manage)
func (h *Handler) handleBASTFullScan(w http.ResponseWriter, r *http.Request, rc requestContext, suffix string) {
	service := h.deps.BASTFullScan
	if service == nil {
		writeUnavailable(w)
		return
	}
	scope, ok := h.regencyScope(w, r, rc.principal)
	if !ok {
		return
	}
	query := r.URL.Query()
	scheduleID, document, date := strings.TrimSpace(query.Get("schedule_id")), strings.TrimSpace(query.Get("document")), strings.TrimSpace(query.Get("date"))
	switch suffix {
	case "":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		status, err := service.Status(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, status)
	case "print":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		preview, err := service.Print(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.Header().Set("X-Page-Count", strconv.Itoa(preview.PageCount))
		writePDF(w, "inline", preview.Filename, bytes.NewReader(preview.PDF))
	case "scan":
		h.handleBASTFullScanFile(w, r, rc, service, scope, document, scheduleID, date)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
	}
}

func (h *Handler) handleBASTFullScanFile(w http.ResponseWriter, r *http.Request, rc requestContext, service BASTFullScanService, scope auth.RegencyScope, document, scheduleID, date string) {
	switch r.Method {
	case http.MethodGet:
		if !h.authorize(w, r, rc.principal, "bast.view") {
			return
		}
		pdf, err := service.Open(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-cache")
		writePDF(w, "inline", "pindaian-"+document+"-"+date+".pdf", bytes.NewReader(pdf))
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		pdf, ok := readFullScanUpload(w, r)
		if !ok {
			return
		}
		if err := service.Upload(r.Context(), rc.principal, document, scheduleID, date, pdf, scope, clientMeta(r)); err != nil {
			var pages *bast.FullScanPageError
			if errors.As(err, &pages) {
				writeError(w, http.StatusUnprocessableEntity, "full_scan_page_count", fmt.Sprintf("Pindaian berisi %d halaman, sedangkan BA %d halaman. Pindai semua halaman BA yang dicetak.", pages.Got, pages.Want))
				return
			}
			writeServiceError(w, err)
			return
		}
		status, err := service.Status(r.Context(), document, scheduleID, date, scope)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, status)
	case http.MethodDelete:
		if !h.authorize(w, r, rc.principal, "bast.manage") {
			return
		}
		if err := service.Delete(r.Context(), rc.principal, document, scheduleID, date, scope, clientMeta(r)); err != nil {
			writeServiceError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]string{"document": document})
	default:
		methodNotAllowed(w, "GET, PUT, DELETE")
	}
}

// readFullScanUpload membaca PDF pindaian dari multipart: satu "file" (PDF)
// atau halaman "page" (JPG/PNG) yang dijilid menjadi satu PDF.
func readFullScanUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, bast.MaxFullScanBytes+(2<<20))
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Berkas pindaian melebihi batas 30 MiB")
		return nil, false
	}
	defer r.MultipartForm.RemoveAll()
	if pages := r.MultipartForm.File["page"]; len(pages) > 0 {
		return assembleRakordaScan(w, pages)
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Berkas pindaian wajib dipilih", map[string]string{"file": "wajib diisi"})
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, bast.MaxFullScanBytes+1))
	if err != nil || len(data) > bast.MaxFullScanBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "Berkas pindaian melebihi batas 30 MiB")
		return nil, false
	}
	return data, true
}
