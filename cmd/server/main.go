package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi"
)

type (
	gauge   float64
	counter int64
)

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
				http.Error(rw, "Invalid type of value", http.StatusBadRequest)
				return
			}
			storage.SetGauge(nameMetric, gauge(valueGauge))
		case "counter":
			valueCounter, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(rw, "Invalid type of value", http.StatusBadRequest)
				return
			}
			storage.SetCounter(nameMetric, counter(valueCounter))
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}
		rw.WriteHeader(http.StatusOK)
		_, err := rw.Write([]byte("OK"))
		if err != nil {
			log.Printf("write error: %v", err)
		}

	}
}

func GetValueHandler(storage *MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		nameMetric := chi.URLParam(r, "metric")

		switch chi.URLParam(r, "typeMetric") {
		case "gauge":
			val, ok := storage.GetGauge(nameMetric)
			if ok {
				rw.WriteHeader(http.StatusOK)
				_, err := rw.Write([]byte(fmt.Sprintf("%g", val)))
				if err != nil {
					log.Printf("write error: %v", err)
				}
			} else {
				rw.WriteHeader(http.StatusNotFound)
				return
			}
		case "counter":
			val, ok := storage.GetCounter(nameMetric)
			if ok {
				rw.WriteHeader(http.StatusOK)
				_, err := rw.Write([]byte(fmt.Sprintf("%d", val)))
				if err != nil {
					log.Printf("write error: %v", err)
				}
			} else {
				rw.WriteHeader(http.StatusNotFound)
				return
			}
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}
	}
}

func GetListHandler(storage *MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		var sb strings.Builder

		for name, val := range storage.Gauges {
			sb.WriteString(fmt.Sprintf("%s: %f\n", name, val))
		}
		for name, val := range storage.Counters {
			sb.WriteString(fmt.Sprintf("%s: %d\n", name, val))
		}

		_, err := rw.Write([]byte(sb.String()))
		if err != nil {
			log.Printf("write error: %v", err)
		}

	}
}

type MemStorage struct {
	mu       sync.Mutex
	Gauges   map[string]gauge
	Counters map[string]counter
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

var addr = flag.String("a", "localhos:8080", "Адрес сервера")

func main() {
	flag.Parse()
	if len(flag.Args()) > 0 {
		log.Fatalf("Неизвестные аргументы: %v", flag.Args())
	}
	storage := NewMemStorage()
	r := chi.NewRouter()

	r.Route("/", func(r chi.Router) {
		r.Get("/", GetListHandler(storage))
		r.Route("/update", func(r chi.Router) {
			r.Post("/{typeMetric}/{metric}/{value}", UpdateValueHandler(storage))
		})
		r.Route("/value", func(r chi.Router) {
			r.Get("/{typeMetric}/{metric}", GetValueHandler(storage))
		})
	})

	err := http.ListenAndServe(*addr, r)
	if err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
