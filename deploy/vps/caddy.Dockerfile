FROM caddy:2.10-builder AS builder
RUN xcaddy build --with github.com/caddy-dns/cloudflare

FROM caddy:2.10
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
# Non-root user; file capability lets it bind :443 on the host network.
# Strip setuid/setgid bits so nothing in the image can regain root.
RUN apk add --no-cache libcap \
 && setcap cap_net_bind_service=+ep /usr/bin/caddy \
 && apk del libcap \
 && find / -xdev -perm /6000 -type f -exec chmod a-s {} + \
 && addgroup -S -g 10010 caddy && adduser -S -D -H -u 10010 -G caddy caddy
USER 10010:10010
