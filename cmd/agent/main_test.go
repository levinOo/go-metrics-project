package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/levinOo/go-metrics-project/internal/agent"
	"github.com/levinOo/go-metrics-project/internal/agent/store"
	"github.com/levinOo/go-metrics-project/internal/models"
)

func TestSendMetrics(t *testing.T) {
	m := &store.Metrics{
		Alloc:       store.Gauge(123.456),
		PollCount:   store.Counter(7),
		RandomValue: store.Gauge(999.0),
	}

	receivedMetrics := make(map[string]models.Metrics)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Method != http.MethodPost {
			t.Errorf("Ожидался метод POST, получен %s", r.Method)
			return
		}

		if r.URL.Path != "/update" {
			t.Errorf("Ожидался путь /update, получен %s", r.URL.Path)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Ожидался Content-Type application/json, получен %s", contentType)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Ошибка чтения тела запроса: %v", err)
			return
		}
		defer r.Body.Close()

		var metric models.Metrics
		err = json.Unmarshal(body, &metric)
		if err != nil {
			t.Errorf("Ошибка парсинга JSON: %v", err)
			return
		}

		receivedMetrics[metric.ID] = metric

		switch metric.ID {
		case "Alloc":
			if metric.MType != "gauge" {
				t.Errorf("Для Alloc ожидается тип gauge, получен %s", metric.MType)
				return
			}
			if metric.Value == nil {
				t.Errorf("Для gauge метрики Alloc должно быть установлено поле Value")
				return
			}
			expected := float64(m.Alloc)
			if *metric.Value != expected {
				t.Errorf("Значение Alloc не совпадает, получили %f, ожидали %f", *metric.Value, expected)
				return
			}

		case "PollCount":
			if metric.MType != "counter" {
				t.Errorf("Для PollCount ожидается тип counter, получен %s", metric.MType)
				return
			}
			if metric.Delta == nil {
				t.Errorf("Для counter метрики PollCount должно быть установлено поле Delta")
				return
			}
			expected := int64(m.PollCount)
			if *metric.Delta != expected {
				t.Errorf("Значение PollCount не совпадает, получили %d, ожидали %d", *metric.Delta, expected)
				return
			}

		case "RandomValue":
			if metric.MType != "gauge" {
				t.Errorf("Для RandomValue ожидается тип gauge, получен %s", metric.MType)
				return
			}
			if metric.Value == nil {
				t.Errorf("Для gauge метрики RandomValue должно быть установлено поле Value")
				return
			}
			expected := float64(m.RandomValue)
			if *metric.Value != expected {
				t.Errorf("Значение RandomValue не совпадает, получили %f, ожидали %f", *metric.Value, expected)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer ts.Close()

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	err := agent.SendAllMetrics(client, ts.URL, *m)
	if err != nil {
		t.Errorf("Ошибка отправки метрик: %v", err)
	}

	expectedMetrics := []string{"Alloc", "PollCount", "RandomValue"}
	for _, metricName := range expectedMetrics {
		if _, exists := receivedMetrics[metricName]; !exists {
			t.Errorf("Метрика %s не была отправлена", metricName)
		}
	}

	if len(receivedMetrics) < 3 {
		t.Errorf("Ожидалось минимум 3 метрики, получено %d", len(receivedMetrics))
	}
}
