// main.go
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rockswe/K8s-PodConfigMapController/controller"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func main() {
	var kubeconfig string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error creating kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating kubernetes clientset: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating dynamic client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopCh
		log.Println("Shutdown signal received, exiting...")
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
			log.Fatalf("Error getting hostname: %v", errHostname)
		}
		log.Println("POD_NAME environment variable not set, using hostname for leader election ID.")
	}

	lockNamespace := os.Getenv("POD_NAMESPACE")
	if lockNamespace == "" {
		log.Println("POD_NAMESPACE environment variable not set, defaulting to 'default' for leader lock.")
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
		log.Fatalf("Error creating resource lock: %v", err)
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				log.Println("Leadership acquired, starting controller")
				ctrl, err := controller.NewController(kubeClient, dynamicClient)
				if err != nil {
					log.Fatalf("Error creating controller: %v", err)
				}
				if err := ctrl.Run(ctx); err != nil {
					log.Fatalf("Error running controller: %v", err)
				}
			},
			OnStoppedLeading: func() {
				log.Println("Leadership lost, shutting down")
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == leaderElectionID {
					return
				}
				log.Printf("New leader elected: %s\n", identity)
			},
		},
	})
}
