# Deploy: vps2 (Ubuntu 26.04, Tailscale 100.67.67.24)

```
                 Internet                                Tailnet
                    │                                       │
   Cloudflare (WAF rules) ── Tunnel ──┐       9r.lampt.works → 100.67.67.24 (split DNS → CoreDNS)
     only ^/v1(/|$) forwarded, rest 404│                       │
                                       ▼                       ▼
                               cloudflared ──► 127.0.0.1:20128 ◄── Caddy :443 (bind 100.67.67.24, LE cert via CF DNS-01)
                                                 9router
```

* Public: `https://9r.lampt.works/v1/*` only (API key required — the dashboard guard rejects non-loopback `/v1` calls without a key).
* Tailnet: everything (dashboard, `/v1`) on the same hostname.
* No inbound ports are opened on the public interface (ufw: only 22 + tailscale0).

## Files (live copy in `/opt/9router`)

| file | purpose |
|---|---|
| `docker-compose.yml` | 9router, caddy, coredns, cloudflared (all `network_mode: host`) |
| `.env` | secrets (`chmod 600`, not in git) — see `.env.example` |
| `Caddyfile`, `caddy.Dockerfile` | tailnet HTTPS entrypoint |
| `Corefile` | answers `9r.lampt.works A 100.67.67.24` for tailnet clients |
| `9router-update.{sh,service,timer}` | pull-based CD, every 10 min |

## CI/CD

1. `ci.yml` — every push/PR to `master`: Docker build + `/api/health` + "no key → 401" check.
2. Release: bump `package.json` + `cli/package.json`, tag `vX.Y.Z`, push tag.
   * `docker-publish.yml` → `ghcr.io/lampt24/9router:X.Y.Z` + `:latest` (amd64 + arm64).
   * `release.yml` → GitHub Release with the installable CLI tarball.
3. vps2 `9router-update.timer` pulls `:latest` within ~10 min and restarts if the digest changed.

Pin a version instead: `NINEROUTER_TAG=0.5.112` in `.env`, then `docker compose up -d 9router`.

## Ops

```bash
cd /opt/9router
docker compose ps
docker compose logs -f 9router
systemctl list-timers 9router-update.timer
journalctl -u 9router-update -n 50
```

## Hardening

| container | runs as | caps | other |
|---|---|---|---|
| 9router | `10020` (dedicated), root `su-exec` entrypoint bypassed | none | `no-new-privileges`, binds 127.0.0.1 only |
| caddy | `10010`, `cap_net_bind_service` file cap, no setuid files in image | `NET_BIND_SERVICE` only | `read_only`, binds Tailscale IP only |
| coredns | `65532` | `NET_BIND_SERVICE` only | `read_only` |
| cloudflared | nonroot | none | `no-new-privileges`, `read_only`, outbound only |

`no-new-privileges` is not set on caddy/coredns because it would block the file capability they need to bind :443/:53 on the host network namespace.
Each service only receives its own secrets (compose interpolation, no shared `env_file`).
Host: `/opt/9router/.env` root:600; `data*` owned by 10020, `caddy-*` by 10010, all 700.

## Relay & admin (added 2026-11-06)

| hostname | public (Cloudflare) | tailnet |
|---|---|---|
| `9r.lampt.works` | only `/v1` (API key) + Access email app on the rest | dashboard via Caddy |
| `gw.lampt.works` | TCP relay → Caddy `127.0.0.1:20129` (only `/v1`, forces XFF) — Access **service token** only | — |
| `manage.lampt.works` | only `/relay/*` static docs (Access email) → Caddy `127.0.0.1:20131` | Cockpit (`127.0.0.1:9090`) + `/relay/` |

* Cockpit: `cockpit-ws` socket bound to 127.0.0.1:9090 (`/etc/systemd/system/cockpit.socket.d/listen.conf`), `/etc/cockpit/cockpit.conf` Origins=https://manage.lampt.works. Login user `vpsadmin` (sudo, password in `/root/vpsadmin.pw`; SSH password auth is disabled so it only works in Cockpit).
* Plugin `cockpit-9router/` → `/usr/share/cockpit/9router/`. Static relay page `manage/` → `/opt/9router/manage` (mounted ro into Caddy).
* Tailscale split DNS: nameserver 100.67.67.24 restricted to `lampt.works`. CoreDNS answers tailnet-hosted names (hosts block in `Corefile`) and forwards the rest of the zone to 1.1.1.1. To add a tailnet-only service: add a line to the hosts block (+ AAAA template) and a Caddy site.
