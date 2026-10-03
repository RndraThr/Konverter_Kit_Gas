# RAKORDA Attendance Design

## Objective

Add a complete RAKORDA attendance workflow to the Berita Acara workspace: configure a location and adjustable blank-row count, preview/download/print a numbered blank attendance PDF, then upload the handwritten result as PDF or images to the program Drive hierarchy.

## Reference and visual direction

The source PDF is A4 portrait and has two visual modes: page one contains the active program logos, centered title and procurement narrative, location/regency/province metadata, event-date narrative, and the beginning of the attendance table; continuation pages contain only the repeated table header and blank rows. The application keeps this structure but uses the current BA logo configuration and current document typography.

The table columns are `No.`, `Nama`, `Pekerjaan`, `No. Telepon`, and `Ttd`. Only `No.` is populated. Numbering is continuous across pages. Empty cells are intentionally large enough for handwriting.

## User workflow

1. Select an active schedule and open the Rakorda tab.
2. Save the RAKORDA location and number of attendance rows. Default row count is 45; valid range is 5-200.
3. Select the event date. Regency, province, program, fiscal year, zone, and logos come from schedule/program configuration.
4. Preview the generated PDF. The operator may download it or invoke browser printing. Preview/printing never creates a Drive artifact.
5. After the paper is filled at the event, upload either PDFs, JPGs, PNGs, or a mixture. Multiple uploads for the same event date form an ordered result set.
6. Uploaded artifacts are downloadable and removable from the RAKORDA panel and are stored beneath `PETANI/ZONA/KABUPATEN/BERITA ACARA (BA)/6. RAKORDA`.

## Scan-assisted capture

The upload area offers direct file upload and `Scan dokumen`. Scanning happens entirely in the browser before upload:

- capture from camera or select an image;
- adjust four corners for perspective correction;
- rotate in 90-degree steps;
- choose `Warna Asli`, `Warna Ditingkatkan`, `Grayscale`, or `Hitam-Putih`;
- add, reorder, or remove pages;
- preview and combine processed pages into one PDF;
- upload only the generated PDF to the server.

`Warna Ditingkatkan` applies restrained contrast, saturation, brightness, and sharpening intended for photographed paper. It must not crush signatures or light handwriting. Direct upload remains available when scanning is unnecessary.

## Data model

Extend `bast_schedule_settings` with:

- `rakorda_location text NOT NULL DEFAULT ''`
- `rakorda_row_count integer NOT NULL DEFAULT 45 CHECK (rakorda_row_count BETWEEN 5 AND 200)`

Create `bast_rakorda_uploads` with schedule/program/regency/date identity, opaque storage key, filename, original filename, MIME type, byte size, checksum, sort order, active/deleted status, uploader, and timestamps. Upload rows are scoped through their schedule and preserve audit history through soft deletion.

Generated blank PDFs are not persisted in this table or in `bast_aggregate_documents`.

## API

- `GET /api/v1/bast/schedules/{schedule_id}/rakorda-settings`
- `PUT /api/v1/bast/schedules/{schedule_id}/rakorda-settings` with `{location,row_count}`
- `POST /api/v1/bast/rakorda/preview` with `{schedule_id,document_date}` and an inline PDF response
- `GET /api/v1/bast/rakorda/uploads?schedule_id=...&date=...`
- `POST /api/v1/bast/rakorda/uploads` multipart fields `schedule_id`, `document_date`, and `file`
- `GET /api/v1/bast/rakorda/uploads/{id}/content`
- `DELETE /api/v1/bast/rakorda/uploads/{id}`

Read operations require `bast.view`; settings, upload, and delete require `bast.manage`. All repository reads and mutations enforce the caller's regency scope.

## Upload rules

- Allowed MIME types: `application/pdf`, `image/jpeg`, `image/png`.
- Maximum size: 20 MiB per file.
- MIME type is detected from bytes; extension and browser content type are not trusted.
- Stored filenames follow `RAKORDA - {REGENCY} - {YYYY-MM-DD} - {NN}.{ext}`.
- Failed database writes remove the newly stored object; failed storage deletion restores the soft-deleted row.

## Validation and failure states

Preview requires an active BA logo, configured program zone, non-empty RAKORDA location, valid date, and valid row count. UI errors explain exactly which setting is missing. Upload is independent of preview availability except that schedule/date must be valid.

## Non-goals

- OCR or automatic population of attendee names.
- Automatic document-edge detection in the first release.
- Storing blank generated previews in Drive.
- Modifying historical distribution/document snapshots.

