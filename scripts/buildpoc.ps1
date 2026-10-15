$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    New-Item -ItemType Directory -Force -Path build/poc | Out-Null
    go build -o build/poc/inputpoc.exe ./cmd/poc/input
    if ($LASTEXITCODE -ne 0) { throw 'Input POC build failed' }
    go build -o build/poc/difficulty.exe ./cmd/poc/difficulty
    if ($LASTEXITCODE -ne 0) { throw 'Difficulty POC build failed' }
    go build -o build/poc/demo.exe ./cmd/poc/demo
    if ($LASTEXITCODE -ne 0) { throw 'Demo generator build failed' }
    go build -o build/poc/chat.exe ./cmd/poc/chat
    if ($LASTEXITCODE -ne 0) { throw 'Chat generator build failed' }
} finally {
    Pop-Location
}
