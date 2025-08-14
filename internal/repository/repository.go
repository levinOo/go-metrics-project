package repository

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
	GetAll() ([]models.Metrics, error)
	Ping(ctx context.Context) error
	InsertMetricsBatch([]models.Metrics) error
}

// --------------------- DBStorage ---------------------

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

func (d *DBStorage) InsertMetricsBatch(metrics []models.Metrics) error {
	valueStrings := make([]string, 0, len(metrics))
	valueArgs := make([]interface{}, 0, len(metrics)*4)
	argIndex := 1

	for _, metric := range metrics {
		var val interface{} = nil
		var delta interface{} = nil

		switch metric.MType {
		case "gauge":
			val = *metric.Value
		case "counter":
			delta = *metric.Delta
		default:
			continue
		}

		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d)", argIndex, argIndex+1, argIndex+2, argIndex+3))
		valueArgs = append(valueArgs, metric.ID, delta, metric.MType, val)
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

func (d *DBStorage) GetAll() ([]models.Metrics, error) {
	var list []models.Metrics

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

		list = append(list, metric)
	}

	return list, nil
}

func (d *DBStorage) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// --------------------- MemStorage ---------------------

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

func (m *MemStorage) InsertMetricsBatch(metrics []models.Metrics) error {
	for _, metric := range metrics {
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

func (m *MemStorage) GetAll() ([]models.Metrics, error) {
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

	return list, nil
}

func (m *MemStorage) Ping(ctx context.Context) error {
	return nil
}
