$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
$oldOS, $oldArch = $env:GOOS, $env:GOARCH
try {
    $env:GOOS = 'js'
    $env:GOARCH = 'wasm'
    New-Item -ItemType Directory -Force build/web | Out-Null
    go build -o build/web/game.wasm ./cmd/game
    if ($LASTEXITCODE -ne 0) { throw 'WebAssembly build failed' }
    $goRoot = go env GOROOT
    Copy-Item -LiteralPath (Join-Path $goRoot 'lib/wasm/wasm_exec.js') -Destination build/web/wasm_exec.js -Force
    New-Item -ItemType Directory -Force build/bin/hud | Out-Null
    Copy-Item -LiteralPath build/web/game.wasm,build/web/wasm_exec.js -Destination build/bin/hud -Force
    # The HTTP server embeds the shared menu HTML/CSS/JS. WASM alone cannot refresh it.
    $env:GOOS, $env:GOARCH = $oldOS, $oldArch
    go build -o build/bin/web.exe ./cmd/web
    if ($LASTEXITCODE -ne 0) { throw 'Web server build failed' }
} finally {
    $env:GOOS, $env:GOARCH = $oldOS, $oldArch
    Pop-Location
}
