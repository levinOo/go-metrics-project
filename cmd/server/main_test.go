package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
	"github.com/levinOo/go-metrics-project/internal/handler"
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/repository"
)

func TestServer(t *testing.T) {
	type want struct {
		code   int
		method string
		body   string
	}

	tests := []struct {
		name   string
		url    string
		method string
		want   want
	}{
		{
			name:   "UpdateValueHandler / Empty name of request",
			url:    "/value/counter/RandomValue/",
			method: http.MethodPost,
			want: want{
				code:   http.StatusNotFound,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "UpdateValueHandler / Counter type of metric, correct type of value",
			url:    "/value/counter/RandomValue/42",
			method: http.MethodPost,
			want: want{
				code:   http.StatusOK,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "UpdateValueHandler / Counter type of metric, uncorrect type of value",
			url:    "/value/counter/RandomValue/hello",
			method: http.MethodPost,
			want: want{
				code:   http.StatusBadRequest,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "UpdateValueHandler / Counter type of metric, uncorrect type of value",
			url:    "/value/counter/RandomValue/hello",
			method: http.MethodPost,
			want: want{
				code:   http.StatusBadRequest,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "UpdateValueHandler / Gauge type of metric, correct type of value",
			url:    "/value/gauge/Alloc/43.54",
			method: http.MethodPost,
			want: want{
				code:   http.StatusOK,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "UpdateValueHandler / Gauge type of metric, uncorrect type of value",
			url:    "/value/counter/Alloc/hello",
			method: http.MethodPost,
			want: want{
				code:   http.StatusBadRequest,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "UpdateValueHandler / Incorrect type of value",
			url:    "/value/gaauge/RandomValue/56",
			method: http.MethodPost,
			want: want{
				code:   http.StatusBadRequest,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "GetValueHandler / Correct value",
			url:    "/value/gauge/Alloc",
			method: http.MethodGet,
			want: want{
				code:   http.StatusOK,
				method: http.MethodPost,
				body:   "",
			},
		},
		{
			name:   "GetValueHandler / Incorrect value",
			url:    "/value/gauge/Allloc",
			method: http.MethodGet,
			want: want{
				code:   http.StatusNotFound,
				method: http.MethodPost,
				body:   "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := repository.NewMemStorage()
			sugar := logger.LoggerInit()
			r := chi.NewRouter()

			switch tt.method {

			case http.MethodPost:
				r.Post("/value/{typeMetric}/{metric}/{value}", handler.UpdateValueHandler(storage, sugar))
				req := httptest.NewRequest(tt.method, tt.url, nil)
				rec := httptest.NewRecorder()

				r.ServeHTTP(rec, req)

				if rec.Code != tt.want.code {
					t.Errorf("got status: %q, want: %q", rec.Code, tt.want.code)
				}

			case http.MethodGet:
				storage.SetGauge("Alloc", 45.56)
				r.Get("/value/{typeMetric}/{metric}", handler.GetValueHandler(storage))
				req := httptest.NewRequest(http.MethodGet, tt.url, nil)
				rec := httptest.NewRecorder()

				r.ServeHTTP(rec, req)

				if rec.Code != tt.want.code {
					t.Errorf("got status: %q, want: %q", rec.Code, tt.want.code)
				}

			}

		})
	}
}
