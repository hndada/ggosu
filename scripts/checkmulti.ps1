param([int]$Port = 18090)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$out = Join-Path $root 'artifacts\11-multiplayer'
New-Item -ItemType Directory -Force -Path $out | Out-Null
$serverExe = Join-Path $root 'build\bin\multi.exe'
$gameExe = Join-Path $root 'build\bin\game.exe'
foreach ($path in @($serverExe, $gameExe)) {
    if (!(Test-Path -LiteralPath $path)) { throw "Build first: $path" }
}

function Wait-Until([scriptblock]$Condition, [int]$Seconds, [string]$Message) {
    $timer = [Diagnostics.Stopwatch]::StartNew()
    while ($timer.Elapsed.TotalSeconds -lt $Seconds) {
        if (& $Condition) { return }
        Start-Sleep -Milliseconds 100
    }
    throw $Message
}
function Start-Game([string]$Name, [string[]]$Extra) {
    $gameArgs = @('-chart', (Join-Path $root 'songs\demo\demo.osu'), '-auto', '-mute', '-duration', '25s',
        '-multi', "ws://127.0.0.1:$Port/ws", '-name', $Name,
        '-multi-report', (Join-Path $out "$Name.json"), '-report', (Join-Path $out "$Name-score.json"),
        '-capture', (Join-Path $out "$Name.png")) + $Extra
    # Start-Process uses the Windows command line; quote each complete argument.
    $quoted = $gameArgs | ForEach-Object { '"' + $_.Replace('"', '\"') + '"' }
    return Start-Process -FilePath $gameExe -WorkingDirectory $root -ArgumentList $quoted -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $out "$Name.stdout") -RedirectStandardError (Join-Path $out "$Name.stderr")
}

$server = $hostGame = $guestGame = $null
try {
    $server = Start-Process -FilePath $serverExe -WorkingDirectory $root -ArgumentList @('-addr', "127.0.0.1:$Port") `
        -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $out 'server.stdout') -RedirectStandardError (Join-Path $out 'server.stderr')
    Wait-Until { try { (Invoke-RestMethod "http://127.0.0.1:$Port/health").version -eq 4 } catch { $false } } 5 'Server did not start'
    $hostGame = Start-Game -Name 'host' -Extra @('-multi-start', '-room-name', 'native sync verification')
    Wait-Until { $rooms = Invoke-RestMethod "http://127.0.0.1:$Port/api/rooms"; $rooms.Count -gt 0 } 10 'Native host did not create a room'
    $room = (Invoke-RestMethod "http://127.0.0.1:$Port/api/rooms")[0]
    if ([string]::IsNullOrEmpty($room.id)) { throw 'Room ID missing from server response' }
    $guestGame = Start-Game -Name 'guest' -Extra @('-room', $room.id, '-offset', '80')
    Wait-Until { $hostGame.Refresh(); $guestGame.Refresh(); $hostGame.HasExited -and $guestGame.HasExited } 30 'Native clients did not finish'
    if ($hostGame.ExitCode -ne 0 -or $guestGame.ExitCode -ne 0) {
        Get-Content (Join-Path $out 'host.stderr'), (Join-Path $out 'guest.stderr')
        throw "Native clients failed: host=$($hostGame.ExitCode) guest=$($guestGame.ExitCode)"
    }
    $a = Get-Content -Raw (Join-Path $out 'host.json') | ConvertFrom-Json
    $b = Get-Content -Raw (Join-Path $out 'guest.json') | ConvertFrom-Json
    $as = Get-Content -Raw (Join-Path $out 'host-score.json') | ConvertFrom-Json
    $bs = Get-Content -Raw (Join-Path $out 'guest-score.json') | ConvertFrom-Json
    if ($a.match_id -ne $b.match_id -or $a.server_start_ns -ne $b.server_start_ns -or !$a.final_ack -or !$b.final_ack) { throw 'Reservation/result acknowledgement mismatch' }
    if ($as.Score.Points -ne 1100000 -or $bs.Score.Points -ne 1100000) { throw 'Unexpected native autoplay result' }
    foreach ($trace in @($a, $b)) {
        if ($trace.start_lag_ns -lt 0 -or $trace.start_lag_ns -gt 100000000) { throw 'Native start missed its deadline' }
        if (@($trace.comparisons).Count -lt 10) { throw 'No useful real-time comparisons recorded' }
        if (@($trace.comparisons | Where-Object { $_.difference -ne 0 }).Count -gt 0) { throw 'Same-time autoplay comparisons differed' }
    }
    # Initial zero watermarks differ under personal offset. Score changes must
    # have identical canonical timestamps and values.
    $aChanges = @($a.points | Where-Object { $_.score.Total -gt 0 })
    $bChanges = @($b.points | Where-Object { $_.score.Total -gt 0 })
    if (($aChanges | ConvertTo-Json -Depth 8 -Compress) -ne ($bChanges | ConvertTo-Json -Depth 8 -Compress)) { throw 'Canonical score changes differ under personal offset' }
    [PSCustomObject]@{ Match = $a.match_id; HostStartLagMS = $a.start_lag_ns / 1e6; GuestStartLagMS = $b.start_lag_ns / 1e6;
        HostRTTMS = $a.rtt_ns / 1e6; GuestRTTMS = $b.rtt_ns / 1e6; HostComparisons = @($a.comparisons).Count;
        GuestComparisons = @($b.comparisons).Count; Score = $as.Score.Points; Artifacts = $out } | Format-List
} catch {
    Get-Content (Join-Path $out 'host.stderr'), (Join-Path $out 'guest.stderr') -ErrorAction SilentlyContinue
    throw
} finally {
    foreach ($process in @($hostGame, $guestGame, $server)) {
        if ($null -ne $process) {
            $process.Refresh()
            if (!$process.HasExited) { Stop-Process -Id $process.Id }
            $process.Dispose()
        }
    }
}
