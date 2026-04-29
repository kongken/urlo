package url

import (
	"context"
	"sync"
)

// MemoryStore is an in-memory Store. Concurrency-safe within a single
// process; not suitable for multi-instance deployments.
type MemoryStore struct {
	mu      sync.RWMutex
	records map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]Record)}
}

func (m *MemoryStore) Create(_ context.Context, r *Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.records[r.Code]; exists {
		return ErrAlreadyExists
	}
	m.records[r.Code] = *r
	return nil
}

func (m *MemoryStore) Get(_ context.Context, code string) (*Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.records[code]
	if !ok {
		return nil, ErrNotFound
	}
	return &r, nil
}

func (m *MemoryStore) Delete(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[code]; !ok {
		return ErrNotFound
	}
	delete(m.records, code)
	return nil
}

func (m *MemoryStore) IncrVisit(_ context.Context, code string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[code]
	if !ok {
		return nil, ErrNotFound
	}
	r.VisitCount++
	m.records[code] = r
	return &r, nil
}
