package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/rockswe/K8s-PodConfigMapController/pkg/errors"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/logging"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/metrics"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/validation"
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
	logger        *logging.Logger
}

func NewController(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface) (*Controller, error) {
	logger := logging.NewLogger("controller")
	logger.Info("Creating new controller")

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
		logger:        logger,
	}

	logger.Info("Setting up event handlers")
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
				logger.Warning("PCMC Update: failed to cast to Unstructured", "oldType", fmt.Sprintf("%T", oldObj), "newType", fmt.Sprintf("%T", newObj))
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
		c.logger.Error(err, "Error getting key for pod object")
		return
	}
	c.podQueue.Add(key)
	metrics.SetQueueDepth("pods", c.podQueue.Len())
}

func (c *Controller) enqueuePcmc(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Error(err, "Error getting key for PCMC object", "type", fmt.Sprintf("%T", obj))
		return
	}
	c.pcmcQueue.Add(key)
	metrics.SetQueueDepth("podconfigmapconfigs", c.pcmcQueue.Len())
}

func (c *Controller) enqueuePcmcForDelete(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Error(err, "Error getting key for deleted PCMC object")
		return
	}
	c.pcmcQueue.Add("DELETED:" + key)
	metrics.SetQueueDepth("podconfigmapconfigs", c.pcmcQueue.Len())
}

func (c *Controller) Run(ctx context.Context) error {
	defer c.podQueue.ShutDown()
	defer c.pcmcQueue.ShutDown()

	c.logger.Info("Starting Pod informer")
	go c.podInformer.Run(ctx.Done())
	c.logger.Info("Starting PodConfigMapConfig informer")
	go c.pcmcInformer.Run(ctx.Done())

	c.logger.Info("Waiting for informer caches to sync")
	if !cache.WaitForCacheSync(ctx.Done(), c.podInformer.HasSynced, c.pcmcInformer.HasSynced) {
		return errors.NewInternalError("cache-sync", "informers", "failed to wait for caches to sync", nil)
	}

	c.logger.Info("Controller caches synchronized. Starting processing loops.")
	go c.runPodWorker(ctx)
	go c.runPcmcWorker(ctx)

	<-ctx.Done()
	c.logger.Info("Shutting down workers")
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
	defer metrics.SetQueueDepth("pods", c.podQueue.Len())

	start := time.Now()
	err := c.reconcilePod(ctx, key.(string))
	metrics.RecordReconciliationDuration("pod", "", time.Since(start).Seconds())

	if err != nil {
		c.logger.Error(err, "Error syncing Pod", "key", key)
		metrics.RecordReconciliationError("pod", "", string(errors.GetErrorType(err)))
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
	defer metrics.SetQueueDepth("podconfigmapconfigs", c.pcmcQueue.Len())

	key := keyObj.(string)
	isDelete := false
	if strings.HasPrefix(key, "DELETED:") {
		isDelete = true
		key = strings.TrimPrefix(key, "DELETED:")
	}

	start := time.Now()
	err := c.reconcilePcmc(ctx, key, isDelete)
	metrics.RecordReconciliationDuration("pcmc", "", time.Since(start).Seconds())

	if err != nil {
		c.logger.Error(err, "Error syncing PodConfigMapConfig", "key", key)
		metrics.RecordReconciliationError("pcmc", "", string(errors.GetErrorType(err)))
		c.pcmcQueue.AddRateLimited(keyObj)
		return true
	}

	c.pcmcQueue.Forget(keyObj)
	return true
}

func (c *Controller) reconcilePod(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.logger.Warning("Invalid resource key", "key", key)
		return nil
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if api_errors.IsNotFound(err) {
			c.logger.Debug("Pod in work queue no longer exists", "key", key)
			return c.handleDeletedPod(ctx, namespace, name)
		}
		return errors.NewAPIError("get-pod", fmt.Sprintf("pods/%s", key), "failed to get pod from lister", err)
	}

	// Validate pod before processing
	if err := validation.ValidatePod(pod); err != nil {
		c.logger.Warning("Skipping invalid pod", "key", key, "error", err)
		return nil
	}

	unstructuredPcmcs, err := c.pcmcInformer.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		return errors.NewAPIError("list-pcmcs", fmt.Sprintf("namespace/%s", namespace), "failed to list PodConfigMapConfigs by indexer", err)
	}

	var lastErr error
	for _, unstructuredObj := range unstructuredPcmcs {
		unstPcmc, ok := unstructuredObj.(*unstructured.Unstructured)
		if !ok {
			c.logger.Warning("Expected Unstructured from PCMC informer but got different type", "type", fmt.Sprintf("%T", unstructuredObj), "namespace", namespace)
			continue
		}
		typedPcmc := &v1alpha1.PodConfigMapConfig{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstPcmc.Object, typedPcmc); err != nil {
			c.logger.Error(err, "Failed to convert Unstructured to PodConfigMapConfig", "namespace", unstPcmc.GetNamespace(), "name", unstPcmc.GetName())
			if lastErr == nil {
				lastErr = fmt.Errorf("failed to convert PCMC %s/%s: %w", unstPcmc.GetNamespace(), unstPcmc.GetName(), err)
			}
			continue
		}

		pcmcCopy := typedPcmc.DeepCopy()
		errSync := c.syncConfigMapForPod(ctx, pod.DeepCopy(), pcmcCopy)
		if errSync != nil {
			c.logger.Error(errSync, "Error syncing ConfigMap for Pod", "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcName", typedPcmc.Name)
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
		c.logger.Warning("Invalid resource key for PCMC", "key", key)
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
		c.logger.Debug("PodConfigMapConfig in work queue no longer exists (handling as delete)", "key", key)
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
		c.logger.Warning("Failed to update status for PCMC. Continuing reconciliation.", "namespace", typedPcmc.Namespace, "name", typedPcmc.Name, "error", err)
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
			c.logger.Error(errSync, "Error syncing ConfigMap for Pod upon PCMC change", "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcName", typedPcmc.Name)
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
		c.logger.Warning("PCMC not found in cache for status update, perhaps it was deleted", "key", key)
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
	c.logger.Info("Successfully updated status for PCMC", "key", key, "observedGeneration", pcmcToUpdate.Generation)
	return nil
}

func (c *Controller) generateConfigMapName(pod *v1.Pod, pcmc *v1alpha1.PodConfigMapConfig) string {
	return fmt.Sprintf("pod-%s-from-%s-cfg", pod.Name, pcmc.Name)
}

func (c *Controller) handleDeletedPod(ctx context.Context, podNamespace, podName string) error {
	c.logger.Info("Handling deletion of Pod", "namespace", podNamespace, "name", podName)
	unstructuredPcmcs, err := c.pcmcInformer.GetIndexer().ByIndex(cache.NamespaceIndex, podNamespace)
	if err != nil {
		return fmt.Errorf("failed to list PCMCs in namespace %s for deleted pod %s by indexer: %w", podNamespace, podName, err)
	}

	var lastErr error
	for _, unstructuredObj := range unstructuredPcmcs {
		unstPcmc, ok := unstructuredObj.(*unstructured.Unstructured)
		if !ok {
			c.logger.Warning("Expected Unstructured from PCMC informer but got different type for deleted pod handling", "type", fmt.Sprintf("%T", unstructuredObj))
			continue
		}
		typedPcmc := &v1alpha1.PodConfigMapConfig{}
		if errConv := runtime.DefaultUnstructuredConverter.FromUnstructured(unstPcmc.Object, typedPcmc); errConv != nil {
			c.logger.Error(errConv, "Failed to convert Unstructured to PCMC for deleted pod handling", "namespace", unstPcmc.GetNamespace(), "name", unstPcmc.GetName())
			if lastErr == nil {
				lastErr = fmt.Errorf("failed to convert PCMC %s/%s for deleted pod: %w", unstPcmc.GetNamespace(), unstPcmc.GetName(), errConv)
			}
			continue
		}
		cmName := c.generateConfigMapName(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}}, typedPcmc)
		c.logger.Info("Deleting ConfigMap for deleted Pod", "configMapName", cmName, "podNamespace", podNamespace, "podName", podName, "pcmcName", typedPcmc.Name)
		errDel := c.deleteConfigMapIfExists(ctx, podNamespace, cmName)
		if errDel != nil {
			c.logger.Error(errDel, "Error deleting ConfigMap for deleted Pod", "configMapName", cmName, "podNamespace", podNamespace, "podName", podName, "pcmcName", typedPcmc.Name)
			if lastErr == nil {
				lastErr = errDel
			}
		}
	}
	return lastErr
}

func (c *Controller) handleDeletedPcmc(ctx context.Context, pcmcNamespace, pcmcName string) error {
	c.logger.Info("Handling deletion of PCMC", "namespace", pcmcNamespace, "name", pcmcName)
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
		c.logger.Info("Deleting ConfigMap for Pod due to deleted PCMC", "configMapName", cmName, "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcName", pcmcName)
		errDel := c.deleteConfigMapIfExists(ctx, pod.Namespace, cmName)
		if errDel != nil {
			c.logger.Error(errDel, "Error deleting ConfigMap for Pod (deleted PCMC)", "configMapName", cmName, "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcName", pcmcName)
			if lastErr == nil {
				lastErr = errDel
			}
		}
	}
	return lastErr
}

func (c *Controller) deleteConfigMapIfExists(ctx context.Context, namespace, name string) error {
	err := c.kubeClient.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !api_errors.IsNotFound(err) {
		metrics.RecordConfigMapOperation("delete", namespace, "error")
		return errors.NewAPIError("delete-configmap", fmt.Sprintf("configmap/%s/%s", namespace, name), "failed to delete ConfigMap", err)
	}
	if err == nil {
		c.logger.Info("ConfigMap deleted successfully", "namespace", namespace, "name", name)
		metrics.RecordConfigMapOperation("delete", namespace, "success")
	}
	return nil
}

func (c *Controller) syncConfigMapForPod(ctx context.Context, pod *v1.Pod, pcmc *v1alpha1.PodConfigMapConfig) error {
	configMapName := c.generateConfigMapName(pod, pcmc)

	// Validate generated ConfigMap name
	if err := validation.ValidateConfigMapName(configMapName); err != nil {
		return errors.NewValidationError("generate-configmap-name", fmt.Sprintf("configmap/%s", configMapName), "invalid ConfigMap name", err)
	}

	if pcmc.Spec.PodSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(pcmc.Spec.PodSelector)
		if err != nil {
			c.logger.Warning("Invalid podSelector in PCMC, skipping pod", "pcmcNamespace", pcmc.Namespace, "pcmcName", pcmc.Name, "podNamespace", pod.Namespace, "podName", pod.Name, "error", err)
			return errors.NewValidationError("parse-pod-selector", fmt.Sprintf("pcmc/%s/%s", pcmc.Namespace, pcmc.Name), "invalid podSelector", err)
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			c.logger.Debug("Pod does not match selector, ensuring ConfigMap is deleted", "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcNamespace", pcmc.Namespace, "pcmcName", pcmc.Name, "configMapName", configMapName)
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

	// Validate ConfigMap data
	if err := validation.ValidateConfigMapData(configData); err != nil {
		return errors.NewValidationError("validate-configmap-data", fmt.Sprintf("configmap/%s", configMapName), "invalid ConfigMap data", err)
	}

	ownerRef := metav1.NewControllerRef(pod, v1.SchemeGroupVersion.WithKind("Pod"))

	cmLabels := map[string]string{
		"podconfig.example.com/generated-by-pcmc": pcmc.Name,
		"podconfig.example.com/pod-uid":           string(pod.UID),
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existingCM, err := c.kubeClient.CoreV1().ConfigMaps(pod.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
		if api_errors.IsNotFound(err) {
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
				c.logger.Info("ConfigMap created for Pod", "configMapNamespace", pod.Namespace, "configMapName", configMapName, "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcName", pcmc.Name)
				metrics.RecordConfigMapOperation("create", pod.Namespace, "success")
			} else {
				metrics.RecordConfigMapOperation("create", pod.Namespace, "error")
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
			c.logger.Debug("ConfigMap is already up-to-date", "configMapNamespace", pod.Namespace, "configMapName", configMapName, "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcName", pcmc.Name)
			return nil
		}

		_, updateErr := c.kubeClient.CoreV1().ConfigMaps(pod.Namespace).Update(ctx, existingCM, metav1.UpdateOptions{})
		if updateErr == nil {
			c.logger.Info("ConfigMap updated for Pod", "configMapNamespace", pod.Namespace, "configMapName", configMapName, "podNamespace", pod.Namespace, "podName", pod.Name, "pcmcName", pcmc.Name)
			metrics.RecordConfigMapOperation("update", pod.Namespace, "success")
		} else {
			metrics.RecordConfigMapOperation("update", pod.Namespace, "error")
		}
		return updateErr
	})
}
