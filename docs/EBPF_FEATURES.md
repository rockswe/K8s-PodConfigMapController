# eBPF Features Documentation

## Overview

The PodConfigMapController now supports attaching eBPF programs to pods for advanced monitoring and security capabilities. This feature allows you to:

- **Monitor syscalls** per pod with granular counting
- **Implement L4 firewall rules** for network traffic control
- **Export metrics** to Prometheus for observability
- **Maintain minimal image footprint** with distroless base image

## Architecture

### eBPF Program Types

1. **Syscall Counter**: Attaches to `raw_syscalls/sys_enter` tracepoint to count system calls per tracked PID
2. **L4 Firewall**: Attaches to TC ingress to filter TCP/UDP traffic based on ports and protocols

### Key Components

- **eBPF Manager** (`pkg/ebpf/manager.go`): Manages lifecycle of eBPF programs per pod
- **Controller Integration**: Extends existing reconciliation loops with eBPF program management
- **Prometheus Metrics**: Exports eBPF data as Prometheus metrics

## Configuration

### CRD Extensions

The `PodConfigMapConfig` CRD now supports an `ebpfConfig` field:

```yaml
apiVersion: podconfig.example.com/v1alpha1
kind: PodConfigMapConfig
metadata:
  name: example-ebpf-config
spec:
  podSelector:
    matchLabels:
      app: my-app
  ebpfConfig:
    syscallMonitoring:
      enabled: true
      syscallNames: ["read", "write", "open", "close"]
    l4Firewall:
      enabled: true
      allowedPorts: [80, 443, 8080]
      blockedPorts: [22, 23, 3389]
      defaultAction: allow
    metricsExport:
      enabled: true
      updateInterval: "30s"
```

### Configuration Fields

#### SyscallMonitoring
- `enabled`: Enable/disable syscall monitoring
- `syscallNames`: List of syscall names to monitor (optional, defaults to all)

#### L4Firewall
- `enabled`: Enable/disable L4 firewall
- `allowedPorts`: List of ports to explicitly allow
- `blockedPorts`: List of ports to explicitly block
- `defaultAction`: Default action for unmatched traffic (`allow` or `block`)

#### MetricsExport
- `enabled`: Enable/disable metrics export
- `updateInterval`: How often to collect metrics from eBPF maps

## Metrics

The controller exports the following eBPF-specific Prometheus metrics:

### Syscall Metrics
```
# Total syscalls per pod
podconfigmap_controller_ebpf_syscall_count_total{namespace="default",pod_name="app-pod",pid="1234"} 1500
```

### L4 Firewall Metrics
```
# L4 firewall statistics
podconfigmap_controller_ebpf_l4_firewall_total{namespace="default",pod_name="app-pod",stat_type="allowed"} 100
podconfigmap_controller_ebpf_l4_firewall_total{namespace="default",pod_name="app-pod",stat_type="blocked"} 5
```

### Program Status Metrics
```
# Number of attached eBPF programs
podconfigmap_controller_ebpf_attached_programs{namespace="default",pod_name="app-pod",program_type="syscall_counter"} 1
podconfigmap_controller_ebpf_attached_programs{namespace="default",pod_name="app-pod",program_type="l4_firewall"} 1
```

### Error Metrics
```
# eBPF program errors
podconfigmap_controller_ebpf_program_errors_total{namespace="default",pod_name="app-pod",program_type="syscall_counter",error_type="attach_failed"} 1
```

## Leader Election & Fail-over

### Leader Election Behavior

The controller maintains the same robust leader election mechanism with eBPF extensions:

1. **eBPF Manager Lifecycle**: 
   - Only the leader instance manages eBPF programs
   - eBPF programs are attached/detached based on leader state
   - Non-leader instances remain in standby mode

2. **Fail-over Process**:
   - When leader fails, new leader takes over within lease renewal deadline (default: 10s)
   - eBPF programs are automatically re-attached to tracked pods
   - Metrics collection continues without interruption

3. **State Synchronization**:
   - eBPF program state is reconciled on leader election
   - Orphaned eBPF programs are cleaned up
   - Pod tracking is restored from Kubernetes API

### Configuration for Leader Election

```yaml
env:
- name: LEADER_ELECTION_ENABLED
  value: "true"
- name: LEADER_ELECTION_LEASE_DURATION
  value: "15s"
- name: LEADER_ELECTION_RENEW_DEADLINE
  value: "10s"
- name: LEADER_ELECTION_RETRY_PERIOD
  value: "2s"
```

### Fail-over Guarantees

- **RTO (Recovery Time Objective)**: < 10 seconds (lease renewal deadline)
- **RPO (Recovery Point Objective)**: 0 (stateless eBPF programs)
- **Consistency**: Strong consistency via Kubernetes API and etcd
- **Availability**: High availability with multi-replica deployment

## Security Considerations

### Required Capabilities

The controller requires the following Linux capabilities to load eBPF programs:

```yaml
securityContext:
  capabilities:
    add:
    - SYS_ADMIN
    - NET_ADMIN
    - BPF
```

### Minimal Attack Surface

- **Distroless base image**: Reduces attack surface with minimal runtime dependencies
- **Non-root user**: Runs as non-root user (uid 65532)
- **Read-only filesystem**: Container filesystem is read-only
- **eBPF sandboxing**: eBPF programs run in kernel space with BPF verifier protection

## Deployment

### Prerequisites

- Kubernetes cluster with kernel >= 5.4
- Nodes with eBPF support enabled
- Container runtime with privileged container support

### Deployment Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: podconfigmap-controller
spec:
  replicas: 2
  selector:
    matchLabels:
      app: podconfigmap-controller
  template:
    metadata:
      labels:
        app: podconfigmap-controller
    spec:
      serviceAccountName: podconfigmap-controller
      containers:
      - name: controller
        image: podconfigmap-controller:latest
        securityContext:
          runAsNonRoot: true
          runAsUser: 65532
          readOnlyRootFilesystem: true
          capabilities:
            add:
            - SYS_ADMIN
            - NET_ADMIN
            - BPF
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "200m"
        ports:
        - containerPort: 8080
          name: metrics
        livenessProbe:
          httpGet:
            path: /metrics
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /metrics
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

### RBAC Requirements

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: podconfigmap-controller
rules:
- apiGroups: [""]
  resources: ["pods", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["podconfig.example.com"]
  resources: ["podconfigmapconfigs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## Monitoring & Observability

### Prometheus Scraping

```yaml
apiVersion: v1
kind: ServiceMonitor
metadata:
  name: podconfigmap-controller
spec:
  selector:
    matchLabels:
      app: podconfigmap-controller
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### Grafana Dashboard

Key metrics to monitor:
- eBPF program attachment success rate
- Syscall counts per pod
- L4 firewall block/allow ratios
- Controller leader election status
- Memory usage (important for eBPF maps)

### Alerting Rules

```yaml
groups:
- name: podconfigmap-controller-ebpf
  rules:
  - alert: EBPFProgramAttachFailure
    expr: rate(podconfigmap_controller_ebpf_program_errors_total[5m]) > 0
    labels:
      severity: warning
    annotations:
      summary: "eBPF program attachment failures detected"
      
  - alert: HighSyscallRate
    expr: rate(podconfigmap_controller_ebpf_syscall_count_total[1m]) > 1000
    labels:
      severity: warning
    annotations:
      summary: "High syscall rate detected for pod {{ $labels.pod_name }}"
```

## Troubleshooting

### Common Issues

1. **eBPF program load failure**:
   - Check kernel version (>= 5.4 required)
   - Verify BPF capability is granted
   - Check dmesg for BPF verifier errors

2. **Metrics not updating**:
   - Verify eBPF maps are populated
   - Check controller logs for collection errors
   - Ensure metrics export is enabled in configuration

3. **Leader election issues**:
   - Check lease resource in Kubernetes
   - Verify RBAC permissions
   - Monitor leader election metrics

### Debug Commands

```bash
# Check eBPF program status
kubectl exec -it <controller-pod> -- ls /sys/fs/bpf/

# View controller logs
kubectl logs -f deployment/podconfigmap-controller

# Check metrics endpoint
kubectl port-forward svc/podconfigmap-controller 8080:8080
curl http://localhost:8080/metrics | grep ebpf
```

## Performance Considerations

### Resource Usage

- **CPU**: eBPF programs run in kernel space with minimal CPU overhead
- **Memory**: Each eBPF map consumes kernel memory (typically < 1MB per pod)
- **Network**: TC programs add minimal latency (< 1µs per packet)

### Scaling

- Controller supports 1000+ pods per cluster
- eBPF programs scale linearly with pod count
- Metrics collection interval can be tuned for large deployments

### Optimization

- Use appropriate map sizes in eBPF programs
- Tune metrics collection interval based on cluster size
- Consider using per-CPU maps for high-throughput scenarios