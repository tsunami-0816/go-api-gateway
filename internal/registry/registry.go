// Package registry implements etcd-based service registration and discovery.
//
// The Registry registers backend service instances with etcd using Lease (TTL=10s)
// and KeepAlive. The Discovery watches for instance changes and maintains an
// in-memory cache with atomic.Value for lock-free reads.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-api-gateway/gateway/internal/config"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Instance represents a registered backend service instance.
type Instance struct {
	ID           string            `json:"id"`
	ServiceName  string            `json:"service_name"` // 修复Bug-P0-002 增加服务名字段，自注册时记录实例归属哪个服务
	Address      string            `json:"address"`
	Weight       int               `json:"weight"`
	Version      string            `json:"version"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	RegisterTime time.Time         `json:"register_time"`
	Healthy      bool              `json:"healthy"`
}

// Registry registers a service instance with etcd.
type Registry struct {
	client    *clientv3.Client
	leaseID   clientv3.LeaseID
	instance  *Instance
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewRegistry creates a new service registry and registers the instance.
// It creates a Lease (TTL=10s), writes the instance key, and starts KeepAlive.
func NewRegistry(cfg config.EtcdConfig, instance *Instance) (*Registry, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("connect etcd: %w", err)
	}

	r := &Registry{client: client, instance: instance}
	if err := r.register(); err != nil {
		client.Close()
		return nil, err
	}
	return r, nil
}

func (r *Registry) register() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create lease with TTL=10s
	leaseResp, err := r.client.Grant(ctx, 10)
	if err != nil {
		return fmt.Errorf("grant lease: %w", err)
	}
	r.leaseID = leaseResp.ID

	// Write instance key
	// 修复Bug-P0-002 原key=/services/{version}/{id}将version放在了服务名位置，
	// 导致extractServiceName解析出的服务名是version而非真实服务名。
	// 修正为 /services/{serviceName}/{id}，与RegisterTestInstance格式统一。
	key := fmt.Sprintf("/services/%s/%s", r.instance.ServiceName, r.instance.ID)
	value, _ := json.Marshal(r.instance)
	if _, err := r.client.Put(ctx, key, string(value), clientv3.WithLease(r.leaseID)); err != nil {
		return fmt.Errorf("put instance: %w", err)
	}

	// Start KeepAlive goroutine (every ~5s)
	keepAliveCtx, keepAliveCancel := context.WithCancel(context.Background())
	r.cancel = keepAliveCancel
	ch, err := r.client.KeepAlive(keepAliveCtx, r.leaseID)
	if err != nil {
		keepAliveCancel()
		return fmt.Errorf("keep alive: %w", err)
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for range ch {
			// Lease renewed automatically
		}
	}()

	return nil
}

// Deregister revokes the lease and removes the instance from etcd.
func (r *Registry) Deregister() {
	if r.cancel != nil {
		r.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if r.leaseID != 0 {
		_, _ = r.client.Revoke(ctx, r.leaseID)
	}
	r.wg.Wait()
	r.client.Close()
}

// Discovery watches etcd for service instance changes and maintains a local cache.
type Discovery struct {
	client   *clientv3.Client
	cache    atomic.Value // map[string]map[string][]*Instance (service -> version -> instances)
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewDiscovery creates a new service discovery, performs initial full fetch,
// and starts watching for changes.
func NewDiscovery(cfg config.EtcdConfig) (*Discovery, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("connect etcd: %w", err)
	}

	d := &Discovery{client: client}
	d.cache.Store(make(map[string]map[string][]*Instance))

	// Initial full fetch
	if err := d.fullFetch(); err != nil {
		client.Close()
		return nil, err
	}

	// Start watch
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.wg.Add(1)
	go d.watch(ctx)

	return d, nil
}

func (d *Discovery) fullFetch() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := d.client.Get(ctx, "/services/", clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("fetch services: %w", err)
	}

	cache := make(map[string]map[string][]*Instance)
	for _, kv := range resp.Kvs {
		var inst Instance
		if err := json.Unmarshal(kv.Value, &inst); err != nil {
			continue
		}
		inst.Healthy = true
		svcName := extractServiceName(string(kv.Key))
		if cache[svcName] == nil {
			cache[svcName] = make(map[string][]*Instance)
		}
		cache[svcName][inst.Version] = append(cache[svcName][inst.Version], &inst)
	}
	d.cache.Store(cache)
	return nil
}

func (d *Discovery) watch(ctx context.Context) {
	defer d.wg.Done()
	ch := d.client.Watch(ctx, "/services/", clientv3.WithPrefix())
	for {
		select {
		case resp, ok := <-ch:
			if !ok {
				return
			}
			for _, ev := range resp.Events {
				d.handleEvent(ev)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) handleEvent(ev *clientv3.Event) {
	cache := d.cloneCache()
	key := string(ev.Kv.Key)
	svcName := extractServiceName(key)

	switch ev.Type {
	case clientv3.EventTypePut:
		var inst Instance
		if err := json.Unmarshal(ev.Kv.Value, &inst); err != nil {
			return
		}
		inst.Healthy = true
		if cache[svcName] == nil {
			cache[svcName] = make(map[string][]*Instance)
		}
		// Replace or append
		found := false
		for i, existing := range cache[svcName][inst.Version] {
			if existing.ID == inst.ID {
				cache[svcName][inst.Version][i] = &inst
				found = true
				break
			}
		}
		if !found {
			cache[svcName][inst.Version] = append(cache[svcName][inst.Version], &inst)
		}
	case clientv3.EventTypeDelete:
		instID := extractInstanceID(key)
		if versions, ok := cache[svcName]; ok {
			for ver, instances := range versions {
				filtered := make([]*Instance, 0, len(instances))
				for _, inst := range instances {
					if inst.ID != instID {
						filtered = append(filtered, inst)
					}
				}
				cache[svcName][ver] = filtered
			}
		}
	}
	d.cache.Store(cache)
}

func (d *Discovery) cloneCache() map[string]map[string][]*Instance {
	old := d.cache.Load().(map[string]map[string][]*Instance)
	cache := make(map[string]map[string][]*Instance, len(old))
	for svc, versions := range old {
		cache[svc] = make(map[string][]*Instance, len(versions))
		for ver, instances := range versions {
			copied := make([]*Instance, len(instances))
			copy(copied, instances)
			cache[svc][ver] = copied
		}
	}
	return cache
}

// GetInstances returns healthy instances for the given service and version.
// If version is empty, returns all versions merged.
func (d *Discovery) GetInstances(serviceName, version string) []*Instance {
	cache := d.cache.Load().(map[string]map[string][]*Instance)
	versions, ok := cache[serviceName]
	if !ok {
		return nil
	}
	var result []*Instance
	if version != "" {
		result = append(result, versions[version]...)
	} else {
		for _, instances := range versions {
			result = append(result, instances...)
		}
	}
	// Filter healthy only
	healthy := make([]*Instance, 0, len(result))
	for _, inst := range result {
		if inst.Healthy {
			healthy = append(healthy, inst)
		}
	}
	return healthy
}

// GetAllServices returns all registered service names.
func (d *Discovery) GetAllServices() []string {
	cache := d.cache.Load().(map[string]map[string][]*Instance)
	services := make([]string, 0, len(cache))
	for svc := range cache {
		services = append(services, svc)
	}
	return services
}

// RegisterTestInstance writes a test service instance directly to etcd for local debugging.
// This is an admin-plane convenience API; production services should self-register via Registry.
// The key format is /services/{serviceName}/{instanceID} with a 60s lease (no keepalive).
func (d *Discovery) RegisterTestInstance(ctx context.Context, serviceName string, inst *Instance) error {
	leaseResp, err := d.client.Grant(ctx, 60)
	if err != nil {
		return fmt.Errorf("grant lease: %w", err)
	}
	key := fmt.Sprintf("/services/%s/%s", serviceName, inst.ID)
	value, _ := json.Marshal(inst)
	if _, err := d.client.Put(ctx, key, string(value), clientv3.WithLease(leaseResp.ID)); err != nil {
		return fmt.Errorf("put instance: %w", err)
	}
	return nil
}

// DeregisterTestInstance removes a test instance from etcd by service name and instance ID.
func (d *Discovery) DeregisterTestInstance(ctx context.Context, serviceName, instanceID string) error {
	key := fmt.Sprintf("/services/%s/%s", serviceName, instanceID)
	_, err := d.client.Delete(ctx, key)
	return err
}

// Close stops watching and closes the etcd client.
func (d *Discovery) Close() {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	d.client.Close()
}

// extractServiceName extracts service name from etcd key /services/{name}/{id}.
func extractServiceName(key string) string {
	// key format: /services/{service-name}/{instance-id}
	// But we stored as /services/{version}/{id} — actually service name is the version field
	// Re-reading: key should be /services/{service-name}/{instance-id}
	// The instance.Version is stored in the value, not the key
	// Let's fix: key = /services/{serviceName}/{instanceID}
	parts := splitKey(key)
	if len(parts) >= 3 {
		return parts[1]
	}
	return ""
}

func extractInstanceID(key string) string {
	parts := splitKey(key)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func splitKey(key string) []string {
	var parts []string
	current := ""
	for _, c := range key {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
