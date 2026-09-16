# Recipient Table Experience Design

## Goal

Make the Data Penerima table fast to scan and operate: keep identity visible while scrolling, expose custom evidence completeness without opening a record, search as the user types, and provide predictable server-side pagination.

## Data contract

Each recipient list item gains `evidence_slots`, ordered by the configured custom-slot order. Every slot contains `code`, `label`, `required`, `min_files`, `accepted_files`, and `complete`. The overall UI status is determined only by required slots meeting `min_files`; optional slots remain visible in the evidence detail.

The list query obtains this summary with one lateral aggregate per recipient query. It must not make a request per row and must return an empty array when a recipient has no distribution record or slots.

## Interaction

- Search applies automatically 350 ms after the last keystroke and resets the page to 1. There is no search-submit button.
- Search and filters remain encoded in URL parameters and react correctly to browser navigation.
- Page size is controlled by `page_size` and accepts 10, 20, 50, or 100; changing it resets the page to 1.
- Pagination includes first, previous, a compact adaptive number window with ellipses, next, and last. It reports the visible range, for example `21–40 dari 237 penerima`.
- The left edge remains ordered and sticky as No. Pembagian, Nama, NIK. The evidence column immediately follows them. Aksi is sticky on the right for authorized users.

## Visual direction

This is an operational government-assistance register, so the table should feel precise and archival rather than decorative.

- Palette: retain the product tokens; use `background` and `card` as the paper surfaces, `border` for ruled separation, `muted` for header context, `primary` for complete evidence, amber for partial evidence, and `muted-foreground` for empty evidence.
- Type: retain the application typeface. Use tabular numerals for distribution numbers, NIK, counts, and pagination ranges; use normal sentence case throughout.
- Layout: dense horizontal register with explicit column widths. Identity columns form a stable frozen pane; the rest scrolls beneath it. Rows use consistent table-cell layout, never `display:flex` on a `td`.
- Memorable element: the compact evidence meter (`3/4 wajib`) and slot detail. Everything else stays restrained.
- Accessibility: keyboard-scrollable table region, named pagination controls, visible focus, text labels in addition to color, and an accessible evidence-detail trigger.

## Responsive behavior

The table remains horizontally scrollable. The three requested identity columns stay sticky at all breakpoints with compact fixed widths; long names truncate visually but remain available through title/accessibility text. Controls wrap above the table, and pagination splits into range/page-size and navigation groups on narrow screens.

