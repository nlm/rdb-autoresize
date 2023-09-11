FROM golang:1.21 AS builder
ARG SCW_RDB_AUTORESIZE_VERSION=dev
ENV SCW_RDB_AUTORESIZE_VERSION=$SCW_RDB_AUTORESIZE_VERSION
COPY . /go/src
WORKDIR /go/src
RUN --mount=type=cache,target=/root/.cache/go-build ENABLE_CGO=0 go build -v -ldflags "-X main.appVersion=${SCW_RDB_AUTORESIZE_VERSION}" -o rdb-autoresize

FROM alpine:3
RUN apk add libc6-compat
COPY --from=builder /go/src/rdb-autoresize /usr/local/bin/rdb-autoresize
CMD ["/usr/local/bin/rdb-autoresize", "-log-json"]
