<#
  9router relay client (Windows).
  Opens http://127.0.0.1:<Port> locally and carries it as TCP-over-WebSocket
  through Cloudflare (HTTPS :443) to relay.lampt.works -> vps2 -> 9router /v1.

  Auth layer 1: Cloudflare Access service token (stored in %USERPROFILE%\.9r-relay\credentials.json)
  Auth layer 2: 9router API key (sent by your client, e.g. Codex)

  First run:   .\9r-relay.ps1 -Setup        (asks for Client ID / Secret, locks file to your user)
  Run:         .\9r-relay.ps1               (Ctrl+C to stop)
  Requires:    cloudflared  ->  winget install --id Cloudflare.cloudflared
#>
param(
  [int]$Port = 20130,
  [string]$Hostname = "relay.lampt.works",
  [switch]$Setup
)
$ErrorActionPreference = "Stop"
$dir  = Join-Path $env:USERPROFILE ".9r-relay"
$cred = Join-Path $dir "credentials.json"

if ($Setup -or -not (Test-Path $cred)) {
  New-Item -ItemType Directory -Force $dir | Out-Null
  $id     = Read-Host "CF-Access-Client-Id"
  $secret = Read-Host "CF-Access-Client-Secret" -AsSecureString
  $plain  = [Runtime.InteropServices.Marshal]::PtrToStringBSTR([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret))
  @{ id = $id; secret = $plain } | ConvertTo-Json | Set-Content -Encoding utf8 $cred
  # Only the current user may read the credentials file.
  icacls $cred /inheritance:r /grant:r "$($env:USERNAME):(R,W)" | Out-Null
  Write-Host "Saved $cred"
}

$c = Get-Content $cred -Raw | ConvertFrom-Json
$cf = (Get-Command cloudflared -ErrorAction SilentlyContinue).Source
if (-not $cf) { throw "cloudflared not found. Install: winget install --id Cloudflare.cloudflared" }

Write-Host "Relay: http://127.0.0.1:$Port/v1  ->  $Hostname  (Ctrl+C to stop)"
# Listener is bound to loopback only; other machines on the LAN cannot use it.
$env:TUNNEL_SERVICE_TOKEN_ID     = $c.id
$env:TUNNEL_SERVICE_TOKEN_SECRET = $c.secret
& $cf access tcp --hostname $Hostname --url "127.0.0.1:$Port" --loglevel warn
