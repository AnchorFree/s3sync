package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func copyFileFromS3(s *s3.S3, bucket string, obj s3.Object, filename string, wg *sync.WaitGroup, verbose string) error {
	wg.Add(1)
	go func() error {
		defer wg.Done()
		params := &s3.GetObjectInput{
			Bucket: aws.String(bucket),   // Required
			Key:    aws.String(*obj.Key), // Required
		}

		if verbose == "true" {
			fmt.Println("Getting ", filename, " from ", bucket)
		}
		resp, err := s.GetObject(params)
		if err != nil {
			fmt.Println("Could not get s3 object due to:", err)
			return err
		}
		if verbose == "true" {
			fmt.Println("Reading certificate ", filename, " from bucket ", bucket)
		}
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Could not download file from s3 due to:", err)
			return err
		}

		if verbose == "true" {
			fmt.Println("Writing file ", filename)
		}
		err = ioutil.WriteFile(filename, data, 0600)
		if err != nil {
			fmt.Println("Could not write file from s3 due to:", err)
			return err
		}
		return os.Chtimes(filename, *obj.LastModified, *obj.LastModified)
	}()
	return nil
}

func saveFileToSecretMapFromS3(s *s3.S3, bucket string, obj s3.Object, filename string, wg *sync.WaitGroup) error {
	defer wg.Done()
	wg.Add(1)
	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),   // Required
		Key:    aws.String(*obj.Key), // Required
	}
	resp, err := s.GetObject(params)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Could not download file from s3 due to:", err)
		return err
	}

	if strings.HasSuffix(filename, ".key") || strings.HasSuffix(filename, ".crt") {

		// If it's a wildcard certificate, we should change
		// the initial underscore in the filename, since we use
		// this name (stripping off '.crt' or '.key' suffix) as
		// the name of k8s secret, and kubernetes doesn't allow
		// a resource name to start with underscore
		secretName := ""
		if strings.HasPrefix(filename, "_.") {
			secretName = "wildcard" + filename[1:len(filename)-4]
		} else {
			secretName = filename[:len(filename)-4]
		}

		if secret_hash[secretName] == nil {
			secret_hash[secretName] = make(map[string][]byte)
		}

		if strings.HasSuffix(filename, ".key") {
			secret_hash[secretName]["key"] = data
		} else {
			secret_hash[secretName]["cert"] = data
		}
	}
	return err
}
