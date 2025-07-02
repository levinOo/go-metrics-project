package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/levinOo/go-metrics-project/internal/repository"
)

func Serve(store *repository.MemStorage, cfg string) error {
	router := newRouter(store)
	srv := &http.Server{
		Addr:    cfg,
		Handler: router,
	}

	return srv.ListenAndServe()
}

func newRouter(storage *repository.MemStorage) *chi.Mux {
	r := chi.NewRouter()

	r.Route("/", func(r chi.Router) {
		r.Get("/", GetListHandler(storage))
		r.Route("/update", func(r chi.Router) {
			r.Post("/{typeMetric}/{metric}/{value}", UpdateValueHandler(storage))
		})
		r.Route("/value", func(r chi.Router) {
			r.Get("/{typeMetric}/{metric}", GetValueHandler(storage))
		})
	})
	return r
}

func UpdateValueHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		nameMetric := chi.URLParam(r, "metric")
		valueMetric := chi.URLParam(r, "value")
		typeMetric := chi.URLParam(r, "typeMetric")

		if nameMetric == "" {
			http.Error(rw, "Metric is empty", http.StatusNotFound)
			return
		}

		switch typeMetric {
		case "gauge":
			valueGauge, err := strconv.ParseFloat(valueMetric, 64)
			if err != nil {
				http.Error(rw, "Invalid type of value", http.StatusBadRequest)
				return
			}
			storage.SetGauge(nameMetric, repository.Gauge(valueGauge))
		case "counter":
			valueCounter, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(rw, "Invalid type of value", http.StatusBadRequest)
				return
			}
			storage.SetCounter(nameMetric, repository.Counter(valueCounter))
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}
		rw.WriteHeader(http.StatusOK)
		_, err := rw.Write([]byte("OK"))
		if err != nil {
			log.Printf("write error: %v", err)
		}

	}
}

func GetValueHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		nameMetric := chi.URLParam(r, "metric")

		switch chi.URLParam(r, "typeMetric") {
		case "gauge":
			val, ok := storage.GetGauge(nameMetric)
			if ok {
				rw.WriteHeader(http.StatusOK)
				_, err := rw.Write([]byte(fmt.Sprintf("%g", val)))
				if err != nil {
					log.Printf("write error: %v", err)
				}
			} else {
				rw.WriteHeader(http.StatusNotFound)
				return
			}
		case "counter":
			val, ok := storage.GetCounter(nameMetric)
			if ok {
				rw.WriteHeader(http.StatusOK)
				_, err := rw.Write([]byte(fmt.Sprintf("%d", val)))
				if err != nil {
					log.Printf("write error: %v", err)
				}
			} else {
				rw.WriteHeader(http.StatusNotFound)
				return
			}
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}
	}
}

func GetListHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		var sb strings.Builder

		for name, val := range storage.Gauges {
			sb.WriteString(fmt.Sprintf("%s: %f\n", name, val))
		}
		for name, val := range storage.Counters {
			sb.WriteString(fmt.Sprintf("%s: %d\n", name, val))
		}

		_, err := rw.Write([]byte(sb.String()))
		if err != nil {
			log.Printf("write error: %v", err)
		}

	}
}
