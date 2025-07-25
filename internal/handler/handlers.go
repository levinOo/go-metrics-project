package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/levinOo/go-metrics-project/internal/handler/config"
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

func Serve(store *repository.MemStorage, cfg config.Config) error {
	router := newRouter(store)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	if cfg.StoreInterval > 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.StoreInterval) * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				if err := saveToFile(store, cfg.FileStorage); err != nil {
					log.Printf("Ошибка при сохранении метрик: %v", err)
				}
			}
		}()
	}

	go func() {
		<-quit
		log.Println("Получен сигнал завершения, сохраняем метрики...")

		if err := saveToFile(store, cfg.FileStorage); err != nil {
			log.Printf("Ошибка при сохранении метрик на выходе: %v", err)
		}

		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("Ошибка завершения сервера: %v", err)
		}
	}()

	return srv.ListenAndServe()
}

func saveToFile(store *repository.MemStorage, fileName string) error {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	data, err := json.MarshalIndent(store.GetAll(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write to file: %w", err)
	}

	return nil
}

func newRouter(storage *repository.MemStorage) *chi.Mux {
	r := chi.NewRouter()

	r.Route("/", func(r chi.Router) {
		r.Get("/", LoggerFuncServer(GetListHandler(storage)))
		r.Route("/update", func(r chi.Router) {
			r.Post("/", LoggerFuncServer(DecompressMiddleware(UpdateJSONHandler(storage))))
			r.Post("/{typeMetric}/{metric}/{value}", LoggerFuncServer(UpdateValueHandler(storage)))
		})
		r.Route("/value", func(r chi.Router) {
			r.Get("/{typeMetric}/{metric}", LoggerFuncServer(GetValueHandler(storage)))
			r.Post("/", LoggerFuncServer(DecompressMiddleware(GetJSONHandler(storage))))
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

func DecompressMiddleware(h http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(rw, "Failed to decompress gzip body", http.StatusBadRequest)
				return
			}
			defer gz.Close()

			body, err := io.ReadAll(gz)
			if err != nil {
				http.Error(rw, "Failed to read decompressed body", http.StatusInternalServerError)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		h.ServeHTTP(rw, r)
	}
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

func UpdateJSONHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var metric models.Metrics

		err := json.NewDecoder(r.Body).Decode(&metric)
		if err != nil {
			http.Error(rw, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
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

		accept := r.Header.Get("Accept")

		if strings.Contains(accept, "application/json") {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			response := map[string]string{"status": "ok"}
			if err := json.NewEncoder(rw).Encode(response); err != nil {
				log.Printf("json encode error: %v", err)
			}

		} else {
			rw.Header().Set("Content-Type", "text/html")
			rw.WriteHeader(http.StatusOK)

			_, err = rw.Write([]byte("<html><body><h1>OK</h1></body></html>"))
			if err != nil {
				log.Printf("write html error: %v", err)
			}
		}
	}
}

func GetJSONHandler(storage *repository.MemStorage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var metric models.Metrics

		err := json.NewDecoder(r.Body).Decode(&metric)
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

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			rw.Header().Set("Content-Encoding", "gzip")
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			gz := gzip.NewWriter(rw)
			defer gz.Close()

			err := json.NewEncoder(gz).Encode(metric)
			if err != nil {
				log.Printf("response gzip encode error: %v", err)
			}
		} else {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			err = json.NewEncoder(rw).Encode(metric)
			if err != nil {
				log.Printf("response encode error: %v", err)

			}
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

		accept := r.Header.Get("Accept")

		if strings.Contains(accept, "text/html") {
			sb.WriteString("<html><body>")
			sb.WriteString("<h1>Metrics</h1>")

			if len(storage.Gauges) > 0 {
				sb.WriteString("<h2>Gauges</h2><ul>")
				for name, val := range storage.Gauges {
					sb.WriteString(fmt.Sprintf("<li>%s: %f</li>", name, val))
				}
				sb.WriteString("</ul>")
			}

			if len(storage.Counters) > 0 {
				sb.WriteString("<h2>Counters</h2><ul>")
				for name, val := range storage.Counters {
					sb.WriteString(fmt.Sprintf("<li>%s: %d</li>", name, val))
				}
				sb.WriteString("</ul>")
			}

			sb.WriteString("</body></html>")
		} else {
			for name, val := range storage.Gauges {
				sb.WriteString(fmt.Sprintf("%s: %f\n", name, val))
			}
			for name, val := range storage.Counters {
				sb.WriteString(fmt.Sprintf("%s: %d\n", name, val))
			}
		}

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			rw.Header().Set("Content-Encoding", "gzip")
			if strings.Contains(accept, "text/html") {
				rw.Header().Set("Content-Type", "text/html")
			} else {
				rw.Header().Set("Content-Type", "text/plain")
			}
			rw.WriteHeader(http.StatusOK)

			gz := gzip.NewWriter(rw)
			defer gz.Close()

			_, err := gz.Write([]byte(sb.String()))
			if err != nil {
				log.Printf("gzip write error: %v", err)
			}
		} else {
			if strings.Contains(accept, "text/html") {
				rw.Header().Set("Content-Type", "text/html")
			} else {
				rw.Header().Set("Content-Type", "text/plain")
			}

			_, err := rw.Write([]byte(sb.String()))
			if err != nil {
				log.Printf("write error: %v", err)
			}
		}
	}
}
