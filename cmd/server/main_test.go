package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
	"github.com/levinOo/go-metrics-project/internal/handler"
	"github.com/levinOo/go-metrics-project/internal/repository"
)

func TestGetValueHandler(t *testing.T) {
	type want struct {
		code        int
		contentType string
		body        string
		method      string
	}

	tests := []struct {
		name   string
		method string
		url    string
		setup  func(repository.MemStorage)
		want   want
	}{
		{
			name:   "Get existing gauge",
			method: http.MethodGet,
			url:    "/value/gauge/HeapSys",
			setup: func(storage repository.MemStorage) {
				storage.SetGauge("HeapSys", 123.456)
			},
			want: want{
				code:        http.StatusOK,
				contentType: "",
				body:        "123.456",
				method:      http.MethodGet,
			},
		},
		{
			name:   "Get existing counter",
			method: http.MethodGet,
			url:    "/value/counter/PollCount",
			setup: func(storage repository.MemStorage) {
				storage.SetCounter("PollCount", 42)
			},
			want: want{
				code:        http.StatusOK,
				contentType: "",
				body:        "42",
				method:      http.MethodGet,
			},
		},
		{
			name:   "Get missing gauge",
			method: http.MethodGet,
			url:    "/value/gauge/NotExist",
			setup: func(storage repository.MemStorage) {
				// не добавляем значение
			},
			want: want{
				code: http.StatusNotFound,
			},
		},
		{
			name:   "Get missing counter",
			method: http.MethodGet,
			url:    "/value/counter/NotExist",
			setup: func(storage repository.MemStorage) {
			},
			want: want{
				code: http.StatusNotFound,
			},
		},
		{
			name:   "Unknown metric type",
			method: http.MethodGet,
			url:    "/value/unknown/Metric",
			setup:  func(storage repository.MemStorage) {},
			want: want{
				code:        http.StatusBadRequest,
				contentType: "text/plain; charset=utf-8",
				body:        "Unknown type of metric\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := repository.NewMemStorage()
			tt.setup(*storage)

			r := chi.NewRouter()
			r.Get("/value/{typeMetric}/{metric}", handler.GetValueHandler(storage))

			req := httptest.NewRequest(tt.method, tt.url, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tt.want.code {
				t.Errorf("got status %d, want %d", rec.Code, tt.want.code)
			}

			if tt.want.contentType != "" {
				gotType := rec.Header().Get("Content-Type")
				if gotType != tt.want.contentType {
					t.Errorf("got content-type %q, want %q", gotType, tt.want.contentType)
				}
			}

			if tt.want.body != "" && rec.Body.String() != tt.want.body {
				t.Errorf("got body %q, want %q", rec.Body.String(), tt.want.body)
			}
		})
	}
}
