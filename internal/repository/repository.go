package repository

import (
	"errors"
	"sync"

	"github.com/levinOo/go-metrics-project/internal/models"
)

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

func (m *MemStorage) GetGauge(name string) (val Gauge, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Gauges[name]
	if !ok {
		err = errors.New("failed to get metric correctly")
	}
	return val, err
}

func (m *MemStorage) SetCounter(name string, value Counter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters[name] += value
}

func (m *MemStorage) GetCounter(name string) (val Counter, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Counters[name]
	if !ok {
		err = errors.New("failed to get metric correctly")
	}
	return val, err
}

func (m *MemStorage) GetAll() []models.Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []models.Metrics

	for name, val := range m.Counters {
		v := int64(val)
		list = append(list, models.Metrics{
			ID:    name,
			MType: "counter",
			Delta: &v,
		})
	}

	for name, val := range m.Gauges {
		v := float64(val)
		list = append(list, models.Metrics{
			ID:    name,
			MType: "gauge",
			Value: &v,
		})
	}

	return list
}
