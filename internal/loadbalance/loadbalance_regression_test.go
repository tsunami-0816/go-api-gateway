package loadbalance

import (
	"testing"
	"time"

	"github.com/go-api-gateway/gateway/internal/registry"
)

// 修复Bug-REGRESSION-001 验证一致性哈希Select()不再锁重入死锁
func TestConsistentHashSelect_NoDeadlock(t *testing.T) {
	ch := NewConsistentHash()
	instances := []*registry.Instance{
		{ID: "inst-1", Address: "127.0.0.1:9001", Weight: 1},
		{ID: "inst-2", Address: "127.0.0.1:9002", Weight: 1},
	}

	// 用带超时的channel验证Select()不会永久阻塞
	done := make(chan struct{})
	go func() {
		// 首次调用 ring 为空，必然触发 needRebuild 和建环
		_, err := ch.Select(instances, "user-001")
		if err != nil {
			t.Errorf("Select() returned error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// 正常返回，死锁已修复
	case <-time.After(2 * time.Second):
		t.Fatal("Select() 阻塞超过2秒，疑似锁重入死锁（REGRESSION-001）")
	}
}

// 修复Bug-REGRESSION-001 验证一致性哈希粘性：相同key始终路由到同一实例
func TestConsistentHashSelect_StickySession(t *testing.T) {
	ch := NewConsistentHash()
	instances := []*registry.Instance{
		{ID: "inst-1", Address: "127.0.0.1:9001", Weight: 1},
		{ID: "inst-2", Address: "127.0.0.1:9002", Weight: 1},
		{ID: "inst-3", Address: "127.0.0.1:9003", Weight: 1},
	}

	first, err := ch.Select(instances, "user-abc")
	if err != nil {
		t.Fatalf("first Select() error: %v", err)
	}

	// 连续调用100次，相同key必须返回同一实例
	for i := 0; i < 100; i++ {
		got, err := ch.Select(instances, "user-abc")
		if err != nil {
			t.Fatalf("Select() iteration %d error: %v", i, err)
		}
		if got.ID != first.ID {
			t.Errorf("粘性会话失效：第%d次返回实例%s，首次返回%s", i, got.ID, first.ID)
		}
	}
}

// 修复Bug-REGRESSION-001 验证实例列表变化时自动重建环
func TestConsistentHashSelect_AutoRebuildOnInstanceChange(t *testing.T) {
	ch := NewConsistentHash()
	instancesV1 := []*registry.Instance{
		{ID: "inst-1", Address: "127.0.0.1:9001", Weight: 1},
		{ID: "inst-2", Address: "127.0.0.1:9002", Weight: 1},
	}

	// 首次建环
	_, err := ch.Select(instancesV1, "key-1")
	if err != nil {
		t.Fatalf("first Select() error: %v", err)
	}

	// 实例列表变化（新增inst-3）
	instancesV2 := append(instancesV1, &registry.Instance{ID: "inst-3", Address: "127.0.0.1:9003", Weight: 1})

	done := make(chan struct{})
	go func() {
		_, err := ch.Select(instancesV2, "key-1")
		if err != nil {
			t.Errorf("Select after instance change error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// 重建环后正常返回
	case <-time.After(2 * time.Second):
		t.Fatal("实例变化后重建环时阻塞，疑似死锁")
	}
}

// 修复Bug-REGRESSION-001 验证单实例场景直接返回，不走环逻辑
func TestConsistentHashSelect_SingleInstance(t *testing.T) {
	ch := NewConsistentHash()
	single := []*registry.Instance{
		{ID: "only", Address: "127.0.0.1:9001", Weight: 5},
	}

	got, err := ch.Select(single, "any-key")
	if err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	if got.ID != "only" {
		t.Errorf("单实例应直接返回instances[0]，实际返回%s", got.ID)
	}
}

// 修复Bug-REGRESSION-001 验证公开方法UpdateRing()仍可独立调用且不死锁
func TestConsistentHash_UpdateRingPublicAPI(t *testing.T) {
	ch := NewConsistentHash()
	instances := []*registry.Instance{
		{ID: "inst-1", Address: "127.0.0.1:9001", Weight: 1},
		{ID: "inst-2", Address: "127.0.0.1:9002", Weight: 1},
	}

	done := make(chan struct{})
	go func() {
		ch.UpdateRing(instances)
		close(done)
	}()

	select {
	case <-done:
		// 公开API正常返回
	case <-time.After(2 * time.Second):
		t.Fatal("UpdateRing() 公开方法阻塞，疑似锁问题")
	}

	// UpdateRing后Select应直接使用已建环，不再触发重建
	ch.mu.RLock()
	ringLen := len(ch.ring)
	ch.mu.RUnlock()
	if ringLen == 0 {
		t.Error("UpdateRing() 后 ring 仍为空")
	}
}
