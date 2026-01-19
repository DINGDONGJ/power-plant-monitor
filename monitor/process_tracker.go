package monitor

import (
	"sync"
	"time"

	"monitor-agent/buffer"
	"monitor-agent/types"
)

// ProcessTracker 进程变化追踪器
type ProcessTracker struct {
	mu sync.RWMutex

	// 上一次的进程快照 (PID -> ProcessInfo)
	lastSnapshot map[int32]*types.ProcessInfo

	// 进程变化事件缓冲区
	changes *buffer.RingBuffer[types.ProcessChange]

	// 首次运行标记
	firstRun bool
}

// NewProcessTracker 创建进程追踪器
func NewProcessTracker(bufferSize int) *ProcessTracker {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &ProcessTracker{
		lastSnapshot: make(map[int32]*types.ProcessInfo),
		changes:      buffer.NewRingBuffer[types.ProcessChange](bufferSize),
		firstRun:     true,
	}
}

// Update 更新进程快照，返回变化列表
func (t *ProcessTracker) Update(processes []types.ProcessInfo) []types.ProcessChange {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	currentPids := make(map[int32]bool)
	var changes []types.ProcessChange

	// 检测新进程
	for i := range processes {
		p := &processes[i]
		currentPids[p.PID] = true

		if _, exists := t.lastSnapshot[p.PID]; !exists {
			// 首次运行不报告新进程（避免启动时大量事件）
			if !t.firstRun {
				change := types.ProcessChange{
					Timestamp: now,
					Type:      "new",
					PID:       p.PID,
					Name:      p.Name,
					Cmdline:   p.Cmdline,
				}
				changes = append(changes, change)
				t.changes.Push(change)
			}
		}
		// 更新快照
		t.lastSnapshot[p.PID] = p
	}

	// 检测消失的进程
	for pid, p := range t.lastSnapshot {
		if !currentPids[pid] {
			change := types.ProcessChange{
				Timestamp: now,
				Type:      "gone",
				PID:       pid,
				Name:      p.Name,
				Cmdline:   p.Cmdline,
			}
			changes = append(changes, change)
			t.changes.Push(change)
			delete(t.lastSnapshot, pid)
		}
	}

	t.firstRun = false
	return changes
}

// GetRecentChanges 获取最近的进程变化
func (t *ProcessTracker) GetRecentChanges(n int) []types.ProcessChange {
	return t.changes.GetRecent(n)
}

// GetSnapshot 获取当前进程快照
func (t *ProcessTracker) GetSnapshot() map[int32]*types.ProcessInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snapshot := make(map[int32]*types.ProcessInfo)
	for pid, p := range t.lastSnapshot {
		snapshot[pid] = p
	}
	return snapshot
}
