# Konkit Shadcn Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign seluruh frontend Konkit dengan design system Shadcn yang konsisten, responsif, dan aksesibel tanpa mengubah perilaku bisnis.

**Architecture:** Tailwind CSS v4 dan Shadcn `base-nova` berbasis Base UI menjadi fondasi primitive di `src/components/ui`; komponen domain tetap berada di `src/components` atau folder feature. Migrasi dilakukan dari fondasi ke shell dan pola bersama, lalu per kelompok halaman, sambil mempertahankan React Query, API, permission, route, accessible name, dan alur E2E.

**Tech Stack:** React 19, TypeScript, Vite 7, Tailwind CSS v4, Shadcn UI `base-nova`, Base UI, Lucide React, TanStack React Query, Vitest, Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-09-08-konkit-shadcn-redesign-design.md`

## Global Constraints

- Warna inti wajib tetap `forest #173D2B`, `leaf #2F7D4A`, `gold #C49A3A`, `canvas #F6F7F2`, `surface #FFFFFF`, dan `line #DDE4DA`.
- Target viewport wajib mencakup desktop `1366×768` dan mobile `375×812`.
- Kontrak API, query key, route, permission, CSRF, form action autentikasi, istilah bisnis, dan urutan workflow tidak boleh berubah.
- Kedua logo serta fotografi lapangan pada login wajib dipertahankan.
- Primitive Shadcn ditempatkan pada `frontend/src/components/ui`; komponen yang memahami domain Konkit tidak ditempatkan di folder tersebut.
- Kontrol utama memiliki target sentuh minimal 44 px; seluruh elemen interaktif memiliki visible focus dan mendukung keyboard.
- Status tidak boleh disampaikan melalui warna saja dan animasi non-esensial wajib menghormati `prefers-reduced-motion`.
- Tidak boleh ada overflow horizontal halaman yang tidak disengaja pada viewport target.
- Shadcn menggunakan konfigurasi Vite/Tailwind v4 resmi, style `base-nova`, Base UI, CSS variables, dan ikon Lucide.

---

### Task 1: Shadcn and Tailwind Foundation

**Files:**
- Create: `frontend/components.json`
- Create: `frontend/src/lib/utils.ts`
- Create: `frontend/src/components/ui/button.tsx`
- Create: `frontend/src/components/ui/badge.tsx`
- Create: `frontend/src/components/ui/input.tsx`
- Create: `frontend/src/components/ui/label.tsx`
- Create: `frontend/src/components/ui/checkbox.tsx`
- Create: `frontend/src/components/ui/select.tsx`
- Create: `frontend/src/components/ui/dialog.tsx`
- Create: `frontend/src/components/ui/sheet.tsx`
- Create: `frontend/src/components/ui/dropdown-menu.tsx`
- Create: `frontend/src/components/ui/tabs.tsx`
- Create: `frontend/src/components/ui/table.tsx`
- Create: `frontend/src/components/ui/card.tsx`
- Create: `frontend/src/components/ui/separator.tsx`
- Create: `frontend/src/components/ui/alert.tsx`
- Create: `frontend/src/components/ui/skeleton.tsx`
- Create: `frontend/src/components/ui/tooltip.tsx`
- Create: `frontend/src/components/ui/sonner.tsx`
- Create: `frontend/src/components/ui/DesignSystem.test.tsx`
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `frontend/tsconfig.json`
- Modify: `frontend/vite.config.ts`
- Modify: `frontend/src/styles/global.css`
- Modify: `frontend/src/main.tsx`

**Interfaces:**
- Produces: the variadic `cn` class-merging utility and reusable Shadcn exports under `@/components/ui/*`.
- Produces: semantic CSS tokens consumed by all later tasks.

- [ ] **Step 1: Restore the dependency baseline**

Run:

```powershell
npm.cmd --prefix frontend ci
npm.cmd --prefix frontend run test
npm.cmd --prefix frontend run build
```

Expected: existing tests pass and the Vite build writes its manifest into `web/static/app`. If `npm ci` requires network permission, request it rather than substituting package versions.

- [ ] **Step 2: Write the failing design-system interaction test**

Create `frontend/src/components/ui/DesignSystem.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test } from 'vitest';
import { Button } from './button';
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from './sheet';

test('provides accessible Konkit controls', async () => {
  render(
    <Sheet>
      <SheetTrigger render={<Button variant="outline" />}>Buka filter</SheetTrigger>
      <SheetContent>
        <SheetTitle>Filter data</SheetTitle>
      </SheetContent>
    </Sheet>,
  );

  await userEvent.click(screen.getByRole('button', { name: 'Buka filter' }));
  expect(screen.getByRole('dialog', { name: 'Filter data' })).toBeInTheDocument();
});
```

- [ ] **Step 3: Verify the test fails for the missing primitives**

Run:

```powershell
npm.cmd --prefix frontend run test -- DesignSystem.test.tsx
```

Expected: FAIL because `./button` and `./sheet` do not exist.

- [ ] **Step 4: Configure Tailwind v4 and the Shadcn registry**

Install the official Vite/Tailwind and Shadcn dependencies:

```powershell
npm.cmd --prefix frontend install tailwindcss @tailwindcss/vite shadcn class-variance-authority cn tw-animate-css sonner
```

Set `frontend/components.json` to:

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "base-nova",
  "rsc": false,
  "tsx": true,
  "tailwind": {
    "config": "",
    "css": "src/styles/global.css",
    "baseColor": "neutral",
    "cssVariables": true,
    "prefix": ""
  },
  "iconLibrary": "lucide",
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils",
    "ui": "@/components/ui",
    "lib": "@/lib",
    "hooks": "@/hooks"
  }
}
```

Add `baseUrl: "."` and `paths: { "@/*": ["./src/*"] }` to `frontend/tsconfig.json`. Add `tailwindcss()` and the `@` alias to `frontend/vite.config.ts`:

```ts
import path from 'node:path';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(__dirname, './src') } },
  test: { environment: 'jsdom', setupFiles: './src/test/setup.ts', include: ['src/**/*.test.{ts,tsx}'] },
  build: { outDir: '../web/static/app', emptyOutDir: false, manifest: true },
});
```

Export the helper from `frontend/src/lib/utils.ts`:

```ts
export { cn } from 'cn';
```

- [ ] **Step 5: Generate only the approved Shadcn primitives**

Run from `frontend`:

```powershell
npx.cmd shadcn@latest add button badge input label checkbox select dialog sheet dropdown-menu tabs table card separator alert skeleton tooltip sonner --yes
```

Review generated files to confirm they use Base UI and imports through `@/lib/utils`.

- [ ] **Step 6: Establish the Konkit semantic theme**

Replace the legacy imports and base rules in `frontend/src/styles/global.css` with Tailwind v4, Shadcn animation CSS, semantic variables, and base layer:

```css
@import "tailwindcss";
@import "tw-animate-css";
@import "shadcn/tailwind.css";

:root {
  --background: #f6f7f2;
  --foreground: #17251c;
  --card: #ffffff;
  --card-foreground: #17251c;
  --popover: #ffffff;
  --popover-foreground: #17251c;
  --primary: #2f7d4a;
  --primary-foreground: #ffffff;
  --secondary: #edf3eb;
  --secondary-foreground: #173d2b;
  --muted: #eef1eb;
  --muted-foreground: #667269;
  --accent: #f5ecd8;
  --accent-foreground: #76591a;
  --destructive: #b3403b;
  --border: #dde4da;
  --input: #d4ddd2;
  --ring: #2f7d4a;
  --radius: 0.625rem;
  --sidebar: #173d2b;
  --sidebar-foreground: #eef6f0;
  --sidebar-primary: #c49a3a;
  --sidebar-primary-foreground: #173d2b;
  --sidebar-accent: #245139;
  --sidebar-accent-foreground: #ffffff;
  --sidebar-border: #315c43;
  --sidebar-ring: #d7b764;
}

@layer base {
  * { @apply border-border outline-ring/50; }
  html { font-family: "Aptos", "Segoe UI Variable", "Segoe UI", sans-serif; }
  body { @apply m-0 min-h-svh bg-background text-foreground antialiased; }
  button, input, select, textarea { font: inherit; }
  :where([data-numeric]) { font-variant-numeric: tabular-nums; }
}
```

Add `<Toaster richColors position="top-right" />` once in `frontend/src/main.tsx`.

- [ ] **Step 7: Verify the design-system test and build pass**

Run:

```powershell
npm.cmd --prefix frontend run test -- DesignSystem.test.tsx
npm.cmd --prefix frontend run build
```

Expected: PASS and successful production build.

- [ ] **Step 8: Commit the foundation**

```powershell
git add frontend/package.json frontend/package-lock.json frontend/components.json frontend/tsconfig.json frontend/vite.config.ts frontend/src/styles/global.css frontend/src/main.tsx frontend/src/lib/utils.ts frontend/src/components/ui
git commit -m "feat: add Konkit Shadcn design system"
```

---

### Task 2: Shared Page, Form, Status, and Data Patterns

**Files:**
- Create: `frontend/src/components/PageHeader.tsx`
- Create: `frontend/src/components/PageHeader.test.tsx`
- Create: `frontend/src/components/DataState.tsx`
- Create: `frontend/src/components/DataState.test.tsx`
- Modify: `frontend/src/components/DataTable.tsx`
- Create: `frontend/src/components/DataTable.test.tsx`
- Modify: `frontend/src/components/FormField.tsx`
- Create: `frontend/src/components/FormField.test.tsx`
- Modify: `frontend/src/components/StatusBadge.tsx`
- Create: `frontend/src/components/StatusBadge.test.tsx`

**Interfaces:**
- Produces: `PageHeader({ title, description, eyebrow?, actions?, context? })`.
- Produces: `DataState({ kind, title, description, action? })` where `kind` is `loading | empty | error`.
- Produces: `DataTable({ children, label, minimumWidth? })` using the Shadcn Table primitives.
- Produces: `FormField` with existing `InputHTMLAttributes` API.
- Produces: `StatusBadge({ active, activeText?, inactiveText? })` with text and icon/dot cues.

- [ ] **Step 1: Write failing behavior tests for shared patterns**

Use these assertions in the new test files:

```tsx
test('associates errors with the input', () => {
  render(<FormField name="email" label="Email" error="Email wajib diisi" />);
  expect(screen.getByLabelText('Email')).toHaveAttribute('aria-invalid', 'true');
  expect(screen.getByLabelText('Email')).toHaveAccessibleDescription('Email wajib diisi');
});

test('labels a scrollable data region', () => {
  render(<DataTable label="Daftar pengguna"><tbody><tr><td>Admin</td></tr></tbody></DataTable>);
  expect(screen.getByRole('region', { name: 'Daftar pengguna' })).toBeInTheDocument();
  expect(screen.getByRole('table', { name: 'Daftar pengguna' })).toBeInTheDocument();
});

test('gives an error state a retry action', async () => {
  const retry = vi.fn();
  render(<DataState kind="error" title="Data gagal dimuat" description="Periksa koneksi." action={{ label: 'Coba lagi', onClick: retry }} />);
  await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
  expect(retry).toHaveBeenCalledOnce();
});
```

- [ ] **Step 2: Verify the tests fail for missing/new behavior**

```powershell
npm.cmd --prefix frontend run test -- PageHeader.test.tsx DataState.test.tsx DataTable.test.tsx FormField.test.tsx StatusBadge.test.tsx
```

Expected: FAIL because the new components and accessible scroll region do not exist.

- [ ] **Step 3: Implement the shared components with Shadcn primitives**

Use a stable page header skeleton:

```tsx
export function PageHeader({ title, description, eyebrow, actions, context }: PageHeaderProps) {
  return <header className="flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-end sm:justify-between">
    <div className="min-w-0 space-y-1.5">
      {eyebrow ? <p className="text-xs font-semibold text-primary">{eyebrow}</p> : null}
      <h1 className="text-balance text-2xl font-semibold tracking-tight sm:text-[1.75rem]">{title}</h1>
      <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{description}</p>
      {context}
    </div>
    {actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
  </header>;
}
```

Implement `DataTable` with a named `role="region"`, `tabIndex={0}`, `overflow-x-auto`, and Shadcn `<Table>`. Implement `FormField` with Shadcn `<Label>` and `<Input>`, keeping `name`, `id`, `aria-invalid`, and error description behavior. Implement `StatusBadge` with Shadcn `<Badge>` and visible status text.

- [ ] **Step 4: Run focused and full tests**

```powershell
npm.cmd --prefix frontend run test -- PageHeader.test.tsx DataState.test.tsx DataTable.test.tsx FormField.test.tsx StatusBadge.test.tsx
npm.cmd --prefix frontend run test
```

Expected: all tests pass.

- [ ] **Step 5: Commit shared patterns**

```powershell
git add frontend/src/components
git commit -m "feat: standardize Konkit interface patterns"
```

---

### Task 3: Responsive App Shell and Navigation

**Files:**
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/app/AppShell.test.tsx`
- Delete: `frontend/src/app/AppShell.module.css`

**Interfaces:**
- Consumes: Shadcn Button, Sheet, Dropdown Menu, Separator, Skeleton, Tooltip, and shared tokens.
- Preserves: `AppShell()` route outlet, permission filtering, `Buka navigasi`, `Menu akun`, `Profil saya`, and logout form behavior.

- [ ] **Step 1: Add failing shell behavior assertions**

Extend `AppShell.test.tsx`:

```tsx
it('exposes the workspace and account actions through landmarks', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(bootstrap), { status: 200 }));
  renderShell();
  expect(await screen.findByRole('navigation', { name: 'Navigasi utama' })).toBeInTheDocument();
  expect(screen.getByRole('main')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Menu akun' }));
  expect(screen.getByRole('menuitem', { name: 'Profil saya' })).toBeInTheDocument();
  expect(screen.getByRole('menuitem', { name: 'Keluar' })).toBeInTheDocument();
});
```

- [ ] **Step 2: Verify the new menu semantics fail**

```powershell
npm.cmd --prefix frontend run test -- AppShell.test.tsx
```

Expected: FAIL because the current account popup does not expose `menuitem` semantics.

- [ ] **Step 3: Rebuild the shell using Shadcn composition**

Keep the existing `groups`, `canSee`, bootstrap query, and route lookup. Replace module CSS with Tailwind classes and these structures:

```tsx
<div className="min-h-svh bg-background md:grid md:grid-cols-[264px_minmax(0,1fr)]">
  <aside className="sticky top-0 hidden h-svh flex-col bg-sidebar text-sidebar-foreground md:flex">
    <Brand />
    <Navigation permissions={user.permissions} />
    <SystemConnectionStatus />
  </aside>
  <div className="min-w-0">
    <header className="sticky top-0 z-30 flex h-16 items-center border-b bg-background/95 px-4 backdrop-blur sm:px-6">
      <MobileNavigation permissions={user.permissions} />
      <PageIdentity currentItem={currentItem} />
      <AccountMenu user={user} csrfToken={bootstrap.data.meta.csrf_token} />
    </header>
    <main className="mx-auto w-full max-w-[1440px] p-4 sm:p-6 lg:p-8"><Outlet /></main>
  </div>
</div>
```

Use Sheet for mobile navigation and Dropdown Menu for account actions. Set the sidebar active link with an inset gold marker plus text/background change. Retain both logo `alt` values and all navigation labels. Use a Skeleton shell while bootstrap is pending and an Alert with a retry button when bootstrap fails.

- [ ] **Step 4: Verify shell tests and permission behavior**

```powershell
npm.cmd --prefix frontend run test -- AppShell.test.tsx
npm.cmd --prefix frontend run test
```

Expected: all tests pass, including hiding `Role & akses` when permission is absent.

- [ ] **Step 5: Commit the responsive shell**

```powershell
git add frontend/src/app/AppShell.tsx frontend/src/app/AppShell.test.tsx frontend/src/app/AppShell.module.css
git commit -m "feat: redesign responsive application shell"
```

---

### Task 4: Login, Dashboard, and Account Pages

**Files:**
- Modify: `frontend/src/pages/LoginPage/LoginPage.tsx`
- Modify: `frontend/src/pages/LoginPage/LoginPage.module.css`
- Create: `frontend/src/pages/LoginPage/LoginPage.test.tsx`
- Modify: `frontend/src/features/dashboard/DashboardPage.tsx`
- Modify: `frontend/src/features/dashboard/DashboardPage.module.css`
- Modify: `frontend/src/features/dashboard/DashboardPage.test.tsx`
- Modify: `frontend/src/features/profile/ProfilePage.tsx`
- Modify: `frontend/src/features/profile/ProfilePage.test.tsx`
- Modify: `frontend/src/features/profile/ChangePasswordForm.tsx`
- Modify: `frontend/src/features/profile/ChangePasswordForm.test.tsx`

**Interfaces:**
- Consumes: PageHeader, DataState, Button, Input, Checkbox, Card, Badge, Separator, Skeleton, and Alert.
- Preserves: `/login` form action, field names, error query codes, dashboard API query, profile mutations, and password validation.

- [ ] **Step 1: Write a failing accessible login-state test**

Create `LoginPage.test.tsx`:

```tsx
test('keeps field imagery, brands, and submission feedback accessible', async () => {
  history.replaceState({}, '', '/login?error=invalid');
  render(<LoginPage />);
  expect(screen.getByRole('main')).toBeInTheDocument();
  expect(screen.getByRole('form', { name: 'Masuk ke dashboard' })).toBeInTheDocument();
  expect(screen.getByAltText('Ergas')).toBeInTheDocument();
  expect(screen.getByAltText('PT Kian Santang Mulitama Tbk')).toBeInTheDocument();
  expect(screen.getByRole('alert')).toHaveTextContent('Email/username atau password tidak sesuai.');
  await userEvent.type(screen.getByLabelText('Email atau username'), 'admin');
  await userEvent.type(screen.getByLabelText('Password'), 'password-rahasia');
  expect(screen.getByRole('button', { name: 'Masuk' })).toBeEnabled();
});
```

- [ ] **Step 2: Verify the test fails on the desired accessible Password name**

```powershell
npm.cmd --prefix frontend run test -- LoginPage.test.tsx
```

Expected: FAIL because the current login form has no accessible name.

- [ ] **Step 3: Redesign login while preserving its distinctive photography**

Keep the six slides, paths, captions, focus points, both logos, POST action, and field names. Use Shadcn Input, Checkbox, Button, and Alert in the form. Keep CSS Modules only for the carousel timing and the asymmetric two-panel photographic composition; use semantic tokens rather than the legacy token file. At `max-width: 820px`, stack the image above the form; at `max-width: 520px`, remove the outer card border and shadow.

- [ ] **Step 4: Migrate dashboard and account pages**

Use `PageHeader` on all four pages. Dashboard metrics use quiet bordered surfaces, tabular numbers, icon labels, and a text status badge. Profile and password forms use a single-column `max-w-2xl` layout with consistent field descriptions and full-width mobile actions. Replace generic empty/error paragraphs with `DataState` or Alert while preserving API calls and current copy required by tests.

Dashboard structure:

```tsx
<div className="space-y-8">
  <PageHeader title="Ringkasan program" description="Pantau fondasi akses dan kesiapan sistem Konkit." />
  <section aria-label="Ringkasan pengguna" className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
    {metrics.map((metric) => <Metric key={metric.label} {...metric} />)}
  </section>
  <div className="grid gap-6 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,.6fr)]">
    <RecentActivity entries={summary.data?.recent_activity ?? []} />
    <OperationalStatus system={summary.data?.system} />
  </div>
</div>
```

Extract `Metric`, `RecentActivity`, and `OperationalStatus` as private components in `DashboardPage.tsx`; they do not fetch data and accept only the props shown above.

- [ ] **Step 5: Run the page tests**

```powershell
npm.cmd --prefix frontend run test -- LoginPage.test.tsx DashboardPage.test.tsx ProfilePage.test.tsx ChangePasswordForm.test.tsx
npm.cmd --prefix frontend run test
```

Expected: all tests pass.

- [ ] **Step 6: Commit authentication and account surfaces**

```powershell
git add frontend/src/pages/LoginPage frontend/src/features/dashboard frontend/src/features/profile
git commit -m "feat: redesign login dashboard and account pages"
```

---

### Task 5: Administration Pages and Dialogs

**Files:**
- Modify: `frontend/src/features/users/UsersPage.tsx`
- Modify: `frontend/src/features/users/UserDialog.tsx`
- Modify: `frontend/src/features/users/UserPasswordDialog.tsx`
- Modify: `frontend/src/features/users/UsersPage.test.tsx`
- Modify: `frontend/src/features/roles/RolesPage.tsx`
- Modify: `frontend/src/features/roles/RoleDialog.tsx`
- Modify: `frontend/src/features/roles/RolesPage.test.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.test.tsx`
- Modify: `frontend/src/features/health/HealthPage.tsx`
- Modify: `frontend/src/features/health/HealthPage.module.css`
- Modify: `frontend/src/features/health/HealthPage.test.tsx`
- Modify: `frontend/src/features/audit/AuditPage.tsx`
- Modify: `frontend/src/features/audit/AuditPage.test.tsx`

**Interfaces:**
- Consumes: PageHeader, DataTable, FormField, StatusBadge, DataState, Dialog, Alert Dialog, Button, Select, Checkbox, Badge, Alert, Skeleton, and toast.
- Preserves: permission-driven mutation controls, create/edit/password actions, system-role protection, regency scope, typed settings, health status parsing, and read-only audit behavior.

- [ ] **Step 1: Add a failing row-action menu test and retry feedback assertion**

Add to the existing administration tests:

```tsx
expect(await screen.findByText('Petugas Wajo')).toBeInTheDocument();
await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));
await userEvent.click(screen.getByRole('button', { name: 'Aksi Petugas Wajo' }));
expect(screen.getByRole('menuitem', { name: 'Edit pengguna' })).toBeInTheDocument();
expect(screen.getByRole('menuitem', { name: 'Atur ulang password' })).toBeInTheDocument();
```

Append these assertions to the existing `lists users and opens the create dialog` test after its current dialog assertion.

In `HealthPage.test.tsx`, assert the degraded state remains a named alert and the refresh button remains available.

- [ ] **Step 2: Verify the new accessible names fail where the legacy dialogs differ**

```powershell
npm.cmd --prefix frontend run test -- UsersPage.test.tsx RolesPage.test.tsx SettingsPage.test.tsx HealthPage.test.tsx AuditPage.test.tsx
```

Expected: FAIL because the current row actions are separate buttons rather than a named Dropdown Menu.

- [ ] **Step 3: Migrate users and roles**

Replace global `pageHeader`, `toolbar`, `tableFrame`, `dialogPopup`, and button classes with shared components and Shadcn primitives. Use Dropdown Menu for row actions, Dialog for create/edit/password forms, Alert Dialog for destructive or activation confirmation, Badge for scope/system role, and toast for successful mutations. Preserve current button/link accessible names used in tests and keep all read-only permission branches.

- [ ] **Step 4: Migrate settings, health, and audit**

Settings use grouped sections with descriptive headers and a sticky mobile save area only when permission allows. Health uses three responsive status surfaces with text plus icons and a named Alert for degraded status. Audit uses DataTable with a stable horizontal scroll region, timestamp tabular numerals, read-only labels, and existing filters.

- [ ] **Step 5: Run focused and complete tests**

```powershell
npm.cmd --prefix frontend run test -- UsersPage.test.tsx RolesPage.test.tsx SettingsPage.test.tsx HealthPage.test.tsx AuditPage.test.tsx
npm.cmd --prefix frontend run test
```

Expected: all tests pass.

- [ ] **Step 6: Commit administration migration**

```powershell
git add frontend/src/features/users frontend/src/features/roles frontend/src/features/settings frontend/src/features/health frontend/src/features/audit
git commit -m "feat: redesign administration workspaces"
```

---

### Task 6: Program Preparation Workspace

**Files:**
- Modify: `frontend/src/features/programs/ProgramSetupPage.tsx`
- Modify: `frontend/src/features/programs/ProgramsPanel.tsx`
- Modify: `frontend/src/features/programs/RegenciesPanel.tsx`
- Modify: `frontend/src/features/programs/SchedulesPanel.tsx`
- Modify: `frontend/src/features/programs/TemplatesPanel.tsx`
- Modify: `frontend/src/features/programs/SetupDialog.tsx`
- Modify: `frontend/src/features/programs/ProgramSetup.module.css`
- Modify: `frontend/src/features/programs/ProgramSetupPage.test.tsx`

**Interfaces:**
- Consumes: PageHeader, DataTable, StatusBadge, FormField, Dialog, Tabs, Button, Select, Checkbox, Badge, Separator, Alert, and toast.
- Preserves: four workspace labels, uppercase normalization, original-case note/supervisor fields, repeated equipment/component/slot rows, permission behavior, and all mutation payloads.

- [ ] **Step 1: Add a failing named-workspace-region test**

Add to `ProgramSetupPage.test.tsx`:

```tsx
test('exposes the active setup workspace as a named region', async () => {
  renderPage(['programs.view', 'programs.manage']);
  expect(await screen.findByRole('region', { name: 'Kabupaten operasional' })).toBeInTheDocument();
});
```

- [ ] **Step 2: Verify the legacy custom tabs fail the keyboard contract**

```powershell
npm.cmd --prefix frontend run test -- ProgramSetupPage.test.tsx
```

Expected: FAIL because the current Kabupaten section is not programmatically named as a region.

- [ ] **Step 3: Migrate the workspace and its panels**

Use Shadcn Tabs with the four existing labels and horizontally scrollable `TabsList`. Replace each setup panel header, data listing, status, actions, and empty state with the shared patterns. Use `SetupDialog` as the single domain wrapper around Shadcn Dialog. Repeated slot/equipment/component rows use responsive CSS grid only where Tailwind utilities would make field alignment unreadable; grid definitions become two columns below 820 px and one column below 640 px.

- [ ] **Step 4: Preserve normalization and repeatable-field behavior**

Do not change input event handlers or values. Only replace their rendering with Shadcn Input, Select, Checkbox, Button, and field descriptions. Keep removal buttons as icon buttons with explicit accessible names naming the row type.

- [ ] **Step 5: Verify program behavior**

```powershell
npm.cmd --prefix frontend run test -- ProgramSetupPage.test.tsx
npm.cmd --prefix frontend run test
```

Expected: all tests pass, including normalization and repeatable rows.

- [ ] **Step 6: Commit the program workspace**

```powershell
git add frontend/src/features/programs
git commit -m "feat: redesign program preparation workspace"
```

---

### Task 7: DCP3 Import Wizard

**Files:**
- Create: `frontend/src/features/dcp3/ImportStepper.tsx`
- Create: `frontend/src/features/dcp3/ImportStepper.test.tsx`
- Modify: `frontend/src/features/dcp3/DCP3ImportPage.tsx`
- Modify: `frontend/src/features/dcp3/ColumnMappingStep.tsx`
- Modify: `frontend/src/features/dcp3/ImportPreviewTable.tsx`
- Modify: `frontend/src/features/dcp3/DCP3Import.module.css`
- Modify: `frontend/src/features/dcp3/DCP3ImportPage.test.tsx`

**Interfaces:**
- Produces: `ImportStepper({ currentStep, steps })`, with `currentStep` as a one-based integer and `steps` as `{ label: string; description: string }[]`.
- Consumes: PageHeader, DataTable, DataState, Button, Select, Badge, Alert, Separator, and Card.
- Preserves: schedule selection, file constraints, header-row recovery, column mapping, preview status, import result, permission behavior, and existing API payloads.

- [ ] **Step 1: Write a failing semantic stepper test**

Create `ImportStepper.test.tsx`:

```tsx
test('announces active and completed import steps', () => {
  render(<ImportStepper currentStep={2} steps={[
    { label: 'Jadwal', description: 'Pilih jadwal aktif' },
    { label: 'Workbook', description: 'Unggah file DCP3' },
    { label: 'Pemetaan', description: 'Cocokkan kolom' },
    { label: 'Tinjau', description: 'Periksa dan import' },
  ]} />);
  expect(screen.getByRole('list', { name: 'Tahapan import DCP3' })).toBeInTheDocument();
  expect(screen.getByText('Workbook').closest('li')).toHaveAttribute('aria-current', 'step');
  expect(screen.getByText('Jadwal').closest('li')).toHaveAttribute('data-state', 'complete');
});
```

- [ ] **Step 2: Verify the stepper test fails**

```powershell
npm.cmd --prefix frontend run test -- ImportStepper.test.tsx
```

Expected: FAIL because `ImportStepper` does not exist.

- [ ] **Step 3: Implement the responsive stepper**

Render an ordered visual sequence on desktop and a two-column compact grid below 820 px. Each item includes its number, label, optional description, `data-state`, and `aria-current="step"` for the active item. Use forest for completed, gold for active, and muted styling for upcoming steps; text/number preserves meaning without color.

- [ ] **Step 4: Migrate all four wizard stages**

Use `PageHeader` with persistent schedule context, Shadcn Select for schedules and column mapping, Alert for validation/error messages, Button for back/next/import actions, and shared DataTable for raw-row and import previews. Keep file input native and visually associate it with a labeled dropzone. On mobile, stack mapping fields, keep action buttons full-width, and allow only table regions—not the page—to scroll horizontally.

- [ ] **Step 5: Verify all DCP3 paths**

```powershell
npm.cmd --prefix frontend run test -- ImportStepper.test.tsx DCP3ImportPage.test.tsx
npm.cmd --prefix frontend run test
```

Expected: all tests pass for the standard four steps, title-row recovery, and read-only rendering.

- [ ] **Step 6: Commit the DCP3 wizard**

```powershell
git add frontend/src/features/dcp3
git commit -m "feat: redesign DCP3 import wizard"
```

---

### Task 8: Distribution and Reports Workspaces

**Files:**
- Modify: `frontend/src/features/distribution/DistributionPage.tsx`
- Modify: `frontend/src/features/distribution/RecipientSearch.tsx`
- Modify: `frontend/src/features/distribution/RecipientWorkspace.tsx`
- Modify: `frontend/src/features/distribution/DocumentationSlot.tsx`
- Modify: `frontend/src/features/distribution/Distribution.module.css`
- Modify: `frontend/src/features/distribution/DistributionPage.test.tsx`
- Modify: `frontend/src/features/distribution/RecipientWorkspace.test.tsx`
- Modify: `frontend/src/features/distribution/DocumentationSlot.test.tsx`
- Modify: `frontend/src/features/reports/ReportsPage.tsx`
- Modify: `frontend/src/features/reports/Reports.module.css`
- Modify: `frontend/src/features/reports/ReportsPage.test.tsx`

**Interfaces:**
- Consumes: PageHeader, DataTable, DataState, FormField, StatusBadge, Button, Select, Badge, Alert, Card, Separator, Skeleton, Tooltip, and toast.
- Preserves: debounced search, masked eligibility result, draft fields, equipment values, camera/gallery inputs, retry upload, finalization, permission behavior, report filters, complete NIK display, and export URLs.

- [ ] **Step 1: Add a failing documentation-region test**

Add to `RecipientWorkspace.test.tsx`:

```tsx
test('groups verification and documentation into named regions', () => {
  renderWorkspace(['distribution.manage']);
  expect(screen.getByRole('region', { name: 'Verifikasi penerima' })).toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Dokumentasi penyerahan' })).toBeInTheDocument();
});
```

Extract the current render fixture as `renderWorkspace()` and keep its real workspace data.

- [ ] **Step 2: Verify the named regions fail before restructuring**

```powershell
npm.cmd --prefix frontend run test -- RecipientWorkspace.test.tsx
```

Expected: FAIL because the current sections do not expose both named regions.

- [ ] **Step 3: Redesign recipient discovery and workspace hierarchy**

Keep search as the first focus. Render results as accessible buttons with masked identity, eligibility text, and completion summary. After selection, use named sections for recipient header, verification, equipment, source data, documentation, and completion. Desktop uses a main column plus quieter source-data rail; below 900 px it becomes one column. Below 640 px fields and documentation become one column and all primary form actions become full-width.

- [ ] **Step 4: Redesign documentation slots without changing upload behavior**

Each slot uses a bordered section with title, required/optional Badge, helper copy, progress, preview grid, and explicit `Buka kamera`, `Pilih galeri`, `Coba lagi`, and remove controls. Preserve input `accept`, `capture`, upload payload, preview URLs, retry behavior, and callback contracts. Use two preview columns on mobile and three on desktop.

- [ ] **Step 5: Redesign reports using the same data language**

Use PageHeader, a responsive schedule/status filter bar, quiet summary metrics, export Buttons rendered as links, and DataTable for rows. Preserve complete NIK rendering, schedule/status query parameters, and Excel/PDF hrefs. Keep the report table horizontally scrollable on mobile because cross-column comparison remains the primary job.

- [ ] **Step 6: Run operational tests**

```powershell
npm.cmd --prefix frontend run test -- DistributionPage.test.tsx RecipientWorkspace.test.tsx DocumentationSlot.test.tsx ReportsPage.test.tsx
npm.cmd --prefix frontend run test
```

Expected: all tests pass, including upload retry, draft save, finalization, filters, and export links.

- [ ] **Step 7: Commit operational pages**

```powershell
git add frontend/src/features/distribution frontend/src/features/reports
git commit -m "feat: redesign distribution and reporting workspaces"
```

---

### Task 9: Legacy Cleanup and End-to-End Visual Verification

**Files:**
- Delete: `frontend/src/styles/dashboardComponents.css`
- Delete: `frontend/src/styles/dashboardTokens.css`
- Delete: `frontend/src/styles/tokens.css`
- Modify: `frontend/src/styles/global.css`
- Modify: `frontend/e2e/administration.spec.ts`
- Modify: `frontend/e2e/dcp3-distribution.spec.ts`
- Modify: `frontend/playwright.config.ts` only if screenshot output configuration is absent
- Modify: `README.md`

**Interfaces:**
- Consumes: all redesigned pages and existing E2E seed flow.
- Produces: a frontend with no live dependency on legacy dashboard classes or token files.

- [ ] **Step 1: Prove legacy selectors are still referenced**

Run:

```powershell
rg -n "dashboardComponents|dashboardTokens|tokens.css|className=\"(page|pageHeader|primaryButton|secondaryButton|tableFrame|dialogPopup|formField|statusBadge)" frontend/src
```

Expected: matches remain before cleanup. Treat this search as the failing migration check; the target state is zero matches.

- [ ] **Step 2: Remove the final legacy imports and selectors**

Replace every remaining legacy class with shared components or Tailwind utilities. Delete the three unused CSS files only after `rg` returns no live import or selector usage. Preserve Login carousel CSS and feature CSS that still encodes complex responsive grids.

- [ ] **Step 3: Add stable visual checkpoints to E2E**

At the end of each major journey, add named screenshots:

```ts
await page.screenshot({
  path: testInfo.outputPath(`konkit-${testInfo.project.name}-final.png`),
  fullPage: true,
});
```

Keep screenshots as run artifacts, not committed golden snapshots. Do not weaken the existing role/name assertions to accommodate markup changes.

- [ ] **Step 4: Run complete automated verification**

```powershell
$env:GOCACHE="$PWD/.cache/go-build"
go test ./... -count=1
go vet ./...
npm.cmd --prefix frontend run test
npm.cmd --prefix frontend run build
npm.cmd --prefix frontend run e2e
```

Expected: Go tests, vet, frontend tests, production build, desktop E2E, and mobile E2E all pass.

- [ ] **Step 5: Inspect desktop and mobile screenshots**

Review the generated `konkit-desktop-final.png` and `konkit-mobile-final.png` artifacts for:

- no page-level horizontal overflow;
- visible sticky header and usable mobile navigation;
- consistent page header alignment and spacing;
- readable table overflow affordance;
- dialogs and sheets contained within the viewport;
- minimum 44 px primary touch controls;
- visible error, loading, empty, disabled, success, and focus states;
- preserved green–gold identity, logos, and login photography.

Fix any visual defect, add a regression assertion when the defect has testable behavior, then rerun the affected focused test and E2E project.

- [ ] **Step 6: Update project documentation**

Add a `Frontend UI` subsection to `README.md` documenting:

```markdown
### Frontend UI

Frontend menggunakan Tailwind CSS v4 dan komponen Shadcn berbasis Base UI. Token tema berada di `frontend/src/styles/global.css`, primitive UI berada di `frontend/src/components/ui`, dan komponen domain tetap berada di folder feature masing-masing.

Tambahkan primitive baru dari direktori `frontend`; sebagai contoh, Button ditambahkan dengan `npx shadcn@latest add button`. Jangan mengubah kontrak API atau permission ketika melakukan perubahan presentasi.
```

- [ ] **Step 7: Commit cleanup and verification**

```powershell
git add frontend/src frontend/e2e frontend/playwright.config.ts README.md
git commit -m "test: verify responsive Konkit redesign"
```
