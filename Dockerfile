FROM golang:1.25.7-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/douyinLive ./cmd/main

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app

COPY --from=builder /out/douyinLive /app/douyinLive
COPY config.example.yaml /app/config.example.yaml

EXPOSE 1088

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 -O - http://127.0.0.1:1088/health || exit 1

ENTRYPOINT ["/app/douyinLive"]
