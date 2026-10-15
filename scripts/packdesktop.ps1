$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $repoRoot 'build/bin'
# Only the demo chart ships: other folders in songs are local libraries whose music we may not redistribute.
$files = @(
    @{ From = Join-Path $bin 'gosu.exe'; To = 'gosu.exe' },
    @{ From = Join-Path $bin 'game.exe'; To = 'game.exe' },
    @{ From = Join-Path $bin 'songs/demo'; To = 'songs/demo' },
    @{ From = Join-Path $bin 'hud'; To = 'hud' },
    @{ From = Join-Path $repoRoot 'LICENSE'; To = 'LICENSE' },
    @{ From = Join-Path $repoRoot 'THIRD_PARTY_NOTICES.txt'; To = 'THIRD_PARTY_NOTICES.txt' }
)
foreach ($file in $files) {
    if (-not (Test-Path -LiteralPath $file.From)) { throw "Desktop package input missing: $($file.From)" }
}
$stage = Join-Path ([IO.Path]::GetTempPath()) ("gosu-pack-" + [guid]::NewGuid())
try {
    foreach ($file in $files) {
        $target = Join-Path $stage $file.To
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
        Copy-Item -LiteralPath $file.From -Destination $target -Recurse
    }
    $destination = Join-Path $repoRoot 'build/web/downloads'
    New-Item -ItemType Directory -Force -Path $destination | Out-Null
    $zip = Join-Path $destination 'gosu-windows.zip'
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip -Force
    Write-Output $zip
} finally {
    Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
}
