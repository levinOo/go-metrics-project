package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestUpdateValueHandler(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		url      string
		wantCode int
		wantBody string
	}{
		{
			name:     "Успешный тест для counter",
			method:   http.MethodPost,
			url:      "/value/counter/PollCount/45",
			wantCode: http.StatusOK,
			wantBody: "OK",
		},
		{
			name:     "Успешный тест для gauge",
			method:   http.MethodPost,
			url:      "/value/gauge/BuckHashSys/67.089",
			wantCode: http.StatusOK,
			wantBody: "OK",
		},
		{
			name:     "Неверный HTTP метод",
			method:   http.MethodGet,
			url:      "/value/counter/PollCount/45",
			wantCode: http.StatusMethodNotAllowed,
			wantBody: "Method Not Allowed\n",
		},
		{
			name:     "Неверное значение counter",
			method:   http.MethodPost,
			url:      "/value/counter/PollCount/invalid",
			wantCode: http.StatusBadRequest,
			wantBody: "Неверный тип метрики\n",
		},
		{
			name:     "Неизвестный тип метрики",
			method:   http.MethodPost,
			url:      "/value/unknown/PollCount/45",
			wantCode: http.StatusBadRequest,
			wantBody: "Неизвестный тип метрики\n",
		},
		{
			name:     "Пустое имя метрики",
			method:   http.MethodPost,
			url:      "/value/counter//45",
			wantCode: http.StatusNotFound,
			wantBody: "Metric is empty\n",
		},
	}

	storage := NewMemStorage()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.url, nil)
			w := httptest.NewRecorder()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
					return
				}

				parts := strings.Split(r.URL.Path, "/")
				if len(parts) < 5 {
					http.Error(w, "Неверный URL", http.StatusBadRequest)
					return
				}
				typeMetric := parts[2]
				nameMetric := parts[3]
				valueMetric := parts[4]

				if nameMetric == "" {
					http.Error(w, "Metric is empty", http.StatusNotFound)
					return
				}

				switch typeMetric {
				case "gauge":
					valueGauge, err := strconv.ParseFloat(valueMetric, 64)
					if err != nil {
						http.Error(w, "Неверный тип метрики", http.StatusBadRequest)
						return
					}
					storage.SetGauge(nameMetric, gauge(valueGauge))
				case "counter":
					valueCounter, err := strconv.ParseInt(valueMetric, 10, 64)
					if err != nil {
						http.Error(w, "Неверный тип метрики", http.StatusBadRequest)
						return
					}
					storage.SetCounter(nameMetric, counter(valueCounter))
				default:
					http.Error(w, "Неизвестный тип метрики", http.StatusBadRequest)
					return
				}

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			})

			handler.ServeHTTP(w, r)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantCode {
				t.Errorf("статус код = %d; ожидалось %d", resp.StatusCode, tt.wantCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("ошибка при чтении тела ответа: %v", err)
			}

			if string(body) != tt.wantBody {
				t.Errorf("тело ответа = %q; ожидалось %q", string(body), tt.wantBody)
			}
		})
	}
}
