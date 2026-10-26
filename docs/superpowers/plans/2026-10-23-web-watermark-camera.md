# Web Watermark Camera Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Konkit web app the mobile app's watermark camera (logo + time/address/GPS block, toggles, drag/scale, colours, backdate), send GPS with distribution photos, and shrink photos before upload.

**Architecture:** A Go `internal/geo` package proxies reverse geocoding to Nominatim behind `GET /api/v1/geo/reverse`. The frontend gets a `features/camera` module: pure settings/resize/layout helpers, a geolocation+address client, a lazy-loaded `WatermarkCamera` dialog that composes the watermark onto a canvas, and a settings page. `DocumentationSlot` and `ActivityDocumentationPage` open the camera instead of the native file input's camera mode.

**Tech Stack:** Go stdlib (`net/http`, `sync`, `time`), React 19 + TS, TanStack Query, Vitest + Testing Library, Canvas 2D, `getUserMedia`, `navigator.geolocation`, `localStorage`.

**Spec:** `docs/superpowers/specs/2026-10-23-web-watermark-camera-design.md` — read it; where it is silent, follow the mobile reference in `D:\KSM\Deployment\Konverter_Kit_Gas_Mobile\lib\features\camera\` and `lib\data\camera_settings.dart`.

## Global Constraints

- No new npm or Go dependencies.
- All UI copy in Bahasa Indonesia.
- Do NOT commit `web/static/app` (CI byte-verifies the bundle; Windows cannot reproduce it). Verify with `npm run typecheck`, `npm run lint`, `npx vitest run <paths>`; `git status` must show no `web/static/app` changes.
- `gofmt -l` must be clean for every Go file touched; `go build ./... && go vet ./...` clean.
- Commit messages end with a blank line then `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Colours (name → hex): Kuning `#FFEB3B` (default text), Putih `#FFFFFF`, Oranye `#FF6B35` (default accent), Hijau `#4ADE80`, Biru `#60A5FA`.
- Text size: Kecil 0.85, Sedang 1.0 (default), Besar 1.2. Logo scale 0.5–1.0 (default 1.0). Element drag-scale range 0.4–3.0.
- Photo quality (longest side): Hemat 1280, Standar 1920 (default), Tinggi 2560, Asli (no resize). JPEG quality 0.92, 0.95 for Asli.
- Timestamp text format `yyyy-MM-dd HH:mm:ss` (24h, local time); coordinates `Lat: x.xxxxx, Lng: y.yyyyy` (5 decimals).
- Geolocation: high accuracy, timeout 8 s, accept a last-known fix < 5 min old. Re-geocode only after moving > 15 m. Address wait ≤ 4 s; fallback text `Alamat tidak tersedia`.
- Backdate changes only the date printed on the watermark; `captured_at` sent to the server is always the real capture time.
- Camera stream: rear camera preferred, ideal 1920×1080.

## Review Focus

- The preview and the uploaded photo show the watermark in the same place even when the camera frame's aspect differs from the on-screen box (portrait phone, 16:9 sensor) — pinned in Task 4 (layout uses the displayed crop) and Task 5 (capture crops to the displayed aspect).
- Geolocation denied or unavailable: photo can still be taken with the coordinate line omitted; only `require_location` slots block the upload with a clear message — pinned in Task 3 and Task 7.
- A laptop front camera: preview and photo are mirrored like a selfie but the watermark text is never mirrored — pinned in Task 5.
- `localStorage` throwing or full (e.g. a large custom logo): settings fall back to defaults without crashing, logo is downscaled to ≤ 512 px before saving — pinned in Task 2.
- Backdate set to an earlier day: watermark shows that date, upload's `captured_at` is the real time — pinned in Task 7.

---

### Task 1: Reverse-geocoding proxy (`internal/geo` + route)

**Files:**
- Create: `internal/geo/reverse.go`, `internal/geo/reverse_test.go`
- Create: `internal/api/geo_routes.go`, `internal/api/geo_routes_test.go`
- Modify: `internal/config/config.go` (add `GeocoderURL`, env `GEOCODER_URL`, default `https://nominatim.openstreetmap.org`), `internal/api/handler.go` (interface + `Dependencies.Geo` + dispatch `case path == "geo/reverse"`), `cmd/server/main.go` (wire)

**Interfaces:**
- Produces: `geo.NewReverser(baseURL string, client *http.Client, userAgent string) *Reverser`; `(*Reverser).Reverse(ctx context.Context, lat, lng float64) string` — returns "" on any upstream failure (never an error to the caller beyond ctx). HTTP: `GET /api/v1/geo/reverse?lat=&lng=` → `200 {"data":{"address":"..."}}`; 400 `validation_failed` for missing/invalid/out-of-range lat/lng; auth via `h.authorizeAny(w, r, rc.principal, fieldReadPermissions...)`.

- [ ] **Step 1: Write failing tests** in `reverse_test.go` against an `httptest.Server` fake Nominatim:
  - `TestReverseComposesIndonesianAddress`: fake returns `{"address":{"road":"Jl. Merdeka","village":"Patila","county":"Pammana","city":"Kab. Wajo","state":"Sulawesi Selatan","postcode":"90971"}}` → result `"Jl. Merdeka, Patila, Pammana, Kab. Wajo, Sulawesi Selatan, 90971"`; asserts request query has `format=jsonv2`, `accept-language=id`, `lat`, `lon`, and header `User-Agent` equals the configured value.
  - `TestReverseDeduplicatesParts`: repeated component values appear once.
  - `TestReverseCachesByRoundedCoordinate`: two calls with coordinates equal at 4 decimals hit the fake once.
  - `TestReverseReturnsEmptyOnUpstreamError`: fake returns 429 → `""`; fake returns 500 → `""`.
  - `TestReverseRespectsRateLimit`: two uncached calls are ≥ 1 s apart at the fake (record arrival times).
- [ ] **Step 2: Run** `go test ./internal/geo/... -v` → FAIL (undefined).
- [ ] **Step 3: Implement** `Reverser`: field order `road, neighbourhood|suburb|village, city_district|county|municipality, city|town|regency|county, state, postcode` (skip empty, dedupe case-insensitively, join `", "`); cache key `fmt.Sprintf("%.4f,%.4f", lat, lng)` in a `sync.Mutex`-guarded map (cap 5000 entries, drop all when full); global limiter: a mutex + last-call timestamp enforcing ≥ 1 s between upstream calls; client timeout 5 s.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Route tests** in `geo_routes_test.go` (model on existing route tests, e.g. `internal/api/bast_routes_test.go` helpers `authenticatedRequest`): field-permission caller gets 200 with the stubbed address; caller without field permissions gets 403; `lat=abc` and `lat=91` → 400. Implement route + config + wiring; User-Agent `Konkit/1.0 (Konverter Kit Gas; PT Kian Santang Muliatama Tbk)`.
- [ ] **Step 6: Run** `go build ./... && go vet ./... && go test ./internal/geo/... ./internal/api/... ./internal/config/...` → PASS; gofmt clean.
- [ ] **Step 7: Commit** `feat(geo): reverse-geocoding proxy for the watermark address`

---

### Task 2: Watermark settings + photo resize helpers

**Files:**
- Create: `frontend/src/features/camera/watermarkSettings.ts`, `watermarkSettings.test.ts`, `imageResize.ts`, `imageResize.test.ts`

**Interfaces:**
- Produces:
  - `type WatermarkColor = { name: 'Kuning'|'Putih'|'Oranye'|'Hijau'|'Biru'; hex: string }`; `WATERMARK_COLORS: WatermarkColor[]`.
  - `type PhotoQuality = 'hemat'|'standar'|'tinggi'|'asli'`; `PHOTO_QUALITY_MAX_SIDE: Record<PhotoQuality, number | null>` (null = asli).
  - `type ElementPlacement = { x: number; y: number; scale: number } | null` (x/y = centre as fraction 0–1 of the photo; null = default position).
  - `type WatermarkSettings = { showLogo: boolean; showTimestamp: boolean; timestampSide: 'left'|'right'; textColor: string; accentColor: string; textScale: 0.85|1|1.2; logoScale: number; customLogo: string | null /* PNG data URL */; logoPlacement: ElementPlacement; timestampPlacement: ElementPlacement; backdate: string | null /* YYYY-MM-DD */; photoQuality: PhotoQuality }`.
  - `DEFAULT_WATERMARK_SETTINGS`, `loadWatermarkSettings(): WatermarkSettings`, `saveWatermarkSettings(s: WatermarkSettings): boolean` (false if storage refused), `resetWatermarkSettings(): WatermarkSettings`. Storage key `konkit.watermark.v1`.
  - `resizeImage(source: Blob, quality: PhotoQuality): Promise<File>` → JPEG `photo.jpg`, longest side ≤ max (no upscaling), quality 0.92 / 0.95 for asli. `downscaleLogo(file: Blob): Promise<string>` → PNG data URL, longest side ≤ 512.
- [ ] **Step 1: Write failing tests**: defaults match Global Constraints values; `load` merges partial stored JSON over defaults and ignores invalid fields (e.g. `textScale: 7` → 1); `load` returns defaults when `localStorage.getItem` throws or JSON is corrupt; `save` returns false (no throw) when `setItem` throws `QuotaExceededError`; `resizeImage` computes target size — mock `createImageBitmap` and `HTMLCanvasElement.prototype.toBlob`/`getContext` (jsdom has no real canvas) and assert canvas width/height for a 4000×3000 source at `standar` = 1920×1440, at `asli` = 4000×3000, and a 800×600 source at `hemat` stays 800×600; `toBlob` called with `'image/jpeg', 0.92` (0.95 for asli).
- [ ] **Step 2: Run** `npx vitest run src/features/camera/` (from `frontend/`) → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** → PASS; typecheck, lint clean.
- [ ] **Step 5: Commit** `feat(camera): watermark settings and photo resize helpers`

---

### Task 3: Location + address client

**Files:**
- Create: `frontend/src/features/camera/location.ts`, `location.test.ts`

**Interfaces:**
- Consumes: Task 1 endpoint.
- Produces:
  - `type Fix = { latitude: number; longitude: number; accuracy: number; at: number }`
  - `getFix(): Promise<Fix | null>` — high accuracy, timeout 8000, `maximumAge` 300000; resolves null (never rejects) when unsupported, denied, or timed out.
  - `class AddressTracker { constructor(fetchAddress?: (lat: number, lng: number) => Promise<string>); update(fix: Fix): void; current(): string; onChange(cb: (address: string) => void): () => void }` — calls `fetchAddress` only when moved > 15 m (haversine) since the last request; default fetcher calls `apiRequest('/api/v1/geo/reverse?lat=..&lng=..')` and aborts after 4000 ms → keeps last known; `current()` returns the last non-empty address or `'Alamat tidak tersedia'`.
  - `distanceMeters(a: {latitude:number;longitude:number}, b: same): number`.
  - `formatCoordinates(fix: Fix): string` → `Lat: -4.12345, Lng: 120.12345`.
- [ ] **Step 1: Write failing tests**: `getFix` resolves null on `PERMISSION_DENIED` and when `navigator.geolocation` is undefined, passes `{ enableHighAccuracy: true, timeout: 8000, maximumAge: 300000 }`; `distanceMeters` ≈ 111 km per degree latitude (±0.5%); tracker fetches once for two fixes 5 m apart and again for a fix 20 m away; tracker keeps the previous address when the fetcher resolves `''` or exceeds 4 s (fake timers); `current()` is `'Alamat tidak tersedia'` before any success; `formatCoordinates` output exact.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat(camera): geolocation fix and address tracker`

---

### Task 4: Watermark layout + canvas renderer

**Files:**
- Create: `frontend/src/features/camera/watermarkLayout.ts`, `watermarkLayout.test.ts`, `watermarkRender.ts`
- Create: `frontend/src/assets/watermark/logo-ksm.png`, `logo-ergas.png` (copied from `Konverter_Kit_Gas_Mobile/assets/images/logo-ksm-cropped.png` and `logo-ergas.png`)

**Interfaces:**
- Consumes: `WatermarkSettings`, `ElementPlacement` (Task 2).
- Produces:
  - `type Rect = { x: number; y: number; width: number; height: number }` (pixels).
  - `coverCrop(sourceW: number, sourceH: number, boxW: number, boxH: number): Rect` — the region of the source frame visible in an `object-fit: cover` box (centre crop).
  - `layoutWatermark(input: { width: number; height: number; settings: WatermarkSettings; logoAspect: number; timestampLines: number }): { logo: Rect | null; timestamp: Rect | null; fontSize: number }` — sizes relative to the photo's shorter side so the same settings give the same proportions at preview size and full size; default positions follow mobile (logo banner top, timestamp block bottom on `timestampSide`); a placement overrides the centre and multiplies the size by `scale` (clamped 0.4–3.0); rects are clamped inside the image.
  - `watermarkLines(now: Date, settings: WatermarkSettings, address: string, coords: string | null): string[]` — line 1 timestamp `yyyy-MM-dd HH:mm:ss` with the date part replaced by `settings.backdate` when set; then address; then coords when non-null.
  - `drawWatermark(ctx: CanvasRenderingContext2D, layout, lines: string[], logo: CanvasImageSource | null, settings: WatermarkSettings): void` (renders accent bar + text with shadow; not unit-tested beyond a smoke call with a mocked context).
- [ ] **Step 1: Write failing tests**: `coverCrop(1920,1080,1080,1920)` crops width to 607.5 centred (x≈656.25, height 1080); `layoutWatermark` returns proportionally identical rects for 400×300 and 4000×3000 (ratio of every field = 10 within 1e-6); hiding logo → `logo: null`; `timestampSide: 'right'` puts the block's right edge near the image's right edge; a placement `{x:0.5,y:0.5,scale:2}` centres the rect at the middle with double default size; a placement outside the image is clamped inside; `watermarkLines` uses backdate date with real time (`new Date('2026-10-23T14:05:09')`, backdate `2026-10-20` → `'2026-10-20 14:05:09'`) and omits the coords line when null.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat(camera): watermark layout and renderer`

---

### Task 5: `WatermarkCamera` dialog

**Files:**
- Create: `frontend/src/features/camera/WatermarkCamera.tsx`, `WatermarkCamera.test.tsx`, `useWatermarkLogo.ts`

**Interfaces:**
- Consumes: Tasks 2–4; `GET /api/v1/bast/branding?program_id=` (returns tender logos; read `internal/api/bast_routes.go` `handleBASTBrandingList` and `frontend/src/features/berita-acara/LogoTenderPanel.tsx` for the item shape and content URL).
- Produces: `export default function WatermarkCamera(props: { title: string; programId?: string; onCapture: (photo: CapturedPhoto) => void; onClose: () => void })` with `type CapturedPhoto = { file: File; capturedAt: Date; latitude: number | null; longitude: number | null }` exported from a new `frontend/src/features/camera/types.ts`. `useWatermarkLogo(programId?: string): HTMLImageElement | null` resolves custom logo → first visible tender logo → built-in banner (KSM left + Ergas right composed into one image).
- Behaviour: full-screen dialog (`role="dialog"`, aria-label = title); `getUserMedia` `{ video: { facingMode: { ideal: 'environment' }, width: { ideal: 1920 }, height: { ideal: 1080 } } }`; camera switcher when > 1 device; mirror preview when the track's facingMode is not `environment`; live overlay rendered with the same `layoutWatermark`/`drawWatermark` on an overlay canvas sized to the displayed box; pointer drag moves an element, wheel/pinch (two pointers) scales it, writing placements back into settings (persisted on release); toggle buttons Logo / Waktu; link "Pengaturan watermark" (`/pengaturan-watermark`); shutter → capture frame into a canvas cropped by `coverCrop` to the displayed aspect (mirrored for front cameras), draw watermark unmirrored, `resizeImage` per settings → review screen "Ambil Ulang" / "Gunakan Foto"; "Gunakan Foto" calls `onCapture` with real `capturedAt` and the fix (null coords if none); stops all tracks on close/unmount; camera errors map to the Indonesian messages already used in `frontend/src/features/distribution/BarcodeScanner.tsx` `cameraErrorMessage` (extract it to a shared `frontend/src/lib/cameraErrors.ts` and import it from both).
- [ ] **Step 1: Write failing tests** (mock `navigator.mediaDevices.getUserMedia`, `navigator.geolocation`, `apiRequest`, canvas `getContext`/`toBlob`, `createImageBitmap`, video `readyState`/`videoWidth`/`videoHeight` — see `BarcodeScanner.test.tsx` native-path tests for the pattern):
  - requests the constraints above;
  - shutter → review shows "Ambil Ulang"/"Gunakan Foto"; "Gunakan Foto" calls `onCapture` once with a JPEG `File`, `latitude`/`longitude` from the mocked fix, and `capturedAt` within 1 s of now;
  - geolocation denied → `onCapture` gets `latitude: null, longitude: null` and the capture still succeeds;
  - front camera (track facingMode `user`) → preview `<video>` has `transform: scaleX(-1)`; the capture path calls `drawImage` on a context whose transform was mirrored for the frame and reset before `drawWatermark` (assert via the mocked context's `setTransform`/`scale` call order);
  - "Ambil Ulang" returns to the live view without calling `onCapture`;
  - permission denied → Indonesian error + "Coba lagi";
  - closing stops every track.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** (lazy-loadable default export). **Step 4: Run** → PASS; also re-run `src/features/distribution/BarcodeScanner.test.tsx` after extracting `cameraErrorMessage`.
- [ ] **Step 5: Commit** `feat(camera): web watermark camera`

---

### Task 6: Watermark settings page

**Files:**
- Create: `frontend/src/features/camera/WatermarkSettingsPage.tsx`, `WatermarkSettingsPage.test.tsx`
- Modify: `frontend/src/app/routes.tsx` (route `pengaturan-watermark`, no permission gate — any signed-in user), `frontend/src/features/profile/ProfilePage.tsx` (a card/link "Kamera watermark" → `/pengaturan-watermark`)

**Interfaces:**
- Consumes: Task 2 load/save/reset + `downscaleLogo`; Task 4 layout/render for the live preview (on a static sample background).
- Controls (labels exact): "Tampilkan logo", "Tampilkan waktu & lokasi", "Posisi waktu" (Kiri/Kanan), "Warna teks", "Warna aksen" (5 swatches), "Ukuran teks" (Kecil/Sedang/Besar), "Ukuran logo" (slider 0.5–1.0), "Logo kustom" (Pilih gambar / Kembalikan logo bawaan), "Backdate" (date input + "Hapus backdate", hint "Hanya tanggal di watermark yang berubah; waktu asli tetap tercatat."), "Kualitas foto" (Hemat 1280 px / Standar 1920 px / Tinggi 2560 px / Asli), "Kembalikan posisi" (clears both placements), "Bawaan" (reset all, with confirm dialog).
- [ ] **Step 1: Write failing tests**: each control persists to `localStorage` key `konkit.watermark.v1` with the right value; "Bawaan" restores `DEFAULT_WATERMARK_SETTINGS` after confirming; picking a custom logo stores a PNG data URL via `downscaleLogo` (mock it); "Hapus backdate" sets `backdate: null`; a toast/inline message appears when `saveWatermarkSettings` returns false ("Pengaturan tidak dapat disimpan di perangkat ini.").
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS; `npx vitest run src/app/` still green.
- [ ] **Step 5: Commit** `feat(camera): watermark settings page`

---

### Task 7: Distribution documentation slots use the watermark camera, GPS and resize

**Files:**
- Modify: `frontend/src/features/distribution/DocumentationSlot.tsx`, `DocumentationSlot.test.tsx`, `frontend/src/features/distribution/DistributionPage.tsx`
- Create: `frontend/src/features/camera/WatermarkProgramContext.tsx` (`WatermarkProgramProvider({ programId, children })`, `useWatermarkProgram(): string | undefined`)

**Interfaces:**
- Consumes: `WatermarkCamera`, `CapturedPhoto` (Task 5), `resizeImage`, `loadWatermarkSettings` (Task 2).
- `DistributionPage` wraps its slot workspace in `WatermarkProgramProvider` with the selected schedule's program id (check the `Schedule` type in `frontend/src/features/distribution/types.ts` / `programs/types.ts` for the field name).
- `QueuedFile` gains `capturedAt: Date` and `latitude: number | null; longitude: number | null`. The upload sends `captured_at` = `capturedAt.toISOString()` (camera: real capture time; gallery: `file.lastModified`), and `latitude`/`longitude` only when non-null.
- "Buka kamera" (images) opens the lazy `WatermarkCamera` (`title` = slot label, `programId` from context) instead of the native capture input; its `onCapture` enqueues the photo with `source: 'camera'`. Video recording stays the native input. Gallery images pass through `resizeImage(file, settings.photoQuality)` before queueing (videos untouched). Existing size/type/capacity checks still apply.
- `slot.require_location` true: a camera photo with null coordinates is rejected before queueing with "Slot ini mewajibkan lokasi GPS. Aktifkan izin lokasi lalu ambil ulang foto."; gallery images are rejected for such slots with "Slot ini mewajibkan foto dari kamera dengan lokasi GPS."
- [ ] **Step 1: Write failing tests** (mock the lazy camera module with a stub that exposes a button calling `onCapture`):
  - camera capture → upload FormData has `source=camera`, `captured_at` equal to the stub's `capturedAt` ISO string, `latitude`, `longitude`;
  - with `backdate` set in stored settings, `captured_at` is still the stub's real `capturedAt` (backdate never leaks);
  - camera capture with null coordinates on a normal slot → uploads without `latitude`/`longitude` fields;
  - `require_location` slot + null coordinates → error message above, no upload; gallery image on such a slot → second message, no upload;
  - gallery image is passed through `resizeImage` with the stored `photoQuality` (mock `resizeImage`), video file is not.
- [ ] **Step 2: Run** `npx vitest run src/features/distribution/` → new tests FAIL. **Step 3: Implement.** **Step 4: Run** → all PASS.
- [ ] **Step 5: Commit** `feat(distribution): watermark camera, GPS and resized photos in documentation slots`

---

### Task 8: Activity documentation uses the watermark camera and resize

**Files:**
- Modify: `frontend/src/features/activities/ActivityDocumentationPage.tsx`, `ActivityDocumentationPage.test.tsx`

**Interfaces:**
- Consumes: `WatermarkCamera` (Task 5, `programId` = the page's selected `program_id` search param), `resizeImage`, `loadWatermarkSettings`.
- "Ambil Foto" opens `WatermarkCamera` (title = the activity label); `onCapture` feeds the existing `choose(file, 'camera')` path. Gallery images go through `resizeImage(file, settings.photoQuality)`; videos ("Rekam Video", gallery videos) untouched. No latitude/longitude fields are sent (spec non-goal).
- [ ] **Step 1: Write failing tests** (stub the lazy camera like Task 7): "Ambil Foto" renders the camera with the selected program id; capture uploads with `source=camera`; gallery image is resized with the stored quality; video is not resized; upload FormData contains no `latitude` key.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** `npx vitest run src/features/activities/ src/features/camera/ src/features/distribution/` → PASS; typecheck/lint clean; `git status` has no `web/static/app`.
- [ ] **Step 5: Commit** `feat(activities): watermark camera and resized photos`
