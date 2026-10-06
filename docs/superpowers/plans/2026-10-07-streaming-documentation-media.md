# Streaming Documentation Media Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe per-slot image/video uploads with 25 MiB image and 500 MiB video limits, streamed through Konkit to Google Drive with mobile progress and cancellation.

**Architecture:** Add an image/video policy to template slots and snapshot it into distribution documentation slots. Replace full-body multipart parsing with a bounded streaming parser, detect content from a 512-byte prefix, and pass the resulting reader directly through the existing `media.Storage` boundary to chunked Google Drive upload. Nginx forwards upload bodies without request buffering; the React client validates size, sends metadata before the file, and uses XHR for progress and cancellation.

**Tech Stack:** Go 1.24, `net/http`, PostgreSQL/goose, Google Drive API v3, React 19, TypeScript, TanStack Query, Vitest/Testing Library, Nginx, Docker Compose.

**Spec:** `docs/superpowers/specs/2026-10-07-streaming-documentation-media-design.md`

## Global Constraints

- `media_kind` values are exactly `image`, `video`, and `image_video`.
- Existing template and instantiated slots default to `image` and are never rewritten to another policy.
- Images accept JPEG, PNG, and WebP up to 25 MiB per file.
- Videos accept MP4, MOV, and WebM up to 500 MiB per file.
- The server must not hold or write a complete uploaded file in VPS RAM or temporary disk.
- MIME validation uses sniffed bytes; only allow-listed video filename extensions may resolve an `application/octet-stream` sniff.
- Google credentials, storage keys, file contents, and personal identifiers must not appear in logs.
- At most three video uploads may run concurrently in one application instance by default.
- The maintained frontend sends bounded metadata parts, including `file_size`, before the single final `file` part.
- No staging data reset is part of this change.

## Review Focus

- A multipart request with a second part after `file` must fail while storage is still reading, so it cannot create accepted metadata.
- A file whose declared size is small but streamed size exceeds its media limit must be deleted from storage and return HTTP 413.
- A `.mp4` filename containing image or arbitrary bytes must not bypass byte sniffing unless the sniff is specifically `application/octet-stream` and the container extension is allow-listed.
- Cancellation after Drive upload starts must propagate through context, create no database row, and avoid an active orphan file.
- The fourth concurrent video must return HTTP 429 without consuming the video body or a Drive upload slot.

---

### Task 1: Persist media policy and expanded media constraints

**Files:**
- Create: `internal/database/migrations/00042_streaming_documentation_media.sql`
- Create: `internal/database/migrations/00042_streaming_documentation_media_test.go`
- Modify: `internal/programs/models.go`
- Modify: `internal/programs/service.go`
- Modify: `internal/programs/service_test.go`
- Modify: `internal/programs/repository.go`
- Modify: `internal/programs/repository_integration_test.go`
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/repository.go`
- Modify: `internal/distribution/repository_integration_test.go`
- Modify: `internal/distribution/schema_integration_test.go`

**Interfaces:**
- Produces: `programs.DocumentationTemplateSlot.MediaKind string`, `programs.DocumentationTemplateSlotInput.MediaKind string`, `distribution.MediaSlot.MediaKind string`, and `distribution.SlotSummary.MediaKind string`.
- Produces: database constraints accepting six MIME types and at most `524288000` bytes in `media_files` and `activity_media`.

- [ ] **Step 1: Write failing migration and service tests**

Add assertions that migrated template/instantiated slots equal `image`, all three policies round-trip through template CRUD, a new distribution slot snapshots `image_video`, and invalid policy `document` returns `ErrTemplateSlotInvalid`.

```go
func TestSaveDocumentationTemplateRejectsInvalidMediaKind(t *testing.T) {
	input := validDocumentationTemplateInput()
	input.Slots[0].MediaKind = "document"
	_, err := service.SaveDocumentationTemplate(context.Background(), auth.Principal{}, input, auth.ClientMeta{})
	if !errors.Is(err, ErrTemplateSlotInvalid) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/database/migrations ./internal/programs ./internal/distribution -run 'MediaKind|StreamingDocumentationMedia' -count=1`

Expected: FAIL because `MediaKind` and migration 42 do not exist.

- [ ] **Step 3: Add migration 42 and model/repository plumbing**

The migration must add both policy columns with defaults/checks, expand `media_files` MIME and size checks, and expand `activity_media.byte_size` to 500 MiB. Use explicit constraint names so the down migration can restore the previous image-only/10 MiB and activity/100 MiB ceilings only after rejecting incompatible rows.

```sql
ALTER TABLE documentation_template_slots
  ADD COLUMN media_kind text NOT NULL DEFAULT 'image'
  CHECK (media_kind IN ('image','video','image_video'));
ALTER TABLE documentation_slots
  ADD COLUMN media_kind text NOT NULL DEFAULT 'image'
  CHECK (media_kind IN ('image','video','image_video'));
```

Update every template slot `SELECT`, `INSERT`, and `Scan`, plus the distribution snapshot insert/select. Normalize `MediaKind` to lowercase; an empty value becomes `image` for backward-compatible API clients.

- [ ] **Step 4: Run focused and package tests**

Run: `go test ./internal/database/migrations ./internal/programs ./internal/distribution -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/00042_streaming_documentation_media.sql internal/database/migrations/00042_streaming_documentation_media_test.go internal/programs internal/distribution
git commit -m "feat: persist documentation media policy"
```

### Task 2: Add shared streaming media detection and Drive chunking

**Files:**
- Create: `internal/media/upload.go`
- Create: `internal/media/upload_test.go`
- Modify: `internal/media/gdrive.go`
- Modify: `internal/media/gdrive_test.go`

**Interfaces:**
- Produces: `media.Kind` constants `KindImage` and `KindVideo`.
- Produces: `media.DetectedUpload { Kind Kind; MimeType string; Extension string; Reader io.Reader; MaxBytes int64 }`.
- Produces: `media.DetectUpload(source io.Reader, filename string) (DetectedUpload, error)` and `media.PolicyAllows(policy string, kind Kind) bool`.
- Produces: `media.ErrUnsupportedUpload` and exported limits `MaxImageBytes`, `MaxVideoBytes`.
- Produces: shared `media.VideoLimiter` with `TryAcquire() (release func(), ok bool)`, constructed once per application instance.

- [ ] **Step 1: Write failing streaming detector tests**

Use a reader that returns at most 37 bytes per call and records total reads. Cover JPEG, PNG, WebP, MP4, WebM, MOV fallback, empty input, arbitrary `.mp4` bytes sniffed as text, policy combinations, and preservation of every byte after detection.

```go
detected, err := DetectUpload(&shortReader{data: jpegBytes}, "camera.jpg")
if err != nil || detected.Kind != KindImage || detected.MimeType != "image/jpeg" { t.Fatalf("detected=%+v err=%v", detected, err) }
got, _ := io.ReadAll(detected.Reader)
if !bytes.Equal(got, jpegBytes) { t.Fatal("detector consumed upload bytes") }
```

- [ ] **Step 2: Run detector tests and verify RED**

Run: `go test ./internal/media -run 'DetectUpload|PolicyAllows' -count=1`

Expected: FAIL because the detector API does not exist.

- [ ] **Step 3: Implement the 512-byte detector and policies**

Read only enough bytes for `http.DetectContentType`, rebuild the reader with `io.MultiReader`, return `.jpg`, `.png`, `.webp`, `.mp4`, `.mov`, or `.webm`, and permit extension fallback only for an `application/octet-stream` sniff.

- [ ] **Step 4: Write a failing Drive chunk configuration test**

Extend the fake Drive API contract so `uploadFile` receives the stream incrementally and assert that `realDriveFilesAPI.uploadFile` calls Drive media upload with an 8 MiB chunk size rather than reading all bytes first.

- [ ] **Step 5: Implement bounded Drive upload chunks**

Use `googleapi.ChunkSize(8 << 20)` on `Files.Create(...).Media(...)`. Keep `hashingReader` as the single checksum/size observer.

Implement `VideoLimiter` as one buffered semaphore with an idempotent release function. Unit-test three successful acquisitions, rejection of the fourth, release/reacquire, and concurrent callers under `go test -race`.

- [ ] **Step 6: Run media tests**

Run: `go test ./internal/media -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/media/upload.go internal/media/upload_test.go internal/media/gdrive.go internal/media/gdrive_test.go
git commit -m "feat: add streaming media detection"
```

### Task 3: Add strict streaming multipart parsing and upload runtime configuration

**Files:**
- Create: `internal/api/media_multipart.go`
- Create: `internal/api/media_multipart_test.go`
- Create: `internal/api/media_upload_log.go`
- Create: `internal/api/media_upload_log_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/server/main.go`
- Modify: `.env.example`
- Modify: `.env.staging.example`

**Interfaces:**
- Produces: `mediaMultipart { Fields map[string]string; Filename string; File io.Reader; DeclaredSize int64 }`.
- Produces: `openMediaMultipart(w http.ResponseWriter, r *http.Request, maxBody int64, allowedFields map[string]int64, requiredFields []string) (mediaMultipart, error)`.
- Produces: `config.Config.MaxConcurrentVideoUploads int`, sourced from `MAX_CONCURRENT_VIDEO_UPLOADS`, default `3`, valid range 1–16.
- Produces: `logMediaUpload(endpoint string, kind media.Kind, size int64, status int, started time.Time)` with a fixed, non-sensitive log schema.

- [ ] **Step 1: Write failing multipart tests**

Cover metadata-before-file success, file-before-metadata rejection, 64 KiB aggregate metadata ceiling, duplicate/unknown fields, invalid/negative `file_size`, multiple files, and a trailing part. The returned `File` must stream in short reads; a wrapper must surface trailing-part errors before it returns final EOF.

```go
upload, err := openMediaMultipart(rec, req, 501<<20, fields, []string{"source", "file_size"})
if err != nil { t.Fatal(err) }
_, err = io.Copy(io.Discard, upload.File)
if !errors.Is(err, errMultipartTrailingPart) { t.Fatalf("err=%v", err) }
```

- [ ] **Step 2: Run parser tests and verify RED**

Run: `go test ./internal/api -run MediaMultipart -count=1`

Expected: FAIL because the helper does not exist.

- [ ] **Step 3: Implement the multipart reader**

Use `r.MultipartReader`, `io.LimitReader` for each text field, an aggregate counter, and a final-file reader whose EOF check calls `NextPart` and accepts only `io.EOF`. Do not call `ParseMultipartForm`, `FormFile`, or `io.ReadAll` on the file.

Add a logger test that captures standard logger output and requires only `endpoint`, `kind`, `size`, `status`, and `duration_ms`; assert that filename, storage key, OAuth token, NIK, and file content sent to the helper never appear.

- [ ] **Step 4: Write failing config and server-timeout tests**

Assert default `3`, configured `5`, and rejection of `0`, negative, nonnumeric, and `17`. Extract `newHTTPServer(addr string, handler http.Handler) *http.Server` and assert a 31-minute read/write timeout with a 5-second header timeout.

- [ ] **Step 5: Implement configuration and server construction**

Add `ErrMaxConcurrentVideoUploadsInvalid`, parse the environment value, create one shared `media.VideoLimiter` in `cmd/server/main.go`, inject that same instance into distribution and activity services, and use 31-minute server read/write timeouts so Nginx—not a 30-second Go timeout—can carry a slow upload.

- [ ] **Step 6: Run focused tests**

Run: `go test ./internal/api ./internal/config ./cmd/server -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/media_multipart.go internal/api/media_multipart_test.go internal/config cmd/server/main.go .env.example .env.staging.example
git commit -m "feat: parse media uploads as streams"
```

### Task 4: Stream distribution documentation with policy and concurrency enforcement

**Files:**
- Modify: `internal/distribution/models.go`
- Modify: `internal/distribution/service.go`
- Modify: `internal/distribution/service_test.go`
- Modify: `internal/api/distribution_routes.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`
- Modify: `internal/api/routes.go`

**Interfaces:**
- Changes: `distribution.UploadMediaInput.Data` from `[]byte` to `io.Reader`.
- Adds: `distribution.UploadMediaInput.DeclaredSize int64`.
- Adds: `distribution.ErrMediaPolicyInvalid`, `ErrMediaTooLarge`, and `ErrVideoUploadBusy` mappings to HTTP 415, 413, and 429.
- Changes: `distribution.NewService(repository any, storage media.Storage, videoLimiter *media.VideoLimiter)` uses the application-wide limiter from Task 2.

- [ ] **Step 1: Write failing distribution service tests**

Cover image/video/mixed policy, declared oversize before storage, actual oversize with a generated repeating reader, delete-after-oversize, repository cleanup, cancellation, the fourth concurrent video, and image uploads proceeding while all video permits are occupied.

```go
input := UploadMediaInput{SlotID: "slot-1", OriginalFilename: "proof.mp4", Source: "gallery", DeclaredSize: media.MaxVideoBytes + 1, Data: bytes.NewReader(validMP4)}
_, err := service.UploadMedia(ctx, actor, input, meta, scope)
if !errors.Is(err, ErrMediaTooLarge) || storage.putCalls != 0 { t.Fatalf("err=%v calls=%d", err, storage.putCalls) }
```

- [ ] **Step 2: Run service tests and verify RED**

Run: `go test ./internal/distribution -run UploadMedia -count=1`

Expected: FAIL because upload input and policy enforcement still use byte slices/image-only logic.

- [ ] **Step 3: Implement streaming distribution service**

Resolve/authorize the slot first, call `media.DetectUpload`, validate `MediaKind`, acquire a video permit without waiting, check declared size, wrap `detected.Reader` in `io.LimitReader(max+1)`, and pass it directly to `media.PutNamed`. Delete the storage object and return `ErrMediaTooLarge` when returned size exceeds the detected limit. Extend visible filename mapping for MP4/MOV/WebM.

- [ ] **Step 4: Write failing route tests**

Build multipart requests with metadata first and a chunked file reader. Assert the route forwards a reader and declared size without pre-reading, returns the service's structured errors, and rejects old file-first requests with `multipart_invalid`.

- [ ] **Step 5: Replace distribution route buffering**

Use `openMediaMultipart` with allowed fields `source`, `file_size`, `captured_at`, `latitude`, and `longitude`; set the request ceiling to `501<<20`; parse metadata into `UploadMediaInput`; remove `ParseMultipartForm` and `io.ReadAll`.

Call `logMediaUpload` exactly once for every accepted or rejected upload attempt after authentication, using `unknown` kind until detection succeeds and never passing filename, slot ID, storage key, or user data.

- [ ] **Step 6: Run distribution and API tests**

Run: `go test ./internal/distribution ./internal/api -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/distribution internal/api/distribution_routes.go internal/api/handler.go internal/api/handler_test.go internal/api/routes.go
git commit -m "feat: stream distribution media uploads"
```

### Task 5: Stream Activity Documentation with split image/video limits

**Files:**
- Modify: `internal/activities/models.go`
- Modify: `internal/activities/service.go`
- Modify: `internal/activities/service_test.go`
- Modify: `internal/api/activities_routes.go`
- Modify: `internal/api/activities_routes_test.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/api/routes.go`

**Interfaces:**
- Changes: `activities.UploadInput.Data` from `[]byte` to `io.Reader` and adds `DeclaredSize int64`.
- Changes: `activities.NewService(repository, storage, resolver, videoLimiter *media.VideoLimiter)` reuses the same application-wide limiter as distribution uploads.
- Reuses: Task 2 detection/limits and Task 3 multipart contract.

- [ ] **Step 1: Write failing activity upload tests**

Cover 25 MiB image and 500 MiB video boundaries with generated readers, MOV fallback, oversize cleanup, cancellation, semaphore saturation, and Drive/repository failure behavior.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/activities ./internal/api -run 'Activit.*Upload|ActivityMedia' -count=1`

Expected: FAIL because Activity Documentation still reads and stores a full byte slice with a single 100 MiB limit.

- [ ] **Step 3: Implement the streaming activity service and route**

Reuse `media.DetectUpload`; Activity Documentation permits both media kinds. Parse `program_id`, `regency_id`, `activity_type`, `source`, and `file_size` before the final file part. Remove the 100 MiB constants, `ParseMultipartForm`, and `io.ReadAll`.

Use the same single-attempt `logMediaUpload` schema as distribution uploads.

- [ ] **Step 4: Run activity and API tests**

Run: `go test ./internal/activities ./internal/api -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/activities internal/api/activities_routes.go internal/api/activities_routes_test.go internal/api/handler.go internal/api/routes.go
git commit -m "feat: stream activity media uploads"
```

### Task 6: Add an authenticated XHR upload client with progress and cancellation

**Files:**
- Create: `frontend/src/lib/upload.ts`
- Create: `frontend/src/lib/upload.test.ts`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/lib/api.test.ts`

**Interfaces:**
- Produces: `uploadRequest<T>(path: string, form: FormData, options: { signal?: AbortSignal; onProgress?: (percent: number) => void }): Promise<T>`.
- Produces: `getCSRFToken(): string` inside the API module without exposing token mutation.

- [ ] **Step 1: Write failing XHR tests**

Use a fake `XMLHttpRequest` and assert same-origin credentials, CSRF header, JSON success parsing, `ApiError` parsing for 413/429, integer progress, abort propagation, network failure copy, and redirect on 401.

```ts
const promise = uploadRequest('/api/v1/test', form, { onProgress });
xhr.upload.dispatchEvent(new ProgressEvent('progress', { lengthComputable: true, loaded: 5, total: 10 }));
expect(onProgress).toHaveBeenCalledWith(50);
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd frontend && npm test -- --run src/lib/upload.test.ts`

Expected: FAIL because `uploadRequest` does not exist.

- [ ] **Step 3: Implement the upload helper**

Set `withCredentials = true`, apply `X-CSRF-Token`, never set multipart `Content-Type` manually, parse the existing error envelope, call `xhr.abort()` from the signal, and remove listeners after settlement.

- [ ] **Step 4: Run frontend library tests**

Run: `cd frontend && npm test -- --run src/lib/upload.test.ts src/lib/api.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/api.ts frontend/src/lib/api.test.ts frontend/src/lib/upload.ts frontend/src/lib/upload.test.ts
git commit -m "feat: add upload progress client"
```

### Task 7: Add media policy to the Program Setup template editor

**Files:**
- Modify: `frontend/src/features/programs/types.ts`
- Modify: `frontend/src/features/programs/TemplatesPanel.tsx`
- Modify: `frontend/src/features/programs/TemplatesPanel.test.tsx`
- Modify: `frontend/e2e/program-setup-dialogs.spec.ts`

**Interfaces:**
- Changes: `DocumentationSlot` adds `media_kind: 'image' | 'video' | 'image_video'`.
- Consumes: Task 1 API field `media_kind`.

- [ ] **Step 1: Write failing template editor tests**

Assert old/missing values normalize to `image`, a new slot defaults to `image`, the **Jenis media** select displays all three Indonesian options, and saving sends `media_kind: 'video'`.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd frontend && npm test -- --run src/features/programs/TemplatesPanel.test.tsx`

Expected: FAIL because the type and selector do not exist.

- [ ] **Step 3: Implement the selector and normalization**

Place the selector beside source/stage controls. Use labels **Foto saja**, **Video saja**, and **Foto & video**; ensure copied published templates retain each slot's value.

- [ ] **Step 4: Run component and E2E type/build checks**

Run: `cd frontend && npm test -- --run src/features/programs/TemplatesPanel.test.tsx && npm run build`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/programs/types.ts frontend/src/features/programs/TemplatesPanel.tsx frontend/src/features/programs/TemplatesPanel.test.tsx frontend/e2e/program-setup-dialogs.spec.ts
git commit -m "feat: configure media type per documentation slot"
```

### Task 8: Add distribution and activity upload progress, video controls, and previews

**Files:**
- Modify: `frontend/src/features/distribution/types.ts`
- Modify: `frontend/src/features/distribution/DocumentationSlot.tsx`
- Modify: `frontend/src/features/distribution/DocumentationSlot.test.tsx`
- Modify: `frontend/src/features/activities/ActivityDocumentationPage.tsx`
- Modify: `frontend/src/features/activities/ActivityDocumentationPage.test.tsx`
- Modify: `frontend/src/components/MediaPreviewDialog.tsx`
- Modify: `frontend/src/components/MediaPreviewDialog.test.tsx`

**Interfaces:**
- Consumes: Task 6 `uploadRequest`.
- Consumes: `media_kind` and MIME types returned by Tasks 1, 4, and 5.
- Queue item adds `progress: number` and `controller: AbortController | null`.

- [ ] **Step 1: Write failing distribution UI tests**

Cover controls for all three policies, `capture="environment"` image/video inputs, 25/500 MiB client rejection, metadata appended before file, sequential queue behavior, progress text, cancel, retry, 413/429 messages, and video thumbnails/previews.

- [ ] **Step 2: Run distribution UI tests and verify RED**

Run: `cd frontend && npm test -- --run src/features/distribution/DocumentationSlot.test.tsx src/components/MediaPreviewDialog.test.tsx`

Expected: FAIL because distribution slots are image-only and use `fetch` without progress.

- [ ] **Step 3: Implement policy-aware distribution uploads**

Use separate photo/video camera inputs for mixed slots. Append `source`, `file_size`, capture/location metadata, then `file` last. Upload one queue item at a time through `uploadRequest`; render image previews with `<img>` and video previews with `<video preload="metadata">`; pass `mediaType` into `MediaPreviewDialog`.

- [ ] **Step 4: Write failing Activity Documentation progress tests**

Assert 25/500 MiB validation, metadata-before-file ordering, percent progress, cancel/retry, and unchanged image/video preview behavior.

- [ ] **Step 5: Implement Activity Documentation XHR uploads**

Replace the mutation's `apiRequest` upload call with `uploadRequest`, preserve TanStack cache invalidation/toasts, and keep one pending activity file at a time.

- [ ] **Step 6: Run all touched frontend tests and build**

Run: `cd frontend && npm test -- --run src/features/distribution/DocumentationSlot.test.tsx src/features/activities/ActivityDocumentationPage.test.tsx src/components/MediaPreviewDialog.test.tsx src/lib/upload.test.ts && npm run build`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/features/distribution frontend/src/features/activities/ActivityDocumentationPage.tsx frontend/src/features/activities/ActivityDocumentationPage.test.tsx frontend/src/components/MediaPreviewDialog.tsx frontend/src/components/MediaPreviewDialog.test.tsx
git commit -m "feat: add mobile media upload progress"
```

### Task 9: Document and test the Nginx streaming configuration

**Files:**
- Modify: `README.md`
- Modify: `.env.staging.example`

**Interfaces:**
- Produces: the exact Konkit Nginx upload location configuration used in Task 11.

- [ ] **Step 1: Add the explicit Nginx configuration to README**

Document the existing proxy headers plus this upload-specific location in the TLS server block:

```nginx
location ~ ^/api/v1/(activities/media|distribution/slots/[^/]+/media)$ {
    proxy_pass http://localhost:8090;
    proxy_http_version 1.1;
    client_max_body_size 510M;
    client_body_timeout 30m;
    proxy_request_buffering off;
    proxy_send_timeout 30m;
    proxy_read_timeout 30m;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Keep the regular `/` location for web/static requests. Explain that application limits remain 25/500 MiB and that `nginx -t` is mandatory before reload.

- [ ] **Step 2: Validate documentation and examples**

Run: `rg -n 'client_max_body_size 510M|proxy_request_buffering off|MAX_CONCURRENT_VIDEO_UPLOADS=3' README.md .env.staging.example`

Expected: one documented value for each setting and no obsolete Konkit 1 MiB guidance.

- [ ] **Step 3: Commit**

```bash
git add README.md .env.staging.example
git commit -m "docs: configure nginx for streaming uploads"
```

### Task 10: Run complete local verification and review the branch

**Files:**
- Verify only; fix only failures caused by Tasks 1–9.

**Interfaces:**
- Consumes all previous task deliverables.

- [ ] **Step 1: Inspect formatting and changes without rewriting unrelated files**

Run: `gofmt -d internal cmd && git diff --check && git status --short`

Expected: `gofmt -d` and `git diff --check` produce no output; only intended files are listed by status. Any formatting defect must be corrected in the exact file named by `gofmt -d`, then that task's focused tests must be rerun.

- [ ] **Step 2: Run the complete Go suite**

Run: `go test -p 1 -count=1 ./...`

Expected: PASS.

- [ ] **Step 3: Run the complete frontend suite and build**

Run: `cd frontend && npm test -- --run --maxWorkers=1 && npm run build`

Expected: PASS.

- [ ] **Step 4: Build the production container**

Run: `docker build -t konkit:streaming-media-test .`

Expected: image builds successfully with the production frontend embedded.

- [ ] **Step 5: Perform final branch review**

Check every acceptance criterion in the spec against a test or deployment verification step. Search for forbidden buffering with:

```bash
rg -n 'ParseMultipartForm|io\.ReadAll' internal/api/distribution_routes.go internal/api/activities_routes.go
```

Expected: no matches in either upload route.

- [ ] **Step 6: Confirm the verified branch state**

Run: `git status --short && git log -10 --oneline`

Expected: no uncommitted implementation changes remain, and each completed task has its own focused commit. If verification uncovered a defect, return to the owning task, add a failing regression test, implement the minimal correction, rerun that task's tests plus Steps 1–4, and commit only that task's files.

### Task 11: Deploy safely to staging and run live smoke tests

**Files:**
- Remote: `/opt/konkit/.env.staging`
- Remote: `/etc/nginx/sites-available/konkit.ptkiansantang.com`
- Remote backups: `/opt/konkit/backups/` and `/etc/nginx/sites-available/`

**Interfaces:**
- Consumes the tested branch and README configuration from Tasks 1–10.

- [ ] **Step 1: Capture pre-deploy state and backups**

On the VPS, record the current commit/container status, create a custom-format PostgreSQL dump, copy `.env.staging`, copy the Nginx site file, and save the currently running app image with `docker image save`. Store all artifacts under one timestamped directory in `/opt/konkit/backups/`; verify every backup is non-empty before continuing.

- [ ] **Step 2: Verify disk capacity and clean only safe build cache if needed**

Run: `df -h / /var/lib/docker /opt` and `docker system df`.

Expected: enough free space to build the image without touching PostgreSQL volumes, `/opt/konkit/backups`, or unrelated site data. If capacity is insufficient, stop and request approval before deleting any cache.

- [ ] **Step 3: Deploy application and migration**

Update `/opt/konkit` to the reviewed commit, set `MAX_CONCURRENT_VIDEO_UPLOADS=3` in `.env.staging`, then run:

```bash
docker compose -f docker-compose.yml -f docker-compose.override.yml -f docker-compose.gdrive.yml --env-file .env.compose up -d --build app
```

Expected: migration 42 applies and `konkit-app-1` remains running.

- [ ] **Step 4: Install and validate Nginx streaming settings**

Add the Task 9 directives while preserving Certbot TLS lines and existing security headers. Run `nginx -t`; only after it succeeds run `systemctl reload nginx`.

- [ ] **Step 5: Verify database and public health**

Assert goose version 42, old templates/slots have `media_kind='image'`, the public root redirects to `/login`, `/api/v1/health` returns 200 when authenticated as appropriate, and application logs contain no migration/storage errors.

- [ ] **Step 6: Run live media smoke tests**

From an authorized staging session:

- upload a JPEG larger than 1 MiB and smaller than 25 MiB;
- configure a draft test slot for `image_video`, publish/use it in a staging schedule, and upload representative MP4/MOV/WebM files;
- verify an image-only slot rejects video;
- cancel an in-progress video and verify no accepted DB row or active Drive object remains;
- attempt a declared oversize request and verify structured 413;
- inspect app RSS and Nginx temporary directories during upload to confirm they do not grow by the file's full size.

- [ ] **Step 7: Report deployment evidence**

Record deployed commit, backup paths, migration version, container status, HTTP results, sample sizes/types, DB/Drive cleanup results, and observed memory/disk behavior. Do not report success until all checks pass.

- [ ] **Step 8: Use the prepared rollback if a required check fails**

Restore `.env.staging` and the Nginx site from the timestamped backup, run `nginx -t`, reload Nginx, load the saved app image with `docker image load`, and recreate only `app` with the full three-file Compose command plus `--no-build --force-recreate app`. Leave additive migration 42 in place because its defaults and expanded constraints remain compatible with the previous application. Re-run the public redirect and container-health checks, then report the failed deployment and rollback evidence instead of continuing smoke tests.
