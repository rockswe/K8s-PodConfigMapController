package ebpf

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf/link"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	v1alpha1 "github.com/rockswe/K8s-PodConfigMapController/api/v1alpha1"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/logging"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/metrics"
)

type Manager struct {
	kubeClient       kubernetes.Interface
	logger           *logging.Logger
	podPrograms      map[string]*PodProgram
	podProgramsMutex sync.RWMutex
	metricsEnabled   bool
	metricsInterval  time.Duration
	stopCh           chan struct{}
}

type PodProgram struct {
	PodUID            string
	PodName           string
	PodNamespace      string
	SyscallCounter    *SyscallCounterObjects
	L4Firewall        *L4FirewallObjects
	SyscallLink       link.Link
	L4FirewallLink    link.Link
	Config            *v1alpha1.EBPFConfig
	ContainerPids     []uint32
}

func NewManager(kubeClient kubernetes.Interface) *Manager {
	return &Manager{
		kubeClient:      kubeClient,
		logger:          logging.NewLogger("ebpf-manager"),
		podPrograms:     make(map[string]*PodProgram),
		metricsEnabled:  false,
		metricsInterval: 30 * time.Second,
		stopCh:          make(chan struct{}),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting eBPF manager")
	
	// Start metrics collection goroutine
	go m.metricsCollector(ctx)
	
	<-ctx.Done()
	m.logger.Info("Stopping eBPF manager")
	close(m.stopCh)
	
	// Clean up all programs
	m.podProgramsMutex.Lock()
	defer m.podProgramsMutex.Unlock()
	
	for podUID, program := range m.podPrograms {
		if err := m.cleanupPodProgram(program); err != nil {
			m.logger.Error(err, "Failed to cleanup pod program", "podUID", podUID)
		}
	}
	
	return nil
}

func (m *Manager) AttachToPod(ctx context.Context, pod *v1.Pod, config *v1alpha1.EBPFConfig) error {
	podUID := string(pod.UID)
	
	m.podProgramsMutex.Lock()
	defer m.podProgramsMutex.Unlock()
	
	// Check if already attached
	if _, exists := m.podPrograms[podUID]; exists {
		m.logger.Debug("eBPF programs already attached to pod", "podUID", podUID)
		return m.updatePodProgram(ctx, pod, config)
	}
	
	m.logger.Info("Attaching eBPF programs to pod", "podNamespace", pod.Namespace, "podName", pod.Name)
	
	program := &PodProgram{
		PodUID:       podUID,
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		Config:       config,
	}
	
	// Get container PIDs
	pids, err := m.getContainerPids(ctx, pod)
	if err != nil {
		m.logger.Warning("Failed to get container PIDs, continuing without PID tracking", "error", err)
	}
	program.ContainerPids = pids
	
	// Setup syscall monitoring if enabled
	if config.SyscallMonitoring != nil && config.SyscallMonitoring.Enabled {
		if err := m.setupSyscallMonitoring(program); err != nil {
			return fmt.Errorf("failed to setup syscall monitoring: %w", err)
		}
	}
	
	// Setup L4 firewall if enabled
	if config.L4Firewall != nil && config.L4Firewall.Enabled {
		if err := m.setupL4Firewall(pod, program); err != nil {
			return fmt.Errorf("failed to setup L4 firewall: %w", err)
		}
	}
	
	m.podPrograms[podUID] = program
	m.logger.Info("Successfully attached eBPF programs to pod", "podNamespace", pod.Namespace, "podName", pod.Name)
	return nil
}

func (m *Manager) DetachFromPod(ctx context.Context, pod *v1.Pod) error {
	podUID := string(pod.UID)
	
	m.podProgramsMutex.Lock()
	defer m.podProgramsMutex.Unlock()
	
	program, exists := m.podPrograms[podUID]
	if !exists {
		m.logger.Debug("No eBPF programs attached to pod", "podUID", podUID)
		return nil
	}
	
	m.logger.Info("Detaching eBPF programs from pod", "podNamespace", pod.Namespace, "podName", pod.Name)
	
	if err := m.cleanupPodProgram(program); err != nil {
		return fmt.Errorf("failed to cleanup pod program: %w", err)
	}
	
	delete(m.podPrograms, podUID)
	m.logger.Info("Successfully detached eBPF programs from pod", "podNamespace", pod.Namespace, "podName", pod.Name)
	return nil
}

func (m *Manager) updatePodProgram(ctx context.Context, pod *v1.Pod, config *v1alpha1.EBPFConfig) error {
	podUID := string(pod.UID)
	program := m.podPrograms[podUID]
	
	// Update configuration
	program.Config = config
	
	// Update container PIDs
	pids, err := m.getContainerPids(ctx, pod)
	if err != nil {
		m.logger.Warning("Failed to get container PIDs during update", "error", err)
	} else {
		program.ContainerPids = pids
		
		// Update tracked PIDs in syscall counter
		if program.SyscallCounter != nil {
			for _, pid := range pids {
				if err := program.SyscallCounter.AddTrackedPid(pid); err != nil {
					m.logger.Warning("Failed to track PID in syscall counter", "pid", pid, "error", err)
				}
			}
		}
	}
	
	return nil
}

func (m *Manager) setupSyscallMonitoring(program *PodProgram) error {
	objs, err := LoadSyscallCounterObjects(nil)
	if err != nil {
		return fmt.Errorf("failed to load syscall counter objects: %w", err)
	}
	
	program.SyscallCounter = objs
	
	// Add container PIDs to tracking
	for _, pid := range program.ContainerPids {
		if err := objs.AddTrackedPid(pid); err != nil {
			m.logger.Warning("Failed to track PID", "pid", pid, "error", err)
		}
	}
	
	// Attach tracepoint
	link, err := objs.AttachTracepoint()
	if err != nil {
		objs.Close()
		return fmt.Errorf("failed to attach syscall tracepoint: %w", err)
	}
	
	program.SyscallLink = link
	return nil
}

func (m *Manager) setupL4Firewall(pod *v1.Pod, program *PodProgram) error {
	objs, err := LoadL4FirewallObjects(nil)
	if err != nil {
		return fmt.Errorf("failed to load L4 firewall objects: %w", err)
	}
	
	program.L4Firewall = objs
	
	// Configure firewall rules
	config := program.Config.L4Firewall
	ruleIndex := uint32(0)
	
	// Add allowed ports
	for _, port := range config.AllowedPorts {
		rule := FirewallRule{
			Port:     uint16(port),
			Protocol: 6, // TCP
			Action:   0, // Allow
		}
		if err := objs.AddFirewallRule(ruleIndex, rule); err != nil {
			m.logger.Warning("Failed to add firewall rule", "port", port, "error", err)
		}
		ruleIndex++
	}
	
	// Add blocked ports
	for _, port := range config.BlockedPorts {
		rule := FirewallRule{
			Port:     uint16(port),
			Protocol: 6, // TCP
			Action:   1, // Block
		}
		if err := objs.AddFirewallRule(ruleIndex, rule); err != nil {
			m.logger.Warning("Failed to add firewall rule", "port", port, "error", err)
		}
		ruleIndex++
	}
	
	// Get network interface for the pod (simplified - using eth0)
	// In a real implementation, you'd need to determine the actual veth interface
	ifindex := 2 // eth0 typically
	
	// Enable firewall on interface
	if err := objs.EnableInterface(uint32(ifindex)); err != nil {
		return fmt.Errorf("failed to enable firewall on interface: %w", err)
	}
	
	// Attach TC program (this is a simplified example)
	// In practice, you'd need to attach to the pod's network namespace
	link, err := objs.AttachTC(ifindex)
	if err != nil {
		objs.Close()
		return fmt.Errorf("failed to attach L4 firewall TC program: %w", err)
	}
	
	program.L4FirewallLink = link
	return nil
}

func (m *Manager) cleanupPodProgram(program *PodProgram) error {
	var lastErr error
	
	// Detach syscall monitoring
	if program.SyscallLink != nil {
		if err := program.SyscallLink.Close(); err != nil {
			m.logger.Error(err, "Failed to close syscall link")
			lastErr = err
		}
	}
	
	if program.SyscallCounter != nil {
		if err := program.SyscallCounter.Close(); err != nil {
			m.logger.Error(err, "Failed to close syscall counter")
			lastErr = err
		}
	}
	
	// Detach L4 firewall
	if program.L4FirewallLink != nil {
		if err := program.L4FirewallLink.Close(); err != nil {
			m.logger.Error(err, "Failed to close L4 firewall link")
			lastErr = err
		}
	}
	
	if program.L4Firewall != nil {
		if err := program.L4Firewall.Close(); err != nil {
			m.logger.Error(err, "Failed to close L4 firewall")
			lastErr = err
		}
	}
	
	return lastErr
}

func (m *Manager) getContainerPids(ctx context.Context, pod *v1.Pod) ([]uint32, error) {
	// This is a simplified implementation
	// In a real scenario, you'd need to:
	// 1. Get container runtime info from the pod status
	// 2. Query the container runtime (containerd/docker) for PIDs
	// 3. Or use cgroup information to get PIDs
	
	var pids []uint32
	
	// For now, we'll simulate getting PIDs from container status
	// In reality, you'd need to implement proper container runtime integration
	if pod.Status.ContainerStatuses != nil {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.ContainerID != "" {
				// Extract PID from container ID (this is a simulation)
				// Real implementation would query container runtime
				parts := strings.Split(containerStatus.ContainerID, "://")
				if len(parts) == 2 {
					// This is just a placeholder - you'd need proper PID resolution
					// For example, using containerd or docker API
					m.logger.Debug("Container ID", "containerID", parts[1])
				}
			}
		}
	}
	
	return pids, nil
}

func (m *Manager) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(m.metricsInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.collectMetrics()
		}
	}
}

func (m *Manager) collectMetrics() {
	m.podProgramsMutex.RLock()
	defer m.podProgramsMutex.RUnlock()
	
	for _, program := range m.podPrograms {
		// Collect syscall metrics
		if program.SyscallCounter != nil {
			counts, err := program.SyscallCounter.GetAllSyscallCounts()
			if err != nil {
				m.logger.Error(err, "Failed to get syscall counts", "podName", program.PodName)
				continue
			}
			
			for pid, count := range counts {
				metrics.RecordEBPFSyscallCount(program.PodNamespace, program.PodName, strconv.FormatUint(uint64(pid), 10), float64(count))
			}
		}
		
		// Collect L4 firewall metrics
		if program.L4Firewall != nil {
			stats, err := program.L4Firewall.GetStats()
			if err != nil {
				m.logger.Error(err, "Failed to get L4 firewall stats", "podName", program.PodName)
				continue
			}
			
			for statName, value := range stats {
				metrics.RecordEBPFL4FirewallStats(program.PodNamespace, program.PodName, statName, float64(value))
			}
		}
	}
}

func (m *Manager) GetPodPrograms() map[string]*PodProgram {
	m.podProgramsMutex.RLock()
	defer m.podProgramsMutex.RUnlock()
	
	result := make(map[string]*PodProgram)
	for k, v := range m.podPrograms {
		result[k] = v
	}
	return result
}