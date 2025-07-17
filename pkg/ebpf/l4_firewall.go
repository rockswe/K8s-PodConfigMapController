package ebpf

import (
	"fmt"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

type FirewallRule struct {
	Port     uint16
	Protocol uint8
	Action   uint8
}

type L4FirewallObjects struct {
	FirewallRules      *ebpf.Map `ebpf:"firewall_rules"`
	EnabledInterfaces  *ebpf.Map `ebpf:"enabled_interfaces"`
	Stats              *ebpf.Map `ebpf:"stats"`
	L4FirewallIngress  *ebpf.Program `ebpf:"l4_firewall_ingress"`
}

func (o *L4FirewallObjects) Close() error {
	if o.FirewallRules != nil {
		o.FirewallRules.Close()
	}
	if o.EnabledInterfaces != nil {
		o.EnabledInterfaces.Close()
	}
	if o.Stats != nil {
		o.Stats.Close()
	}
	if o.L4FirewallIngress != nil {
		o.L4FirewallIngress.Close()
	}
	return nil
}

func LoadL4FirewallObjects(opts *ebpf.CollectionOptions) (*L4FirewallObjects, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock: %w", err)
	}

	spec := &ebpf.CollectionSpec{
		Maps: map[string]*ebpf.MapSpec{
			"firewall_rules": {
				Type:       ebpf.Hash,
				KeySize:    4,  // uint32 (rule index)
				ValueSize:  4,  // sizeof(FirewallRule)
				MaxEntries: 256,
			},
			"enabled_interfaces": {
				Type:       ebpf.Hash,
				KeySize:    4,  // uint32 (interface index)
				ValueSize:  1,  // uint8 (enabled flag)
				MaxEntries: 1024,
			},
			"stats": {
				Type:       ebpf.PerCPUArray,
				KeySize:    4,  // uint32
				ValueSize:  8,  // uint64
				MaxEntries: 4,
			},
		},
		Programs: map[string]*ebpf.ProgramSpec{
			"l4_firewall_ingress": {
				Type: ebpf.SchedCLS,
				// In a real implementation, this would be the compiled bytecode
				Instructions: asm.Instructions{
					asm.LoadImm(asm.R0, 0, asm.DWord), // BPF_MOV64_IMM(r0, 0)
					asm.Return(),                      // BPF_EXIT
				},
			},
		},
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	return &L4FirewallObjects{
		FirewallRules:     coll.Maps["firewall_rules"],
		EnabledInterfaces: coll.Maps["enabled_interfaces"],
		Stats:             coll.Maps["stats"],
		L4FirewallIngress: coll.Programs["l4_firewall_ingress"],
	}, nil
}

func (o *L4FirewallObjects) AttachTC(ifindex int) (link.Link, error) {
	// For now, return nil since TC attachment requires netlink operations
	// In a real implementation, you would use netlink to attach TC programs
	return nil, fmt.Errorf("TC attachment not implemented in demo")
}

func (o *L4FirewallObjects) AddFirewallRule(index uint32, rule FirewallRule) error {
	return o.FirewallRules.Put(unsafe.Pointer(&index), unsafe.Pointer(&rule))
}

func (o *L4FirewallObjects) RemoveFirewallRule(index uint32) error {
	return o.FirewallRules.Delete(unsafe.Pointer(&index))
}

func (o *L4FirewallObjects) EnableInterface(ifindex uint32) error {
	var flag uint8 = 1
	return o.EnabledInterfaces.Put(unsafe.Pointer(&ifindex), unsafe.Pointer(&flag))
}

func (o *L4FirewallObjects) DisableInterface(ifindex uint32) error {
	var flag uint8 = 0
	return o.EnabledInterfaces.Put(unsafe.Pointer(&ifindex), unsafe.Pointer(&flag))
}

func (o *L4FirewallObjects) GetStats() (map[string]uint64, error) {
	stats := make(map[string]uint64)
	keys := []string{"allowed", "blocked", "tcp_packets", "udp_packets"}
	
	for i, key := range keys {
		var value uint64
		idx := uint32(i)
		err := o.Stats.Lookup(unsafe.Pointer(&idx), unsafe.Pointer(&value))
		if err != nil {
			return nil, fmt.Errorf("failed to get stat %s: %w", key, err)
		}
		stats[key] = value
	}
	
	return stats, nil
}

