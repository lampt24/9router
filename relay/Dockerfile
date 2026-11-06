# syntax=docker/dockerfile:1.7
# 9r-relay-server: static Go binary on distroless, non-root.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/9r-relay-server ./cmd/9r-relay-server

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/lampt24/9router" \
      org.opencontainers.image.title="9r-relay-server"
COPY --from=build /out/9r-relay-server /9r-relay-server
USER 65532:65532
ENTRYPOINT ["/9r-relay-server"]
CMD ["serve"]
