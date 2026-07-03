# syntax=docker/dockerfile:1
#
# The builder runs on the native BUILDPLATFORM and cross-compiles for the
# requested TARGETARCH, so multi-arch images build without QEMU-emulating the Go
# toolchain. TARGETOS/TARGETARCH are supplied automatically by BuildKit (they
# default to the host platform for a plain `docker build`).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X github.com/piotrsenkow/mlsgrid-mcp/internal/cli.version=${VERSION}" \
    -o /out/mlsgrid-mcp ./cmd/mlsgrid-mcp

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 mlsgrid
USER mlsgrid
COPY --from=builder /out/mlsgrid-mcp /usr/local/bin/mlsgrid-mcp
ENTRYPOINT ["mlsgrid-mcp"]
