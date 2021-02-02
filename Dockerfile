FROM golang:1.15.7 AS builder

ARG ARCH

COPY . /build

RUN cd /build && \
    CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -ldflags="-w -s" -o n4d .

# Real image
FROM scratch

COPY --from=builder /build/n4d /bin/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT [ "/bin/n4d" ]
