package bast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"konkit/internal/audit"
	"konkit/internal/auth"

	"github.com/jackc/pgx/v5"
)

// SignatoryCard adalah satu kartu penandatangan: nama/NIP dari Konfigurasi
// BA jadwal serta tanda tangan dan stempel yang tersimpan.
type SignatoryCard struct {
	Role               string     `json:"role"`
	Name               string     `json:"name"`
	NIP                string     `json:"nip"`
	SignatureUpdatedAt *time.Time `json:"signature_updated_at,omitempty"`
	StampUpdatedAt     *time.Time `json:"stamp_updated_at,omitempty"`
}

// SignatureSettingsView adalah isi panel "Penandatangan" satu BA.
type SignatureSettingsView struct {
	DocumentKey string `json:"document_key"`
	Override    bool   `json:"override"`
	// WetSignature: halaman tanda tangan dicetak, ditandatangani basah, lalu
	// hasil pindainya diunggah. WetSupported: BA ini mendukung mode tersebut.
	WetSignature bool            `json:"wet_signature"`
	WetSupported bool            `json:"wet_supported"`
	Rules        SignatureRules  `json:"rules"`
	GlobalRules  SignatureRules  `json:"global_rules"`
	Signatories  []SignatoryCard `json:"signatories"`
}

// signatoryNameColumns: kolom nama (dan NIP) setiap pihak di bast_schedule_settings.
var signatoryNameColumns = map[string]string{
	SignatoryAgricultureOffice: "agriculture_office_name",
	SignatoryInstaller:         "installer_name",
	SignatorySupervisor:        "supervisor_name",
	SignatoryPertamina:         "pertamina_rep_name",
}

// lockSchedule memastikan jadwal ada dan berada dalam cakupan kabupaten pengguna.
func lockSchedule(ctx context.Context, tx pgx.Tx, scheduleID string, scope auth.RegencyScope) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM program_schedules WHERE id::text=$1 AND ($2 OR regency_id::text = ANY($3)) FOR UPDATE`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock schedule: %w", err)
	}
	return nil
}

// storedSignatureRules adalah satu baris bast_signature_rules.
type storedSignatureRules struct {
	rules    SignatureRules
	override bool
	wet      bool
	found    bool
}

func (r *Repository) getSignatureRules(ctx context.Context, scheduleID, documentKey string) (storedSignatureRules, error) {
	var raw []byte
	var stored storedSignatureRules
	err := r.pool.QueryRow(ctx, `SELECT rules, override, wet_signature FROM bast_signature_rules WHERE schedule_id::text=$1 AND document_key=$2`, scheduleID, documentKey).Scan(&raw, &stored.override, &stored.wet)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedSignatureRules{}, nil
	}
	if err != nil {
		return storedSignatureRules{}, fmt.Errorf("get signature rules: %w", err)
	}
	rules := SignatureRules{}
	if err := json.Unmarshal(raw, &rules); err != nil {
		return storedSignatureRules{}, fmt.Errorf("decode signature rules: %w", err)
	}
	stored.rules, stored.found = rules.normalized(), true
	return stored, nil
}

// effectiveRules adalah aturan cetak yang berlaku untuk satu BA.
type effectiveRules struct {
	global    SignatureRules
	rules     SignatureRules
	override  bool
	globalWet bool
	wet       bool
}

// effectiveSignatureRules mengembalikan aturan global jadwal (atau bawaan)
// dan aturan yang berlaku untuk BA (khusus bila diaktifkan), termasuk mode
// tanda tangan basah.
func (r *Repository) effectiveSignatureRules(ctx context.Context, scheduleID, documentKey string) (effectiveRules, error) {
	global, err := r.getSignatureRules(ctx, scheduleID, SignatureRulesGlobal)
	if err != nil {
		return effectiveRules{}, err
	}
	if !global.found {
		global.rules = defaultSignatureRules()
	}
	result := effectiveRules{global: global.rules, rules: global.rules, globalWet: global.wet, wet: global.wet}
	document, err := r.getSignatureRules(ctx, scheduleID, documentKey)
	if err != nil {
		return effectiveRules{}, err
	}
	if document.found && document.override {
		result.rules, result.override, result.wet = document.rules, true, document.wet
	}
	return result, nil
}

func upsertSignatureRules(ctx context.Context, tx pgx.Tx, actor auth.Principal, scheduleID, documentKey string, override bool, rules SignatureRules, wet bool) error {
	raw, err := json.Marshal(rules.normalized())
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO bast_signature_rules(schedule_id, document_key, override, rules, wet_signature, updated_by)
		VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6,'')::uuid)
		ON CONFLICT (schedule_id, document_key) DO UPDATE SET override=EXCLUDED.override, rules=EXCLUDED.rules, wet_signature=EXCLUDED.wet_signature, updated_by=EXCLUDED.updated_by, updated_at=now()
	`, scheduleID, documentKey, override, raw, wet, actor.UserID)
	if err != nil {
		return fmt.Errorf("upsert signature rules: %w", err)
	}
	return nil
}

// SaveSignatureRules menyimpan aturan cetak dari panel satu BA. override=false
// berarti BA mengikuti global: aturan yang dikirim disimpan sebagai global
// jadwal dan aturan khusus BA yang sudah ada dipertahankan (hanya nonaktif).
func (r *Repository) SaveSignatureRules(ctx context.Context, actor auth.Principal, scheduleID, documentKey string, override bool, rules SignatureRules, wet bool, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin signature rules: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	if override {
		if err := upsertSignatureRules(ctx, tx, actor, scheduleID, documentKey, true, rules, wet); err != nil {
			return err
		}
	} else {
		if err := upsertSignatureRules(ctx, tx, actor, scheduleID, SignatureRulesGlobal, false, rules, wet); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bast_signature_rules SET override=false, updated_by=NULLIF($3,'')::uuid, updated_at=now() WHERE schedule_id::text=$1 AND document_key=$2`, scheduleID, documentKey, actor.UserID); err != nil {
			return fmt.Errorf("disable signature override: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.signature_rules_updated", ResourceType: "bast_signature_rules", ResourceID: scheduleID, Metadata: map[string]any{"document_key": documentKey, "override": override, "wet_signature": wet}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SaveSignatoryName menyimpan nama (dan NIP untuk Dinas) satu pihak ke
// Konfigurasi BA jadwal.
func (r *Repository) SaveSignatoryName(ctx context.Context, actor auth.Principal, scheduleID, role, name, nip string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	column, ok := signatoryNameColumns[role]
	if !ok {
		return ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin signatory name: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	// column berasal dari peta tetap di atas, bukan dari input.
	query := fmt.Sprintf(`INSERT INTO bast_schedule_settings(schedule_id, %[1]s) VALUES ($1::uuid, NULLIF($2,''))
		ON CONFLICT (schedule_id) DO UPDATE SET %[1]s=EXCLUDED.%[1]s, updated_at=now()`, column)
	if _, err := tx.Exec(ctx, query, scheduleID, name); err != nil {
		return fmt.Errorf("save signatory name: %w", err)
	}
	if role == SignatoryAgricultureOffice {
		if _, err := tx.Exec(ctx, `UPDATE bast_schedule_settings SET agriculture_office_nip=NULLIF($2,''), updated_at=now() WHERE schedule_id::text=$1`, scheduleID, nip); err != nil {
			return fmt.Errorf("save signatory NIP: %w", err)
		}
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.schedule_settings_updated", ResourceType: "bast_schedule_setting", ResourceID: scheduleID, Metadata: map[string]any{"signatory": role}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) PutSignatoryAsset(ctx context.Context, actor auth.Principal, scheduleID, role, kind string, image []byte, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin signatory asset: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bast_signatory_assets(schedule_id, role, kind, image_png, byte_size, updated_by)
		VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6,'')::uuid)
		ON CONFLICT (schedule_id, role, kind) DO UPDATE SET image_png=EXCLUDED.image_png, byte_size=EXCLUDED.byte_size, updated_by=EXCLUDED.updated_by, updated_at=now()
	`, scheduleID, role, kind, image, len(image), actor.UserID); err != nil {
		return fmt.Errorf("save signatory asset: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.signatory_asset_uploaded", ResourceType: "bast_signatory_asset", ResourceID: scheduleID, Metadata: map[string]any{"role": role, "kind": kind}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) DeleteSignatoryAsset(ctx context.Context, actor auth.Principal, scheduleID, role, kind string, scope auth.RegencyScope, meta auth.ClientMeta) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete signatory asset: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSchedule(ctx, tx, scheduleID, scope); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bast_signatory_assets WHERE schedule_id::text=$1 AND role=$2 AND kind=$3`, scheduleID, role, kind); err != nil {
		return fmt.Errorf("delete signatory asset: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorUserID: actor.UserID, Action: "bast.signatory_asset_deleted", ResourceType: "bast_signatory_asset", ResourceID: scheduleID, Metadata: map[string]any{"role": role, "kind": kind}, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) OpenSignatoryAsset(ctx context.Context, scheduleID, role, kind string, scope auth.RegencyScope) ([]byte, error) {
	var image []byte
	err := r.pool.QueryRow(ctx, `
		SELECT a.image_png FROM bast_signatory_assets a
		JOIN program_schedules ps ON ps.id = a.schedule_id
		WHERE a.schedule_id::text=$1 AND a.role=$2 AND a.kind=$3 AND ($4 OR ps.regency_id::text = ANY($5))
	`, scheduleID, role, kind, scope.Unrestricted, scope.RegencyIDs).Scan(&image)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open signatory asset: %w", err)
	}
	return image, nil
}

func (r *Repository) GetSignatureSettings(ctx context.Context, scheduleID, documentKey string, scope auth.RegencyScope) (SignatureSettingsView, error) {
	settings, err := r.GetScheduleSettings(ctx, scheduleID, scope)
	if err != nil {
		return SignatureSettingsView{}, err
	}
	effective, err := r.effectiveSignatureRules(ctx, scheduleID, documentKey)
	if err != nil {
		return SignatureSettingsView{}, err
	}
	updated := map[string]time.Time{}
	rows, err := r.pool.Query(ctx, `SELECT role, kind, updated_at FROM bast_signatory_assets WHERE schedule_id::text=$1`, scheduleID)
	if err != nil {
		return SignatureSettingsView{}, fmt.Errorf("list signatory assets: %w", err)
	}
	for rows.Next() {
		var role, kind string
		var at time.Time
		if err := rows.Scan(&role, &kind, &at); err != nil {
			rows.Close()
			return SignatureSettingsView{}, err
		}
		updated[role+":"+kind] = at
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return SignatureSettingsView{}, err
	}
	names := map[string]string{
		SignatoryAgricultureOffice: settings.AgricultureOfficeName,
		SignatoryInstaller:         settings.InstallerName,
		SignatorySupervisor:        settings.SupervisorName,
		SignatoryPertamina:         settings.PertaminaRepName,
	}
	view := SignatureSettingsView{
		DocumentKey: documentKey, Override: effective.override, Rules: effective.rules, GlobalRules: effective.global,
		WetSignature: effective.wet, WetSupported: wetSignatureDocuments[documentKey], Signatories: []SignatoryCard{},
	}
	for _, role := range signatureDocuments[documentKey] {
		card := SignatoryCard{Role: role, Name: names[role]}
		if role == SignatoryAgricultureOffice {
			card.NIP = settings.AgricultureOfficeNIP
		}
		if at, ok := updated[role+":"+SignatoryKindSignature]; ok {
			card.SignatureUpdatedAt = &at
		}
		if at, ok := updated[role+":"+SignatoryKindStamp]; ok {
			card.StampUpdatedAt = &at
		}
		view.Signatories = append(view.Signatories, card)
	}
	return view, nil
}

// ResolveSignatoryMarks menentukan tanda yang dicetak pada satu BA: aturan
// khusus BA bila aktif, selain itu aturan global jadwal; pihak tanpa gambar
// dilewati (kotak dibiarkan kosong untuk tanda tangan basah).
func (r *Repository) ResolveSignatoryMarks(ctx context.Context, scheduleID, documentKey string) (SignatureMarks, map[string][]byte, error) {
	roles := signatureDocuments[documentKey]
	if len(roles) == 0 {
		return nil, nil, nil
	}
	effective, err := r.effectiveSignatureRules(ctx, scheduleID, documentKey)
	if err != nil {
		return nil, nil, err
	}
	rules := effective.rules
	rows, err := r.pool.Query(ctx, `SELECT role, kind, image_png FROM bast_signatory_assets WHERE schedule_id::text=$1`, scheduleID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve signatory marks: %w", err)
	}
	defer rows.Close()
	wanted := map[string]bool{}
	for _, role := range roles {
		wanted[role] = true
	}
	marks, images := SignatureMarks{}, map[string][]byte{}
	for rows.Next() {
		var role, kind string
		var image []byte
		if err := rows.Scan(&role, &kind, &image); err != nil {
			return nil, nil, err
		}
		rule := rules[role]
		if !wanted[role] || (kind == SignatoryKindSignature && !rule.Signature) || (kind == SignatoryKindStamp && !rule.Stamp) {
			continue
		}
		mark := marks[role]
		if kind == SignatoryKindSignature {
			mark.Signature = true
			mark.SignaturePlacement = rule.SignaturePlacement
		} else {
			mark.Stamp = true
			mark.StampPlacement = rule.StampPlacement
		}
		marks[role] = mark
		images[signatoryImageKey(role, kind)] = image
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return marks, images, nil
}

// ScheduleForRegency mengembalikan jadwal program di satu kabupaten (BA
// Perorangan diminta per program dan kabupaten, tanda tangan disimpan per jadwal).
func (r *Repository) ScheduleForRegency(ctx context.Context, programID, regencyID string) (string, error) {
	var scheduleID string
	err := r.pool.QueryRow(ctx, `SELECT id::text FROM program_schedules WHERE program_id::text=$1 AND regency_id::text=$2 ORDER BY created_at LIMIT 1`, programID, regencyID).Scan(&scheduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("schedule for regency: %w", err)
	}
	return scheduleID, nil
}

// ScheduleProgramRegency mengembalikan program dan kabupaten satu jadwal.
func (r *Repository) ScheduleProgramRegency(ctx context.Context, scheduleID string, scope auth.RegencyScope) (string, string, error) {
	var programID, regencyID string
	err := r.pool.QueryRow(ctx, `SELECT program_id::text, regency_id::text FROM program_schedules WHERE id::text=$1 AND ($2 OR regency_id::text = ANY($3))`, scheduleID, scope.Unrestricted, scope.RegencyIDs).Scan(&programID, &regencyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("schedule program regency: %w", err)
	}
	return programID, regencyID, nil
}
