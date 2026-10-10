# 《go-api-gateway 第二轮回归测试 Bug 修复报告》

**修复日期**：2026-10-05
**修复版本**：第二轮回归测试后版本（dev）
**修复人**：资深开发工程师
**编译验证**：`go build ./...` exit 0，`go vet ./internal/...` exit 0
**测试验证**：`go test -v -timeout 30s ./internal/loadbalance/...` 5/5 PASS

---

## 一、修改影响分析

### 本轮需修复的 P0/P1 级 Bug

| 编号 | 严重度 | 描述 | 根因 |
|---|---|---|---|
| **REGRESSION-001** | P1 | 一致性哈希 `Select()` 锁重入死锁，所有 `consistent_hash` 路由请求超时（同时导致 BUG-P1-004 部分通过、BUG-P1-005 未通过） | `Select()` 持有 `ch.mu.Lock()` 时调用 `UpdateRing()`，而 `UpdateRing()` 内部再次 `ch.mu.Lock()`；Go 的 `sync.RWMutex` 不可重入，goroutine 永久阻塞直至请求超时 |

### 每个 Bug 的修改点 / 涉及文件 / 影响范围

**REGRESSION-001**

- **修改方案**：采用回归测试报告推荐方案 A——提取不带锁的内部方法 `buildRingLocked()`，`Select()` 持锁时直接调用；`UpdateRing()` 作为公开方法保留锁并委托调用。
- **涉及文件**：`internal/loadbalance/loadbalance.go`
- **涉及函数**：
  - 新增 `ConsistentHash.buildRingLocked(instances)` —— 纯建环逻辑，调用方须已持有写锁
  - 修改 `ConsistentHash.UpdateRing(instances)` —— 加锁 → 调用 `buildRingLocked()` → 解锁
  - 修改 `ConsistentHash.Select()` 的 `needRebuild` 分支 —— `ch.UpdateRing(instances)` → `ch.buildRingLocked(instances)`
- **可能影响的其他功能**：
  - 所有 `load_balance=consistent_hash` 路由恢复正常（修复目标）
  - WRR 策略完全不受影响（独立 `wrrLB` 实例，无锁交叉）
  - `UpdateRing()` 公开 API 签名与行为不变，外部调用者无感知
  - 单实例场景 `Select()` 提前返回 `instances[0]`，行为不变

---

## 二、修改后的代码片段

**文件**：`internal/loadbalance/loadbalance.go`

### 片段 1：`UpdateRing()` 改为委托给内部无锁方法

```go
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
```

### 片段 2：`Select()` 的 `needRebuild` 分支调用无锁内部方法

```go
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
```

> 其余代码（`Select()` 读锁查询环、`buildInstanceKey`、`murmur3Hash`、`WeightedRoundRobin` 等）均未改动。

---

## 三、自测说明

### 每个 Bug 的验证方法

**REGRESSION-001（一致性哈希锁重入死锁）**

| 验证项 | 命令 / 方法 | 预期结果 | 实际结果 |
|---|---|---|---|
| 编译验证 | `go build ./...` | exit 0 | ✅ exit 0 |
| 静态检查 | `go vet ./internal/...` | exit 0 | ✅ exit 0 |
| 死锁消除 | `TestConsistentHashSelect_NoDeadlock`：首次调用 `Select()`（ring 为空，必然触发建环），用 2s 超时 channel 检测是否阻塞 | 2s 内正常返回 | ✅ PASS（0.00s 返回） |
| 粘性会话 | `TestConsistentHashSelect_StickySession`：相同 requestKey 连续调用 100 次 | 始终返回同一实例 ID | ✅ PASS |
| 实例变化自动重建 | `TestConsistentHashSelect_AutoRebuildOnInstanceChange`：首次建环后新增实例再次调用 `Select()` | 2s 内正常返回，不阻塞 | ✅ PASS |
| 单实例短路 | `TestConsistentHashSelect_SingleInstance`：仅 1 个实例时调用 `Select()` | 直接返回 `instances[0]`，不走环逻辑 | ✅ PASS |
| 公开 API 兼容 | `TestConsistentHash_UpdateRingPublicAPI`：独立调用 `UpdateRing()` 后检查 ring 非空 | 正常返回且 ring 已构建 | ✅ PASS |

**测试执行命令**：

```bash
cd D:\lg\project\go-api-gateway
go test -v -timeout 30s ./internal/loadbalance/...
```

**测试结果**：5/5 全部 PASS，耗时 0.465s。

### 端到端验证建议（需运行网关）

修复后可按第二轮回归测试报告中的复现步骤验证：

1. 启动网关 + 测试后端（9001、9002）
2. 创建路由 `path=/api/chash, load_balance=consistent_hash, hash_header=X-User-ID`
3. 发送 `GET http://127.0.0.1:8080/api/chash -H "X-User-ID: user-123"`
4. **预期**：快速返回 200（修复前挂起 5s 后超时），相同 `X-User-ID` 始终路由到同一后端实例

---

## 四、修复总结

- 本轮仅修改 `internal/loadbalance/loadbalance.go` 一个文件
- 通过提取无锁内部方法 `buildRingLocked()` 消除了 `Select()` → `UpdateRing()` 的锁重入死锁
- 未触碰清单外任何逻辑，未做任何重构或优化
- 新增回归测试文件 `internal/loadbalance/loadbalance_regression_test.go`（5 个测试用例）
- 编译、vet、5 项回归测试全部通过

*报告结束*
