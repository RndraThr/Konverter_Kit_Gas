# Recipient Table Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the Data Penerima list with evidence summaries, frozen identity columns, realtime search, and flexible server-side pagination.

**Architecture:** Extend the existing recipient read model with a per-row JSON evidence summary aggregated in the list query. Keep table state URL-driven in React, add small pure pagination/evidence helpers, and render a fixed-width frozen-pane table without changing shared table semantics.

**Tech Stack:** Go, PostgreSQL/pgx, React 19, TanStack Query, React Router, Shadcn/Base UI, Vitest/Testing Library.

**Spec:** `docs/superpowers/specs/2026-09-16-recipient-table-experience-design.md`

## Global Constraints

- Required evidence completeness is `accepted_files >= min_files`; optional slots never block the overall status.
- Optional slots remain visible in evidence details.
- Recipient listing performs no per-row HTTP or database query.
- URL parameters remain the source of truth for server filters and pagination.
- Allowed page sizes are exactly 10, 20, 50, and 100.
- Preserve all existing login/auth work and unrelated dirty-worktree changes.

---

### Task 1: Recipient evidence read model

**Files:**
- Modify: `internal/recipients/models.go`
- Modify: `internal/recipients/repository.go`
- Modify: `internal/recipients/repository_integration_test.go`

**Interfaces:**
- Produces: `EvidenceSlotSummary` and `Recipient.EvidenceSlots []EvidenceSlotSummary` serialized as `evidence_slots`.
- Each summary contains `slot_code`, `label`, `is_required`, `min_files`, `accepted_files`, and `complete`.

- [ ] **Step 1: Write the failing integration test**

Seed a `distribution_records` row for `fixture.needsReviewAllocID`, two required slots (one satisfied and one missing), one satisfied optional slot, and accepted media for the satisfied slots. Call `Repository.List`, locate the allocation, and assert the literal ordered results: required `1/2` complete and optional `1/1` complete.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/recipients -run TestListIncludesOrderedEvidenceSlotCompleteness -count=1 -v`

Expected: compile failure because `Recipient.EvidenceSlots` does not exist.

- [ ] **Step 3: Add the minimal model and aggregate query**

Add:

```go
type EvidenceSlotSummary struct {
    SlotCode      string `json:"slot_code"`
    Label         string `json:"label"`
    IsRequired    bool   `json:"is_required"`
    MinFiles      int    `json:"min_files"`
    AcceptedFiles int    `json:"accepted_files"`
    Complete      bool   `json:"complete"`
}
```

Add an ordered `LEFT JOIN LATERAL` JSON aggregation over `documentation_slots` and accepted `media_files`, scan the JSON payload, and unmarshal it to a non-nil empty slice. Reuse the select for list and single-recipient reads.

- [ ] **Step 4: Run focused and package tests and verify GREEN**

Run: `go test ./internal/recipients -run TestListIncludesOrderedEvidenceSlotCompleteness -count=1 -v`, then `go test ./internal/recipients -count=1`.

---

### Task 2: Pure pagination and evidence presentation behavior

**Files:**
- Create: `frontend/src/features/dashboard/recipientTable.ts`
- Create: `frontend/src/features/dashboard/recipientTable.test.ts`
- Modify: `frontend/src/features/dashboard/types.ts`

**Interfaces:**
- Produces: `buildPageItems(currentPage: number, totalPages: number): Array<number | 'ellipsis-start' | 'ellipsis-end'>`.
- Produces: `summarizeEvidence(slots: EvidenceSlot[]): { state: 'complete' | 'partial' | 'empty' | 'not-configured'; requiredComplete: number; requiredTotal: number; optionalComplete: number; optionalTotal: number }`.

- [ ] **Step 1: Write failing literal table tests**

Cover one page, a leading window, a middle window, a trailing window, required completeness with optional slots, no accepted files, and no configured slots. Assertions use hand-derived literal arrays/counts.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `npm test -- --run src/features/dashboard/recipientTable.test.ts` from `frontend`.

Expected: module-not-found failure.

- [ ] **Step 3: Implement minimal pure helpers and types**

Keep at most five numeric page buttons plus boundary pages and ellipses. Determine overall completeness from required slots only; report optional counts independently.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run the same focused test command and confirm all helper tests pass.

---

### Task 3: Dashboard realtime controls and frozen register table

**Files:**
- Modify: `frontend/src/features/dashboard/DashboardPage.test.tsx`
- Modify: `frontend/src/features/dashboard/DashboardPage.tsx`

**Interfaces:**
- Consumes: evidence types/helpers from Task 2 and existing `/api/v1/recipients` URL contract.
- Produces: 350 ms debounced search, page-size control, adaptive pagination, reordered/sticky columns, and accessible evidence detail.

- [ ] **Step 1: Write failing component tests**

Add tests proving: no Cari button exists; typing does not request immediately but requests with `search=Siti&page=1` after 350 ms; page size 50 requests `page_size=50&page=1`; a large result exposes adaptive page navigation; the first headers are `No. Pembagian`, `Nama`, `NIK`, `Kelengkapan evidence`; evidence shows `1/2 wajib` and optional detail; action-cell buttons remain inside a semantic `td`.

- [ ] **Step 2: Run component tests and verify RED**

Run: `npm test -- --run src/features/dashboard/DashboardPage.test.tsx` from `frontend`.

Expected: failures for the existing submit search, old order, absent evidence, and simple pagination.

- [ ] **Step 3: Implement the minimal dashboard behavior**

Use a 350 ms effect to update `search` and reset `page`; synchronize the input when URL search changes. Read page size from the response/URL, set only allowed values, and clamp navigation between 1 and `Math.ceil(total/pageSize)`. Render explicit fixed widths and sticky offsets for the three left columns, a subtle right-edge divider, a compact evidence trigger/detail, and a sticky action column. Put action buttons in an inner flex wrapper so the `td` remains a table cell.

- [ ] **Step 4: Run component tests and verify GREEN**

Run the focused dashboard tests until all pass without warnings.

- [ ] **Step 5: Refactor while green**

Extract only repeated sticky-class constants or small render helpers needed to keep `DashboardPage.tsx` readable; do not generalize the feature beyond this table.

---

### Task 4: Full verification and visual review

**Files:**
- Modify only if verification reveals a tested defect.

**Interfaces:**
- Produces: verified backend/frontend build and a browser-reviewed `/dashboard` table.

- [ ] **Step 1: Run formatting and static checks**

Run: `gofmt -w internal/recipients/models.go internal/recipients/repository.go internal/recipients/repository_integration_test.go`, `go vet ./...`, `npx tsc --noEmit`, and `npm run build`.

- [ ] **Step 2: Run full automated suites**

Run: `go test ./... -count=1` and `npm test`.

- [ ] **Step 3: Browser-review the result**

At `http://localhost:8080/dashboard`, verify desktop and narrow viewports, horizontal scrolling, all frozen offsets, keyboard focus, evidence detail, realtime search, page-size changes, and boundary pagination. Capture a screenshot for visual inspection if the local server is available.

- [ ] **Step 4: Review the diff**

Confirm no unrelated login/auth changes or `.claude/` files were modified, and ensure generated static assets only change if the final build intentionally regenerates them.

