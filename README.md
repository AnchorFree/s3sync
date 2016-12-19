It is a fork of https://github.com/koblas/s3-cli without most of cli functionality except of sync. 
It functionality to run arbitrary commands on sync change, and use md5 sum check by default. 


In order to run the application please use following:
```
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=YOURKEYAKIA
export AWS_SECRET_ACCESS_KEY=YOURSecretToAPI
./s3sync  --exec-on-change "touch /tmp/blah123" -v sync  s3://bucket-name/path/very/long/path /tmp/certificates

```

or you can use environment variable `ON_CHANGE` to configure the post changes action
