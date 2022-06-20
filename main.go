package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"github.com/stolostron/hub-of-hubs-repo/pkg/repo"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var (
	// errFlagParameterEmpty        = errors.New("flag parameter empty")
	errFlagParameterIllegalValue = errors.New("flag parameter illegal value")
)

func parseFlags() (*repo.RepoConfig, error) {
	repoConfig := &repo.RepoConfig{
		ChartDir: "./charts/",
		RepoDir:  "/repo/charts",
		Version:  "2.5.0",
		Port:     3000,
	}

	pflag.StringVar(&repoConfig.ChartDir, "chart-dir", "./charts/", "directory of reading charts from.")
	pflag.StringVar(&repoConfig.RepoDir, "repo-dir", "/repo/charts", "directory of writing helm charts to.")
	pflag.StringVar(&repoConfig.Version, "version", "2.5.0", "version of helm charts.")
	// pflag.StringVar(&repoConfig.Host, "host", "", "The host for the helm chart repo.")
	pflag.IntVar(&repoConfig.Port, "port", 3000, "The port for the helm chart repo.")

	// add flags for logger
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	// if repoConfig.Host == "" {
	// 	return nil, fmt.Errorf("host for the helm chart repo: %w", errFlagParameterEmpty)
	// }

	if repoConfig.Port < 1024 || repoConfig.Port > 65535 {
		return nil, fmt.Errorf("%w - port for the helm chart repo should be in the range: 1024 - 65535", errFlagParameterIllegalValue)
	}

	return repoConfig, nil
}

func initKubeClient() (dynamic.Interface, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return dynClient, nil
}

func getRouteDomain(dynClient dynamic.Interface) (string, error) {
	var ingressControllerResource = schema.GroupVersionResource{Group: "operator.openshift.io", Version: "v1", Resource: "ingresscontrollers"}
	defaultIngressController, err := dynClient.Resource(ingressControllerResource).Namespace("openshift-ingress-operator").Get(context.TODO(), "default", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	status, ok := defaultIngressController.Object["status"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected status of default ingresscontroller")
	}

	domain, ok := status["domain"].(string)
	if !ok {
		return "", fmt.Errorf("domain doesn't exist in the status of default ingresscontroller")
	}

	return domain, nil
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain() int {
	log.Printf("Go Version: %s", runtime.Version())
	log.Printf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)

	// create repo configuration from command parameters
	repoConfig, err := parseFlags()
	if err != nil {
		log.Println(err)
		return 1
	}

	dynClient, err := initKubeClient()
	if err != nil {
		log.Println(err)
		return 1
	}

	routeDomain, err := getRouteDomain(dynClient)
	if err != nil {
		log.Println(err)
		return 1
	}

	repoConfig.Host = routeDomain

	repoServer, err := repo.NewRepoServer(repoConfig)
	if err != nil {
		log.Println(err)
		return 1
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		log.Printf("serving on port %d", repoServer.Config.Port)
		if err := repoServer.Server.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	// Kubernetes sends a SIGTERM, waits for a grace period, and then a SIGKILL
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	sig := <-sigCh
	log.Printf("Received signal: %s", sig.String())

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if err := repoServer.Server.Shutdown(ctx); err != nil {
		log.Println(err)
	}

	log.Println("exiting...")
	return 0
}

func main() {
	os.Exit(doMain())
}
