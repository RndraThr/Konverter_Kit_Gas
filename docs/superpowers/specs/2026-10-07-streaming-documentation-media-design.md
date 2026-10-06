# Streaming Documentation Media Design

**Date:** 2026-10-07

## Summary

Konkit will support configurable image and video uploads per distribution documentation slot without buffering large request bodies in VPS memory or temporary disk. Images are limited to 25 MiB per file and videos to 500 MiB per file. Existing templates and instantiated slots remain image-only unless an administrator publishes a new template version with a different media policy.

The existing Activity Documentation flow will use the same streaming pipeline and limits. Uploads continue to be stored in Google Drive, while PostgreSQL stores authoritative metadata only after storage succeeds.

## Goals

- Allow each documentation template slot to accept images, videos, or both.
- Preserve image-only behavior for all existing templates and distribution slots.
- Accept JPEG, PNG, and WebP images up to 25 MiB.
- Accept MP4, MOV, and WebM videos up to 500 MiB.
- Stream uploads through Nginx and the Go application to Google Drive with bounded memory and no full-file VPS temporary copy.
- Give mobile users upload progress, cancellation, retry, and appropriate image/video previews.
- Keep incomplete, oversized, invalid, or database-orphaned uploads out of the active dataset.

## Non-goals

- Transcoding, resizing, thumbnail generation, or video compression.
- Resuming a partially uploaded file after the browser or mobile connection has been closed. A failed upload can be retried from the beginning.
- Allowing arbitrary file formats or executable content.
- Changing the number of files allowed in a slot.
- Changing historical distribution slots when a template is edited.

## Current Constraints

- Distribution documentation currently accepts only JPEG, PNG, and WebP files up to 10 MiB.
- Activity Documentation accepts images and videos up to 100 MiB but reads the complete file into a byte slice.
- The distribution and activity HTTP handlers use `ParseMultipartForm` and `io.ReadAll`, which can consume substantial RAM or temporary disk.
- The staging Nginx virtual host has no `client_max_body_size`, so its default limit rejects ordinary mobile-camera files before they reach the application.
- At design time, the VPS root filesystem is 96% full with approximately 1.9 GB available. Full request buffering is therefore unsafe for 500 MiB uploads.
- `media.Storage` and the Google Drive backend already accept an `io.Reader`, so the storage boundary can stream without introducing a second storage API.

## Media Policy Model

Add a `media_kind` enum-like text field with three allowed values:

- `image`: JPEG, PNG, or WebP only.
- `video`: MP4, MOV, or WebM only.
- `image_video`: either supported image or supported video.

The field is added to both:

- `documentation_template_slots`, where administrators configure the policy.
- `documentation_slots`, where the policy is snapshotted when a distribution slot is created.

Both columns are `NOT NULL`, default to `image`, and have a database check constraint. This keeps existing templates and instantiated slots image-only. Editing a published template continues to create a new draft version; schedules only gain the new media policy after they reference a published version, and already-created distribution slots retain their snapshot.

Program Setup exposes a required selector labelled **Jenis media** with **Foto saja**, **Video saja**, and **Foto & video**. New slots default to **Foto saja**.

API template and distribution response models expose `media_kind`. Repository reads, writes, template validation, snapshot creation, and integration tests are updated accordingly.

## Limits and Validation

The application owns the authoritative limits:

| Media | Allowed MIME types | Maximum file size |
| --- | --- | ---: |
| Image | `image/jpeg`, `image/png`, `image/webp` | 25 MiB |
| Video | `video/mp4`, `video/quicktime`, `video/webm` | 500 MiB |

The frontend rejects an obviously oversized `File` before starting the network request, but server-side checks remain authoritative.

The server reads at most the initial 512 bytes to sniff content. Image types must be recognized from their bytes. Video containers may use an allow-listed filename extension as a fallback when Go reports `application/octet-stream`, matching the existing Activity Documentation behavior. Client-supplied `Content-Type` is never sufficient by itself.

The `media_files` check constraints are expanded to the six allowed MIME types and a maximum of 500 MiB. The `activity_media` size constraint is expanded to the same database ceiling. Application validation still applies the smaller 25 MiB limit to images in both flows.

## Streaming HTTP Contract

Distribution and Activity Documentation uploads remain multipart requests, but their handlers use `MultipartReader` instead of `ParseMultipartForm`.

Small metadata fields, including the browser-reported `file_size`, must appear before the single `file` part. The maintained frontend always constructs `FormData` in that order. `file_size` permits an early rejection but is untrusted; the streamed byte count remains authoritative. The handler:

1. Applies `http.MaxBytesReader` with a small multipart-overhead allowance above 500 MiB.
2. Parses metadata parts with strict per-field and aggregate size limits.
3. Rejects duplicates, unknown parts, multiple files, missing metadata, or a file part arriving before required metadata.
4. Authorizes the request and resolves the slot/program context before storage begins.
5. Reads only the sniff prefix, determines the media type and applicable limit, then reconstructs the stream with `io.MultiReader`.
6. Wraps the stream in a limit of the allowed size plus one byte and passes it to `media.PutNamed`.
7. Uses the storage result's streamed byte count and checksum to enforce the final size.

If the stored byte count exceeds the media limit, the just-created Drive object is deleted and the API returns HTTP 413. If the metadata transaction fails after storage succeeds, the existing compensating deletion remains in effect.

Request cancellation propagates through the request context into the Google Drive upload. No database row is created unless the complete storage operation succeeds.

## Google Drive Upload

`GoogleDriveStorage` continues to send an `io.Reader` to the Drive client. The upload is configured with a bounded resumable chunk size so the library may retry Drive-bound chunks without buffering the complete file. SHA-256 is computed by the existing hashing reader as bytes pass through the pipeline.

Visible filenames retain the existing business labels and gain the correct extension for all six MIME types. Folder resolution and folder caching are unchanged.

At most three video uploads may be active in one application instance. A fourth video upload receives HTTP 429 with an Indonesian retry message. Image uploads do not consume the video semaphore. The concurrency value is a named configuration setting with a default of three so it can be tuned without code changes.

## Nginx

The Konkit virtual host receives upload-specific handling that:

- Sets `client_max_body_size 510M`.
- Disables `proxy_request_buffering` for documentation media upload endpoints, preventing Nginx from writing the complete body to its temporary directory.
- Uses upload-appropriate proxy send/read timeouts of 30 minutes.
- Retains existing TLS, forwarded headers, and security headers.

The application limits remain authoritative. Nginx's slightly larger limit permits multipart overhead and allows the application to return structured JSON for a file that exceeds its media-specific limit.

The configuration is backed up before editing, checked with `nginx -t`, and reloaded only after validation succeeds.

## Mobile User Experience

An image-only slot keeps the current camera and gallery controls. A video-only slot shows **Rekam Video** and **Pilih Galeri**. A mixed slot shows **Ambil Foto**, **Rekam Video**, and **Pilih Galeri** with appropriate `accept` and `capture` attributes.

Uploads are queued sequentially per browser to avoid saturating a mobile connection. Each queued item shows:

- image thumbnail or video poster/player treatment;
- upload percentage and current state;
- cancel while uploading;
- retry or remove after failure.

The upload client uses `XMLHttpRequest` for upload progress and cancellation while preserving the existing session cookie and CSRF header. API errors are decoded into the existing `ApiError` shape. HTTP 413 identifies the correct image or video limit; HTTP 429 asks the user to wait for another video upload to finish.

Stored images use the existing image preview. Stored videos use `<video controls preload="metadata">` through the shared media preview dialog. UI labels use “file” or “media” for mixed/video slots instead of always saying “foto”.

## Failure and Cleanup Behavior

- Invalid type or policy mismatch: reject before Drive upload.
- Client-declared oversize: reject before streaming when possible.
- Actual oversize discovered while streaming: delete the newly created Drive object and return 413.
- Browser cancellation or network loss: cancel the Drive request through context and create no database record.
- Drive failure: create no database record and return a retryable error.
- Database failure after Drive success: delete the Drive object using the existing compensating action.
- Video concurrency limit: return 429 without reading or storing the file body.
- Preview/download: retain `X-Content-Type-Options: nosniff` and serve only stored allow-listed MIME types.

Application logs record the endpoint, media kind, accepted/rejected size, status, and duration without logging file content, OAuth data, personal identifiers, or storage keys.

## Compatibility and Rollout

The schema migration is additive and defaults old rows to `image`. Application and frontend deployment occurs together so multipart field ordering remains consistent. No staging data reset is required.

Deployment order:

1. Back up the PostgreSQL database and Nginx site configuration.
2. Build and run the automated test suites.
3. Deploy the application image and run the additive migration.
4. Install the Nginx upload configuration, validate it, and reload Nginx.
5. Verify health, authentication redirect, image upload, video upload, cancellation, preview, and Drive metadata on staging.
6. Monitor application and Nginx logs for 413, 429, 499, 5xx, Drive errors, memory, and disk use.

Rollback restores the previous Nginx configuration and application image. The added columns and expanded constraints may remain because their defaults are compatible with the previous image; template administrators must not enable video slots until the new application is active.

## Testing Strategy

Tests are written before production changes.

### Backend unit and route tests

- Existing slots default to `image`.
- Template validation accepts only the three media policy values.
- Multipart metadata must precede the file and remains size-bounded.
- JPEG/PNG/WebP are accepted only by image-capable slots.
- MP4/MOV/WebM are accepted only by video-capable slots.
- Mixed slots accept both groups.
- Image size boundaries cover 25 MiB and 25 MiB plus one byte.
- Video size boundaries cover 500 MiB and 500 MiB plus one byte using synthetic streaming readers rather than allocating complete buffers.
- MIME spoofing, unknown extensions, empty files, duplicate file parts, and multiple files are rejected.
- Storage and repository failure paths perform compensating cleanup.
- Cancellation reaches storage.
- The fourth simultaneous video upload returns 429.

### Storage tests

- Google Drive upload receives an incremental reader and configured chunking.
- Streaming size and SHA-256 are correct.
- Local storage still commits atomically and cleans temporary files after failure.

### Database integration tests

- Migration defaults old template and instantiated slots to `image`.
- Template CRUD persists `media_kind`.
- Creating a distribution slot snapshots the selected policy.
- `media_files` accepts the six supported MIME types up to the database ceiling.

### Frontend tests

- Template editor renders and submits the media selector.
- Each policy renders the correct camera, video, and gallery controls.
- Client-side limits distinguish images from videos.
- Multipart metadata is appended before the file.
- Progress, cancellation, retry, 413, and 429 states are accessible.
- Stored video opens in a video player while stored images retain image preview.

### Staging verification

- Upload a normal mobile-camera image larger than the former Nginx limit.
- Upload representative MP4/MOV/WebM samples to enabled slots.
- Confirm image-only slots reject video and video-only slots reject images.
- Cancel an in-progress video and confirm no active database or Drive object remains.
- Confirm the new Drive root receives the expected folder and visible filename.
- Observe bounded app memory and no proportional growth in Nginx temporary disk usage during a large streaming upload.

## Acceptance Criteria

- A 5.1 MiB mobile-camera JPEG uploads successfully on staging.
- Valid images up to 25 MiB and valid videos up to 500 MiB are accepted only according to slot policy.
- Existing templates and slots remain image-only.
- Uploading a large video does not allocate or write a full-file copy on the VPS.
- The user sees progress and can cancel or retry an upload.
- Failed, cancelled, oversized, or metadata-failed uploads leave no active database record or orphaned Drive file.
- Nginx returns requests to the application up to the configured ceiling and the application returns structured Indonesian errors.
- All backend, integration, frontend, and end-to-end tests pass before staging deployment.
