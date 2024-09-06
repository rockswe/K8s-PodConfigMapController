package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "path/filepath"
    "time"

    "k8s.io/client-go/informers"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/cache"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    corev1 "k8s.io/api/core/v1"
)

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

    // Create an informer factory for the default namespace
    informerFactory := informers.NewSharedInformerFactoryWithOptions(clientset, time.Second*30, informers.WithNamespace("default"))

    // Get pod informer
    podInformer := informerFactory.Core().V1().Pods().Informer()

    // Add event handlers to handle pod create, update, and delete events
    podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            pod := obj.(*corev1.Pod)
            log.Printf("Pod Created: %s/%s\n", pod.Namespace, pod.Name)

            // Create a ConfigMap for the pod
            createConfigMapForPod(clientset, pod)
        },
        DeleteFunc: func(obj interface{}) {
            pod := obj.(*corev1.Pod)
            log.Printf("Pod Deleted: %s/%s\n", pod.Namespace, pod.Name)

            // Delete the ConfigMap associated with the pod
            deleteConfigMapForPod(clientset, pod)
        },
    })

    stopCh := make(chan struct{})
    defer close(stopCh)

    // Start the informer to watch for Pod changes
    informerFactory.Start(stopCh)

    // Wait for the cache to synchronize
    if !cache.WaitForCacheSync(stopCh, podInformer.HasSynced) {
        log.Fatalf("Error syncing cache")
    }

    // Block the main thread
    <-stopCh
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
        log.Printf("Error creating ConfigMap for Pod %s: %v\n", pod.Name, err)
    } else {
        log.Printf("ConfigMap created for Pod %s\n", pod.Name)
    }
}

// Delete the ConfigMap associated with the pod
func deleteConfigMapForPod(clientset *kubernetes.Clientset, pod *corev1.Pod) {
    configMapName := fmt.Sprintf("configmap-%s", pod.Name)
    err := clientset.CoreV1().ConfigMaps(pod.Namespace).Delete(context.TODO(), configMapName, metav1.DeleteOptions{})
    if err != nil {
        log.Printf("Error deleting ConfigMap for Pod %s: %v\n", pod.Name, err)
    } else {
        log.Printf("ConfigMap deleted for Pod %s\n", pod.Name)
    }
}

podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
        pod := obj.(*corev1.Pod)
        log.Printf("Pod Created: %s/%s\n", pod.Namespace, pod.Name)
        createConfigMapForPod(clientset, pod)
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
        deleteConfigMapForPod(clientset, pod)
    },
})
