package controller

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"

	v1alpha1 "github.com/rockswe/K8s-PodConfigMapController/api/v1alpha1"
)

var (
	podConfigMapConfigGVR = schema.GroupVersionResource{Group: "podconfig.example.com", Version: "v1alpha1", Resource: "podconfigmapconfigs"}
)

type Controller struct {
	kubeClient    kubernetes.Interface
	dynamicClient dynamic.Interface
	podInformer   cache.SharedIndexInformer
	podLister     corev1listers.PodLister
	pcmcInformer  cache.SharedIndexInformer
	podQueue      workqueue.RateLimitingInterface
	pcmcQueue     workqueue.RateLimitingInterface
}

func NewController(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface) (*Controller, error) {
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, time.Minute*10)
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()

	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 0, metav1.NamespaceAll, nil)
	pcmcInformer := dynamicInformerFactory.ForResource(podConfigMapConfigGVR).Informer()

	c := &Controller{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
		podInformer:   podInformer,
		podLister:     kubeInformerFactory.Core().V1().Pods().Lister(),
		pcmcInformer:  pcmcInformer,
		podQueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pods"),
		pcmcQueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "podconfigmapconfigs"),
	}

	log.Println("Setting up event handlers")
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueuePod,
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldPod, okOld := oldObj.(*v1.Pod)
			newPod, okNew := newObj.(*v1.Pod)
			if !okOld || !okNew {
				c.enqueuePod(newObj)
				return
			}
			if oldPod.ResourceVersion == newPod.ResourceVersion &&
				reflect.DeepEqual(oldPod.Spec, newPod.Spec) &&
				reflect.DeepEqual(oldPod.Labels, newPod.Labels) &&
				reflect.DeepEqual(oldPod.Annotations, newPod.Annotations) {
				return
			}
			c.enqueuePod(newObj)
		},
		DeleteFunc: c.enqueuePod,
	})

	pcmcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueuePcmc,
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldUnst, okOld := oldObj.(*unstructured.Unstructured)
			newUnst, okNew := newObj.(*unstructured.Unstructured)
			if !okOld || !okNew {
				log.Printf("PCMC Update: failed to cast to Unstructured. Old: %T, New: %T", oldObj, newObj)
				if key, err := cache.MetaNamespaceKeyFunc(newObj); err == nil {
					c.pcmcQueue.Add(key)
				}
				return
			}
			if oldUnst.GetResourceVersion() == newUnst.GetResourceVersion() &&
				reflect.DeepEqual(oldUnst.Object["spec"], newUnst.Object["spec"]) &&
				reflect.DeepEqual(oldUnst.Object["status"], newUnst.Object["status"]) {
				return
			}
			c.enqueuePcmc(newObj)
		},
		DeleteFunc: c.enqueuePcmcForDelete,
	})

	return c, nil
}

func (c *Controller) enqueuePod(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Printf("Error getting key for pod object: %v", err)
		return
	}
	c.podQueue.Add(key)
}

func (c *Controller) enqueuePcmc(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Printf("Error getting key for PCMC object: %v (type: %T)", err, obj)
		return
	}
	c.pcmcQueue.Add(key)
}

func (c *Controller) enqueuePcmcForDelete(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Printf("Error getting key for deleted PCMC object: %v", err)
		return
	}
	c.pcmcQueue.Add("DELETED:" + key)
}

func (c *Controller) Run(ctx context.Context) error {
	defer c.podQueue.ShutDown()
	defer c.pcmcQueue.ShutDown()

	log.Println("Starting Pod informer")
	go c.podInformer.Run(ctx.Done())
	log.Println("Starting PodConfigMapConfig informer")
	go c.pcmcInformer.Run(ctx.Done())

	log.Println("Waiting for informer caches to sync")
	if !cache.WaitForCacheSync(ctx.Done(), c.podInformer.HasSynced, c.pcmcInformer.HasSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	log.Println("Controller caches synchronized. Starting processing loops.")
	go c.runPodWorker(ctx)
	go c.runPcmcWorker(ctx)

	<-ctx.Done()
	log.Println("Shutting down workers")
	return nil
}

func (c *Controller) runPodWorker(ctx context.Context) {
	for c.processNextPodWorkItem(ctx) {
	}
}

func (c *Controller) runPcmcWorker(ctx context.Context) {
	for c.processNextPcmcWorkItem(ctx) {
	}
}

func (c *Controller) processNextPodWorkItem(ctx context.Context) bool {
	key, shutdown := c.podQueue.Get()
	if shutdown {
		return false
	}
	defer c.podQueue.Done(key)

	err := c.reconcilePod(ctx, key.(string))
	if err != nil {
		log.Printf("Error syncing Pod '%s': %v", key, err)
		c.podQueue.AddRateLimited(key)
		return true
	}

	c.podQueue.Forget(key)
	return true
}

func (c *Controller) processNextPcmcWorkItem(ctx context.Context) bool {
	keyObj, shutdown := c.pcmcQueue.Get()
	if shutdown {
		return false
	}
	defer c.pcmcQueue.Done(keyObj)

	key := keyObj.(string)
	isDelete := false
	if strings.HasPrefix(key, "DELETED:") {
		isDelete = true
		key = strings.TrimPrefix(key, "DELETED:")
	}

	err := c.reconcilePcmc(ctx, key, isDelete)
	if err != nil {
		log.Printf("Error syncing PodConfigMapConfig '%s': %v", key, err)
		c.pcmcQueue.AddRateLimited(keyObj)
		return true
	}

	c.pcmcQueue.Forget(keyObj)
	return true
}

func (c *Controller) reconcilePod(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Printf("Invalid resource key: %s", key)
		return nil
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Printf("Pod '%s' in work queue no longer exists", key)
			return c.handleDeletedPod(ctx, namespace, name)
		}
		return err
	}

	unstructuredPcmcs, err := c.pcmcInformer.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		return fmt.Errorf("failed to list PodConfigMapConfigs in namespace %s by indexer: %w", namespace, err)
	}

	var lastErr error
	for _, unstructuredObj := range unstructuredPcmcs {
		unstPcmc, ok := unstructuredObj.(*unstructured.Unstructured)
		if !ok {
			log.Printf("Expected Unstructured from PCMC informer but got %T in namespace %s", unstructuredObj, namespace)
			continue
		}
		typedPcmc := &v1alpha1.PodConfigMapConfig{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstPcmc.Object, typedPcmc); err != nil {
			log.Printf("Failed to convert Unstructured to PodConfigMapConfig (obj: %s/%s): %v", unstPcmc.GetNamespace(), unstPcmc.GetName(), err)
			if lastErr == nil {
				lastErr = fmt.Errorf("failed to convert PCMC %s/%s: %w", unstPcmc.GetNamespace(), unstPcmc.GetName(), err)
			}
			continue
		}

		pcmcCopy := typedPcmc.DeepCopy()
		errSync := c.syncConfigMapForPod(ctx, pod.DeepCopy(), pcmcCopy)
		if errSync != nil {
			log.Printf("Error syncing ConfigMap for Pod %s/%s with PCMC %s: %v", pod.Namespace, pod.Name, typedPcmc.Name, errSync)
			if lastErr == nil {
				lastErr = errSync
			}
		}
	}
	return lastErr
}

func (c *Controller) reconcilePcmc(ctx context.Context, key string, isDelete bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Printf("Invalid resource key for PCMC: %s", key)
		return nil
	}

	if isDelete {
		return c.handleDeletedPcmc(ctx, namespace, name)
	}

	obj, exists, err := c.pcmcInformer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("error fetching PCMC '%s' from cache: %w", key, err)
	}
	if !exists {
		log.Printf("PodConfigMapConfig '%s' in work queue no longer exists (handling as delete)", key)
		return c.handleDeletedPcmc(ctx, namespace, name)
	}

	unstructuredPcmc, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("expected Unstructured from PCMC informer but got %T for key %s", obj, key)
	}
	typedPcmc := &v1alpha1.PodConfigMapConfig{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredPcmc.Object, typedPcmc); err != nil {
		return fmt.Errorf("failed to convert Unstructured to PodConfigMapConfig for %s: %w", key, err)
	}

	if err := c.updatePcmcStatus(ctx, typedPcmc); err != nil {
		log.Printf("Failed to update status for PCMC %s/%s: %v. Continuing reconciliation.", typedPcmc.Namespace, typedPcmc.Name, err)
	}

	pods, err := c.podLister.Pods(namespace).List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace %s for PCMC %s: %w", namespace, typedPcmc.Name, err)
	}

	var lastErr error
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		pcmcCopy := typedPcmc.DeepCopy()
		errSync := c.syncConfigMapForPod(ctx, pod.DeepCopy(), pcmcCopy)
		if errSync != nil {
			log.Printf("Error syncing ConfigMap for Pod %s/%s upon PCMC %s change: %v", pod.Namespace, pod.Name, typedPcmc.Name, errSync)
			if lastErr == nil {
				lastErr = errSync
			}
		}
	}
	return lastErr
}

func (c *Controller) updatePcmcStatus(ctx context.Context, pcmc *v1alpha1.PodConfigMapConfig) error {
	key := pcmc.Namespace + "/" + pcmc.Name
	obj, exists, err := c.pcmcInformer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("failed to get current PCMC %s from cache for status update: %w", key, err)
	}
	if !exists {
		log.Printf("PCMC %s not found in cache for status update, perhaps it was deleted.", key)
		return nil
	}

	currentUnstPcmc, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("expected Unstructured from PCMC informer for status update but got %T for key %s", obj, key)
	}
	currentTypedPcmc := &v1alpha1.PodConfigMapConfig{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(currentUnstPcmc.Object, currentTypedPcmc); err != nil {
		return fmt.Errorf("failed to convert current Unstructured to PCMC for status update (%s): %w", key, err)
	}

	if currentTypedPcmc.Status.ObservedGeneration == currentTypedPcmc.Generation {
		return nil
	}

	pcmcToUpdate := currentTypedPcmc.DeepCopy()
	if pcmcToUpdate == nil {
		return fmt.Errorf("DeepCopy of PCMC %s resulted in nil", key)
	}
	pcmcToUpdate.Status.ObservedGeneration = currentTypedPcmc.Generation

	updatedUnstructuredData, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pcmcToUpdate)
	if err != nil {
		return fmt.Errorf("failed to convert PCMC to Unstructured for status update (%s): %w", key, err)
	}

	_, err = c.dynamicClient.Resource(podConfigMapConfigGVR).Namespace(pcmcToUpdate.Namespace).UpdateStatus(ctx, &unstructured.Unstructured{Object: updatedUnstructuredData}, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update status for PCMC %s: %w", key, err)
	}
	log.Printf("Successfully updated status for PCMC %s to observedGeneration %d", key, pcmcToUpdate.Generation)
	return nil
}

func (c *Controller) generateConfigMapName(pod *v1.Pod, pcmc *v1alpha1.PodConfigMapConfig) string {
	return fmt.Sprintf("pod-%s-from-%s-cfg", pod.Name, pcmc.Name)
}

func (c *Controller) handleDeletedPod(ctx context.Context, podNamespace, podName string) error {
	log.Printf("Handling deletion of Pod %s/%s", podNamespace, podName)
	unstructuredPcmcs, err := c.pcmcInformer.GetIndexer().ByIndex(cache.NamespaceIndex, podNamespace)
	if err != nil {
		return fmt.Errorf("failed to list PCMCs in namespace %s for deleted pod %s by indexer: %w", podNamespace, podName, err)
	}

	var lastErr error
	for _, unstructuredObj := range unstructuredPcmcs {
		unstPcmc, ok := unstructuredObj.(*unstructured.Unstructured)
		if !ok {
			log.Printf("Expected Unstructured from PCMC informer but got %T for deleted pod handling", unstructuredObj)
			continue
		}
		typedPcmc := &v1alpha1.PodConfigMapConfig{}
		if errConv := runtime.DefaultUnstructuredConverter.FromUnstructured(unstPcmc.Object, typedPcmc); errConv != nil {
			log.Printf("Failed to convert Unstructured to PCMC for deleted pod handling (obj: %s/%s): %v", unstPcmc.GetNamespace(), unstPcmc.GetName(), errConv)
			if lastErr == nil {
				lastErr = fmt.Errorf("failed to convert PCMC %s/%s for deleted pod: %w", unstPcmc.GetNamespace(), unstPcmc.GetName(), errConv)
			}
			continue
		}
		cmName := c.generateConfigMapName(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}}, typedPcmc)
		log.Printf("Deleting ConfigMap %s for deleted Pod %s/%s (due to PCMC %s)", cmName, podNamespace, podName, typedPcmc.Name)
		errDel := c.deleteConfigMapIfExists(ctx, podNamespace, cmName)
		if errDel != nil {
			log.Printf("Error deleting ConfigMap %s for deleted Pod %s/%s (PCMC %s): %v", cmName, podNamespace, podName, typedPcmc.Name, errDel)
			if lastErr == nil {
				lastErr = errDel
			}
		}
	}
	return lastErr
}

func (c *Controller) handleDeletedPcmc(ctx context.Context, pcmcNamespace, pcmcName string) error {
	log.Printf("Handling deletion of PCMC %s/%s", pcmcNamespace, pcmcName)
	pods, err := c.podLister.Pods(pcmcNamespace).List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace %s for deleted PCMC %s: %w", pcmcNamespace, pcmcName, err)
	}
	var lastErr error
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		cmName := c.generateConfigMapName(pod, &v1alpha1.PodConfigMapConfig{ObjectMeta: metav1.ObjectMeta{Name: pcmcName, Namespace: pcmcNamespace}})
		log.Printf("Deleting ConfigMap %s for Pod %s/%s (due to deleted PCMC %s)", cmName, pod.Namespace, pod.Name, pcmcName)
		errDel := c.deleteConfigMapIfExists(ctx, pod.Namespace, cmName)
		if errDel != nil {
			log.Printf("Error deleting ConfigMap %s for Pod %s/%s (deleted PCMC %s): %v", cmName, pod.Namespace, pod.Name, pcmcName, errDel)
			if lastErr == nil {
				lastErr = errDel
			}
		}
	}
	return lastErr
}

func (c *Controller) deleteConfigMapIfExists(ctx context.Context, namespace, name string) error {
	err := c.kubeClient.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap %s/%s: %w", namespace, name, err)
	}
	if err == nil {
		log.Printf("ConfigMap %s/%s deleted successfully", namespace, name)
	}
	return nil
}

func (c *Controller) syncConfigMapForPod(ctx context.Context, pod *v1.Pod, pcmc *v1alpha1.PodConfigMapConfig) error {
	configMapName := c.generateConfigMapName(pod, pcmc)

	if pcmc.Spec.PodSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(pcmc.Spec.PodSelector)
		if err != nil {
			log.Printf("Invalid podSelector in PCMC %s/%s: %v. Skipping pod %s/%s.", pcmc.Namespace, pcmc.Name, err, pod.Namespace, pod.Name)
			return fmt.Errorf("invalid podSelector in PCMC %s/%s: %w", pcmc.Namespace, pcmc.Name, err)
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			log.Printf("Pod %s/%s does not match selector of PCMC %s/%s. Ensuring ConfigMap %s is deleted.", pod.Namespace, pod.Name, pcmc.Namespace, pcmc.Name, configMapName)
			return c.deleteConfigMapIfExists(ctx, pod.Namespace, configMapName)
		}
	}

	configData := map[string]string{
		"podName":   pod.Name,
		"namespace": pod.Namespace,
		"nodeName":  pod.Spec.NodeName,
		"phase":     string(pod.Status.Phase),
		"pcmcName":  pcmc.Name,
	}

	for _, labelKey := range pcmc.Spec.LabelsToInclude {
		if val, ok := pod.Labels[labelKey]; ok {
			configData["label_"+labelKey] = val
		}
	}
	for _, annotationKey := range pcmc.Spec.AnnotationsToInclude {
		if val, ok := pod.Annotations[annotationKey]; ok {
			configData["annotation_"+annotationKey] = val
		}
	}

	ownerRef := metav1.NewControllerRef(pod, v1.SchemeGroupVersion.WithKind("Pod"))

	cmLabels := map[string]string{
		"podconfig.example.com/generated-by-pcmc": pcmc.Name,
		"podconfig.example.com/pod-uid":           string(pod.UID),
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existingCM, err := c.kubeClient.CoreV1().ConfigMaps(pod.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:            configMapName,
					Namespace:       pod.Namespace,
					OwnerReferences: []metav1.OwnerReference{*ownerRef},
					Labels:          cmLabels,
				},
				Data: configData,
			}
			_, createErr := c.kubeClient.CoreV1().ConfigMaps(pod.Namespace).Create(ctx, cm, metav1.CreateOptions{})
			if createErr == nil {
				log.Printf("ConfigMap %s/%s created for Pod %s/%s (via PCMC %s)", pod.Namespace, configMapName, pod.Namespace, pod.Name, pcmc.Name)
			}
			return createErr
		}
		if err != nil {
			return err
		}

		needsUpdate := false
		if !reflect.DeepEqual(existingCM.Data, configData) {
			needsUpdate = true
			existingCM.Data = configData
		}
		if !reflect.DeepEqual(existingCM.OwnerReferences, []metav1.OwnerReference{*ownerRef}) {
			needsUpdate = true
			existingCM.OwnerReferences = []metav1.OwnerReference{*ownerRef}
		}
		if existingCM.Labels == nil {
			existingCM.Labels = make(map[string]string)
		}
		for k, v := range cmLabels {
			if existingCM.Labels[k] != v {
				needsUpdate = true
				existingCM.Labels[k] = v
			}
		}

		if !needsUpdate {
			log.Printf("ConfigMap %s/%s for Pod %s/%s (via PCMC %s) is already up-to-date.", pod.Namespace, configMapName, pod.Namespace, pod.Name, pcmc.Name)
			return nil
		}

		_, updateErr := c.kubeClient.CoreV1().ConfigMaps(pod.Namespace).Update(ctx, existingCM, metav1.UpdateOptions{})
		if updateErr == nil {
			log.Printf("ConfigMap %s/%s updated for Pod %s/%s (via PCMC %s)", pod.Namespace, configMapName, pod.Namespace, pod.Name, pcmc.Name)
		}
		return updateErr
	})
}
