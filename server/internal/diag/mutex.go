package diag

import (
	"sync"
	"sync/atomic"
	"time"
)

// Mutex is a sync.Mutex that knows how long it has been held, so that the
// watchdog sees a lock nobody lets go of without trying to take it.
type Mutex struct {
	mu    sync.Mutex
	since atomic.Int64 // unix ns it was taken at; 0 while free
}

func (m *Mutex) Lock() {
	m.mu.Lock()
	m.since.Store(time.Now().UnixNano())
}

func (m *Mutex) TryLock() bool {
	if !m.mu.TryLock() {
		return false
	}
	m.since.Store(time.Now().UnixNano())
	return true
}

func (m *Mutex) Unlock() {
	m.since.Store(0)
	m.mu.Unlock()
}

// Held is how long the mutex has been held; 0 while it is free.
func (m *Mutex) Held() time.Duration { return held(&m.since) }

// RWMutex is a sync.RWMutex that knows how long it has been held for
// writing (readers are not followed).
type RWMutex struct {
	mu    sync.RWMutex
	since atomic.Int64
}

func (m *RWMutex) Lock() {
	m.mu.Lock()
	m.since.Store(time.Now().UnixNano())
}

func (m *RWMutex) TryLock() bool {
	if !m.mu.TryLock() {
		return false
	}
	m.since.Store(time.Now().UnixNano())
	return true
}

func (m *RWMutex) Unlock() {
	m.since.Store(0)
	m.mu.Unlock()
}

func (m *RWMutex) RLock()              { m.mu.RLock() }
func (m *RWMutex) TryRLock() bool      { return m.mu.TryRLock() }
func (m *RWMutex) RUnlock()            { m.mu.RUnlock() }
func (m *RWMutex) Held() time.Duration { return held(&m.since) }

func held(since *atomic.Int64) time.Duration {
	if t := since.Load(); t != 0 {
		return max(0, time.Duration(time.Now().UnixNano()-t))
	}
	return 0
}
