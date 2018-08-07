FROM golang:1.10-alpine3.7 as builder

RUN apk add --no-cache curl git
COPY . /go/src/github.com/anchorfree/s3sync
RUN curl https://glide.sh/get | sh && cd /go/src/github.com/anchorfree/s3sync  && glide install -v
RUN cd /go/src/github.com/anchorfree/s3sync && CGO_ENABLED=0 GOOS=linux go build -o /build/s3sync main.go


FROM alpine:3.7
LABEL maintainer="v.zorin@anchorfree.com"

RUN apk add --no-cache ca-certificates curl netcat-openbsd
COPY --from=builder /build/s3sync /
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
