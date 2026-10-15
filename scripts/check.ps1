param([switch]$POC)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$previous = @{}
foreach ($name in @('GOMODCACHE', 'GOCACHE', 'TEMP', 'TMP')) {
    $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}

Push-Location $root
try {
    $cache = Join-Path $root '.cache'
    foreach ($dir in @('mod', 'build', 'tmp')) {
        New-Item -ItemType Directory -Force -Path (Join-Path $cache $dir) | Out-Null
    }
    $env:GOMODCACHE = Join-Path $cache 'mod'
    $env:GOCACHE = Join-Path $cache 'build'
    $env:TEMP = Join-Path $cache 'tmp'
    $env:TMP = $env:TEMP

    $commands = @(go list ./cmd/...)
    if ($LASTEXITCODE -ne 0) { throw '실행 패키지 확인 실패' }
    $product = @($commands | Where-Object { $_ -notlike 'ggosu/cmd/poc/*' }) + @('./internal/...', './scenes', './songlist', './chartedit', './audio', './preferences')
    $dependencies = go list -deps -f '{{.ImportPath}}' @product
    if ($LASTEXITCODE -ne 0) { throw '제품 의존성 확인 실패' }
    if ($dependencies | Where-Object { $_ -like 'ggosu/cmd/poc/*' }) {
        throw '제품이 POC 코드를 참조합니다.'
    }

    $packages = $product
    if ($POC) { $packages += './cmd/poc/...' }
    go test @packages
    if ($LASTEXITCODE -ne 0) { throw 'Go 테스트 실패' }
    go vet @packages
    if ($LASTEXITCODE -ne 0) { throw 'Go vet 실패' }

    $ui = @('scenes', 'songlist', 'chartedit', 'audio', 'preferences')
    $tests = Get-ChildItem $ui -Recurse -Filter '*.test.js' | Where-Object { $_.FullName -notmatch '[\\/]wailsjs[\\/]' } | ForEach-Object { $_.FullName }
    node --test $tests
    if ($LASTEXITCODE -ne 0) { throw 'UI 테스트 실패' }
    $sources = @(Get-ChildItem $ui -Recurse -Filter '*.js' | Where-Object { $_.FullName -notmatch '[\\/]wailsjs[\\/]' }) + @(Get-ChildItem internal/server/web/static -Filter '*.js')
    foreach ($file in $sources) {
        node --check $file.FullName
        if ($LASTEXITCODE -ne 0) { throw "JS 문법 오류: $($file.Name)" }
    }
    Write-Host '검증 완료'
} finally {
    foreach ($name in $previous.Keys) {
        [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
    }
    Pop-Location
}
