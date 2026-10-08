# Local development with auto-reload:
#   - Vite rebuilds web/static/app whenever frontend/src changes (refresh the browser to see it).
#   - air re-runs migrations, rebuilds, and restarts the Go server whenever Go/SQL/templates change.
# Usage (from the project root): powershell -ExecutionPolicy Bypass -File scripts/dev.ps1
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$air = Get-Command air -ErrorAction SilentlyContinue
$airPath = if ($air) { $air.Source } else { Join-Path (go env GOPATH) 'bin\air.exe' }
if (-not (Test-Path $airPath)) {
    throw "air tidak ditemukan. Install dengan: go install github.com/air-verse/air@latest"
}
if (-not (Test-Path 'frontend\node_modules')) {
    npm.cmd --prefix frontend ci
}

# One full build first so the server never starts without a bundle.
npm.cmd --prefix frontend run build
$vite = Start-Process -FilePath 'npm.cmd' -ArgumentList '--prefix', 'frontend', 'run', 'build', '--', '--watch' -NoNewWindow -PassThru

try {
    & $airPath -c .air.toml
}
finally {
    if ($vite -and -not $vite.HasExited) {
        # npm starts node as a child process; stop the whole tree.
        taskkill /PID $vite.Id /T /F | Out-Null
    }
}
