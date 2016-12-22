#!/bin/sh

SLEEP_TIME=${SLEEP_TIME:-300}

while true; 
do
    /usr/local/bin/s3 sync --check-md5 "$@"
    sleep ${SLEEP_TIME}
done
