package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/slotexamples"
)

// DocumentationExampleService keeps the example photo of each distribution
// documentation slot code.
type DocumentationExampleService interface {
	List(context.Context) ([]slotexamples.Slot, error)
	Save(context.Context, auth.Principal, slotexamples.SaveInput, auth.ClientMeta) (slotexamples.Example, error)
	Delete(ctx context.Context, actor auth.Principal, slotCode string, meta auth.ClientMeta) error
	Open(ctx context.Context, slotCode string) (slotexamples.Content, error)
}

const documentationExamplesManage = "documentation_examples.manage"

var documentationExampleReadPermissions = append([]string{documentationExamplesManage}, fieldReadPermissions...)

// handleDocumentationExamples: GET lists every slot code with its example.
func (h *Handler) handleDocumentationExamples(w http.ResponseWriter, r *http.Request, rc requestContext) {
	if h.deps.DocumentationExamples == nil {
		writeUnavailable(w)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorizeAny(w, r, rc.principal, documentationExampleReadPermissions...) {
		return
	}
	result, err := h.deps.DocumentationExamples.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

// handleDocumentationExample: {code} PUT (multipart caption + optional file)
// and DELETE for super admins; {code}/image GET for field officers.
func (h *Handler) handleDocumentationExample(w http.ResponseWriter, r *http.Request, rc requestContext, rest string) {
	if h.deps.DocumentationExamples == nil {
		writeUnavailable(w)
		return
	}
	code, image := strings.CutSuffix(rest, "/image")
	if code == "" || strings.Contains(code, "/") {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan")
		return
	}
	if image {
		h.serveDocumentationExampleImage(w, r, rc, code)
		return
	}
	switch r.Method {
	case http.MethodPut:
		if !h.authorize(w, r, rc.principal, documentationExamplesManage) {
			return
		}
		h.saveDocumentationExample(w, r, rc, code)
	case http.MethodDelete:
		if !h.authorize(w, r, rc.principal, documentationExamplesManage) {
			return
		}
		if err := h.deps.DocumentationExamples.Delete(r.Context(), rc.principal, code, clientMeta(r)); err != nil {
			writeDocumentationExampleError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "PUT, DELETE")
	}
}

func (h *Handler) saveDocumentationExample(w http.ResponseWriter, r *http.Request, rc requestContext, code string) {
	r.Body = http.MaxBytesReader(w, r.Body, slotexamples.MaxImageBytes+(1<<20))
	if err := r.ParseMultipartForm(slotexamples.MaxImageBytes); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Contoh foto melebihi batas 10 MiB")
		} else {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Format unggahan tidak valid")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	input := slotexamples.SaveInput{SlotCode: code, Caption: r.FormValue("caption")}
	if file, header, err := r.FormFile("file"); err == nil {
		defer file.Close()
		data, readErr := io.ReadAll(io.LimitReader(file, slotexamples.MaxImageBytes+1))
		if readErr != nil {
			writeError(w, http.StatusBadRequest, "multipart_invalid", "Contoh foto tidak dapat dibaca")
			return
		}
		input.Image, input.OriginalFilename = data, header.Filename
	}
	example, err := h.deps.DocumentationExamples.Save(r.Context(), rc.principal, input, clientMeta(r))
	if err != nil {
		writeDocumentationExampleError(w, err)
		return
	}
	writeData(w, http.StatusOK, example)
}

func (h *Handler) serveDocumentationExampleImage(w http.ResponseWriter, r *http.Request, rc requestContext, code string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !h.authorizeAny(w, r, rc.principal, documentationExampleReadPermissions...) {
		return
	}
	content, err := h.deps.DocumentationExamples.Open(r.Context(), code)
	if err != nil {
		writeDocumentationExampleError(w, err)
		return
	}
	defer content.Reader.Close()
	w.Header().Set("Content-Type", content.MimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": content.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The URL carries the update time, so a replaced photo gets a new URL.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Reader)
}

func writeDocumentationExampleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, slotexamples.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Contoh foto belum ada")
	case errors.Is(err, slotexamples.ErrImageRequired):
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Contoh foto wajib dipilih", map[string]string{"file": "Contoh foto wajib dipilih"})
	case errors.Is(err, slotexamples.ErrImageInvalid):
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Gunakan foto JPEG, PNG, atau WebP", map[string]string{"file": "Gunakan foto JPEG, PNG, atau WebP"})
	case errors.Is(err, slotexamples.ErrImageTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Contoh foto melebihi batas 10 MiB")
	case errors.Is(err, slotexamples.ErrInvalidInput):
		writeFieldError(w, http.StatusBadRequest, "validation_failed", "Keterangan maksimal 500 karakter", map[string]string{"caption": "Keterangan maksimal 500 karakter"})
	default:
		writeServiceError(w, err)
	}
}
