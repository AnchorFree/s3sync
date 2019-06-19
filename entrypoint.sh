#!/usr/bin/env sh

TIMEOUT=${TIMEOUT:-"300"}

if [ -z ${S3_PATH} ]; then
        echo "S3_PATH and LOCAL_PATH are mandatory"
        exit 1
fi

if [ -z ${CREATE_K8S_SECRET} ] || [ ${CREATE_K8S_SECRET} != "true" ]; then
        if [ -z ${LOCAL_PATH} ]; then
                echo "S3_PATH and LOCAL_PATH are mandatory if CREATE_K8S_SECRET is not defined"
                exit 1
        fi
fi

AWS_REGION=${AWS_REGION:-'us-east-1'}

if [ -z "${MATCH_REGEXP_OVERRIDE}" ]; then
    MATCH_REGEXP=$(cat /etc/af/array_name).*${MATCH_REGEXP}
else
    MATCH_REGEXP="${MATCH_REGEXP_OVERRIDE}"
fi

export MATCH_REGEXP AWS_REGION

ALIVE=1
trap ALIVE=0 SIGTERM

if [ ! -z "${EXEC_ON_START}" -a -x "${EXEC_ON_START}" ]; then
    ${EXEC_ON_START}
fi

while [ "$ALIVE" -eq "1" ]; do
    if [ ! -z "${EXEC_PRE_ITERATION}" -a -x "${EXEC_PRE_ITERATION}" ]; then
        . ${EXEC_PRE_ITERATION}
    fi

    /s3sync sync ${S3_PATH} ${LOCAL_PATH}
    sleep ${TIMEOUT} &
    wait
done

