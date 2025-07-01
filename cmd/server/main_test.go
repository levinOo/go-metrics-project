package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
)

func TestUpdateValueHandler(t *testing.T) {
	storage := NewMemStorage()
	r := chi.NewRouter()
	r.Route("/update", func(r chi.Router) {
		r.Post("/{typeMetric}/{metric}/{value}", UpdateValueHandler(storage))
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
	})

	tests := []struct {
		name         string
		method       string
		url          string
		wantCode     int
		wantBody     string
		checkStorage func(t *testing.T)
	}{
		{
			name:     "Успешное обновление счётчика",
			method:   http.MethodPost,
			url:      "/update/counter/PollCount/10",
			wantCode: http.StatusOK,
			wantBody: "OK",
			checkStorage: func(t *testing.T) {
				val, ok := storage.GetCounter("PollCount")
				if !ok {
					t.Errorf("Счётчик PollCount не найден в хранилище")
					return
				}
				if val != 10 {
					t.Errorf("Значение счётчика PollCount = %d; ожидалось 10", val)
				}
			},
		},
		{
			name:     "Успешное обновление gauge",
			method:   http.MethodPost,
			url:      "/update/gauge/HeapAlloc/123.456",
			wantCode: http.StatusOK,
			wantBody: "OK",
			checkStorage: func(t *testing.T) {
				val, ok := storage.GetGauge("HeapAlloc")
				if !ok {
					t.Errorf("Gauge HeapAlloc не найден в хранилище")
					return
				}
				if val != gauge(123.456) {
					t.Errorf("Значение gauge HeapAlloc = %f; ожидалось 123.456", val)
				}
			},
		},
		{
			name:     "Неподдерживаемый HTTP метод",
			method:   http.MethodGet,
			url:      "/update/counter/PollCount/10",
			wantCode: http.StatusMethodNotAllowed,
			wantBody: "Метод не разрешён\n",
		},
		{
			name:     "Неверное значение счётчика",
			method:   http.MethodPost,
			url:      "/update/counter/PollCount/abc",
			wantCode: http.StatusBadRequest,
			wantBody: "Неверный тип метрики\n",
		},
		{
			name:     "Неизвестный тип метрики",
			method:   http.MethodPost,
			url:      "/update/unknown/PollCount/10",
			wantCode: http.StatusBadRequest,
			wantBody: "Неизвестный тип метрики\n",
		},
		{
			name:     "Пустое имя метрики",
			method:   http.MethodPost,
			url:      "/update/counter//10",
			wantCode: http.StatusNotFound,
			wantBody: "Metric is empty\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tt.wantCode {
				t.Errorf("Код ответа = %d; ожидалось %d", resp.StatusCode, tt.wantCode)
			}
			if string(body) != tt.wantBody {
				t.Errorf("Тело ответа = %q; ожидалось %q", string(body), tt.wantBody)
			}

			if tt.checkStorage != nil {
				tt.checkStorage(t)
			}
		})
	}
}
