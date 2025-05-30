# PodConfigMapController

## Overview

The K8s Pod ConfigMap Controller is a Kubernetes controller that dynamically generates and manages ConfigMaps based on the metadata of running Pods. Its behavior is configured through a Custom Resource Definition (CRD) called `PodConfigMapConfig`, allowing users to specify which Pods to target and what information to include in the generated ConfigMaps.

This controller helps in scenarios where you need Pod-specific information to be available as a ConfigMap, for example, to be consumed by other applications, monitoring systems, or for easier debugging and visibility into Pod metadata.

## Features

*   **Dynamic ConfigMap Generation**: Automatically creates, updates, and deletes ConfigMaps based on Pod lifecycle and `PodConfigMapConfig` resources.
*   **CRD Driven Configuration**: Uses the `PodConfigMapConfig` Custom Resource to define behavior per namespace.
*   **Selective Pod Targeting**: Utilizes `podSelector` within the `PodConfigMapConfig` CRD to precisely target which Pods should have ConfigMaps generated for them.
*   **Metadata Inclusion**: Allows specifying which Pod labels and annotations should be included in the ConfigMap data.
*   **Status Reporting**: The `PodConfigMapConfig` resource reports its `observedGeneration` in its status, indicating if the controller has processed the latest specification.
*   **Namespace-Scoped**: Operates within specific namespaces, allowing for different configurations across a cluster.

## Custom Resource Definition (CRD): `PodConfigMapConfig`

The `PodConfigMapConfig` CRD is central to configuring the controller's behavior within a namespace.

**Purpose**: It defines rules for how ConfigMaps should be generated for Pods in the same namespace as the `PodConfigMapConfig` resource. Multiple `PodConfigMapConfig` resources can exist in a namespace, each potentially targeting different sets of Pods.

**Specification (`spec`)**:

*   `labelsToInclude`: (Optional) An array of strings. Each string is a Pod label key. If a Pod has any of these labels, the key-value pair will be included in the generated ConfigMap's data, prefixed with `label_`.
*   `annotationsToInclude`: (Optional) An array of strings. Each string is a Pod annotation key. If a Pod has any of these annotations, the key-value pair will be included in the generated ConfigMap's data, prefixed with `annotation_`.
*   `podSelector`: (Optional) A standard `metav1.LabelSelector` (e.g., `matchLabels`, `matchExpressions`). If specified, this `PodConfigMapConfig` will only apply to Pods within its namespace that match this selector. If omitted or nil, it applies to all Pods in the namespace (subject to other `PodConfigMapConfig` resources).
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

## Getting Started

### Prerequisites

*   A running Kubernetes cluster (e.g., Minikube, Kind, Docker Desktop).
*   `kubectl` command-line tool configured to interact with your cluster.

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

## Development

### Building the Controller

This project uses Go modules.
```bash
# Build the controller binary
make build 
# or
# go build -o bin/controller main.go

# Build the Docker image (see Makefile or Dockerfile for details)
make docker-build IMG=<your-registry>/podconfigmap-controller:latest
# Push the Docker image
make docker-push IMG=<your-registry>/podconfigmap-controller:latest
```

### Code Generation

If you modify the API types in `api/v1alpha1/types.go`, you might need to regenerate CRD manifests and potentially client code (if using generated clients/listers, though this controller currently uses dynamic clients and generic informers for PCMCs).

```bash
# (If using controller-gen)
# make manifests
# make generate
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

## Future Enhancements

*   **ConfigMap Data Templating**: Allow `PodConfigMapConfig.spec.configMapDataTemplate` to define Go templates for the `data` section of the generated ConfigMap, enabling highly flexible configuration generation based on Pod metadata.
*   **Enhanced Status Conditions**: Beyond `observedGeneration`, implement a standard `status.conditions` array on `PodConfigMapConfig` to provide more detailed status reporting (e.g., "Ready", "ErrorInPodSelector").
*   **Validation Webhooks**: Add validating admission webhooks for the `PodConfigMapConfig` CRD to provide earlier feedback on invalid configurations (e.g., malformed `podSelector` or template syntax errors).
*   **Metrics**: Expose Prometheus metrics for controller operations (e.g., number of ConfigMaps managed, reconciliation latency, errors).

## Contributing

Contributions are welcome! Please feel free to submit issues, and pull requests.

## License

This project is licensed under the [Apache License 2.0](LICENSE).
