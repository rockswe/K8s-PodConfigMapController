# PodConfigMapController

PodConfigMapController is a k8s controller that dynamically generates and manages ConfigMaps based on the metadata of running Pods. Its behavior is configured through a Custom Resource Definition (CRD) called `PodConfigMapConfig`, allowing users to specify which Pods to target and what information to include in the generated ConfigMaps.

This controller helps in scenarios where you need Pod-specific information to be available as a ConfigMap, for example, to be consumed by other applications, monitoring systems, or for easier debugging and visibility into Pod metadata.

## Features

*   **Dynamic ConfigMap Generation**: Automatically creates, updates, and deletes ConfigMaps based on Pod lifecycle and `PodConfigMapConfig` resources.
*   **CRD Driven Configuration**: Uses the `PodConfigMapConfig` Custom Resource to define behavior per namespace.
*   **Selective Pod Targeting**: Utilizes `podSelector` within the `PodConfigMapConfig` CRD to precisely target which Pods should have ConfigMaps generated for them.
*   **Metadata Inclusion**: Allows specifying which Pod labels and annotations should be included in the ConfigMap data.
*   **Status Reporting**: The `PodConfigMapConfig` resource reports its `observedGeneration` in its status, indicating if the controller has processed the latest specification.
*   **Namespace-Scoped**: Operates within specific namespaces, allowing for different configurations across a cluster.
*   **eBPF Integration**: Attach tiny eBPF programs per Pod for syscall monitoring and L4 network filtering with Prometheus metrics export.
*   **Structured Logging**: Comprehensive logging with klog/v2 using structured key-value pairs, configurable log levels, and JSON output support.
*   **Prometheus Metrics**: Built-in observability with metrics for ConfigMap operations, reconciliation duration, errors, queue depth, and PCMC status.
*   **Input Validation**: Comprehensive validation for CRD fields, ConfigMap names/data, Pod metadata, and label/annotation keys.
*   **Error Handling**: Typed errors with context, error aggregation, proper propagation, and retry logic with exponential backoff.
*   **Configuration Management**: Extensive configuration via environment variables for leader election, controller settings, logging, and metrics.
*   **Leader Election**: Multi-replica deployment support with configurable leader election using Lease locks.
*   **Minimal Container Image**: Distroless-based image with eBPF toolchain, running as non-root with read-only filesystem.

## Custom Resource Definition (CRD): `PodConfigMapConfig`

The `PodConfigMapConfig` CRD is central to configuring the controller's behavior within a namespace.

**Purpose**: It defines rules for how ConfigMaps should be generated for Pods in the same namespace as the `PodConfigMapConfig` resource. Multiple `PodConfigMapConfig` resources can exist in a namespace, each potentially targeting different sets of Pods.

**Specification (`spec`)**:

*   `labelsToInclude`: (Optional) An array of strings. Each string is a Pod label key. If a Pod has any of these labels, the key-value pair will be included in the generated ConfigMap's data, prefixed with `label_`.
*   `annotationsToInclude`: (Optional) An array of strings. Each string is a Pod annotation key. If a Pod has any of these annotations, the key-value pair will be included in the generated ConfigMap's data, prefixed with `annotation_`.
*   `podSelector`: (Optional) A standard `metav1.LabelSelector` (e.g., `matchLabels`, `matchExpressions`). If specified, this `PodConfigMapConfig` will only apply to Pods within its namespace that match this selector. If omitted or nil, it applies to all Pods in the namespace (subject to other `PodConfigMapConfig` resources).
*   `ebpfConfig`: (Optional) eBPF program configuration for advanced monitoring and security capabilities per Pod.
*   `configMapDataTemplate`: (Planned Future Enhancement) A map where keys are ConfigMap data keys and values are Go template strings. This will allow for more complex ConfigMap data generation based on Pod metadata.

**Status (`status`)**:

*   `observedGeneration`: An integer representing the `metadata.generation` of the `PodConfigMapConfig` resource that the controller has successfully processed. This helps users and other tools understand if the controller is up-to-date with the latest changes to the CRD.

**Example `PodConfigMapConfig`**:

```yaml
apiVersion: podconfig.example.com/v1alpha1
kind: PodConfigMapConfig
metadata:
  name: app-specific-config
  namespace: my-namespace
spec:
  podSelector:
    matchLabels:
      app: my-production-app
      environment: prod
  labelsToInclude:
    - "app"
    - "version"
    - "region"
  annotationsToInclude:
    - "buildID"
    - "commitSHA"
status:
  observedGeneration: 1 # Updated by the controller
```

**Example with eBPF Configuration**:

```yaml
apiVersion: podconfig.example.com/v1alpha1
kind: PodConfigMapConfig
metadata:
  name: ebpf-monitoring-config
  namespace: my-namespace
spec:
  podSelector:
    matchLabels:
      app: my-production-app
      tier: frontend
  labelsToInclude:
    - "app"
    - "version"
    - "tier"
  annotationsToInclude:
    - "deployment.kubernetes.io/revision"
  # eBPF configuration for advanced monitoring and security
  ebpfConfig:
    # Monitor system calls per pod
    syscallMonitoring:
      enabled: true
      syscallNames: ["read", "write", "open", "close", "connect", "accept"]
    # Simple L4 firewall rules
    l4Firewall:
      enabled: true
      allowedPorts: [80, 443, 8080, 8443]
      blockedPorts: [22, 23, 3389, 1433, 3306]
      defaultAction: allow
    # Export metrics to Prometheus
    metricsExport:
      enabled: true
      updateInterval: "30s"
status:
  observedGeneration: 1 # Updated by the controller
```

## How it Works

The controller operates by watching three primary types of resources:

1.  **Pods**: For additions, updates, and deletions.
2.  **`PodConfigMapConfig` Custom Resources**: For changes in configuration that define how ConfigMaps should be generated.

**Reconciliation Logic**:

*   **Pod Events**: When a Pod is added or updated, the controller evaluates it against all `PodConfigMapConfig` resources in the Pod's namespace.
    *   For each `PodConfigMapConfig` whose `spec.podSelector` matches the Pod (or if the `podSelector` is nil), a ConfigMap is synchronized.
    *   The generated ConfigMap is named `pod-<pod-name>-from-<pcmc-name>-cfg` to ensure uniqueness and indicate its origin.
    *   The ConfigMap's data includes basic Pod information (`podName`, `namespace`, `nodeName`, `phase`) and any labels/annotations specified in the `PodConfigMapConfig`'s `labelsToInclude` and `annotationsToInclude` fields.
    *   If a Pod no longer matches a `PodConfigMapConfig`'s selector (e.g., Pod labels changed or `podSelector` in CRD changed), the corresponding ConfigMap is deleted.
*   **`PodConfigMapConfig` Events**:
    *   When a `PodConfigMapConfig` is added or updated, the controller re-evaluates all Pods in that namespace to apply the new/updated configuration. This may involve creating new ConfigMaps, updating existing ones, or deleting ConfigMaps if Pods no longer match the selector.
    *   The controller updates the `status.observedGeneration` of the `PodConfigMapConfig` after successfully processing its specification.
    *   When a `PodConfigMapConfig` is deleted, the controller attempts to delete all ConfigMaps that were generated based on it.
*   **ConfigMap Naming**: ConfigMaps are named `pod-<pod-name>-from-<pcmc-name>-cfg`, linking them to both the Pod and the specific `PodConfigMapConfig` instance that triggered their creation.
*   **Owner References**: Generated ConfigMaps have an OwnerReference pointing to the Pod they represent. This ensures that when a Pod is deleted, its associated ConfigMaps are automatically garbage-collected by Kubernetes.
*   **Labels**: Generated ConfigMaps are labeled with `podconfig.example.com/generated-by-pcmc: <pcmc-name>` and `podconfig.example.com/pod-uid: <pod-uid>` for easier identification and potential cleanup.

## eBPF Integration

The controller now supports attaching tiny eBPF programs to pods for advanced monitoring and security capabilities. This feature enables:

### **Syscall Monitoring**
- **Per-pod system call counting**: Track read, write, open, close, connect, accept, and other syscalls
- **Granular filtering**: Monitor only specific syscalls of interest
- **Real-time metrics**: Export syscall counts to Prometheus for observability

### **L4 Network Firewall**
- **Port-based filtering**: Allow/block specific TCP/UDP ports per pod
- **Default policies**: Configure default allow/block behavior
- **Traffic statistics**: Monitor allowed vs blocked connections
- **Minimal overhead**: Efficient eBPF programs with < 1µs latency impact

### **Prometheus Metrics**
- `podconfigmap_controller_ebpf_syscall_count_total` - Syscall counts per pod/PID
- `podconfigmap_controller_ebpf_l4_firewall_total` - L4 firewall statistics (allowed/blocked)
- `podconfigmap_controller_ebpf_attached_programs` - Number of attached eBPF programs
- `podconfigmap_controller_ebpf_program_errors_total` - eBPF program error rates

### **ConfigMap Integration**
Generated ConfigMaps automatically include eBPF metadata:
```yaml
data:
  podName: "my-app-12345"
  namespace: "production"
  ebpf_enabled: "true"
  ebpf_syscall_monitoring: "true"
  ebpf_l4_firewall: "true"
  ebpf_l4_firewall_default_action: "allow"
  ebpf_metrics_export: "true"
```

### **Leader Election & Failover**
- **Leader-only management**: Only the leader replica manages eBPF programs
- **Fast failover**: < 10s recovery time (configurable lease duration)
- **State reconciliation**: Automatic eBPF program re-attachment on leader change
- **Zero RPO**: Stateless eBPF programs with no data loss during failover

### **Security & Requirements**
- **Kernel requirements**: Linux kernel >= 5.4 with eBPF support
- **Required capabilities**: `SYS_ADMIN`, `NET_ADMIN`, `BPF`
- **Secure by default**: Non-root user, read-only filesystem, distroless base image
- **eBPF verifier**: Kernel-level program verification ensures safety

### **Usage Example**
```bash
# Deploy with eBPF support
kubectl apply -f examples/deployment-with-ebpf.yaml

# Create eBPF monitoring configuration
kubectl apply -f examples/ebpf-config.yaml

# View metrics
kubectl port-forward svc/podconfigmap-controller-metrics 8080:8080
curl http://localhost:8080/metrics | grep ebpf
```

For detailed eBPF configuration, deployment requirements, and troubleshooting, see [`docs/EBPF_FEATURES.md`](docs/EBPF_FEATURES.md).

## Configuration

The controller supports extensive configuration via environment variables:

### Leader Election
- `LEADER_ELECTION_ENABLED` - Enable/disable leader election (default: true)
- `LEADER_ELECTION_LEASE_DURATION` - Lease duration (default: 15s)
- `LEADER_ELECTION_RENEW_DEADLINE` - Renew deadline (default: 10s)
- `LEADER_ELECTION_RETRY_PERIOD` - Retry period (default: 2s)
- `LEADER_ELECTION_LOCK_NAME` - Lock name (default: podconfigmap-controller-lock)
- `LEADER_ELECTION_LOCK_NAMESPACE` - Lock namespace (default: default)

### Controller Settings
- `CONTROLLER_RESYNC_PERIOD` - Informer resync period (default: 10m)
- `CONTROLLER_POD_WORKERS` - Number of pod workers (default: 1)
- `CONTROLLER_PCMC_WORKERS` - Number of PCMC workers (default: 1)
- `CONTROLLER_MAX_RETRIES` - Maximum retry attempts (default: 5)
- `CONTROLLER_RECONCILIATION_TIMEOUT` - Reconciliation timeout (default: 30s)

### Logging
- `LOG_LEVEL` - Log level (default: info)
- `LOG_FORMAT` - Log format (default: text)
- `LOG_JSON_FORMAT` - Enable JSON logging (default: false)
- `DEBUG` - Enable debug logging (default: false)

### Metrics
- `METRICS_ADDR` - Metrics server address (default: :8080)

## Observability

### Metrics
The controller exposes Prometheus metrics at `/metrics`:

**Core Controller Metrics**:
- `podconfigmap_controller_configmap_operations_total` - ConfigMap operations counter
- `podconfigmap_controller_reconciliation_duration_seconds` - Reconciliation duration histogram
- `podconfigmap_controller_reconciliation_errors_total` - Reconciliation errors counter
- `podconfigmap_controller_queue_depth` - Work queue depth gauge
- `podconfigmap_controller_pcmc_status` - PCMC status gauge
- `podconfigmap_controller_active_configmaps` - Active ConfigMaps count gauge

**eBPF-Specific Metrics**:
- `podconfigmap_controller_ebpf_syscall_count_total` - Total syscalls per pod/PID
- `podconfigmap_controller_ebpf_l4_firewall_total` - L4 firewall statistics (allowed/blocked/tcp/udp)
- `podconfigmap_controller_ebpf_attached_programs` - Number of attached eBPF programs per pod
- `podconfigmap_controller_ebpf_program_errors_total` - eBPF program errors (attach/detach failures)

### Structured Logging
- Uses klog/v2 with structured key-value pairs
- Supports different log levels (info, warning, error, debug)
- Configurable output format (text/JSON)
- Contextual logging with operation details

## Getting Started

### Prerequisites

*   A running Kubernetes cluster (e.g., Minikube, Kind, Docker Desktop).
*   `kubectl` command-line tool configured to interact with your cluster.
*   **For eBPF features**: Linux kernel >= 5.4 with eBPF support enabled on cluster nodes.

### Installation

1.  **Apply the CustomResourceDefinition (CRD)**:
    ```bash
    kubectl apply -f crd/podconfigmapconfig_crd.yaml
    ```

2.  **Apply RBAC Rules (ClusterRole, ClusterRoleBinding, ServiceAccount)**:
    These allow the controller to access necessary resources.
    ```bash
    kubectl apply -f manifests/rbac.yaml
    ```
    *(Note: Ensure the ServiceAccount name in `manifests/rbac.yaml` matches the one used in the Deployment if they are in separate files or defined differently.)*

3.  **Deploy the Controller**:
    ```bash
    kubectl apply -f manifests/deployment.yaml
    ```
    *(Review `manifests/deployment.yaml` to ensure the image path points to your controller's image if you've built and pushed it to a custom registry.)*

### Usage Example

1.  **Create a `PodConfigMapConfig` resource in a namespace**:
    Save the example YAML from the CRD section above (or your own version) to a file (e.g., `my-pcmc.yaml`) and apply it:
    ```bash
    kubectl apply -f my-pcmc.yaml -n my-namespace
    ```
    *(Ensure `my-namespace` exists or create it: `kubectl create namespace my-namespace`)*

2.  **Create/Label Pods**:
    Deploy Pods in `my-namespace` that match the `podSelector` defined in your `PodConfigMapConfig`. For the example above, Pods would need the labels `app: my-production-app` and `environment: prod`.

    Example Pod:
    ```yaml
    apiVersion: v1
    kind: Pod
    metadata:
      name: my-sample-pod
      namespace: my-namespace
      labels:
        app: my-production-app
        environment: prod
        version: "1.2.3"
        region: "us-west-1"
      annotations:
        buildID: "build-12345"
        commitSHA: "abcdef123456"
    spec:
      containers:
      - name: my-container
        image: nginx
    ```
    Apply it: `kubectl apply -f my-sample-pod.yaml -n my-namespace`

3.  **Verify ConfigMap Creation**:
    Check for the generated ConfigMap:
    ```bash
    kubectl get configmap -n my-namespace pod-my-sample-pod-from-app-specific-config-cfg -o yaml
    ```
    You should see data derived from the Pod's metadata as configured by `app-specific-config`.

4.  **Verify `PodConfigMapConfig` Status**:
    ```bash
    kubectl get podconfigmapconfig app-specific-config -n my-namespace -o yaml
    ```
    Check that `status.observedGeneration` matches `metadata.generation`.

## Package Structure

The controller is organized into several packages for better maintainability:

- **`api/v1alpha1/`** - API types and CRD definitions
- **`controller/`** - Main controller logic and reconciliation
- **`pkg/ebpf/`** - eBPF program management and lifecycle
- **`pkg/logging/`** - Structured logging with klog/v2
- **`pkg/metrics/`** - Prometheus metrics for observability
- **`pkg/validation/`** - Input validation for CRDs and ConfigMaps
- **`pkg/errors/`** - Structured error handling with context
- **`pkg/config/`** - Configuration management with environment variables
- **`ebpf/`** - eBPF C programs for syscall monitoring and L4 firewall
- **`examples/`** - Example configurations and deployment manifests
- **`docs/`** - Comprehensive documentation including eBPF features
- **`main.go`** - Entry point with leader election and graceful shutdown

## Development

### Building the Controller

This project uses Go modules and includes comprehensive testing:
```bash
# Build the controller binary
make build 
# or
# go build -o bin/controller main.go

# Run tests with coverage
make test

# Format code
make fmt

# Run go vet
make vet

# Build the Docker image (see Makefile or Dockerfile for details)
make docker-build IMG=<your-registry>/podconfigmap-controller:latest
# Push the Docker image
make docker-push IMG=<your-registry>/podconfigmap-controller:latest
```

### Code Generation

If you modify the API types in `api/v1alpha1/types.go`, you might need to regenerate CRD manifests and potentially client code (if using generated clients/listers, though this controller currently uses dynamic clients and generic informers for PCMCs).

```bash
# Generate deepcopy methods for API types
make generate

# Generate CRD manifests from API types
make manifests
```

## Design Principles & Best Practices

Developing robust Kubernetes controllers requires careful consideration of API design and controller mechanics. This project aims to follow best practices, drawing inspiration from idiomatic Kubernetes controller development as discussed by community leaders like Ahmet Alp Balkan.

### CRD Design Philosophy

*   **`spec` vs. `status`**: The `PodConfigMapConfig` CRD clearly separates the desired state (`spec`) from the observed state (`status`).
    *   **`spec`**: Defined by the user, detailing *what* ConfigMaps to create and *how* (e.g., `podSelector`, `labelsToInclude`).
    *   **`status`**: Updated by the controller, reflecting *what it has observed and done*. A key field here is `status.observedGeneration`. When `status.observedGeneration == metadata.generation`, it signifies that the controller has processed the latest changes to the `spec`. This is crucial for users and other automated systems to understand if the controller's actions reflect the most recent configuration.
*   **Reusing Kubernetes Core Types**: The `spec.podSelector` field uses `metav1.LabelSelector`, a standard Kubernetes API type. This promotes consistency and allows users to leverage familiar selection mechanisms.
*   **Field Semantics**: Fields like `podSelector` are optional. If not provided, the controller has a defined default behavior (applying to all Pods in the namespace, in the context of that specific `PodConfigMapConfig`). Clear default behaviors are essential for predictable APIs.

### Controller Responsibility

*   **Single, Well-Defined Job**: This controller has a clear responsibility: to manage ConfigMaps based on Pod metadata and `PodConfigMapConfig` configurations. It doesn't try to manage Pod deployments, services, or other unrelated concerns. Its inputs are Pods and `PodConfigMapConfig`s, and its primary output is the creation/management of ConfigMaps. This follows the UNIX philosophy of tools doing one thing well.

### Reconciliation Logic

*   **Informer-Based Reads**: The controller uses informers to watch for changes to Pods and `PodConfigMapConfig` resources. This means reads (e.g., listing Pods or `PodConfigMapConfig`s) are typically served from an in-memory cache, which is efficient and reduces load on the Kubernetes API server.
*   **Event-Driven Requeues**: Changes to Pods or `PodConfigMapConfig`s trigger reconciliation for the relevant objects.
*   **Status Updates**: After processing a `PodConfigMapConfig`, its `status.observedGeneration` is updated to reflect the generation of the spec that was acted upon.

### Status Reporting: `observedGeneration`

*   As highlighted by Ahmet Alp Balkan, simply having a `Ready` condition is often not enough. The `observedGeneration` field in the `status` of `PodConfigMapConfig` is important. It tells consumers whether the current status (and by extension, the ConfigMaps managed by this CRD instance) reflects the *latest* version of the `PodConfigMapConfig`'s `spec`. If `cond.observedGeneration != metadata.generation`, the status information might be stale because the controller hasn't yet reconciled the most recent update to the CRD.

### Understanding Cached Clients

*   The controller primarily uses cached clients provided by `controller-runtime` (via informers). This means that writes (e.g., creating or updating a ConfigMap) go directly to the API server, but subsequent reads might come from the cache, which might not immediately reflect the write. The controller logic is designed with this eventual consistency in mind. For example, when creating a ConfigMap, it retries on conflict.

### Fast and Offline Reconciliation (Goal)

*   An ideal controller reconciles objects that are already in their desired state very quickly and without unnecessary API calls. This controller aims for this by:
    *   Using `observedGeneration` to avoid redundant status updates for `PodConfigMapConfig`s.
    *   Performing checks (e.g., comparing data) before updating an existing ConfigMap.
    *   Relying on cached reads for Pods and `PodConfigMapConfig`s.

### Reconcile Return Values

*   The reconciliation functions (e.g., `reconcilePod`, `reconcilePcmc`) are designed to:
    *   Return an error if a transient issue occurs, causing `controller-runtime` to requeue the item with backoff.
    *   Return `nil` on success or if no further action is needed for an item (e.g., a Pod doesn't match any selectors). `controller-runtime` handles requeueing if event sources change.

### Workqueue/Resync Mechanics

*   The controller uses separate workqueues for Pod events and `PodConfigMapConfig` events. This allows for independent processing and rate limiting if necessary. It's understood that updates made by the controller (e.g., to a ConfigMap or a `PodConfigMapConfig` status) can trigger further watch events and requeues, which is a natural part of the reconciliation loop. Periodic resyncs (a default behavior in `controller-runtime`) also ensure that all objects are eventually reconciled, even if some watch events are missed.
## Contributing

Contributions are welcome! Please feel free to submit issues, and pull requests.

## License

This project is licensed under the [Apache License 2.0](LICENSE).
