FROM golang:1.8

COPY . /go/src/github.com/anchorfree/s3sync
RUN curl https://glide.sh/get | sh \
    && cd /go/src/github.com/anchorfree/s3sync \
    && glide install -v
RUN cd /go/src/github.com/anchorfree/s3sync \
    && CGO_ENABLED=0 go build -o /build/s3sync main.go


FROM alpine
COPY --from=0 /build/s3sync /

ENTRYPOINT ["/s3sync", "sync"]
