# Stage 1: build
FROM cgr.dev/chainguard/go:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /engram ./cmd/engram

# Stage 2: minimal runtime (no shell, no OS tools, CA certs only)
FROM cgr.dev/chainguard/static:latest
COPY --from=build /engram /engram
ENTRYPOINT ["/engram"]
CMD ["server"]
