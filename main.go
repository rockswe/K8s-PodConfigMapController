package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "time"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/client-go/informers"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/cache"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/tools/leaderelection"
    "k8s.io/client-go/tools/leaderelection/resourcelock"
    "k8s.io/client-go/util/homedir"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    podsCreated = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "pods_created_total",
        Help: "Total number of pods created.",
    })
    podsDeleted = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "pods_deleted_total",
        Help: "Total number of pods deleted.",
    })
)

func init() {
    prometheus.MustRegister(podsCreated)
    prometheus.MustRegister(podsDeleted)
}

func main() {
    var kubeconfig string
    if home := homedir.HomeDir(); home != "" {
        kubeconfig = filepath.Join(home, ".kube", "config")
    } else {
        kubeconfig = ""
    }

    flag.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "(optional) absolute path to the kubeconfig file")
    flag.Parse()

    config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
    if err != nil {
        log.Fatalf("Error building kubeconfig: %v", err)
    }

    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        log.Fatalf("Error creating Kubernetes client: %v", err)
    }

    // Set up leader election
    id, err := os.Hostname()
    if err != nil {
        log.Fatalf("Failed to get hostname: %v", err)
    }

    lock := &resourcelock.LeaseLock{
        LeaseMeta: metav1.ObjectMeta{
            Name:      "my-controller-leader-election",
            Namespace: "default",
        },
        Client: clientset.CoordinationV1(),
        LockConfig: resourcelock.ResourceLockConfig{
            Identity: id,
        },
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go func() {
        http.Handle("/metrics", promhttp.Handler())
        log.Println("Starting Prometheus metrics server on :8080")
        if err := http.ListenAndServe(":8080", nil); err != nil {
            log.Fatalf("Failed to start metrics server: %v", err)
        }
    }()

    leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
        Lock:          lock,
        LeaseDuration: 15 * time.Second,
        RenewDeadline: 10 * time.Second,
        RetryPeriod:   2 * time.Second,
        Callbacks: leaderelection.LeaderCallbacks{
            OnStartedLeading: func(ctx context.Context) {
                runController(ctx, clientset)
            },
            OnStoppedLeading: func() {
                log.Printf("Leader lost: %s", id)
                os.Exit(0)
            },
        },
        ReleaseOnCancel: true,
        Name:            "my-controller",
    })
}

func runController(ctx context.Context, clientset *kubernetes.Clientset) {
    // Create an informer factory for all namespaces
    informerFactory := informers.NewSharedInformerFactory(clientset, time.Second*30)

    // Get pod informer
    podInformer := informerFactory.Core().V1().Pods().Informer()

    // Add event handlers to handle pod create, update, and delete events
    podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            pod := obj.(*corev1.Pod)
            log.Printf("Pod Created: %s/%s\n", pod.Namespace, pod.Name)

            // Create a ConfigMap for the pod
            createConfigMapForPod(clientset, pod)
            podsCreated.Inc()
        },
        UpdateFunc: func(oldObj, newObj interface{}) {
            oldPod := oldObj.(*corev1.Pod)
            newPod := newObj.(*corev1.Pod)

            if oldPod.Status.Phase != newPod.Status.Phase {
                log.Printf("Pod Updated: %s/%s. Status: %s -> %s\n", newPod.Namespace, newPod.Name, oldPod.Status.Phase, newPod.Status.Phase)
            }
        },
        DeleteFunc: func(obj interface{}) {
            pod := obj.(*corev1.Pod)
            log.Printf("Pod Deleted: %s/%s\n", pod.Namespace, pod.Name)

            // Delete the ConfigMap associated with the pod
            deleteConfigMapForPod(clientset, pod)
            podsDeleted.Inc()
        },
    })

    // Start the informer to watch for Pod changes
    informerFactory.Start(ctx.Done())

    // Wait for the cache to synchronize
    if !cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced) {
        log.Fatalf("Error syncing cache")
    }

    log.Println("Controller has started, waiting for events...")

    // Block until the context is canceled
    <-ctx.Done()
}

// Create a ConfigMap for the pod
func createConfigMapForPod(clientset *kubernetes.Clientset, pod *corev1.Pod) {
    configMap := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("configmap-%s", pod.Name),
            Namespace: pod.Namespace,
        },
        Data: map[string]string{
            "podName": pod.Name,
            "podIP":   pod.Status.PodIP,
        },
    }

    _, err := clientset.CoreV1().ConfigMaps(pod.Namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
    if err != nil {
        log.Printf("Error creating ConfigMap for Pod %s/%s: %v\n", pod.Namespace, pod.Name, err)
    } else {
        log.Printf("ConfigMap created for Pod %s/%s\n", pod.Namespace, pod.Name)
    }
}

// Delete the ConfigMap associated with the pod
func deleteConfigMapForPod(clientset *kubernetes.Clientset, pod *corev1.Pod) {
    configMapName := fmt.Sprintf("configmap-%s", pod.Name)
    err := clientset.CoreV1().ConfigMaps(pod.Namespace).Delete(context.TODO(), configMapName, metav1.DeleteOptions{})
    if err != nil {
        log.Printf("Error deleting ConfigMap for Pod %s/%s: %v\n", pod.Namespace, pod.Name, err)
    } else {
        log.Printf("ConfigMap deleted for Pod %s/%s\n", pod.Namespace, pod.Name)
    }
}
