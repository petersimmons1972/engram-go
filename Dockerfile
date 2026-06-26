# Stage 1: build engram + starter
# Pinned 2026-06-26: re-pin when Go minor version changes (crane digest cgr.dev/chainguard/go:latest)
FROM cgr.dev/chainguard/go:latest@sha256:faf3f70ddc6b4780f0506724bdd813f91511b253b76c4a2c6e94a01f99130219 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /engram ./cmd/engram
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /starter ./cmd/starter
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /engram-setup ./cmd/engram-setup

# Stage 2: minimal runtime — no shell, no OS tools, CA certs only
# starter (the ENTRYPOINT) authenticates to Infisical, injects ENGRAM_API_KEY,
# then exec-replaces itself with /engram. No secrets on disk.
# Pinned 2026-06-26: re-pin on Chainguard static updates (crane digest cgr.dev/chainguard/static:latest)
FROM cgr.dev/chainguard/static:latest@sha256:77d8b8925dc27970ec2f48243f44c7a260d52c49cd778288e4ee97566e0cb75b
COPY --from=build /engram /engram
COPY --from=build /starter /starter
COPY --from=build /engram-setup /engram-setup
USER nonroot
# Exec form required — cgr.dev/chainguard/static has no shell or wget.
# /engram --healthcheck probes its own /health endpoint and exits 0/1.
# Closes #341.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD ["/engram", "--healthcheck"]
ENTRYPOINT ["/starter"]
CMD ["server"]
