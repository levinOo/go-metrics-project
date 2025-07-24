package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/levinOo/go-metrics-project/internal/agent"
	"github.com/levinOo/go-metrics-project/internal/agent/store"
)

func TestSendMetrics(t *testing.T) {
	m := &store.Metrics{
		Alloc:       123.456,
		PollCount:   7,
		RandomValue: 999,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Ожидался метод POST, получен %s", r.Method)
			return
		}

		path := r.URL.Path
		parts := strings.Split(path, "/")
		if len(parts) != 5 || parts[1] != "update" {
			t.Errorf("Непредвиденный путь URL: %s", path)
			return
		}

		metricType := parts[2]
		metricName := parts[3]
		metricValue := parts[4]

		switch metricName {
		case "Alloc":
			if metricType != "gauge" {
				t.Errorf("Для Alloc ожидается тип gauge, получен %s", metricType)
				return
			}
			expected := strconv.FormatFloat(float64(m.Alloc), 'f', -1, 64)
			if metricValue != expected {
				t.Errorf("Значение Alloc не совпадает, получили %s, ожидали %s", metricValue, expected)
				return
			}
		case "PollCount":
			if metricType != "counter" {
				t.Errorf("Для PollCount ожидается тип counter, получен %s", metricType)
				return
			}
			expected := strconv.FormatInt(int64(m.PollCount), 10)
			if metricValue != expected {
				t.Errorf("Значение PollCount не совпадает, получили %s, ожидали %s", metricValue, expected)
				return
			}
		case "RandomValue":
			if metricType != "gauge" {
				t.Errorf("Для RandomValue ожидается тип counter, получен %s", metricType)
				return
			}
			expected := strconv.FormatInt(int64(m.RandomValue), 10)
			if metricValue != expected {
				t.Errorf("Значение RandomValue не совпадает, получили %s, ожидали %s", metricValue, expected)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer ts.Close()

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	agent.SendAllMetrics(client, ts.URL, *m)
}
