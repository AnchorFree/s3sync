FROM scratch
ADD ./s3sync /usr/local/bin/s3
ENTRYPOINT ["s3", "sync"]
