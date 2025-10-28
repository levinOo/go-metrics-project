package repository_test

import (
	"context"
	"fmt"
	"log"

	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
)

// Example_memStorageGauge демонстрирует работу с gauge-метриками в памяти.
func Example_memStorageGauge() {
	storage := repository.NewMemStorage()

	// Устанавливаем gauge-метрику
	err := storage.SetGauge("temperature", 23.5)
	if err != nil {
		log.Fatal(err)
	}

	// Получаем значение
	value, err := storage.GetGauge("temperature")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Temperature: %.1f\n", value)
	// Output: Temperature: 23.5
}

// Example_memStorageCounter демонстрирует работу с counter-метриками в памяти.
func Example_memStorageCounter() {
	storage := repository.NewMemStorage()

	// Увеличиваем counter несколько раз
	storage.SetCounter("requests", 10)
	storage.SetCounter("requests", 5)
	storage.SetCounter("requests", 3)

	// Получаем итоговое значение
	value, err := storage.GetCounter("requests")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Total requests: %d\n", value)
	// Output: Total requests: 18
}

// Example_batchInsert демонстрирует пакетную вставку метрик.
func Example_batchInsert() {
	storage := repository.NewMemStorage()

	// Подготавливаем пакет метрик
	value1 := 100.5
	value2 := 200.7
	delta1 := int64(50)
	delta2 := int64(25)

	batch := models.ListMetrics{
		List: []models.Metrics{
			{ID: "cpu_usage", MType: "gauge", Value: &value1},
			{ID: "memory_usage", MType: "gauge", Value: &value2},
			{ID: "request_count", MType: "counter", Delta: &delta1},
			{ID: "error_count", MType: "counter", Delta: &delta2},
		},
	}

	// Вставляем пакет
	err := storage.InsertMetricsBatch(batch)
	if err != nil {
		log.Fatal(err)
	}

	// Проверяем результаты
	cpu, _ := storage.GetGauge("cpu_usage")
	requests, _ := storage.GetCounter("request_count")

	fmt.Printf("CPU: %.1f, Requests: %d\n", cpu, requests)
	// Output: CPU: 100.5, Requests: 50
}

// Example_getAllMetrics демонстрирует получение всех метрик из хранилища.
func Example_getAllMetrics() {
	storage := repository.NewMemStorage()

	// Добавляем несколько метрик
	storage.SetGauge("temp", 22.5)
	storage.SetCounter("visits", 100)

	// Получаем все метрики
	metrics, err := storage.GetAll()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Total metrics: %d\n", len(metrics.List))
	// Output: Total metrics: 2
}

// Example_counterAccumulation демонстрирует накопительное поведение counter.
func Example_counterAccumulation() {
	storage := repository.NewMemStorage()

	// Первое добавление
	storage.SetCounter("page_views", 100)
	val1, _ := storage.GetCounter("page_views")

	// Второе добавление к той же метрике
	storage.SetCounter("page_views", 50)
	val2, _ := storage.GetCounter("page_views")

	// Третье добавление
	storage.SetCounter("page_views", 25)
	val3, _ := storage.GetCounter("page_views")

	fmt.Printf("Step 1: %d, Step 2: %d, Step 3: %d\n", val1, val2, val3)
	// Output: Step 1: 100, Step 2: 150, Step 3: 175
}

// Example_gaugeOverwrite демонстрирует перезапись значений gauge.
func Example_gaugeOverwrite() {
	storage := repository.NewMemStorage()

	// Устанавливаем начальное значение
	storage.SetGauge("cpu", 45.5)
	val1, _ := storage.GetGauge("cpu")

	// Перезаписываем значение
	storage.SetGauge("cpu", 78.2)
	val2, _ := storage.GetGauge("cpu")

	// Снова перезаписываем
	storage.SetGauge("cpu", 32.1)
	val3, _ := storage.GetGauge("cpu")

	fmt.Printf("Value 1: %.1f, Value 2: %.1f, Value 3: %.1f\n", val1, val2, val3)
	// Output: Value 1: 45.5, Value 2: 78.2, Value 3: 32.1
}

// Example_ping демонстрирует проверку доступности хранилища.
func Example_ping() {
	storage := repository.NewMemStorage()
	ctx := context.Background()

	// Проверяем доступность
	err := storage.Ping(ctx)
	if err != nil {
		fmt.Println("Storage unavailable")
	} else {
		fmt.Println("Storage OK")
	}
	// Output: Storage OK
}

// Example_metricNotFound демонстрирует обработку несуществующих метрик.
func Example_metricNotFound() {
	storage := repository.NewMemStorage()

	// Пытаемся получить несуществующую метрику
	_, err := storage.GetGauge("nonexistent")
	if err != nil {
		fmt.Println("Metric not found")
	}
	// Output: Metric not found
}

// Example_batchWithDuplicates демонстрирует поведение при дублирующихся метриках в пакете.
func Example_batchWithDuplicates() {
	storage := repository.NewMemStorage()

	// Создаем пакет с дублирующимися counter-метриками
	delta1 := int64(10)
	delta2 := int64(20)
	delta3 := int64(30)

	batch := models.ListMetrics{
		List: []models.Metrics{
			{ID: "clicks", MType: "counter", Delta: &delta1},
			{ID: "clicks", MType: "counter", Delta: &delta2},
			{ID: "clicks", MType: "counter", Delta: &delta3},
		},
	}

	storage.InsertMetricsBatch(batch)

	// Получаем итоговое значение
	total, _ := storage.GetCounter("clicks")
	fmt.Printf("Total clicks: %d\n", total)
	// Output: Total clicks: 60
}

// Example_mixedMetricTypes демонстрирует работу с разными типами метрик одновременно.
func Example_mixedMetricTypes() {
	storage := repository.NewMemStorage()

	// Gauge метрики
	storage.SetGauge("cpu_percent", 45.5)
	storage.SetGauge("memory_percent", 78.3)

	// Counter метрики
	storage.SetCounter("http_requests", 1000)
	storage.SetCounter("errors", 5)

	// Получаем все метрики
	all, _ := storage.GetAll()

	gaugeCount := 0
	counterCount := 0
	for _, m := range all.List {
		if m.MType == "gauge" {
			gaugeCount++
		} else if m.MType == "counter" {
			counterCount++
		}
	}

	fmt.Printf("Gauges: %d, Counters: %d\n", gaugeCount, counterCount)
	// Output: Gauges: 2, Counters: 2
}
