FROM golang:1.21rc4-alpine3.18 as builder
# hadolint ignore=DL3003,SC1035,DL3019,DL3002,DL3018

ENV GO111MODULE=on
RUN apk add --no-cache curl git
COPY . /cmd
RUN cd /cmd && go build

FROM alpine:3.12
LABEL maintainer="v.zorin@anchorfree.com"

RUN apk add --no-cache ca-certificates curl netcat-openbsd
COPY --from=builder /cmd/docker-s3sync /s3sync
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
