package repository

//go:generate go run ../../cmd/reset/main.go

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/levinOo/go-metrics-project/internal/models"
)

type (
	Gauge   float64
	Counter int64
)

type Storage interface {
	SetGauge(name string, value Gauge) error
	GetGauge(name string) (Gauge, error)
	SetCounter(name string, value Counter) error
	GetCounter(name string) (Counter, error)
	GetAll() (*models.ListMetrics, error)
	Ping(ctx context.Context) error
	InsertMetricsBatch(models.ListMetrics) error
}

// --------------------- DBStorage ---------------------

// generate:reset
type DBStorage struct {
	db *sql.DB
}

func NewDBStorage(db *sql.DB) *DBStorage {
	return &DBStorage{db: db}
}

func (d *DBStorage) SetGauge(name string, value Gauge) error {
	_, err := d.db.Exec(`
		INSERT INTO metrics (name, value, type) VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value
	`, name, float64(value), "gauge")
	return err
}

func (d *DBStorage) GetGauge(name string) (Gauge, error) {
	var val float64
	err := d.db.QueryRow(`SELECT value FROM metrics WHERE name=$1`, name).Scan(&val)
	if err == sql.ErrNoRows {
		return 0, errors.New("metric not found")
	}
	return Gauge(val), err
}

func (d *DBStorage) SetCounter(name string, value Counter) error {
	_, err := d.db.Exec(`
		INSERT INTO metrics (name, delta, type) VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET delta = metrics.delta + EXCLUDED.delta
	`, name, int64(value), "counter")
	return err
}

func (d *DBStorage) GetCounter(name string) (Counter, error) {
	var val int64
	err := d.db.QueryRow(`SELECT delta FROM metrics WHERE name=$1`, name).Scan(&val)
	if err == sql.ErrNoRows {
		return 0, errors.New("metric not found")
	}
	return Counter(val), err
}

func (d *DBStorage) InsertMetricsBatch(metrics models.ListMetrics) error {
	if len(metrics.List) == 0 {
		return nil
	}

	type batchItem struct {
		MType string
		Value *float64
		Delta *int64
	}

	tmp := make(map[string]batchItem)
	for _, metric := range metrics.List {
		if metric.ID == "" || metric.MType == "" {
			continue
		}

		b := tmp[metric.ID]

		switch metric.MType {
		case "gauge":
			if metric.Value != nil {
				b.MType = "gauge"
				b.Value = metric.Value
			}
		case "counter":
			if metric.Delta != nil {
				b.MType = "counter"
				if b.Delta == nil {
					b.Delta = new(int64)
				}
				*b.Delta += *metric.Delta
			}
		}

		tmp[metric.ID] = b
	}

	if len(tmp) == 0 {
		return nil
	}

	valueStrings := make([]string, 0, len(tmp))
	valueArgs := make([]interface{}, 0, len(tmp)*4)
	argIndex := 1

	for id, b := range tmp {
		var val interface{} = nil
		var delta interface{} = nil

		switch b.MType {
		case "gauge":
			val = *b.Value
		case "counter":
			delta = *b.Delta
		}

		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d)", argIndex, argIndex+1, argIndex+2, argIndex+3))
		valueArgs = append(valueArgs, id, delta, b.MType, val)
		argIndex += 4
	}

	query := fmt.Sprintf(`
		INSERT INTO metrics (name, delta, type, value)
		VALUES %s
		ON CONFLICT (name) DO UPDATE
		SET type = EXCLUDED.type,
			delta = CASE 
				WHEN EXCLUDED.type = 'counter' THEN metrics.delta + EXCLUDED.delta 
				ELSE EXCLUDED.delta 
			END,
			value = CASE 
				WHEN EXCLUDED.type = 'gauge' THEN EXCLUDED.value 
				ELSE metrics.value 
			END
	`, strings.Join(valueStrings, ","))

	_, err := d.db.Exec(query, valueArgs...)
	if err != nil {
		log.Printf("Batch insert error: %v", err)
		return err
	}

	return nil
}

func (d *DBStorage) GetAll() (*models.ListMetrics, error) {
	var list models.ListMetrics

	rows, err := d.db.Query(`SELECT name, type, value, delta FROM metrics`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			name  string
			mtype string
			value sql.NullFloat64
			delta sql.NullInt64
		)

		if err := rows.Scan(&name, &mtype, &value, &delta); err != nil {
			return nil, err
		}

		if rows.Err() != nil {
			return nil, err
		}

		metric := models.Metrics{
			ID:    name,
			MType: mtype,
		}

		if mtype == "gauge" && value.Valid {
			metric.Value = &value.Float64
		} else if mtype == "counter" && delta.Valid {
			metric.Delta = &delta.Int64
		}

		list.List = append(list.List, metric)
	}

	return &list, nil
}

func (d *DBStorage) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// --------------------- MemStorage ---------------------

// generate:reset
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

func (m *MemStorage) SetGauge(name string, value Gauge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Gauges[name] = value
	return nil
}

func (m *MemStorage) GetGauge(name string) (Gauge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Gauges[name]
	if !ok {
		return 0, errors.New("metric not found")
	}
	return val, nil
}

func (m *MemStorage) SetCounter(name string, value Counter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters[name] += value
	return nil
}

func (m *MemStorage) GetCounter(name string) (Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Counters[name]
	if !ok {
		return 0, errors.New("metric not found")
	}
	return val, nil
}

func (m *MemStorage) InsertMetricsBatch(metrics models.ListMetrics) error {
	for _, metric := range metrics.List {
		switch metric.MType {
		case "gauge":
			err := m.SetGauge(metric.ID, Gauge(*metric.Value))
			if err != nil {
				log.Printf("Failed to set gauge %s: %v", metric.ID, err)
			}
		case "counter":
			err := m.SetCounter(metric.ID, Counter(*metric.Delta))
			if err != nil {
				log.Printf("Failed to set counter %s: %v", metric.ID, err)
			}
		default:
			continue
		}
	}

	return nil
}

func (m *MemStorage) GetAll() (*models.ListMetrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list models.ListMetrics

	for name, val := range m.Counters {
		v := int64(val)
		list.List = append(list.List, models.Metrics{
			ID:    name,
			MType: "counter",
			Delta: &v,
		})
	}

	for name, val := range m.Gauges {
		v := float64(val)
		list.List = append(list.List, models.Metrics{
			ID:    name,
			MType: "gauge",
			Value: &v,
		})
	}

	return &list, nil
}

func (m *MemStorage) Ping(ctx context.Context) error {
	return nil
}
