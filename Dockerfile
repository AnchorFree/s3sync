FROM golang:1.21.0-alpine3.18 as builder

ENV GO111MODULE=on
# hadolint ignore=DL3018
RUN apk add --no-cache curl git
# hadolint ignore=DL3003
COPY . /cmd
# hadolint ignore=DL3003
RUN cd /cmd && go build

FROM alpine:3.18

# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates curl netcat-openbsd

COPY --from=builder /cmd/s3sync /s3sync
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
