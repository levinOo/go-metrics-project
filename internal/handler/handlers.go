package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
)

type ResponseData struct {
	status int
	size   int
}

type loggingRW struct {
	http.ResponseWriter
	responseData *ResponseData
}

func (r *loggingRW) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size
	return size, err
}

func (r *loggingRW) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode
}

var sugar *zap.SugaredLogger

func init() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	sugar = logger.Sugar()
}

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
		r.Get("/", LoggerFuncServer(GetListHandler(storage)))
		r.Route("/update", func(r chi.Router) {
			r.Post("/", LoggerFuncServer(UpdateHandler(storage)))
			r.Post("/{typeMetric}/{metric}/{value}", LoggerFuncServer(UpdateValueHandler(storage)))
		})
		r.Route("/value", func(r chi.Router) {
			r.Get("/{typeMetric}/{metric}", LoggerFuncServer(GetValueHandler(storage)))
			r.Post("/", LoggerFuncServer(ValueHandler(storage)))
		})
	})
	return r
}

func LoggerFuncServer(h http.Handler) http.HandlerFunc {
	logFn := func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()

		responseData := &ResponseData{
			size:   0,
			status: 0,
		}
		lw := loggingRW{
			ResponseWriter: rw,
			responseData:   responseData,
		}

		h.ServeHTTP(&lw, r)

		dur := time.Since(start)

		sugar.Infoln(
			"uri", r.RequestURI,
			"method", r.Method,
			"duration", dur,
			"status", responseData.status,
			"size", responseData.size,
		)

	}
	return http.HandlerFunc(logFn)
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
			log.Printf("write status code error: %v", err)
		}

	}
}

func UpdateHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(rw, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		var metric models.Metrics

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		err = json.Unmarshal(body, &metric)
		if err != nil {
			http.Error(rw, "", http.StatusBadRequest)
			return
		}

		switch metric.MType {
		case "gauge":
			storage.SetGauge(metric.ID, repository.Gauge(*metric.Value))
		case "counter":
			storage.SetCounter(metric.ID, repository.Counter(*metric.Delta))
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}

		rw.WriteHeader(http.StatusOK)
		_, err = rw.Write([]byte("OK"))
		if err != nil {
			log.Printf("write status code error: %v", err)
		}
	}
}

func ValueHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var metric models.Metrics

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		err = json.Unmarshal(body, &metric)
		if err != nil {
			http.Error(rw, "", http.StatusBadRequest)
			return
		}

		switch metric.MType {
		case "gauge":
			val, err := storage.GetGauge(metric.ID)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			metric.Value = new(float64)
			*metric.Value = float64(val)

		case "counter":
			val, err := storage.GetCounter(metric.ID)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			metric.Delta = new(int64)
			*metric.Delta = int64(val)

		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		err = json.NewEncoder(rw).Encode(metric)
		if err != nil {
			log.Printf("response encode error: %v", err)
		}
	}
}

func GetValueHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		nameMetric := chi.URLParam(r, "metric")

		switch chi.URLParam(r, "typeMetric") {
		case "gauge":
			val, err := storage.GetGauge(nameMetric)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			rw.WriteHeader(http.StatusOK)
			_, err = rw.Write([]byte(fmt.Sprintf("%g", val)))
			if err != nil {
				log.Printf("write error: %v", err)
			}
		case "counter":
			val, err := storage.GetCounter(nameMetric)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			rw.WriteHeader(http.StatusOK)
			_, err = rw.Write([]byte(fmt.Sprintf("%d", val)))
			if err != nil {
				log.Printf("write error: %v", err)
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
