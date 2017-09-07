#!/usr/bin/env sh

TIMEOUT=${TIMEOUT:-"300"}

if [ -z ${S3_PATH} ]; then
	echo "S3_PATH and LOCAL_PATH are mandatory"
	exit 1
fi

if [ -z ${LOCAL_PATH} ]; then
	echo "S3_PATH and LOCAL_PATH are mandatory"
	exit 1
fi

MATCH_REGEXP=$(cat /etc/af/array_name).*${MATCH_REGEXP}

while true; do
    /s3sync sync ${S3_PATH} ${LOCAL_PATH}
    sleep ${TIMEOUT} &
    wait
done
