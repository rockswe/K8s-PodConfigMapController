// main.go
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rockswe/K8s-PodConfigMapController/controller"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/errors"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/logging"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func main() {
	// Setup logging
	logging.SetupLogging()
	logger := logging.NewLogger("main")

	var kubeconfig string
	var metricsAddr string
	var leaseDuration time.Duration
	var renewDeadline time.Duration
	var retryPeriod time.Duration

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "address to serve metrics on")
	flag.DurationVar(&leaseDuration, "lease-duration", 15*time.Second, "leader election lease duration")
	flag.DurationVar(&renewDeadline, "renew-deadline", 10*time.Second, "leader election renew deadline")
	flag.DurationVar(&retryPeriod, "retry-period", 2*time.Second, "leader election retry period")
	flag.Parse()

	logger.Info("Starting PodConfigMapController", "version", "v1.0.0")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		controllerErr := errors.NewConfigurationError("build-config", "kubeconfig", "failed to build kubeconfig", err)
		logger.Error(controllerErr, "Failed to build kubeconfig")
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		controllerErr := errors.NewConfigurationError("create-client", "kubernetes", "failed to create kubernetes client", err)
		logger.Error(controllerErr, "Failed to create kubernetes client")
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		controllerErr := errors.NewConfigurationError("create-client", "dynamic", "failed to create dynamic client", err)
		logger.Error(controllerErr, "Failed to create dynamic client")
		os.Exit(1)
	}

	// Start metrics server
	go func() {
		logger.Info("Starting metrics server", "address", metricsAddr)
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			logger.Error(err, "Failed to start metrics server")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopCh
		logger.Info("Shutdown signal received, exiting gracefully")
		cancel()
	}()

	podName := os.Getenv("POD_NAME")
	var leaderElectionID string
	if podName != "" {
		leaderElectionID = podName
	} else {
		var errHostname error
		leaderElectionID, errHostname = os.Hostname()
		if errHostname != nil {
			controllerErr := errors.NewConfigurationError("get-hostname", "hostname", "failed to get hostname for leader election", errHostname)
			logger.Error(controllerErr, "Failed to get hostname for leader election")
			os.Exit(1)
		}
		logger.Warning("POD_NAME environment variable not set, using hostname for leader election ID")
	}

	lockNamespace := os.Getenv("POD_NAMESPACE")
	if lockNamespace == "" {
		logger.Warning("POD_NAMESPACE environment variable not set, defaulting to 'default' for leader lock")
		lockNamespace = "default"
	}

	rl, err := resourcelock.New(resourcelock.LeasesResourceLock,
		lockNamespace,
		"podconfigmap-controller-lock",
		kubeClient.CoreV1(),
		kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: leaderElectionID,
		})
	if err != nil {
		controllerErr := errors.NewConfigurationError("create-lock", "resourcelock", "failed to create resource lock", err)
		logger.Error(controllerErr, "Failed to create resource lock")
		os.Exit(1)
	}

	logger.Info("Starting leader election", "identity", leaderElectionID, "namespace", lockNamespace)

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: leaseDuration,
		RenewDeadline: renewDeadline,
		RetryPeriod:   retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				logger.Info("Leadership acquired, starting controller")
				ctrl, err := controller.NewController(kubeClient, dynamicClient)
				if err != nil {
					controllerErr := errors.NewInternalError("create-controller", "controller", "failed to create controller", err)
					logger.Error(controllerErr, "Failed to create controller")
					os.Exit(1)
				}
				if err := ctrl.Run(ctx); err != nil {
					controllerErr := errors.NewInternalError("run-controller", "controller", "controller run failed", err)
					logger.Error(controllerErr, "Controller run failed")
					os.Exit(1)
				}
			},
			OnStoppedLeading: func() {
				logger.Info("Leadership lost, shutting down")
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == leaderElectionID {
					return
				}
				logger.Info("New leader elected", "identity", identity)
			},
		},
	})
}
