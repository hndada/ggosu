param(
    [ValidateSet('build', 'dev')][string]$Mode = 'build',
    [string[]]$WailsArgs = @()
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$wails = Join-Path $root '.cache/wails.exe'
if (-not (Test-Path -LiteralPath $wails)) { throw 'Wails CLI가 없습니다. docs/dev/structure.md의 설치 안내를 확인하세요.' }

Push-Location (Join-Path $root 'cmd/desktop')
try {
    & $wails $Mode @WailsArgs
    if ($LASTEXITCODE -ne 0) { throw "Wails $Mode 실패" }
} finally {
    Pop-Location
}
if ($Mode -eq 'build') { & (Join-Path $root 'scripts/buildweb.ps1') }
