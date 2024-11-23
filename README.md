# PodConfigMapController (NOT WORKING - IN MAINTENANCE)

A custom Kubernetes controller written in Go that watches for Pod events and automatically creates or deletes ConfigMaps containing their metadata. The controller supports:

- **Pod Events**: Handles create, update, and delete events for Pods.
- **ConfigMap Management**: Creates and deletes ConfigMaps corresponding to Pods.
- **Custom Resource Definition (CRD)**: Integrates a CRD for dynamic behavior customization.
- **Leader Election**: Ensures only one active controller instance operates at a time.
- **Prometheus Metrics**: Exposes metrics for monitoring via Prometheus.

## Table of Contents

- [Introduction](#introduction)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Usage](#usage)
- [Custom Resource Definition (CRD)](#custom-resource-definition-crd)
- [Leader Election](#leader-election)
- [Prometheus Metrics](#prometheus-metrics)
- [Contributing](#contributing)
- [License](#license)

## Introduction

PodConfigMapController automates the management of ConfigMaps based on Pod metadata. It listens to Pod events across all namespaces and ensures that a corresponding ConfigMap is created or deleted when a Pod is created or deleted. The controller can be customized using a CRD and supports leader election for high availability.

## Prerequisites

- **Go**: Version 1.19 or higher.
- **Kubernetes Cluster**: Version 1.28 or compatible.
- **Kubectl**: Configured to communicate with your cluster.
- **Docker**: For containerizing the controller (optional).
- **Kubernetes Code Generator**: If you plan to modify or extend the CRD functionality.

## Installation

### Clone the Repository

```bash
git clone https://github.com/yourusername/PodConfigMapController.git
cd PodConfigMapController
```

### Build The Controller
Ensure you have Go installed and your GOPATH is set up.
```bash
go build -o podconfigmapcontroller main.go
```

### Run the Controller
You can run the controller locally for development purposes or for testing.

#### Running Locally with Kubeconfig
```bash
./podconfigmapcontroller --kubeconfig=/path/to/your/kubeconfig
```
P.S: Replace /path/to/your/kubeconfig with the path to your Kubernetes configuration file. If you're running this within a cluster or the default kubeconfig is sufficient, you can omit the --kubeconfig flag.

#### Running Inside the Cluster
```bash
./podconfigmapcontroller
```
When running inside the cluster, the controller can use the in-cluster configuration. Ensure that the ServiceAccount running the controller has the necessary permissions.

### Containerize and Deploy to Kubernetes
#### Build Docker Image
Build Dockerfile first in your project dir
```bash
FROM golang:1.19 as builder

WORKDIR /app

# Copy the Go modules manifest
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the controller binary
RUN go build -o podconfigmapcontroller main.go

# Create a minimal image
FROM alpine:latest

# Set working directory
WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/podconfigmapcontroller .

# Command to run when starting the container
CMD ["./podconfigmapcontroller"]
```
Build the Docker image:
```bash
docker build -t yourdockerhubusername/podconfigmapcontroller:latest .
```
Push Docker Image
```bash
docker push yourdockerhubusername/podconfigmapcontroller:latest
```

### Deploy to Kubernetes
deployment.yaml (e.g.)
```bash
apiVersion: apps/v1
kind: Deployment
metadata:
  name: podconfigmapcontroller
spec:
  replicas: 1
  selector:
    matchLabels:
      app: podconfigmapcontroller
  template:
    metadata:
      labels:
        app: podconfigmapcontroller
    spec:
      serviceAccountName: podconfigmapcontroller-sa
      containers:
        - name: podconfigmapcontroller
          image: yourdockerhubusername/podconfigmapcontroller:latest
          args:
            - --kubeconfig=
          ports:
            - containerPort: 8080
              name: metrics
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: podconfigmapcontroller-sa
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: podconfigmapcontroller-role
rules:
  - apiGroups: [""]
    resources: ["pods", "configmaps"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "create", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: podconfigmapcontroller-rolebinding
subjects:
  - kind: ServiceAccount
    name: podconfigmapcontroller-sa
    namespace: default
roleRef:
  kind: ClusterRole
  name: podconfigmapcontroller-role
  apiGroup: rbac.authorization.k8s.io
```
Apply the deployment:
```bash
kubectl apply -f deployment.yaml
```
Verify that pods are running:
```bash
kubectl get pods
```

### Usage
Once the controller is running, it will automatically watch for Pod events and manage ConfigMaps accordingly. You can test this by creating and deleting Pods:
```bash
kubectl run test-pod --image=nginx
```
Check the logs of the controller to see if it has created a ConfigMap:
```bash
kubectl logs deployment/podconfigmapcontroller
```
