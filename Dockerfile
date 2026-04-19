# Stage 1: build engram + starter
FROM cgr.dev/chainguard/go:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /engram ./cmd/engram
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /starter ./cmd/starter
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /engram-setup ./cmd/engram-setup

# Stage 2: minimal runtime — no shell, no OS tools, CA certs only
# starter (the ENTRYPOINT) authenticates to Infisical, injects ENGRAM_API_KEY,
# then exec-replaces itself with /engram. No secrets on disk.
FROM cgr.dev/chainguard/static:latest
COPY --from=build /engram /engram
COPY --from=build /starter /starter
COPY --from=build /engram-setup /engram-setup
USER nonroot
ENTRYPOINT ["/starter"]
CMD ["server"]
