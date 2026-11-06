#!/usr/bin/env bash
# 9router relay client (Linux/macOS). See README.md.
#   first run:  ./9r-relay.sh setup
#   run:        ./9r-relay.sh            (PORT=20130 HOSTNAME_RELAY=gw.lampt.works)
set -euo pipefail
PORT="${PORT:-20130}"; HOST="${HOSTNAME_RELAY:-gw.lampt.works}"
CRED="$HOME/.9r-relay/credentials"
if [[ "${1:-}" == setup || ! -f "$CRED" ]]; then
  mkdir -p "$HOME/.9r-relay"; chmod 700 "$HOME/.9r-relay"
  read -rp  "CF-Access-Client-Id: " id
  read -rsp "CF-Access-Client-Secret: " secret; echo
  umask 077; printf 'TUNNEL_SERVICE_TOKEN_ID=%s\nTUNNEL_SERVICE_TOKEN_SECRET=%s\n' "$id" "$secret" > "$CRED"
  echo "saved $CRED"
fi
set -a; . "$CRED"; set +a
echo "Relay: http://127.0.0.1:$PORT/v1 -> $HOST"
exec cloudflared access tcp --hostname "$HOST" --url "127.0.0.1:$PORT" --loglevel warn
