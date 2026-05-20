FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /controller .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /controller /controller
ENTRYPOINT ["/controller"]
