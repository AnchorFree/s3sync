FROM alpine
RUN apk add --update ca-certificates # Certificates for SSL \
        && update-ca-certificates \
        && apk add openssl
ADD ./s3sync /usr/local/bin/s3
ADD ./entrypoint.sh /usr/local/bin/entrypoint.sh
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
