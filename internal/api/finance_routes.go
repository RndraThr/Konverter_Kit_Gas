package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"konkit/internal/auth"
	"konkit/internal/finance"
	"konkit/internal/notifications"
)

// FinanceService runs pengajuan keuangan.
type FinanceService interface {
	Profile(context.Context, auth.Principal, finance.Access) (finance.Profile, error)
	List(context.Context, auth.Principal, finance.Access, finance.Filter) (finance.Page, error)
	Get(context.Context, auth.Principal, finance.Access, string) (finance.Request, error)
	SaveDraft(context.Context, auth.Principal, finance.Access, string, finance.DraftInput) (finance.Request, error)
	Delete(context.Context, auth.Principal, string) error
	Act(context.Context, auth.Principal, finance.Access, string, string, string, auth.ClientMeta) (finance.Request, error)
	SaveSettlement(context.Context, auth.Principal, finance.Access, string, finance.SettlementInput) (finance.Request, error)
	AddAttachment(context.Context, auth.Principal, finance.Access, string, string, string, string, io.Reader) (finance.Attachment, error)
	VoucherSettings(context.Context) (finance.VoucherSettings, error)
	SaveVoucherSettings(context.Context, auth.Principal, finance.VoucherSettings, auth.ClientMeta) (finance.VoucherSettings, error)
	SaveLogo(context.Context, io.Reader) error
	Logo(context.Context) ([]byte, error)
	Vouchers(context.Context, auth.Principal, finance.Access) ([]finance.Voucher, error)
	Voucher(context.Context, auth.Principal, finance.Access, string) (finance.Voucher, error)
	VoucherCandidates(context.Context, auth.Principal, finance.Access, string, string) ([]finance.Request, error)
	ComposeVoucher(context.Context, auth.Principal, finance.Access, []string) (finance.VoucherDraft, error)
	SaveVoucher(context.Context, auth.Principal, finance.Access, string, finance.VoucherInput, auth.ClientMeta) (finance.Voucher, error)
	PreviewVoucher(context.Context, auth.Principal, finance.Access, finance.VoucherInput) ([]byte, error)
	OpenVoucher(context.Context, auth.Principal, finance.Access, string) (finance.VoucherContent, error)
	DeleteAttachment(context.Context, auth.Principal, finance.Access, string, string) error
	OpenAttachment(context.Context, auth.Principal, finance.Access, string, string) (finance.AttachmentContent, error)
	Summary(context.Context, auth.Principal, finance.Access, finance.Filter) (finance.Summary, error)
	DailyReport(context.Context, auth.Principal, finance.Access, string, string, string) ([]byte, error)
	ExportReport(context.Context, auth.Principal, finance.Access, finance.Filter) ([]byte, error)
	Teams(context.Context) ([]finance.Team, error)
	TeamOptions(context.Context) (finance.TeamOptions, error)
	SaveTeam(context.Context, auth.Principal, string, finance.TeamInput, auth.ClientMeta) (finance.Team, error)
	DeleteTeam(context.Context, auth.Principal, string, auth.ClientMeta) error
	Payees(context.Context, auth.Principal, finance.Access, string) ([]finance.Payee, error)
	SavePayee(context.Context, auth.Principal, finance.Access, string, finance.Payee) (finance.Payee, error)
	DeletePayee(context.Context, auth.Principal, finance.Access, string) error
}

// NotificationService is each user's inbox, push settings and devices.
type NotificationService interface {
	List(context.Context, string, int) ([]notifications.Notification, int, error)
	MarkRead(context.Context, string, string) error
	MarkAllRead(context.Context, string) error
	Preferences(context.Context, string) (map[string]bool, error)
	SetPreferences(context.Context, string, map[string]bool) (map[string]bool, error)
	RegisterDevice(context.Context, string, string, string) error
	RemoveDevice(context.Context, string, string) error
	Status(context.Context, string) (notifications.PushStatus, error)
	TestPush(context.Context, string) (notifications.TestResult, error)
}

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func (h *Handler) financeAccess(w http.ResponseWriter, r *http.Request, principal auth.Principal) (finance.Access, bool) {
	var access finance.Access
	for _, check := range []struct {
		permission string
		target     *bool
	}{
		{"finance.request", &access.CanRequest}, {"finance.review", &access.CanReview},
		{"finance.monitor", &access.CanMonitor}, {"finance.manage", &access.CanManage},
	} {
		allowed, err := h.deps.Auth.Can(r.Context(), principal, check.permission)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Tidak dapat memeriksa akses")
			return finance.Access{}, false
		}
		*check.target = allowed
	}
	if !access.CanRequest && !access.CanReview && !access.CanMonitor && !access.CanManage {
		writeError(w, http.StatusForbidden, "forbidden", "Anda tidak memiliki akses")
		return finance.Access{}, false
	}
	return access, true
}

func writeFinanceError(w http.ResponseWriter, err error) {
	var status int
	var code, message string
	var proof *finance.ProofError
	switch {
	case errors.As(err, &proof):
		status, code, message = http.StatusUnprocessableEntity, "finance_proof_unreadable", fmt.Sprintf("Bukti %s tidak bisa dibaca. Ganti berkasnya.", proof.Filename)
	case errors.Is(err, finance.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "Data tidak ditemukan"
	case errors.Is(err, finance.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", "Anda tidak memiliki akses"
	case errors.Is(err, finance.ErrSelfReview):
		status, code, message = http.StatusForbidden, "finance_self_review", "Finance tidak dapat memproses pengajuannya sendiri"
	case errors.Is(err, finance.ErrNotLeader):
		status, code, message = http.StatusForbidden, "finance_not_leader", "Anda bukan leader tim ini"
	case errors.Is(err, finance.ErrInvalidInput):
		status, code, message = http.StatusBadRequest, "invalid_request", "Isian belum benar. Periksa lagi."
	case errors.Is(err, finance.ErrIncomplete):
		status, code, message = http.StatusUnprocessableEntity, "finance_incomplete", "Lengkapi semua isian wajib, rincian, dan lampiran yang dibutuhkan"
	case errors.Is(err, finance.ErrSettlementSoon):
		status, code, message = http.StatusUnprocessableEntity, "finance_settlement_soon", "Batas lapor kasbon minimal 3 hari setelah tanggal dana dibutuhkan"
	case errors.Is(err, finance.ErrInvalidStatus):
		status, code, message = http.StatusConflict, "finance_invalid_status", "Pengajuan sudah berubah. Muat ulang lalu coba lagi."
	case errors.Is(err, finance.ErrNoHandler):
		status, code, message = http.StatusConflict, "finance_no_handler", "Tim ini belum punya finance. Hubungi admin."
	case errors.Is(err, finance.ErrNoteRequired):
		status, code, message = http.StatusUnprocessableEntity, "finance_note_required", "Catatan wajib diisi"
	case errors.Is(err, finance.ErrProofRequired):
		status, code, message = http.StatusUnprocessableEntity, "finance_proof_required", "Unggah bukti bayar dulu"
	case errors.Is(err, finance.ErrVoucherNotReady):
		status, code, message = http.StatusConflict, "finance_voucher_not_ready", "Hanya pengajuan Selesai dari tim Anda yang bisa masuk voucher"
	case errors.Is(err, finance.ErrVoucherSettings):
		status, code, message = http.StatusUnprocessableEntity, "finance_voucher_settings", "Pilih rekening pada kotak Bank, Kas, atau Lainnya"
	case errors.Is(err, finance.ErrUnbalanced):
		status, code, message = http.StatusConflict, "finance_voucher_unbalanced", "Debit dan kredit voucher tidak sama"
	case errors.Is(err, finance.ErrVoucherNumber):
		status, code, message = http.StatusConflict, "finance_voucher_number", "Nomor voucher sudah dipakai"
	case errors.Is(err, finance.ErrTeamInUse):
		status, code, message = http.StatusConflict, "finance_team_in_use", "Tim sudah punya pengajuan. Nonaktifkan saja."
	case errors.Is(err, finance.ErrTeamNameInUse):
		status, code, message = http.StatusConflict, "finance_team_name", "Nama tim sudah dipakai"
	case errors.Is(err, finance.ErrInvalidMember):
		status, code, message = http.StatusUnprocessableEntity, "finance_invalid_member", "Anggota tim tidak punya akses yang dibutuhkan"
	case errors.Is(err, finance.ErrAttachmentType):
		status, code, message = http.StatusUnprocessableEntity, "finance_attachment_type", "Lampiran harus JPG, PNG, atau PDF"
	case errors.Is(err, finance.ErrAttachmentSize):
		status, code, message = http.StatusRequestEntityTooLarge, "finance_attachment_size", "Lampiran maksimal 15 MB"
	case errors.Is(err, finance.ErrTooManyFiles):
		status, code, message = http.StatusUnprocessableEntity, "finance_too_many_files", "Lampiran sudah mencapai batas"
	default:
		writeServiceError(w, err)
		return
	}
	writeError(w, status, code, message)
}

func financeFilter(r *http.Request) finance.Filter {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))
	return finance.Filter{Scope: query.Get("scope"), Type: query.Get("type"), Status: query.Get("status"), TeamID: query.Get("team_id"),
		From: query.Get("from"), To: query.Get("to"), Search: query.Get("q"), Limit: limit, Offset: offset}
}

// handleFinance serves /api/v1/finance/*.
func (h *Handler) handleFinance(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	if h.deps.Finance == nil {
		writeUnavailable(w)
		return
	}
	access, ok := h.financeAccess(w, r, rc.principal)
	if !ok {
		return
	}
	switch {
	case path == "profile" && r.Method == http.MethodGet:
		result, err := h.deps.Finance.Profile(r.Context(), rc.principal, access)
		writeFinanceResult(w, result, err)
	case path == "summary" && r.Method == http.MethodGet:
		result, err := h.deps.Finance.Summary(r.Context(), rc.principal, access, financeFilter(r))
		writeFinanceResult(w, result, err)
	case path == "reports/daily" && r.Method == http.MethodGet:
		query := r.URL.Query()
		data, err := h.deps.Finance.DailyReport(r.Context(), rc.principal, access, query.Get("date"), query.Get("scope"), query.Get("team_id"))
		writeFinanceExcel(w, data, err, "laporan-keuangan-"+query.Get("date")+".xlsx")
	case path == "reports/export" && r.Method == http.MethodGet:
		data, err := h.deps.Finance.ExportReport(r.Context(), rc.principal, access, financeFilter(r))
		writeFinanceExcel(w, data, err, "laporan-keuangan.xlsx")
	case path == "requests" && r.Method == http.MethodGet:
		result, err := h.deps.Finance.List(r.Context(), rc.principal, access, financeFilter(r))
		writeFinanceResult(w, result, err)
	case path == "requests":
		methodNotAllowed(w, http.MethodGet)
	case strings.HasPrefix(path, "requests/"):
		h.handleFinanceRequest(w, r, rc, access, strings.Split(strings.TrimPrefix(path, "requests/"), "/"))
	case path == "payees" || strings.HasPrefix(path, "payees/"):
		h.financePayees(w, r, rc, access, strings.TrimPrefix(strings.TrimPrefix(path, "payees"), "/"))
	case path == "vouchers" || strings.HasPrefix(path, "vouchers/"):
		h.financeVouchers(w, r, rc, access, strings.TrimPrefix(strings.TrimPrefix(path, "vouchers"), "/"))
	case path == "voucher-settings":
		h.financeVoucherSettings(w, r, rc, access)
	case path == "voucher-settings/logo":
		h.financeVoucherLogo(w, r, access)
	case path == "teams" || path == "team-options" || strings.HasPrefix(path, "teams/"):
		h.handleFinanceTeams(w, r, rc, access, path)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	}
}

func writeFinanceResult(w http.ResponseWriter, result any, err error) {
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func writeFinanceExcel(w http.ResponseWriter, data []byte, err error, filename string) {
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	w.Header().Set("Content-Type", xlsxContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (h *Handler) handleFinanceRequest(w http.ResponseWriter, r *http.Request, rc requestContext, access finance.Access, parts []string) {
	id := parts[0]
	service := h.deps.Finance
	switch {
	case len(parts) == 1:
		switch r.Method {
		case http.MethodGet:
			result, err := service.Get(r.Context(), rc.principal, access, id)
			writeFinanceResult(w, result, err)
		case http.MethodPut:
			var input finance.DraftInput
			if !decodeJSON(w, r, &input) {
				return
			}
			result, err := service.SaveDraft(r.Context(), rc.principal, access, id, input)
			writeFinanceResult(w, result, err)
		case http.MethodDelete:
			if err := service.Delete(r.Context(), rc.principal, id); err != nil {
				writeFinanceError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w, "GET, PUT, DELETE")
		}
	case len(parts) == 2 && parts[1] == "settlement":
		if r.Method != http.MethodPut {
			methodNotAllowed(w, http.MethodPut)
			return
		}
		var input finance.SettlementInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SaveSettlement(r.Context(), rc.principal, access, id, input)
		writeFinanceResult(w, result, err)
	case len(parts) == 3 && parts[1] == "actions":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var input struct {
			Note string `json:"note"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.Act(r.Context(), rc.principal, access, id, parts[2], input.Note, clientMeta(r))
		writeFinanceResult(w, result, err)
	case len(parts) == 2 && parts[1] == "attachments":
		h.uploadFinanceAttachment(w, r, rc, access, id)
	case len(parts) == 3 && parts[1] == "attachments":
		h.financeAttachment(w, r, rc, access, id, parts[2])
	default:
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	}
}

func (h *Handler) uploadFinanceAttachment(w http.ResponseWriter, r *http.Request, rc requestContext, access finance.Access, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, finance.MaxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeFinanceError(w, finance.ErrAttachmentSize)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required", "Berkas wajib diunggah")
		return
	}
	defer file.Close()
	result, err := h.deps.Finance.AddAttachment(r.Context(), rc.principal, access, id, r.FormValue("kind"), r.FormValue("item_key"), header.Filename, file)
	if err != nil {
		writeFinanceError(w, err)
		return
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) financeAttachment(w http.ResponseWriter, r *http.Request, rc requestContext, access finance.Access, id, attachmentID string) {
	switch r.Method {
	case http.MethodGet:
		content, err := h.deps.Finance.OpenAttachment(r.Context(), rc.principal, access, id, attachmentID)
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", content.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", content.Filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = io.Copy(w, content.Reader)
	case http.MethodDelete:
		if err := h.deps.Finance.DeleteAttachment(r.Context(), rc.principal, access, id, attachmentID); err != nil {
			writeFinanceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "GET, DELETE")
	}
}

func (h *Handler) financePayees(w http.ResponseWriter, r *http.Request, rc requestContext, access finance.Access, id string) {
	service := h.deps.Finance
	switch {
	case id == "" && r.Method == http.MethodGet:
		result, err := service.Payees(r.Context(), rc.principal, access, r.URL.Query().Get("team_id"))
		writeFinanceResult(w, result, err)
	case id == "" && r.Method == http.MethodPost:
		var input struct {
			TeamID string `json:"team_id"`
			finance.Payee
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SavePayee(r.Context(), rc.principal, access, input.TeamID, input.Payee)
		writeFinanceResult(w, result, err)
	case id == "":
		methodNotAllowed(w, "GET, POST")
	case r.Method == http.MethodDelete:
		if err := service.DeletePayee(r.Context(), rc.principal, access, id); err != nil {
			writeFinanceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodDelete)
	}
}

func (h *Handler) financeVouchers(w http.ResponseWriter, r *http.Request, rc requestContext, access finance.Access, path string) {
	service := h.deps.Finance
	parts := strings.Split(path, "/")
	switch {
	case path == "" && r.Method == http.MethodGet:
		result, err := service.Vouchers(r.Context(), rc.principal, access)
		writeFinanceResult(w, result, err)
	case path == "" && r.Method == http.MethodPost:
		var input finance.VoucherInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SaveVoucher(r.Context(), rc.principal, access, "", input, clientMeta(r))
		writeFinanceResult(w, result, err)
	case path == "candidates" && r.Method == http.MethodGet:
		query := r.URL.Query()
		result, err := service.VoucherCandidates(r.Context(), rc.principal, access, query.Get("from"), query.Get("to"))
		writeFinanceResult(w, result, err)
	case path == "preview" && r.Method == http.MethodPost:
		var input finance.VoucherInput
		if !decodeJSON(w, r, &input) {
			return
		}
		data, err := service.PreviewVoucher(r.Context(), rc.principal, access, input)
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(data)
	case path == "compose" && r.Method == http.MethodPost:
		var input struct {
			RequestIDs []string `json:"request_ids"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.ComposeVoucher(r.Context(), rc.principal, access, input.RequestIDs)
		writeFinanceResult(w, result, err)
	case len(parts) == 1 && r.Method == http.MethodGet:
		result, err := service.Voucher(r.Context(), rc.principal, access, parts[0])
		writeFinanceResult(w, result, err)
	case len(parts) == 1 && r.Method == http.MethodPut:
		var input finance.VoucherInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SaveVoucher(r.Context(), rc.principal, access, parts[0], input, clientMeta(r))
		writeFinanceResult(w, result, err)
	case len(parts) == 2 && parts[1] == "file" && r.Method == http.MethodGet:
		content, err := service.OpenVoucher(r.Context(), rc.principal, access, parts[0])
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		defer content.Reader.Close()
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", content.Filename))
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = io.Copy(w, content.Reader)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	}
}

func (h *Handler) financeVoucherSettings(w http.ResponseWriter, r *http.Request, rc requestContext, access finance.Access) {
	switch r.Method {
	case http.MethodGet:
		result, err := h.deps.Finance.VoucherSettings(r.Context())
		writeFinanceResult(w, result, err)
	case http.MethodPut:
		if !access.CanManage {
			writeError(w, http.StatusForbidden, "forbidden", "Anda tidak memiliki akses")
			return
		}
		var input finance.VoucherSettings
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := h.deps.Finance.SaveVoucherSettings(r.Context(), rc.principal, input, clientMeta(r))
		writeFinanceResult(w, result, err)
	default:
		methodNotAllowed(w, "GET, PUT")
	}
}

func (h *Handler) financeVoucherLogo(w http.ResponseWriter, r *http.Request, access finance.Access) {
	if r.Method == http.MethodGet {
		data, err := h.deps.Finance.Logo(r.Context())
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(data))
		w.Header().Set("Cache-Control", "private, no-cache")
		_, _ = w.Write(data)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "GET, POST")
		return
	}
	if !access.CanManage {
		writeError(w, http.StatusForbidden, "forbidden", "Anda tidak memiliki akses")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required", "Berkas wajib diunggah")
		return
	}
	defer file.Close()
	if err := h.deps.Finance.SaveLogo(r.Context(), file); err != nil {
		writeFinanceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleFinanceTeams(w http.ResponseWriter, r *http.Request, rc requestContext, access finance.Access, path string) {
	if !access.CanManage && !access.CanMonitor {
		writeError(w, http.StatusForbidden, "forbidden", "Anda tidak memiliki akses")
		return
	}
	service := h.deps.Finance
	if r.Method != http.MethodGet && !access.CanManage {
		writeError(w, http.StatusForbidden, "forbidden", "Anda tidak memiliki akses")
		return
	}
	id := strings.TrimPrefix(strings.TrimPrefix(path, "teams"), "/")
	switch {
	case path == "team-options" && r.Method == http.MethodGet:
		result, err := service.TeamOptions(r.Context())
		writeFinanceResult(w, result, err)
	case path == "team-options":
		methodNotAllowed(w, http.MethodGet)
	case r.Method == http.MethodGet && id == "":
		result, err := service.Teams(r.Context())
		writeFinanceResult(w, result, err)
	case r.Method == http.MethodPost && id == "", r.Method == http.MethodPut && id != "":
		var input finance.TeamInput
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SaveTeam(r.Context(), rc.principal, id, input, clientMeta(r))
		writeFinanceResult(w, result, err)
	case r.Method == http.MethodDelete && id != "":
		if err := service.DeleteTeam(r.Context(), rc.principal, id, clientMeta(r)); err != nil {
			writeFinanceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "GET, POST, PUT, DELETE")
	}
}

// handleNotifications serves /api/v1/notifications/* and the push settings;
// every route acts on the signed-in user only.
func (h *Handler) handleNotifications(w http.ResponseWriter, r *http.Request, rc requestContext, path string) {
	service := h.deps.Notifications
	if service == nil {
		writeUnavailable(w)
		return
	}
	userID := rc.principal.UserID
	switch {
	case path == "" && r.Method == http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		rows, unread, err := service.List(r.Context(), userID, limit)
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		writeData(w, http.StatusOK, map[string]any{"rows": rows, "unread": unread})
	case path == "read-all" && r.Method == http.MethodPost:
		if err := service.MarkAllRead(r.Context(), userID); err != nil {
			writeNotificationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case path == "preferences" && r.Method == http.MethodGet:
		result, err := service.Preferences(r.Context(), userID)
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case path == "preferences" && r.Method == http.MethodPut:
		var input map[string]bool
		if !decodeJSON(w, r, &input) {
			return
		}
		result, err := service.SetPreferences(r.Context(), userID, input)
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case path == "devices" && r.Method == http.MethodGet:
		result, err := service.Status(r.Context(), userID)
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case path == "test" && r.Method == http.MethodPost:
		result, err := service.TestPush(r.Context(), userID)
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		writeData(w, http.StatusOK, result)
	case path == "devices" && (r.Method == http.MethodPost || r.Method == http.MethodDelete):
		var input struct {
			Token    string `json:"token"`
			Platform string `json:"platform"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		var err error
		if r.Method == http.MethodPost {
			err = service.RegisterDevice(r.Context(), userID, input.Token, input.Platform)
		} else {
			err = service.RemoveDevice(r.Context(), userID, input.Token)
		}
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(path, "/read") && r.Method == http.MethodPost:
		if err := service.MarkRead(r.Context(), userID, strings.TrimSuffix(path, "/read")); err != nil {
			writeNotificationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	}
}

func writeNotificationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, notifications.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Data tidak ditemukan")
	case errors.Is(err, notifications.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "Data tidak valid")
	default:
		writeServiceError(w, err)
	}
}
