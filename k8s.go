package main

import (
	"fmt"
	"time"

	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	wait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
)

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

	// var v *discovery.Info

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
		_, err = client.Discovery().ServerVersion()
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
