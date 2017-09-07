FROM golang:1.8

COPY . /go/src/github.com/anchorfree/s3sync
RUN curl https://glide.sh/get | sh \
    && cd /go/src/github.com/anchorfree/s3sync \
    && glide install -v
RUN cd /go/src/github.com/anchorfree/s3sync \
    && CGO_ENABLED=0 GOOS=linux go build -o /build/s3sync main.go


FROM alpine
RUN apk add --update-cache --no-cache ca-certificates curl
COPY --from=0 /build/s3sync /
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
