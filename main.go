// +build example

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/urfave/cli"
)

// Lists all objects in a bucket using pagination
//
// Usage:

type S3Address struct {
	Bucket string
	Prefix string
}

type SyncObject struct {
	Bucket   string
	O        s3.Object
	Filename string
}

func parseURL(path string) S3Address {
	var s S3Address

	if !strings.HasPrefix(path, "s3://") {
		path = fmt.Sprintf("s3://%s", path)
	}

	u, err := url.Parse(path)
	if err != nil {
		log.Fatal("Could not parse URL due to error: ", err)
	}

	if u.Scheme != "" && u.Scheme != "s3" {
		log.Fatal("Invalid URI scheme must be one of s3/NONE")
	}

	if strings.HasPrefix(u.Path, "/") {
		s.Prefix = u.Path[1:]
	} else {
		s.Prefix = u.Path
	}

	s.Bucket = u.Host
	return s
}

// listObjects <bucket>
func main() {
	app := cli.NewApp()
	app.Name = "s3sync"
	app.Version = "0.0.4"

	cli.VersionFlag = cli.BoolFlag{
		Name:  "version, V",
		Usage: "print version number",
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "exec-on-change",
			Usage:  "run a command in case of changes after sync",
			EnvVar: "EXEC_ON_CHANGE,ON_CHANGE_EXEC,ON_CHANGE",
		},
		cli.StringFlag{
			Name:   "match-regexp",
			Usage:  "match s3 bucket contents with following regexp",
			EnvVar: "MATCH_REGEXP",
		},
		cli.StringFlag{
			Name:   "filter-out-regexp",
			Usage:  "filter out s3 bucket with following regexp",
			EnvVar: "FILTER_OUT_REGEXP",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:   "sync",
			Usage:  "Synchronize a directory tree to S3 -- LOCAL_DIR s3://BUCKET[/PREFIX] or s3://BUCKET[/PREFIX] LOCAL_DIR",
			Action: CmdSync,
			Flags:  app.Flags,
		},
	}
	app.Run(os.Args)
}

func CmdSync(c *cli.Context) error {
	sess, err := session.NewSession()

	if err != nil {
		fmt.Println("failed to create session,", err)
		return err
	}

	svc := s3.New(sess)
	s3_path := c.Args().Get(0)
	local_path := c.Args().Get(1)

	var wg sync.WaitGroup

	u := parseURL(s3_path)

	params := &s3.ListObjectsV2Input{
		Bucket:     aws.String(u.Bucket), // Required
		FetchOwner: aws.Bool(true),
		Prefix:     aws.String(u.Prefix),
	}

	i := 0
	var ActionRequired bool
	err = svc.ListObjectsV2Pages(params,
		func(p *s3.ListObjectsV2Output, last bool) (shouldContinue bool) {
			i++
			for _, obj := range p.Contents {
				var sync_obj SyncObject
				// make magic here
				var filter_out bool
				matched := true

				if c.GlobalString("match-regexp") != "" {
					matched, _ = regexp.MatchString(c.GlobalString("match-regexp"), *obj.Key)
				}

				if c.GlobalString("filter-out-regexp") != "" {
					filter_out, _ = regexp.MatchString(c.GlobalString("filter-out-regexp"), *obj.Key)
				}

				if matched && filter_out != true {
					filename := filepath.Join(local_path, filepath.Base(*obj.Key))
					fileInfo, err := os.Stat(filename)
					if os.IsNotExist(err) {
						fmt.Println("file: ", filename, "does not exists, copying from s3")
						go func() {
							wg.Add(1)
							defer wg.Done()
							sync_obj.Bucket = u.Bucket
							sync_obj.O = *obj
							sync_obj.Filename = filename
							copyFileFromS3(sync_obj)

						}()
						//copyFileFromS3(svc, u.Bucket, *obj, filename)
						ActionRequired = true
					} else {
						if fileInfo.ModTime().UTC() != *obj.LastModified {
							fmt.Println("modification time of file:", filename, "does not match one from s3, ordering copy")
							go func() {
								wg.Add(1)
								defer wg.Done()
								sync_obj.Bucket = u.Bucket
								sync_obj.O = *obj
								sync_obj.Filename = filename
								copyFileFromS3(sync_obj)
							}()
							ActionRequired = true
						}
					}
				}
			}
			return true
		})

	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		fmt.Println(err.Error())
		return err
	}

	wg.Wait()

	if ActionRequired {
		action := fmt.Sprintf("%v", c.GlobalString("exec-on-change"))
		fmt.Println("Executing command", action)

		cmd := exec.Command("/bin/sh", "-c", action)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			fmt.Println("could not execute due to error:", err)
		}
	}
	return nil

}

func copyFileFromS3(so SyncObject) error {
	sess, err := session.NewSession()

	if err != nil {
		fmt.Println("failed to create session,", err)
		return err
	}

	s := s3.New(sess)
	params := &s3.GetObjectInput{
		Bucket: aws.String(so.Bucket), // Required
		Key:    aws.String(*so.O.Key), // Required
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

	err = ioutil.WriteFile(so.Filename, data, 0666)
	if err != nil {
		fmt.Println("Could not write file from s3 due to:", err)
		return err
	}
	return os.Chtimes(so.Filename, *so.O.LastModified, *so.O.LastModified)
}
