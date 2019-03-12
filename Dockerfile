FROM golang:1.12-alpine3.9 as builder

ENV GO111MODULE=on
RUN apk add --no-cache curl git
COPY . /go/src/github.com/anchorfree/s3sync
RUN cd /go/src/github.com/anchorfree/s3sync && go mod download && CGO_ENABLED=0 GOOS=linux go build -o /build/s3sync .


FROM alpine:3.9
LABEL maintainer="v.zorin@anchorfree.com"

RUN apk add --no-cache ca-certificates curl netcat-openbsd
COPY --from=builder /build/s3sync /
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
