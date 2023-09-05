FROM golang:1.21 AS builder
COPY . /go/src
WORKDIR /go/src
RUN ENABLE_CGO=0 go build -v -o rdb-autoresize

FROM alpine:3
RUN apk add libc6-compat
COPY --from=builder /go/src/rdb-autoresize /usr/local/bin/rdb-autoresize
CMD ["/usr/local/bin/rdb-autoresize"]
