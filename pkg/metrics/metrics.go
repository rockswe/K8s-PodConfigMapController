package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ConfigMapOperations tracks ConfigMap operations (create, update, delete)
	ConfigMapOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "podconfigmap_controller_configmap_operations_total",
			Help: "Total number of ConfigMap operations performed",
		},
		[]string{"operation", "namespace", "result"},
	)

	// ReconciliationDuration tracks reconciliation duration
	ReconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "podconfigmap_controller_reconciliation_duration_seconds",
			Help:    "Duration of reconciliation operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"resource_type", "namespace"},
	)

	// ReconciliationErrors tracks reconciliation errors
	ReconciliationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "podconfigmap_controller_reconciliation_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"resource_type", "namespace", "error_type"},
	)

	// QueueDepth tracks workqueue depth
	QueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "podconfigmap_controller_queue_depth",
			Help: "Current depth of work queues",
		},
		[]string{"queue_name"},
	)

	// PodConfigMapConfigStatus tracks PCMC status
	PodConfigMapConfigStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "podconfigmap_controller_pcmc_status",
			Help: "Status of PodConfigMapConfig resources (1=ready, 0=not ready)",
		},
		[]string{"name", "namespace"},
	)

	// ActiveConfigMaps tracks the number of active ConfigMaps
	ActiveConfigMaps = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "podconfigmap_controller_active_configmaps",
			Help: "Number of active ConfigMaps managed by the controller",
		},
		[]string{"namespace", "pcmc_name"},
	)
)

// RecordConfigMapOperation records a ConfigMap operation
func RecordConfigMapOperation(operation, namespace, result string) {
	ConfigMapOperations.WithLabelValues(operation, namespace, result).Inc()
}

// RecordReconciliationDuration records reconciliation duration
func RecordReconciliationDuration(resourceType, namespace string, duration float64) {
	ReconciliationDuration.WithLabelValues(resourceType, namespace).Observe(duration)
}

// RecordReconciliationError records a reconciliation error
func RecordReconciliationError(resourceType, namespace, errorType string) {
	ReconciliationErrors.WithLabelValues(resourceType, namespace, errorType).Inc()
}

// SetQueueDepth sets the queue depth metric
func SetQueueDepth(queueName string, depth int) {
	QueueDepth.WithLabelValues(queueName).Set(float64(depth))
}

// SetPodConfigMapConfigStatus sets the PCMC status metric
func SetPodConfigMapConfigStatus(name, namespace string, ready bool) {
	value := 0.0
	if ready {
		value = 1.0
	}
	PodConfigMapConfigStatus.WithLabelValues(name, namespace).Set(value)
}

// SetActiveConfigMaps sets the active ConfigMaps metric
func SetActiveConfigMaps(namespace, pcmcName string, count int) {
	ActiveConfigMaps.WithLabelValues(namespace, pcmcName).Set(float64(count))
}
