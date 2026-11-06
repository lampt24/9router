# 9r-relay

Encrypted relay that lets a machine on a restrictive network use 9router's `/v1`
API on vps2. Design modelled on Orca's mobile relay (outbound WebSockets on both
sides, pairing offer with a pinned server key, per-device credentials, end-to-end
encryption the relay path cannot read).

```
Codex/SDK ─http─► 9r-relay (127.0.0.1:20130, Windows service)
   ═ wss://gw.lampt.works/v1/tunnel (HTTPS 443) ═► Cloudflare Access (service token)
   ─► cloudflared tunnel ─► 9r-relay-server (127.0.0.1:20140)
   ─► Caddy 127.0.0.1:20129 (only /v1, forces X-Forwarded-For) ─► 9router (API key)
```

## Protocol (v1)

* One WebSocket per relayed TCP connection; binary messages, compression disabled
  (compression + encryption leaks length information), 64 KiB message limit.
* `Noise_IK_25519_ChaChaPoly_SHA256`, prologue `9r-relay/v1` ([flynn/noise](https://github.com/flynn/noise)).
  * The client pins the server static key from the pairing code — a relay or
    TLS-inspecting proxy that substitutes keys fails the handshake.
  * The device static key is the credential. It is generated on the device, never
    leaves it, and travels encrypted inside handshake message 1 (identity hidden).
  * Ephemeral-ephemeral DH gives forward secrecy per tunnel.
* After the handshake each message is one AEAD chunk (≤ 32 KiB plaintext);
  nonces are implicit counters, so reordering/replay breaks the session.
* Pairing: `pair-new` stores `sha256(invite)` with a ≤ 10 minute expiry. The code
  (`9r1.<base64url json>`) carries relay URL, server key, invite and the Cloudflare
  Access service token. The invite rides inside the encrypted hello and is single use;
  a revoked key can never re-pair.
* Server hardening: loopback listener behind cloudflared, per-IP failed-handshake
  limit (20 / 5 min → 429), 256 global / 64 per-device tunnels, 15 s handshake
  deadline, live sessions re-check revocation every 30 s, 25 s pings keep tunnels
  alive through Cloudflare. Runs as uid 65532, read-only rootfs, no capabilities.

## Operate

```bash
docker exec relay-server /9r-relay-server pair-new --name company-pc   # 10-minute single-use code
docker exec relay-server /9r-relay-server devices
docker exec relay-server /9r-relay-server revoke company-pc
```
Cockpit (manage.lampt.works → 9router & Relay) has the same actions.

Client (Windows, elevated PowerShell):
```powershell
.\9r-relay-windows-amd64.exe install 9r1.<code> [--proxy http://proxy:8080]
& "C:\Program Files\9r-relay\9r-relay.exe" status
& "C:\Program Files\9r-relay\9r-relay.exe" test      # expects 401 from 9router = path OK
```
Linux/macOS: `9r-relay pair <code>` then `9r-relay run` (e.g. as a systemd user unit).

## Release

`git tag relay-vX.Y.Z && git push origin relay-vX.Y.Z` → `.github/workflows/relay.yml`
tests, publishes client binaries + `SHA256SUMS.txt` to a GitHub Release and the server
image `ghcr.io/lampt24/9r-relay-server:{X.Y.Z,latest}`. vps2's update timer pulls the
image and mirrors the Windows binaries to `manage.lampt.works/relay/download/`.

## Develop

```bash
cd relay && go test ./... && go vet ./... && GOOS=windows go vet ./cmd/9r-relay
```
