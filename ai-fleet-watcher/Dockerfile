# Multi-arch: build on host arch, cross-compile for target.
# docker buildx build --platform linux/amd64,linux/arm64 -t ... .
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /watcher .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /watcher /watcher
ENTRYPOINT ["/watcher"]
