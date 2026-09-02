# React UI Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate the current Go-template login UI into a custom React + TypeScript frontend while keeping the Go server as the local backend/static host.

**Architecture:** The Go server remains responsible for routing and serving static files. The React app is built with Vite into `web/static/app`, and `/login` serves a lightweight HTML shell that loads the compiled frontend. Styling uses CSS Modules and global design tokens, with Base UI added when interactive headless components are needed.

**Tech Stack:** Go, React, TypeScript, Vite, CSS Modules, Base UI, plain CSS variables, local static asset hosting.

**Spec:** `docs/superpowers/specs/2026-09-01-konkit-system-design.md`

## Global Constraints

- Backend stays in Go.
- Local development first; Docker deployment is planned later.
- Login page must use Ergas logo, KSM logo, and three provided field photos.
- UI must fit desktop viewport at 75-100% browser zoom.
- UI must be responsive for mobile.
- Do not show the visible text `Super Admin` on the login page.
- KSM logo must not render with a black background.
- Keep the scope to login UI migration; do not implement real authentication yet.

---

## File Structure

- Create `frontend/package.json`: frontend scripts and dependencies.
- Create `frontend/index.html`: Vite HTML entry.
- Create `frontend/tsconfig.json`: TypeScript compiler settings.
- Create `frontend/vite.config.ts`: Vite build configuration that outputs to `web/static/app`.
- Create `frontend/src/main.tsx`: React entry point.
- Create `frontend/src/pages/LoginPage/LoginPage.tsx`: login page component.
- Create `frontend/src/pages/LoginPage/LoginPage.module.css`: page-specific CSS module.
- Create `frontend/src/styles/tokens.css`: global design tokens.
- Create `frontend/src/styles/global.css`: global reset and base styles.
- Modify `web/templates/login.html`: replace full login markup with a React mount shell.
- Modify `internal/web/server.go`: keep `/login` route and render the shell.
- Modify `internal/web/server_test.go`: assert shell, assets, and login constraints.

## Task 1: Frontend Project Skeleton

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/index.html`
- Create: `frontend/tsconfig.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/src/main.tsx`

**Interfaces:**
- Produces: Vite build output under `web/static/app`.
- Produces: React app mounted at `<div id="konkit-root"></div>`.

- [ ] **Step 1: Write the failing test**

Add this assertion to `internal/web/server_test.go` inside `TestLoginPageRendersBrandingAndCarouselAssets`:

```go
expectedSnippets := []string{
	"Sistem Manajemen Program Konkit Gas",
	"/static/app/assets/",
	"id=\"konkit-root\"",
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```powershell
$env:GOCACHE='D:\KSM\Deployment\konkit\.cache\go-build'; go test ./...
```

Expected: FAIL because `/static/app/assets/` and `konkit-root` are not present in the login shell yet.

- [ ] **Step 3: Create minimal frontend skeleton**

Create `frontend/package.json`:

```json
{
  "scripts": {
    "dev": "vite --host 127.0.0.1 --port 5173",
    "build": "vite build",
    "preview": "vite preview --host 127.0.0.1 --port 4173"
  },
  "dependencies": {
    "@base-ui/react": "^1.0.0",
    "@vitejs/plugin-react": "^5.0.0",
    "vite": "^7.0.0",
    "typescript": "^5.0.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {}
}
```

Create `frontend/index.html`:

```html
<!doctype html>
<html lang="id">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Sistem Manajemen Program Konkit Gas</title>
  </head>
  <body>
    <div id="konkit-root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Create `frontend/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["DOM", "DOM.Iterable", "ES2022"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx"
  },
  "include": ["src"],
  "references": []
}
```

Create `frontend/vite.config.ts`:

```ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../web/static/app',
    emptyOutDir: true,
    manifest: true,
  },
});
```

Create `frontend/src/main.tsx`:

```tsx
import { createRoot } from 'react-dom/client';

function App() {
  return <div>Sistem Manajemen Program Konkit Gas</div>;
}

const rootElement = document.getElementById('konkit-root');

if (!rootElement) {
  throw new Error('Missing #konkit-root element');
}

createRoot(rootElement).render(<App />);
```

- [ ] **Step 4: Install dependencies and build**

Run:

```powershell
cd frontend
npm install
npm run build
```

Expected: build emits files under `web/static/app`.

- [ ] **Step 5: Commit**

Skip commit until the workspace is initialized as a git repository.

## Task 2: Go Login Shell For Built React App

**Files:**
- Modify: `web/templates/login.html`
- Modify: `internal/web/server.go`
- Test: `internal/web/server_test.go`

**Interfaces:**
- Consumes: Vite manifest at `web/static/app/.vite/manifest.json`.
- Produces: login shell with `id="konkit-root"` and script/link tags pointing to Vite build assets.

- [ ] **Step 1: Write the failing test**

Add a test that asserts built assets are represented in the shell:

```go
func TestLoginPageServesReactShell(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	body := response.Body.String()
	if !strings.Contains(body, `id="konkit-root"`) {
		t.Fatal("expected React mount element")
	}
	if !strings.Contains(body, `/static/app/assets/`) {
		t.Fatal("expected Vite built asset path")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```powershell
$env:GOCACHE='D:\KSM\Deployment\konkit\.cache\go-build'; go test ./...
```

Expected: FAIL because `login.html` still contains the server-rendered form.

- [ ] **Step 3: Implement Vite manifest loading**

Add structs and function in `internal/web/server.go`:

```go
type LoginPageData struct {
	Title   string
	Script  string
	Styles  []string
}

type viteManifestEntry struct {
	File string   `json:"file"`
	CSS  []string `json:"css"`
}

func loadViteEntry(root string) (script string, styles []string) {
	path := filepath.Join(root, "web", "static", "app", ".vite", "manifest.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return "/static/app/assets/index.js", nil
	}

	var manifest map[string]viteManifestEntry
	if err := json.Unmarshal(content, &manifest); err != nil {
		return "/static/app/assets/index.js", nil
	}

	entry, ok := manifest["index.html"]
	if !ok {
		return "/static/app/assets/index.js", nil
	}

	for _, css := range entry.CSS {
		styles = append(styles, "/static/app/"+css)
	}

	return "/static/app/" + entry.File, styles
}
```

Update `login` handler:

```go
script, styles := loadViteEntry(projectRoot())
data := LoginPageData{
	Title:  "Sistem Manajemen Program Konkit Gas",
	Script: script,
	Styles: styles,
}
```

- [ ] **Step 4: Replace `web/templates/login.html`**

Use this shell:

```html
<!doctype html>
<html lang="id">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
  {{ range .Styles }}
  <link rel="stylesheet" href="{{ . }}">
  {{ end }}
</head>
<body>
  <div id="konkit-root"></div>
  <script type="module" src="{{ .Script }}"></script>
</body>
</html>
```

- [ ] **Step 5: Run tests**

Run:

```powershell
$env:GOCACHE='D:\KSM\Deployment\konkit\.cache\go-build'; go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

Skip commit until the workspace is initialized as a git repository.

## Task 3: React Login Page

**Files:**
- Create: `frontend/src/pages/LoginPage/LoginPage.tsx`
- Create: `frontend/src/pages/LoginPage/LoginPage.module.css`
- Create: `frontend/src/styles/tokens.css`
- Create: `frontend/src/styles/global.css`
- Modify: `frontend/src/main.tsx`

**Interfaces:**
- Consumes static assets under `/static/images/`.
- Produces a responsive login page with field carousel, Ergas logo, KSM logo, and login form.

- [ ] **Step 1: Write the failing test**

Add a lightweight Node/Vite test script later when test tooling is available. For this local phase, keep the Go shell tests and visual HTTP checks as the executable boundary.

- [ ] **Step 2: Implement design tokens**

Create `frontend/src/styles/tokens.css`:

```css
:root {
  --ergas-green: #59b947;
  --leaf-deep: #27763b;
  --leaf-ink: #153c2a;
  --ksm-gold: #c99a32;
  --mint-field: #eaf6e4;
  --paper: #fbfdf8;
  --line: #d8e4d2;
  --muted: #657066;
  --text: #172018;
  --white: #ffffff;
}
```

Create `frontend/src/styles/global.css`:

```css
@import './tokens.css';

* {
  box-sizing: border-box;
}

html {
  min-height: 100%;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

body {
  min-height: 100vh;
  margin: 0;
  color: var(--text);
  background: var(--mint-field);
}
```

- [ ] **Step 3: Implement `LoginPage.tsx`**

Create a component that renders:

```tsx
const slides = [
  {
    imagePath: '/static/images/konkit-field-1.jpg',
    alt: 'Foto lapangan penerima program Konkit',
    label: 'Serah Terima',
    caption: 'Paket konkit, nomor penerima, dan dokumen lapangan terhubung dalam satu alur kerja.',
  },
  {
    imagePath: '/static/images/konkit-field-2.jpeg',
    alt: 'Foto kegiatan program Konkit kabupaten',
    label: 'Program Kabupaten',
    caption: 'Setiap kegiatan kabupaten dapat dipantau tanpa memisahkan database program.',
  },
  {
    imagePath: '/static/images/konkit-training.jpeg',
    alt: 'Foto pelatihan teknis Konkit',
    label: 'Pelatihan Teknis',
    caption: 'Data penerima, pemasangan, dan BAST disiapkan untuk operasional yang lebih tertib.',
  },
];
```

Render the same visual structure as the current login page, but in React.

- [ ] **Step 4: Implement CSS Module**

Port the current `web/static/css/login.css` into `LoginPage.module.css`, converting class names to module references.

Ensure:

- `.loginShell` uses `min-height: 100dvh`.
- `.fieldPanel` uses `min-height: min(650px, calc(100dvh - 48px))`.
- `.ksmLogo` uses `width: min(190px, 42vw)` and `height: 34px`.
- `@media (max-width: 900px)` stacks form before carousel.
- `@media (prefers-reduced-motion: reduce)` disables carousel animation.

- [ ] **Step 5: Update `main.tsx`**

Use:

```tsx
import { createRoot } from 'react-dom/client';
import { LoginPage } from './pages/LoginPage/LoginPage';
import './styles/global.css';

const rootElement = document.getElementById('konkit-root');

if (!rootElement) {
  throw new Error('Missing #konkit-root element');
}

createRoot(rootElement).render(<LoginPage />);
```

- [ ] **Step 6: Build frontend**

Run:

```powershell
cd frontend
npm run build
```

Expected: Vite build succeeds and emits manifest plus assets.

- [ ] **Step 7: Run Go tests**

Run:

```powershell
$env:GOCACHE='D:\KSM\Deployment\konkit\.cache\go-build'; go test ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

Skip commit until the workspace is initialized as a git repository.

## Task 4: Local Server Verification

**Files:**
- Modify only if verification reveals a real issue.

**Interfaces:**
- Consumes: built frontend assets.
- Produces: working login page at `http://localhost:8080/login`.

- [ ] **Step 1: Build backend**

Run:

```powershell
$env:GOCACHE='D:\KSM\Deployment\konkit\.cache\go-build'; go build -o .\.tmp\konkit-server.exe .\cmd\server
```

Expected: exit code 0.

- [ ] **Step 2: Restart local server**

Run:

```powershell
Get-Process | Where-Object { $_.Path -eq 'D:\KSM\Deployment\konkit\.tmp\konkit-server.exe' } | Stop-Process
Start-Process -FilePath "D:\KSM\Deployment\konkit\.tmp\konkit-server.exe" -WorkingDirectory "D:\KSM\Deployment\konkit" -WindowStyle Hidden
```

Expected: process listens on port `8080`.

- [ ] **Step 3: Verify HTTP**

Run:

```powershell
Invoke-WebRequest -UseBasicParsing http://localhost:8080/login | Select-Object -ExpandProperty StatusCode
Invoke-WebRequest -UseBasicParsing http://localhost:8080/static/images/logo-ksm-transparent.png | Select-Object -ExpandProperty StatusCode
```

Expected: both return `200`.

- [ ] **Step 4: Manual browser check**

Open:

```text
http://localhost:8080/login
```

Expected:

- KSM logo appears larger but not tall.
- No black KSM logo background.
- No visible `Super Admin` text.
- Login fits one desktop viewport at normal zoom.
- Mobile layout shows the form first and carousel below.

- [ ] **Step 5: Commit**

Skip commit until the workspace is initialized as a git repository.

## Self-Review

Spec coverage:

- Go backend remains the server.
- React TypeScript custom UI is introduced.
- Static assets stay under `web/static/images`.
- Login UI remains the only feature in scope.

Placeholder scan:

- No `TBD` or `TODO` placeholders are present.
- Task 3 intentionally defers frontend unit test tooling and states the executable boundary for this local phase.

Type consistency:

- React mount id is consistently `konkit-root`.
- Vite output is consistently `web/static/app`.
- KSM transparent logo path is consistently `/static/images/logo-ksm-transparent.png`.

