FROM golang:1.25.7-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/douyinLive ./cmd/main

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /out/douyinLive /app/douyinLive
COPY config.example.yaml /app/config.example.yaml

EXPOSE 1088

ENTRYPOINT ["/app/douyinLive"]
