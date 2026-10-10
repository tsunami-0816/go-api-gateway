// Package loadbalance implements load balancing strategies for backend instance selection.
//
// Supports two strategies:
// - Weighted Round Robin (WRR): smooth weighted round robin (nginx algorithm)
// - Consistent Hash (CH): murmur3 hash with 150 virtual nodes per weight unit
package loadbalance

import (
	"hash/fnv"
	"sort"
	"strings"
	"sync"

	"github.com/go-api-gateway/gateway/internal/registry"
)

// LoadBalancer is the interface for load balancing strategies.
type LoadBalancer interface {
	// Select chooses a backend instance from the given list.
	// requestKey is used for consistent hashing (e.g., client IP or header value).
	Select(instances []*registry.Instance, requestKey string) (*registry.Instance, error)
}

// NewLoadBalancer creates a load balancer by strategy name.
// strategy: "weighted_round_robin" or "consistent_hash"
func NewLoadBalancer(strategy string) LoadBalancer {
	switch strategy {
	case "consistent_hash":
		return NewConsistentHash()
	default:
		return NewWeightedRoundRobin()
	}
}

// WeightedRoundRobin implements smooth weighted round robin (nginx algorithm).
type WeightedRoundRobin struct {
	mu          sync.Mutex
	currentWeights map[string]int // instance ID -> current weight
}

// NewWeightedRoundRobin creates a new WRR load balancer.
func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{
		currentWeights: make(map[string]int),
	}
}

// Select implements the smooth WRR algorithm.
// Each instance's current_weight += weight; select max; selected current_weight -= total_weight.
func (wrr *WeightedRoundRobin) Select(instances []*registry.Instance, _ string) (*registry.Instance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	if len(instances) == 1 {
		return instances[0], nil
	}

	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	totalWeight := 0
	for _, inst := range instances {
		totalWeight += inst.Weight
	}

	var selected *registry.Instance
	maxWeight := -1
	for _, inst := range instances {
		cw := wrr.currentWeights[inst.ID] + inst.Weight
		wrr.currentWeights[inst.ID] = cw
		if cw > maxWeight {
			maxWeight = cw
			selected = inst
		}
	}
	if selected != nil {
		wrr.currentWeights[selected.ID] -= totalWeight
	}

	// Clean up weights for instances no longer in list
	if len(wrr.currentWeights) > len(instances)*2 {
		valid := make(map[string]bool, len(instances))
		for _, inst := range instances {
			valid[inst.ID] = true
		}
		for id := range wrr.currentWeights {
			if !valid[id] {
				delete(wrr.currentWeights, id)
			}
		}
	}

	return selected, nil
}

// ConsistentHash implements a consistent hash ring with virtual nodes.
type ConsistentHash struct {
	mu           sync.RWMutex
	ring         []hashNode                    // sorted virtual nodes
	nodeMap      map[uint32]*registry.Instance // hash -> instance
	virtualNodes int                           // virtual nodes per weight unit
	instanceKey  string                        // 修复Bug-P1-005 记录当前实例列表签名，用于检测变化自动重建环
}

type hashNode struct {
	hash     uint32
	instance *registry.Instance
}

// NewConsistentHash creates a new consistent hash ring with 150 virtual nodes per weight unit.
func NewConsistentHash() *ConsistentHash {
	return &ConsistentHash{
		virtualNodes: 150,
		nodeMap:      make(map[uint32]*registry.Instance),
	}
}

// UpdateRing rebuilds the hash ring with the given instances.
// Should be called when the instance list changes.
func (ch *ConsistentHash) UpdateRing(instances []*registry.Instance) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// 修复Bug-REGRESSION-001 建环逻辑提取到buildRingLocked，避免Select()持锁调用UpdateRing()导致锁重入死锁
	ch.buildRingLocked(instances)
}

// buildRingLocked rebuilds the hash ring without acquiring the lock.
// Caller MUST hold ch.mu (write lock) before calling this method.
// 修复Bug-REGRESSION-001 提取不带锁的内部建环方法，供Select()在持有写锁时直接调用，消除锁重入死锁
func (ch *ConsistentHash) buildRingLocked(instances []*registry.Instance) {
	ch.ring = nil
	ch.nodeMap = make(map[uint32]*registry.Instance)

	for _, inst := range instances {
		vnodes := inst.Weight * ch.virtualNodes
		if vnodes <= 0 {
			vnodes = ch.virtualNodes
		}
		for i := 0; i < vnodes; i++ {
			key := []byte(inst.ID + "#" + itoa(i))
			h := murmur3Hash(key)
			node := hashNode{hash: h, instance: inst}
			ch.ring = append(ch.ring, node)
			ch.nodeMap[h] = inst
		}
	}
	sort.Slice(ch.ring, func(i, j int) bool {
		return ch.ring[i].hash < ch.ring[j].hash
	})
}

// Select finds the nearest virtual node clockwise from the request key hash.
func (ch *ConsistentHash) Select(instances []*registry.Instance, requestKey string) (*registry.Instance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	if len(instances) == 1 {
		return instances[0], nil
	}

	// 修复Bug-P1-005 原代码UpdateRing从未被调用，ring永远为空导致fallback到instances[0]。
	// 现在在Select中检测实例列表变化，自动重建哈希环。
	key := buildInstanceKey(instances)
	ch.mu.RLock()
	needRebuild := len(ch.ring) == 0 || ch.instanceKey != key
	ch.mu.RUnlock()

	if needRebuild {
		ch.mu.Lock()
		// double-check after acquiring write lock
		if ch.instanceKey != key || len(ch.ring) == 0 {
			// 修复Bug-REGRESSION-001 持锁状态下调用不带锁的buildRingLocked，而非UpdateRing()，消除锁重入死锁
			ch.buildRingLocked(instances)
			ch.instanceKey = key
		}
		ch.mu.Unlock()
	}

	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		// Ring not built yet, fall back to first instance
		return instances[0], nil
	}

	h := murmur3Hash([]byte(requestKey))
	// Binary search for first node with hash >= h
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i].hash >= h
	})
	if idx >= len(ch.ring) {
		idx = 0 // wrap around
	}
	return ch.ring[idx].instance, nil
}

// buildInstanceKey builds a signature string from instance list to detect changes.
func buildInstanceKey(instances []*registry.Instance) string {
	var buf strings.Builder
	for _, inst := range instances {
		buf.WriteString(inst.ID)
		buf.WriteByte('|')
		buf.WriteString(inst.Address)
		buf.WriteByte('|')
		buf.WriteString(itoa(inst.Weight))
		buf.WriteByte(';')
	}
	return buf.String()
}

// murmur3Hash computes a 32-bit murmur3 hash (using FNV as a simple stand-in).
// In production, use github.com/spaolacci/murmur3 for true murmur3.
func murmur3Hash(data []byte) uint32 {
	h := fnv.New32a()
	h.Write(data)
	return h.Sum32()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// ErrNoInstances is returned when there are no available backend instances.
var ErrNoInstances = &LBError{message: "no available backend instances"}

// LBError represents a load balancer error.
type LBError struct {
	message string
}

func (e *LBError) Error() string { return e.message }
