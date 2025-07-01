package main

import (
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi"
)

type (
	gauge   float64
	counter int64
)

type MemStorage struct {
	mu       sync.Mutex
	Gauges   map[string]gauge
	Counters map[string]counter
}

func UpdateValueHandler(storage *MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		nameMetric := chi.URLParam(r, "metric")
		valueMetric := chi.URLParam(r, "value")
		typeMetric := chi.URLParam(r, "typeMetric")

		if nameMetric == "" {
			http.Error(rw, "Metric is empty", http.StatusNotFound)
			return
		}

		switch typeMetric {
		case "gauge":
			valueGauge, err := strconv.ParseFloat(valueMetric, 64)
			if err != nil {
				http.Error(rw, "Неверный тип метрики", http.StatusBadRequest)
				return
			}
			storage.SetGauge(nameMetric, gauge(valueGauge))
		case "counter":
			valueCounter, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(rw, "Неверный тип метрики", http.StatusBadRequest)
				return
			}
			storage.SetCounter(nameMetric, counter(valueCounter))
		default:
			http.Error(rw, "Неизвестный тип метрики", http.StatusBadRequest)
			return
		}
		rw.WriteHeader(http.StatusOK)
		_, err := rw.Write([]byte("OK"))
		if err != nil {
			log.Printf("Ошиька вывода: %v", err)
		}

	}
}

type MetricsStorage interface {
	SetGauge(name string, value gauge)
	GetGauge(name string) (gauge, bool)
	SetCounter(name string, value counter)
	GetCounter(name string) (counter, bool)
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		Gauges:   make(map[string]gauge),
		Counters: make(map[string]counter),
	}
}

func (m *MemStorage) SetGauge(name string, value gauge) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Gauges[name] = value
}

func (m *MemStorage) GetGauge(name string) (gauge, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Gauges[name]
	return val, ok
}

func (m *MemStorage) SetCounter(name string, value counter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters[name] += value
}

func (m *MemStorage) GetCounter(name string) (counter, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Counters[name]
	return val, ok
}

func main() {

	storage := NewMemStorage()
	r := chi.NewRouter()

	r.Route("/update", func(r chi.Router) {
		r.Post("/{typeMetric}/{metric}/{value}", UpdateValueHandler(storage))
	})

	err := http.ListenAndServe("localhost:8080", r)
	if err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
