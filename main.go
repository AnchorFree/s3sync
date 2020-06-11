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

const (
	// High enough QPS to fit all expected use cases. QPS=0 is not set here, because
	// client code is overriding it.
	defaultQPS = 1e6
	// High enough Burst to fit all expected use cases. Burst=0 is not set here, because
	// client code is overriding it.
	defaultBurst = 1e6
)

var secret_hash map[string]map[string][]byte

type S3Address struct {
	Bucket string
	Prefix string
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
	app.Version = "0.0.6"

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
		cli.StringFlag{
			Name:   "create-k8s-secret",
			Usage:  "If true, will create secret on Kubernetes cluster instead of local file",
			EnvVar: "CREATE_K8S_SECRET",
		},
		cli.StringFlag{
			Name:   "k8s-secret-namespace",
			Usage:  "Kubernetes cluster Namesace Name for secrets",
			EnvVar: "K8S_SECRET_NAMESPACE",
		},
		cli.StringFlag{
			Name:   "k8s-custom-label-name",
			Usage:  "Kubernetes label name that will be assign to secret",
			EnvVar: "K8S_CUSTOM_LABEL_NAME",
		},
		cli.StringFlag{
			Name:   "k8s-custom-label-value",
			Usage:  "Kubernetes label value that will be assign to secret",
			EnvVar: "K8S_CUSTOM_LABEL_VALUE",
		},
		cli.StringFlag{
			Name:   "verbose",
			Usage:  "Verbose flag",
			EnvVar: "VERBOSE",
		},
		cli.StringFlag{
			Name:   "force",
			Usage:  "Force mode will delete files which are not in the bucket",
			EnvVar: "FORCE",
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
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal("Failed to run the app", err)
	}

}

func CmdSync(c *cli.Context) error {
	sess, err := session.NewSession()

	var wg sync.WaitGroup

	if err != nil {
		fmt.Println("failed to create session,", err)
		return err
	}

	svc := s3.New(sess)
	s3_path := c.Args().Get(0)
	local_path := c.Args().Get(1)

	u := parseURL(s3_path)

	params := &s3.ListObjectsV2Input{
		Bucket:     aws.String(u.Bucket), // Required
		FetchOwner: aws.Bool(true),
		Prefix:     aws.String(u.Prefix),
	}

	i := 0
	var ActionRequired bool
	secret_hash = make(map[string]map[string][]byte)

	// keep files inside s3 that should exists on the local disk
	var s3files []string

	err = svc.ListObjectsV2Pages(params,
		func(p *s3.ListObjectsV2Output, last bool) (shouldContinue bool) {
			i++
			for _, obj := range p.Contents {
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
					s3files = append(s3files, filepath.Base(*obj.Key))
					if c.GlobalString("create-k8s-secret") == "true" {
						// just to stfu gosec, proper error handling requires refactoring of the whole app...
						_ = saveFileToSecretMapFromS3(svc, u.Bucket, *obj, filepath.Base(*obj.Key), &wg)
					} else {
						filename := filepath.Join(local_path, filepath.Base(*obj.Key))

						if _, err := os.Stat(filename); os.IsNotExist(err) {
							fmt.Println("file: ", filename, "does not exists, copying from s3")
							_ = copyFileFromS3(svc, u.Bucket, *obj, filename, &wg, c.GlobalString("verbose"))
							ActionRequired = true
						} else {
							md5sum := strings.TrimSuffix(strings.TrimPrefix(*obj.ETag, "\""), "\"")
							match, err := fileMD5Match(filename, md5sum)
							if err != nil {
								// we basically don't care at this stage
							}
							if !match {
								fmt.Printf("file %s md5sum does not equal to registered in s3: %s copying from s3\n", filename, md5sum)
								_ = copyFileFromS3(svc, u.Bucket, *obj, filename, &wg, c.GlobalString("verbose"))
								ActionRequired = true
							}
						}
					}
				}
			}
			wg.Wait()
			return true
		})

	if c.GlobalString("force") == "true" {
		dirFiles, err := ioutil.ReadDir(local_path)
		if err != nil {
			fmt.Printf("Could not list directory %s due to %v\n", local_path, err)
		}
		for _, file := range dirFiles {
			if !Contains(s3files, file.Name()) {
				filename := filepath.Join(local_path, file.Name())
				fmt.Printf("Removing file %s because it doesn't not exist in s3 bucket\n", filename)
				err = os.Remove(filename)
				ActionRequired = true
				if err != nil {
					fmt.Printf("Could not remove file %s due to: %v\n", filename, err)
				}
			}
		}
	}

	if c.GlobalString("create-k8s-secret") == "true" {
		secret_namespace := "default"

		if c.GlobalString("k8s-secret-namespace") != "" {
			secret_namespace = c.GlobalString("k8s-secret-namespace")
		}

		err = saveSecretMapToK8s(secret_hash, secret_namespace, c.GlobalString("k8s-custom-label-name"), c.GlobalString("k8s-custom-label-value"), c.GlobalString("verbose"))
	}

	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		fmt.Println(err.Error())
		return err
	}

	if ActionRequired {
		action := fmt.Sprintf("%v", c.GlobalString("exec-on-change"))
		fmt.Println("Executing command", action)
		// This is potentially harmful, because the value of action can be really anything.
		// However, there is no quick fix, this is actually by design, so we'll have to leave it as is for now...
		cmd := exec.Command("/bin/sh", "-c", action) // #nosec
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			fmt.Println("could not execute due to error:", err)
		}
	}
	return nil

}
