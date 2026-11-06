# 9router relay client

For machines whose network blocks direct access to `https://9r.lampt.works/v1`.

```
Codex/SDK ──http──► 127.0.0.1:20130 (cloudflared access tcp)
          ══ WebSocket over HTTPS :443 ══► Cloudflare Access (service token required)
          ──► tunnel 9router-vps2 ──► 127.0.0.1:20129 Caddy (only /v1, forces X-Forwarded-For)
          ──► 9router 127.0.0.1:20128 (API key required)
```

## Security model
- **Two independent credentials**: Cloudflare Access service token (blocks everything at the edge) + 9router API key.
- Use a **dedicated API key per machine** (dashboard → Endpoint → API keys) so it can be revoked alone.
- The relay origin only exposes `/v1/*`; dashboard and admin APIs return 404 even with valid credentials.
- The local listener binds `127.0.0.1` only — other LAN hosts cannot use it (any local user of that PC can, but still needs an API key).
- Credentials file is readable only by your user (`icacls` / `chmod 600`). Rotate the service token in Cloudflare Zero Trust → Access → Service credentials.
- Traffic between client and Cloudflare is TLS. If the corporate proxy does TLS inspection it can see payloads; that is the company's own control.

## Setup (Windows) — background task, no window
```powershell
winget install --id Cloudflare.cloudflared
.9r-relay.ps1 -Install        # self-elevates, asks Client ID + Secret once
.9r-relay.ps1 -Status         # task state, listener, last log lines
```
Registers Task Scheduler task **9router Relay**: runs as SYSTEM at boot (before logon), hidden, restarts on failure.
Script, a private copy of cloudflared.exe and the token live in `C:\ProgramData\9r-relay` (SYSTEM + Administrators only),
so nothing executed as SYSTEM is writable by a normal user. Other switches: `-Start -Stop -Restart -Uninstall [-Purge] -Install -ResetCredentials -Foreground`.
Re-run `-Install` after upgrading cloudflared or the script.

## Codex on that machine (`~/.codex/config.toml`)
```toml
model = "cx/gpt-6.1-sol"
model_provider = "9router"

[model_providers.9router]
name = "9Router (relay)"
base_url = "http://127.0.0.1:20130/v1"
wire_api = "responses"
http_headers = { Authorization = "Bearer <9router API key for this machine>" }
```

Test: `curl http://127.0.0.1:20130/v1/models -H "Authorization: Bearer <key>"` → 200.
