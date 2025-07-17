package ebpf

import (
	"fmt"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

type SyscallCounterObjects struct {
	SyscallCounts *ebpf.Map `ebpf:"syscall_counts"`
	TrackedPids   *ebpf.Map `ebpf:"tracked_pids"`
	TraceSysEnter *ebpf.Program `ebpf:"trace_sys_enter"`
}

func (o *SyscallCounterObjects) Close() error {
	if o.SyscallCounts != nil {
		o.SyscallCounts.Close()
	}
	if o.TrackedPids != nil {
		o.TrackedPids.Close()
	}
	if o.TraceSysEnter != nil {
		o.TraceSysEnter.Close()
	}
	return nil
}

func LoadSyscallCounterObjects(opts *ebpf.CollectionOptions) (*SyscallCounterObjects, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock: %w", err)
	}

	spec := &ebpf.CollectionSpec{
		Maps: map[string]*ebpf.MapSpec{
			"syscall_counts": {
				Type:       ebpf.Hash,
				KeySize:    4,  // uint32
				ValueSize:  8,  // uint64
				MaxEntries: 1024,
			},
			"tracked_pids": {
				Type:       ebpf.Hash,
				KeySize:    4,  // uint32
				ValueSize:  4,  // uint32
				MaxEntries: 1024,
			},
		},
		Programs: map[string]*ebpf.ProgramSpec{
			"trace_sys_enter": {
				Type: ebpf.TracePoint,
				// In a real implementation, this would be the compiled bytecode
				// For now, we'll create a stub that can be replaced with actual compiled bytecode
				Instructions: asm.Instructions{
					asm.Return(),
				},
			},
		},
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	return &SyscallCounterObjects{
		SyscallCounts: coll.Maps["syscall_counts"],
		TrackedPids:   coll.Maps["tracked_pids"],
		TraceSysEnter: coll.Programs["trace_sys_enter"],
	}, nil
}

func (o *SyscallCounterObjects) AttachTracepoint() (link.Link, error) {
	// For now, return nil since tracepoint attachment requires proper kernel support
	// In a real implementation, you would use the actual tracepoint attachment
	return nil, fmt.Errorf("tracepoint attachment not implemented in demo")
}

func (o *SyscallCounterObjects) AddTrackedPid(pid uint32) error {
	var flag uint32 = 1
	return o.TrackedPids.Put(unsafe.Pointer(&pid), unsafe.Pointer(&flag))
}

func (o *SyscallCounterObjects) RemoveTrackedPid(pid uint32) error {
	return o.TrackedPids.Delete(unsafe.Pointer(&pid))
}

func (o *SyscallCounterObjects) GetSyscallCount(pid uint32) (uint64, error) {
	var count uint64
	err := o.SyscallCounts.Lookup(unsafe.Pointer(&pid), unsafe.Pointer(&count))
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (o *SyscallCounterObjects) GetAllSyscallCounts() (map[uint32]uint64, error) {
	result := make(map[uint32]uint64)
	var key uint32
	var value uint64
	
	iter := o.SyscallCounts.Iterate()
	for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
		result[key] = value
	}
	
	return result, iter.Err()
}

