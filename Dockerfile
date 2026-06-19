FROM golang:1.26.3-alpine3.22 AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG BUILD_TAG=dev
ARG BUILD_COMMIT=unknown
ARG BUILD_DATE=unknown
ARG BUILD_SOURCE=local
ARG DEFAULT_SIGN_PROVIDER=local

WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w -X main.buildTag=${BUILD_TAG} -X main.buildCommit=${BUILD_COMMIT} -X main.buildDate=${BUILD_DATE} -X main.buildSource=${BUILD_SOURCE} -X main.defaultSignProvider=${DEFAULT_SIGN_PROVIDER}" -o /out/douyinLive ./cmd/main

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /out/douyinLive /app/douyinLive
COPY config.example.yaml /app/config.example.yaml

EXPOSE 1088

ENTRYPOINT ["/app/douyinLive"]
