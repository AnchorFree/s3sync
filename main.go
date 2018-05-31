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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/urfave/cli"

	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	wait "k8s.io/apimachinery/pkg/util/wait"
	discovery "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
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
	app.Version = "0.0.5"

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
					if c.GlobalString("create-k8s-secret") == "true" {
						saveFileToSecretMapFromS3(svc, u.Bucket, *obj, filepath.Base(*obj.Key), &wg)
					} else {
						filename := filepath.Join(local_path, filepath.Base(*obj.Key))
						fileInfo, err := os.Stat(filename)

						if os.IsNotExist(err) {
							fmt.Println("file: ", filename, "does not exists, copying from s3")
							copyFileFromS3(svc, u.Bucket, *obj, filename, &wg, c.GlobalString("verbose"))
							ActionRequired = true
						} else {
							if fileInfo.ModTime().UTC() != *obj.LastModified {
								fmt.Println("modification time of file:", filename, "does not match one from s3, ordering copy")
								copyFileFromS3(svc, u.Bucket, *obj, filename, &wg, c.GlobalString("verbose"))
								ActionRequired = true
							}
						}
					}
				}
			}
			wg.Wait()
			return true
		})

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
		err = ioutil.WriteFile(filename, data, 0666)
		if err != nil {
			fmt.Println("Could not write file from s3 due to:", err)
			return err
		}
		return os.Chtimes(filename, *obj.LastModified, *obj.LastModified)
	}()
	return nil
}

func createApiserverClient() (*kubernetes.Clientset, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("could not execute due to error:", err)
		return nil, err
	}

	cfg.QPS = defaultQPS
	cfg.Burst = defaultBurst
	cfg.ContentType = "application/vnd.kubernetes.protobuf"

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Println("could not execute due to error:", err)
		return nil, err
	}

	var v *discovery.Info

	// In some environments is possible the client cannot connect the API server in the first request
	// https://github.com/kubernetes/ingress-nginx/issues/1968
	defaultRetry := wait.Backoff{
		Steps:    10,
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}

	var lastErr error
	retries := 0
	err = wait.ExponentialBackoff(defaultRetry, func() (bool, error) {
		v, err = client.Discovery().ServerVersion()
		if err == nil {
			return true, nil
		}

		lastErr = err
		retries++
		return false, nil
	})

	// err is not null only if there was a timeout in the exponential backoff (ErrWaitTimeout)
	if err != nil {
		return nil, lastErr
	}

	return client, nil
}

func EnsureSecret(secret *apiv1.Secret, verbose string) (*apiv1.Secret, error) {
	kubeClient, err := createApiserverClient()
	s, err := kubeClient.CoreV1().Secrets(secret.Namespace).Create(secret)
	if err != nil {
		if k8sErrors.IsAlreadyExists(err) {
			if verbose == "true" {
				fmt.Println("Secret", secret.Name, " already exist in namespace ", secret.Namespace, ", updating")
			}
			return kubeClient.CoreV1().Secrets(secret.Namespace).Update(secret)
		}
		fmt.Println("could not execute due to error:", err)
		return nil, err
	}
	if verbose == "true" {
		fmt.Println("Secret", secret.Name, " created in namespace ", secret.Namespace)
	}
	return s, nil
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

	r_secret_name, err := regexp.Compile("^(.+)\\.(?:key|crt)$")

	match_key, err := regexp.MatchString("^(.+)\\.key$", filename)
	match_cert, err := regexp.MatchString("^(.+)\\.crt$", filename)

	if match_key {
		if secret_hash[r_secret_name.FindStringSubmatch(filename)[1]] == nil {
			secret_hash[r_secret_name.FindStringSubmatch(filename)[1]] = make(map[string][]byte)
		}
		secret_hash[r_secret_name.FindStringSubmatch(filename)[1]]["key"] = data
	}
	if match_cert {
		if secret_hash[r_secret_name.FindStringSubmatch(filename)[1]] == nil {
			secret_hash[r_secret_name.FindStringSubmatch(filename)[1]] = make(map[string][]byte)
		}
		secret_hash[r_secret_name.FindStringSubmatch(filename)[1]]["cert"] = data
	}

	return err
}

func saveSecretMapToK8s(sh map[string]map[string][]byte, namespace string, label_name string, label_value string, verbose string) (err error) {
	if verbose == "true" {
		fmt.Println("Start creating secrets in Kubernetes")
	}

	for secret_domain := range sh {
		var err_message string
		if sh[secret_domain]["cert"] == nil {
			err_message = "ERROR: " + secret_domain + " do not have cert file"
			fmt.Println(err_message)
		} else if sh[secret_domain]["key"] == nil {
			err_message = "ERROR: " + secret_domain + " do not have key file"
			fmt.Println(err_message)
		} else {
			var labels map[string]string
			labels = make(map[string]string)
			labels["cert_domain"] = secret_domain
			labels["created_by"] = "s3sync"
			if label_name != "" && label_value != "" {
				labels[label_name] = label_value
			}

			_, err = EnsureSecret(&apiv1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret_domain,
					Namespace: namespace,
					Labels:    labels,
				},
				Data: map[string][]byte{
					apiv1.TLSCertKey:       sh[secret_domain]["cert"],
					apiv1.TLSPrivateKeyKey: sh[secret_domain]["key"],
				},
				Type: apiv1.SecretType("kubernetes.io/tls"),
			}, verbose)

			if err != nil {
				fmt.Println(err.Error())
				return err
			}
		}
	}
	return nil
}
