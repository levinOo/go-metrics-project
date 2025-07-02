package repository

import "sync"

type (
	Gauge   float64
	Counter int64
)

type MemStorage struct {
	mu       *sync.Mutex
	Gauges   map[string]Gauge
	Counters map[string]Counter
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		mu:       &sync.Mutex{},
		Gauges:   make(map[string]Gauge),
		Counters: make(map[string]Counter),
	}
}

func (m *MemStorage) SetGauge(name string, value Gauge) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Gauges[name] = value
}

func (m *MemStorage) GetGauge(name string) (Gauge, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Gauges[name]
	return val, ok
}

func (m *MemStorage) SetCounter(name string, value Counter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters[name] += value
}

func (m *MemStorage) GetCounter(name string) (Counter, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Counters[name]
	return val, ok
}
