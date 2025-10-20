package service_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/levinOo/go-metrics-project/internal/config"
	"github.com/levinOo/go-metrics-project/internal/handler"
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"github.com/levinOo/go-metrics-project/internal/service"
)

// Example_updateGaugeMetric демонстрирует обновление gauge-метрики через API.
func Example_updateGaugeMetric() {
	// Создаем in-memory хранилище для тестирования
	storage := repository.NewMemStorage()
	sugar := logger.NewLogger()
	cfg := config.Config{
		Addr:          "localhost:8080",
		StoreInterval: 0,
		FileStorage:   "",
		Restore:       false,
		Key:           "",
	}

	// Создаем тестовый сервер
	router := handler.NewRouter(storage, sugar, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Подготавливаем данные для отправки
	value := 42.5
	metric := models.Metrics{
		ID:    "TestMetric",
		MType: "gauge",
		Value: &value,
	}

	body, _ := json.Marshal(metric)

	// Отправляем POST-запрос
	resp, err := http.Post(ts.URL+"/update/", "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)
	// Output: Status: 200
}

// Example_updateCounterMetric демонстрирует обновление counter-метрики через API.
func Example_updateCounterMetric() {
	storage := repository.NewMemStorage()
	sugar := logger.NewLogger()
	cfg := config.Config{
		Addr:          "localhost:8080",
		StoreInterval: 0,
		FileStorage:   "",
		Restore:       false,
		Key:           "",
	}

	router := handler.NewRouter(storage, sugar, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Отправляем counter метрику
	delta := int64(10)
	metric := models.Metrics{
		ID:    "RequestCount",
		MType: "counter",
		Delta: &delta,
	}

	body, _ := json.Marshal(metric)
	resp, err := http.Post(ts.URL+"/update/", "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)
	// Output: Status: 200
}

// Example_getMetricValue демонстрирует получение значения метрики через API.
func Example_getMetricValue() {
	storage := repository.NewMemStorage()
	sugar := logger.NewLogger()
	cfg := config.Config{
		Addr:          "localhost:8080",
		StoreInterval: 0,
		FileStorage:   "",
		Restore:       false,
		Key:           "",
	}

	// Предварительно добавляем метрику
	storage.SetGauge("Temperature", 23.5)

	router := handler.NewRouter(storage, sugar, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Запрашиваем метрику
	metric := models.Metrics{
		ID:    "Temperature",
		MType: "gauge",
	}

	body, _ := json.Marshal(metric)
	resp, err := http.Post(ts.URL+"/value/", "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var result models.Metrics
	json.NewDecoder(resp.Body).Decode(&result)

	fmt.Printf("Metric: %s, Value: %.1f\n", result.ID, *result.Value)
	// Output: Metric: Temperature, Value: 23.5
}

// Example_batchUpdateMetrics демонстрирует пакетное обновление метрик.
func Example_batchUpdateMetrics() {
	storage := repository.NewMemStorage()
	sugar := logger.NewLogger()
	cfg := config.Config{
		Addr:          "localhost:8080",
		StoreInterval: 0,
		FileStorage:   "",
		Restore:       false,
		Key:           "",
	}

	router := handler.NewRouter(storage, sugar, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Создаем несколько метрик
	value1 := 100.0
	value2 := 200.0
	delta := int64(5)

	metrics := []models.Metrics{
		{ID: "CPUUsage", MType: "gauge", Value: &value1},
		{ID: "MemoryUsage", MType: "gauge", Value: &value2},
		{ID: "RequestCount", MType: "counter", Delta: &delta},
	}

	body, _ := json.Marshal(metrics)
	resp, err := http.Post(ts.URL+"/updates/", "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)
	// Output: Status: 200
}

// Example_periodicSaver демонстрирует использование PeriodicSaver.
func Example_periodicSaver() {
	storage := repository.NewMemStorage()
	sugar := logger.NewLogger()

	// Добавляем тестовые данные
	storage.SetGauge("TestGauge", 42.0)

	// Создаем и запускаем периодическое сохранение
	saver := service.NewPeriodicSaver(
		storage,
		"/tmp/metrics_test.json",
		2*time.Second,
		sugar,
	)

	saver.Start()

	// Работаем с метриками
	time.Sleep(3 * time.Second)

	// Останавливаем сохранение
	saver.Stop()

	fmt.Println("Periodic saver stopped")
	// Output: Periodic saver stopped
}

// Example_getAllMetrics демонстрирует получение списка всех метрик.
func Example_getAllMetrics() {
	storage := repository.NewMemStorage()
	sugar := logger.NewLogger()
	cfg := config.Config{
		Addr:          "localhost:8080",
		StoreInterval: 0,
		FileStorage:   "",
		Restore:       false,
		Key:           "",
	}

	// Добавляем несколько метрик
	storage.SetGauge("CPU", 45.5)
	storage.SetGauge("Memory", 78.2)
	storage.SetCounter("Requests", 100)

	router := handler.NewRouter(storage, sugar, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Получаем список всех метрик
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)
	// Output: Status: 200
}

// Example_healthCheck демонстрирует проверку работоспособности сервера.
func Example_healthCheck() {
	storage := repository.NewMemStorage()
	sugar := logger.NewLogger()
	cfg := config.Config{
		Addr:          "localhost:8080",
		StoreInterval: 0,
		FileStorage:   "",
		Restore:       false,
		Key:           "",
	}

	router := handler.NewRouter(storage, sugar, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Проверяем здоровье сервиса
	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)
	// Output: Status: 200
}
