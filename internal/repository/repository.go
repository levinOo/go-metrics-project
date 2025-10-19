// Package repository предоставляет интерфейс и реализации для хранения метрик.
// Поддерживает два типа хранилищ: в памяти (MemStorage) и в базе данных PostgreSQL (DBStorage).
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

// Gauge представляет метрику типа gauge - значение с плавающей точкой,
// которое может увеличиваться или уменьшаться.
type Gauge float64

// Counter представляет метрику типа counter - целочисленное значение,
// которое только увеличивается.
type Counter int64

// Storage определяет интерфейс для работы с хранилищем метрик.
// Поддерживает операции с gauge и counter метриками, а также пакетную вставку.
type Storage interface {
	// SetGauge устанавливает значение gauge-метрики. При повторном вызове
	// с тем же именем значение перезаписывается.
	SetGauge(name string, value Gauge) error

	// GetGauge возвращает значение gauge-метрики по имени.
	// Возвращает ошибку, если метрика не найдена.
	GetGauge(name string) (Gauge, error)

	// SetCounter увеличивает значение counter-метрики на указанное значение.
	// Если метрика не существует, создается новая с указанным значением.
	SetCounter(name string, value Counter) error

	// GetCounter возвращает текущее значение counter-метрики по имени.
	// Возвращает ошибку, если метрика не найдена.
	GetCounter(name string) (Counter, error)

	// GetAll возвращает список всех метрик в хранилище.
	GetAll() ([]models.Metrics, error)

	// Ping проверяет доступность хранилища. Для MemStorage всегда возвращает nil,
	// для DBStorage выполняет ping к базе данных.
	Ping(ctx context.Context) error

	// InsertMetricsBatch выполняет пакетную вставку метрик.
	// Для counter-метрик с одинаковыми именами значения суммируются.
	InsertMetricsBatch(models.ListMetrics) error
}

// DBStorage реализует интерфейс Storage с использованием PostgreSQL базы данных.
// Обеспечивает персистентное хранение метрик и поддержку транзакций.
type DBStorage struct {
	db *sql.DB
}

// NewDBStorage создает новый экземпляр DBStorage с указанным подключением к базе данных.
// База данных должна содержать таблицу metrics с соответствующей структурой.
func NewDBStorage(db *sql.DB) Storage {
	return &DBStorage{db: db}
}

// SetGauge сохраняет gauge-метрику в базу данных.
// Использует INSERT ... ON CONFLICT для обновления существующих значений.
func (d *DBStorage) SetGauge(name string, value Gauge) error {
	_, err := d.db.Exec(`
		INSERT INTO metrics (name, value, type) VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value
	`, name, float64(value), "gauge")
	return err
}

// GetGauge извлекает значение gauge-метрики из базы данных.
func (d *DBStorage) GetGauge(name string) (Gauge, error) {
	var val float64
	err := d.db.QueryRow(`SELECT value FROM metrics WHERE name=$1`, name).Scan(&val)
	if err == sql.ErrNoRows {
		return 0, errors.New("metric not found")
	}
	return Gauge(val), err
}

// SetCounter увеличивает counter-метрику в базе данных на указанное значение.
// При первой вставке создается новая запись, при повторных - значение суммируется.
func (d *DBStorage) SetCounter(name string, value Counter) error {
	_, err := d.db.Exec(`
		INSERT INTO metrics (name, delta, type) VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET delta = metrics.delta + EXCLUDED.delta
	`, name, int64(value), "counter")
	return err
}

// GetCounter извлекает значение counter-метрики из базы данных.
func (d *DBStorage) GetCounter(name string) (Counter, error) {
	var val int64
	err := d.db.QueryRow(`SELECT delta FROM metrics WHERE name=$1`, name).Scan(&val)
	if err == sql.ErrNoRows {
		return 0, errors.New("metric not found")
	}
	return Counter(val), err
}

// InsertMetricsBatch выполняет пакетную вставку метрик в базу данных за один запрос.
// Метрики с одинаковыми именами объединяются перед вставкой: для counter суммируются значения,
// для gauge берется последнее значение. Использует оптимизированный bulk insert.
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

// GetAll возвращает все метрики из базы данных.
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

// Ping проверяет соединение с базой данных.
func (d *DBStorage) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// MemStorage реализует интерфейс Storage с использованием хранения в оперативной памяти.
// Обеспечивает потокобезопасный доступ к метрикам через mutex.
// Данные теряются при перезапуске приложения.
type MemStorage struct {
	mu       *sync.Mutex
	Gauges   map[string]Gauge
	Counters map[string]Counter
}

// NewMemStorage создает новый экземпляр MemStorage с инициализированными картами метрик.
func NewMemStorage() Storage {
	return &MemStorage{
		mu:       &sync.Mutex{},
		Gauges:   make(map[string]Gauge),
		Counters: make(map[string]Counter),
	}
}

// SetGauge сохраняет gauge-метрику в памяти. Потокобезопасно.
func (m *MemStorage) SetGauge(name string, value Gauge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Gauges[name] = value
	return nil
}

// GetGauge извлекает значение gauge-метрики из памяти. Потокобезопасно.
func (m *MemStorage) GetGauge(name string) (Gauge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Gauges[name]
	if !ok {
		return 0, errors.New("metric not found")
	}
	return val, nil
}

// SetCounter увеличивает counter-метрику на указанное значение. Потокобезопасно.
func (m *MemStorage) SetCounter(name string, value Counter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters[name] += value
	return nil
}

// GetCounter извлекает значение counter-метрики из памяти. Потокобезопасно.
func (m *MemStorage) GetCounter(name string) (Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.Counters[name]
	if !ok {
		return 0, errors.New("metric not found")
	}
	return val, nil
}

// InsertMetricsBatch выполняет пакетную вставку метрик в память.
// Обрабатывает каждую метрику последовательно.
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

// GetAll возвращает все метрики из памяти. Потокобезопасно.
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

// Ping для MemStorage всегда возвращает nil, так как проверка не требуется.
func (m *MemStorage) Ping(ctx context.Context) error {
	return nil
}
