<#
.SYNOPSIS
  9router relay client for Windows - runs in the background as a SYSTEM task.

.DESCRIPTION
  Opens http://127.0.0.1:<Port> on this machine and carries it as TCP-over-WebSocket
  through Cloudflare (HTTPS :443) to gw.lampt.works -> vps2 -> 9router /v1.

    Auth 1: Cloudflare Access service token (stored admin-only in C:\ProgramData\9r-relay)
    Auth 2: 9router API key (sent by your client, e.g. Codex)

  The relay is registered as the Task Scheduler task "9router Relay":
  runs as SYSTEM at boot, no window, restarts automatically, works before logon.

.EXAMPLE
  .\9r-relay.ps1 -Install          # first time: asks for Client ID / Secret, installs + starts
  .\9r-relay.ps1 -Status           # state, listener check, last log lines
  .\9r-relay.ps1 -Restart | -Stop | -Start
  .\9r-relay.ps1 -Install -ResetCredentials   # replace the service token
  .\9r-relay.ps1 -Uninstall [-Purge]          # remove task (and credentials with -Purge)
  .\9r-relay.ps1 -Foreground       # run in this console (testing), Ctrl+C to stop

  Requires cloudflared:  winget install --id Cloudflare.cloudflared
#>
[CmdletBinding()]
param(
  [switch]$Install,
  [switch]$Uninstall,
  [switch]$Purge,
  [switch]$Status,
  [switch]$Start,
  [switch]$Stop,
  [switch]$Restart,
  [switch]$Foreground,
  [switch]$ResetCredentials,
  [switch]$Run,                      # internal: background loop used by the task
  [int]$Port = 20130,
  [string]$Hostname = "gw.lampt.works"
)

$ErrorActionPreference = "Stop"
$TaskName  = "9router Relay"
$AppDir    = Join-Path $env:ProgramData "9r-relay"
$BinDir    = Join-Path $AppDir "bin"
$CredFile  = Join-Path $AppDir "credentials.json"
$ConfFile  = Join-Path $AppDir "config.json"
$LogFile   = Join-Path $AppDir "relay.log"
$ScriptDst = Join-Path $AppDir "9r-relay.ps1"
$SID_SYSTEM = "*S-1-5-18"       # NT AUTHORITY\SYSTEM (locale independent)
$SID_ADMINS = "*S-1-5-32-544"   # BUILTIN\Administrators

function Test-Admin {
  $p = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
  return $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Invoke-Elevated {
  # Re-launch this script elevated with the same switches, and wait for it.
  $a = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "`"$PSCommandPath`"")
  foreach ($k in $PSBoundParameters.Keys) {
    $v = $PSBoundParameters[$k]
    if ($v -is [switch]) { if ($v.IsPresent) { $a += "-$k" } } else { $a += "-$k"; $a += "`"$v`"" }
  }
  Write-Host "Administrator rights required - opening an elevated window..."
  $p = Start-Process -FilePath "powershell.exe" -Verb RunAs -ArgumentList $a -Wait -PassThru
  exit $p.ExitCode
}

function Protect-Path([string]$Path, [switch]$Dir) {
  # Only SYSTEM and Administrators: a non-admin user must not be able to read the
  # token nor replace anything that runs as SYSTEM (privilege escalation).
  if ($Dir) {
    icacls $Path /inheritance:r /grant:r "$($SID_SYSTEM):(OI)(CI)F" "$($SID_ADMINS):(OI)(CI)F" | Out-Null
  } else {
    icacls $Path /inheritance:r /grant:r "$($SID_SYSTEM):F" "$($SID_ADMINS):F" | Out-Null
  }
}

function Find-Cloudflared {
  $candidates = @(
    (Join-Path ${env:ProgramFiles(x86)} "cloudflared\cloudflared.exe"),
    (Join-Path $env:ProgramFiles "cloudflared\cloudflared.exe")
  )
  $cmd = Get-Command cloudflared -ErrorAction SilentlyContinue
  if ($cmd) { $candidates += $cmd.Source }
  foreach ($c in $candidates) { if ($c -and (Test-Path $c)) { return (Resolve-Path $c).Path } }
  return $null
}

function Get-RelayProcesses {
  Get-CimInstance Win32_Process -Filter "Name = 'cloudflared.exe'" |
    Where-Object { $_.CommandLine -like "*access tcp*" -and $_.CommandLine -like "*127.0.0.1:*" -and $_.ExecutablePath -like "$BinDir*" }
}

function Stop-Relay {
  if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) { Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue }
  # Stopping the task ends powershell.exe, not necessarily its child cloudflared.
  Get-RelayProcesses | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

function Test-Listener([int]$P) {
  $c = New-Object Net.Sockets.TcpClient
  try { $iar = $c.BeginConnect("127.0.0.1", $P, $null, $null); return ($iar.AsyncWaitHandle.WaitOne(1500) -and $c.Connected) }
  catch { return $false } finally { $c.Close() }
}

function Read-Config {
  if (-not (Test-Path $ConfFile) -or -not (Test-Path $CredFile)) { throw "Not installed. Run: .\9r-relay.ps1 -Install" }
  $conf = Get-Content $ConfFile -Raw | ConvertFrom-Json
  $cred = Get-Content $CredFile -Raw | ConvertFrom-Json
  return @{ conf = $conf; cred = $cred }
}

function Start-RelayProcess($cfg, [switch]$Console) {
  $env:TUNNEL_SERVICE_TOKEN_ID     = $cfg.cred.id
  $env:TUNNEL_SERVICE_TOKEN_SECRET = $cfg.cred.secret
  $a = @("access", "tcp", "--hostname", $cfg.conf.hostname, "--url", "127.0.0.1:$($cfg.conf.port)", "--loglevel", "info")
  if ($Console) { & $cfg.conf.cloudflared @a; return }
  if ((Test-Path $LogFile) -and ((Get-Item $LogFile).Length -gt 5MB)) { Move-Item $LogFile "$LogFile.1" -Force }
  & $cfg.conf.cloudflared @a --logfile $LogFile *> $null
}

# ---------------------------------------------------------------- internal loop
if ($Run) {
  $cfg = Read-Config
  while ($true) {
    try { Start-RelayProcess $cfg } catch { Add-Content $LogFile "$(Get-Date -Format s) relay error: $_" }
    Start-Sleep -Seconds 5      # cloudflared exited (network drop, etc.) -> restart
  }
}

# ---------------------------------------------------------------- foreground
if ($Foreground) {
  if (Test-Path $ConfFile) { $cfg = Read-Config }
  else {
    $cf = Find-Cloudflared; if (-not $cf) { throw "cloudflared not found. Install: winget install --id Cloudflare.cloudflared" }
    $id = Read-Host "CF-Access-Client-Id"
    $sec = Read-Host "CF-Access-Client-Secret" -AsSecureString
    $cfg = @{ conf = @{ cloudflared = $cf; hostname = $Hostname; port = $Port }
              cred = @{ id = $id; secret = [Runtime.InteropServices.Marshal]::PtrToStringBSTR([Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec)) } }
  }
  Write-Host "Relay (foreground): http://127.0.0.1:$($cfg.conf.port)/v1 -> $($cfg.conf.hostname)   Ctrl+C to stop"
  Start-RelayProcess $cfg -Console
  exit 0
}

# ---------------------------------------------------------------- status (no admin needed for the basics)
if ($Status -or -not ($Install -or $Uninstall -or $Start -or $Stop -or $Restart)) {
  if (-not $Status) { Get-Help $PSCommandPath -Detailed | Out-Host; exit 0 }
  $t = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
  if (-not $t) { Write-Host "Task '$TaskName': not installed"; exit 1 }
  $i = Get-ScheduledTaskInfo -TaskName $TaskName
  $p = if (Test-Path $ConfFile) { (Get-Content $ConfFile -Raw | ConvertFrom-Json).port } else { $Port }
  Write-Host ("Task      : {0} ({1})" -f $TaskName, $t.State)
  Write-Host ("Last run  : {0}  result=0x{1:X}" -f $i.LastRunTime, $i.LastTaskResult)
  Write-Host ("Listener  : 127.0.0.1:{0} {1}" -f $p, $(if (Test-Listener $p) { "OPEN" } else { "CLOSED" }))
  Write-Host ("Base URL  : http://127.0.0.1:{0}/v1" -f $p)
  if ((Test-Admin) -and (Test-Path $LogFile)) { Write-Host "--- last log lines"; Get-Content $LogFile -Tail 8 }
  exit 0
}

if (-not (Test-Admin)) { Invoke-Elevated }

# ---------------------------------------------------------------- start / stop / restart
if ($Stop -or $Restart) { Stop-Relay; Write-Host "Stopped." }
if ($Start -or $Restart) { Start-ScheduledTask -TaskName $TaskName; Write-Host "Started." }
if ($Start -or $Stop -or $Restart) { exit 0 }

# ---------------------------------------------------------------- uninstall
if ($Uninstall) {
  Stop-Relay
  if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) { Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false }
  if ($Purge) { Remove-Item $AppDir -Recurse -Force -ErrorAction SilentlyContinue; Write-Host "Removed task and $AppDir (credentials deleted)." }
  else { Write-Host "Removed task. Credentials kept in $AppDir (use -Purge to delete)." }
  exit 0
}

# ---------------------------------------------------------------- install
if ($Install) {
  $src = Find-Cloudflared
  if (-not $src) { throw "cloudflared not found. Install it first: winget install --id Cloudflare.cloudflared" }

  Stop-Relay
  New-Item -ItemType Directory -Force $AppDir, $BinDir | Out-Null
  Protect-Path $AppDir -Dir

  # Private copies of the binary and this script in the admin-only folder: the task
  # (SYSTEM) never executes anything a normal user can modify.
  Copy-Item $src (Join-Path $BinDir "cloudflared.exe") -Force
  if ($PSCommandPath -ne $ScriptDst) { Copy-Item $PSCommandPath $ScriptDst -Force }

  if ($ResetCredentials -or -not (Test-Path $CredFile)) {
    $id  = Read-Host "CF-Access-Client-Id"
    $sec = Read-Host "CF-Access-Client-Secret" -AsSecureString
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR([Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec))
    if (-not $id -or -not $plain) { throw "Client ID and Secret are required." }
    @{ id = $id.Trim(); secret = $plain.Trim() } | ConvertTo-Json | Set-Content -Encoding ASCII $CredFile
    Protect-Path $CredFile
  }
  @{ cloudflared = (Join-Path $BinDir "cloudflared.exe"); hostname = $Hostname; port = $Port } | ConvertTo-Json | Set-Content -Encoding ASCII $ConfFile
  Protect-Path $ConfFile

  $action    = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$ScriptDst`" -Run"
  $trigger   = New-ScheduledTaskTrigger -AtStartup
  $principal = New-ScheduledTaskPrincipal -UserId "NT AUTHORITY\SYSTEM" -LogonType ServiceAccount
  $settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable `
                 -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -MultipleInstances IgnoreNew
  Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings `
    -Description "9router relay: http://127.0.0.1:$Port/v1 -> $Hostname (Cloudflare Access service token)" -Force | Out-Null
  Start-ScheduledTask -TaskName $TaskName

  $ok = $false; for ($i = 0; $i -lt 15 -and -not $ok; $i++) { Start-Sleep 1; $ok = Test-Listener $Port }
  Write-Host ""
  Write-Host "Installed task '$TaskName' (SYSTEM, at boot, hidden, auto-restart)."
  Write-Host ("Listener 127.0.0.1:{0}: {1}" -f $Port, $(if ($ok) { "OPEN" } else { "not open yet - check: .\9r-relay.ps1 -Status" }))
  Write-Host "Base URL for clients: http://127.0.0.1:$Port/v1"
  Write-Host "Logs: $LogFile (admin only)"
  Read-Host "Press Enter to close" | Out-Null
}
