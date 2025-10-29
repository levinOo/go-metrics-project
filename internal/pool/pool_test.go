package pool

import (
	"testing"

	"github.com/levinOo/go-metrics-project/internal/models"
)

// TestMetricsPoolGetPut проверяет базовую работу Get/Put для Metrics
func TestMetricsPoolGetPut(t *testing.T) {
	p := New[*models.Metrics](func() *models.Metrics {
		return &models.Metrics{}
	})

	m := p.Get()
	if m == nil {
		t.Fatal("expected non-nil Metrics from pool")
	}

	delta := int64(100)
	m.ID = "test_counter"
	m.MType = "counter"
	m.Delta = &delta
	m.Hash = "abc123"

	p.Put(m)

	m2 := p.Get()
	if m2 == nil {
		t.Fatal("expected non-nil Metrics from pool after Put")
	}

	if m2.ID != "" {
		t.Errorf("expected ID to be reset, got: %s", m2.ID)
	}
	if m2.MType != "" {
		t.Errorf("expected MType to be reset, got: %s", m2.MType)
	}
	if m2.Delta != nil {
		t.Errorf("expected Delta to be nil, got: %v", *m2.Delta)
	}
	if m2.Value != nil {
		t.Errorf("expected Value to be nil, got: %v", *m2.Value)
	}
	if m2.Hash != "" {
		t.Errorf("expected Hash to be reset, got: %s", m2.Hash)
	}
}

// TestMetricsPoolEmptyPool проверяет поведение при пустом пуле
func TestMetricsPoolEmptyPool(t *testing.T) {
	p := New[*models.Metrics](func() *models.Metrics {
		return &models.Metrics{}
	})

	m1 := p.Get()
	m2 := p.Get()
	m3 := p.Get()

	if m1 == nil || m2 == nil || m3 == nil {
		t.Fatal("expected non-nil metrics from factory")
	}

	m1.ID = "m1"
	m2.ID = "m2"
	m3.ID = "m3"

	if m1.ID == m2.ID || m2.ID == m3.ID {
		t.Error("expected different objects from factory")
	}
}

// TestMetricsReset проверяет корректность работы метода Reset
func TestMetricsReset(t *testing.T) {
	m := &models.Metrics{}
	delta := int64(100)
	value := 42.5

	m.ID = "test"
	m.MType = "counter"
	m.Delta = &delta
	m.Value = &value
	m.Hash = "hash123"

	m.Reset()

	if m.ID != "" {
		t.Errorf("expected ID to be empty, got: %s", m.ID)
	}
	if m.MType != "" {
		t.Errorf("expected MType to be empty, got: %s", m.MType)
	}
	if m.Delta != nil {
		t.Errorf("expected Delta to be nil, got: %v", *m.Delta)
	}
	if m.Value != nil {
		t.Errorf("expected Value to be nil, got: %v", *m.Value)
	}
	if m.Hash != "" {
		t.Errorf("expected Hash to be empty, got: %s", m.Hash)
	}
}
