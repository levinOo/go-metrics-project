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
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
)

func Serve(cfg config.Config) error {
	sugar := logger.LoggerInit()
	sugar.Infow("Starting server with config",
		"address", cfg.Addr,
		"storeInterval", cfg.StoreInterval,
		"fileStorage", cfg.FileStorage,
		"restore", cfg.Restore)

	store := repository.NewMemStorage()

	if cfg.Restore {
		if err := loadFromFile(store, cfg.FileStorage, sugar); err != nil {
			return fmt.Errorf("failed to load metrics: %w", err)
		}
	}

	router := newRouter(store, sugar)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	serverErr := make(chan error, 1)

	go func() {
		sugar.Infow("Starting server", "address", cfg.Addr)
		serverErr <- srv.ListenAndServe()
	}()

	var saveStop chan struct{}
	if cfg.StoreInterval > 0 {
		sugar.Infow("Starting periodic save", "interval", cfg.StoreInterval, "file", cfg.FileStorage)
		saveStop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.StoreInterval) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					sugar.Debugw("Periodic save triggered")
					if err := saveToFile(store, cfg.FileStorage, sugar); err != nil {
						sugar.Errorw("Failed to save metrics", "error", err)
					} else {
						sugar.Debugw("Metrics saved successfully", "file", cfg.FileStorage)
					}
				case <-saveStop:
					sugar.Debugw("Stopping periodic save")
					return
				}
			}
		}()
	} else {
		sugar.Infow("Periodic save disabled", "storeInterval", cfg.StoreInterval)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		sugar.Infoln("Shutting down server...")

		if saveStop != nil {
			close(saveStop)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			sugar.Errorw("Server shutdown error", "error", err)
		}

		sugar.Infow("Performing final save on shutdown", "file", cfg.FileStorage)
		if err := saveToFile(store, cfg.FileStorage, sugar); err != nil {
			return fmt.Errorf("failed to save metrics on shutdown: %w", err)
		}

		sugar.Infoln("Metrics saved and server stopped gracefully")
		return nil

	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}
}

func loadFromFile(store *repository.MemStorage, fileName string, sugar *zap.SugaredLogger) error {
	if fileName == "" {
		return nil
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			sugar.Infow("Metrics file does not exist, starting with empty storage", "file", fileName)
			return nil
		}
		return fmt.Errorf("failed to read metrics file %s: %w", fileName, err)
	}

	if len(data) == 0 {
		sugar.Infow("Metrics file is empty, starting with empty storage", "file", fileName)
		return nil
	}

	var metrics []models.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return fmt.Errorf("failed to unmarshal metrics from %s: %w", fileName, err)
	}

	count := 0
	for _, m := range metrics {
		switch m.MType {
		case "gauge":
			if m.Value != nil {
				store.SetGauge(m.ID, repository.Gauge(*m.Value))
				count++
			}
		case "counter":
			if m.Delta != nil {
				store.SetCounter(m.ID, repository.Counter(*m.Delta))
				count++
			}
		default:
			sugar.Warnw("Unknown metric type in saved data", "type", m.MType, "id", m.ID)
		}
	}

	sugar.Infow("Metrics loaded successfully", "file", fileName, "count", count)
	return nil
}

func saveToFile(store *repository.MemStorage, fileName string, sugar *zap.SugaredLogger) error {
	if fileName == "" {
		sugar.Debugw("Save skipped - no filename specified")
		return nil
	}

	sugar.Debugw("Starting save to file", "file", fileName)

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", fileName, err)
	}
	defer file.Close()

	allMetrics := store.GetAll()
	sugar.Debugw("Retrieved metrics from storage", "count", len(allMetrics))

	data, err := json.MarshalIndent(allMetrics, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	sugar.Debugw("Marshaled metrics", "size", len(data))

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	sugar.Debugw("Successfully wrote to file", "file", fileName, "size", len(data))
	return nil
}

func newRouter(storage *repository.MemStorage, sugar *zap.SugaredLogger) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/", LoggerFuncServer(GetListHandler(storage), sugar))

	r.Route("/update", func(r chi.Router) {
		r.Post("/", LoggerFuncServer(DecompressMiddleware(UpdateJSONHandler(storage)), sugar))
		r.Post("/{typeMetric}/{metric}/{value}", LoggerFuncServer(UpdateValueHandler(storage, sugar), sugar))
	})

	r.Route("/value", func(r chi.Router) {
		r.Get("/{typeMetric}/{metric}", LoggerFuncServer(GetValueHandler(storage), sugar))
		r.Post("/", LoggerFuncServer(DecompressMiddleware(GetJSONHandler(storage)), sugar))
	})

	return r
}

func LoggerFuncServer(h http.Handler, sugar *zap.SugaredLogger) http.HandlerFunc {
	logFn := func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()

		responseData := &logger.ResponseData{
			Size:   0,
			Status: 0,
		}
		lw := logger.LoggingRW{
			ResponseWriter: rw,
			ResponseData:   responseData,
		}

		h.ServeHTTP(&lw, r)

		dur := time.Since(start)

		sugar.Infoln(
			"uri", r.RequestURI,
			"method", r.Method,
			"duration", dur,
			"status", responseData.Status,
			"size", responseData.Size,
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

func UpdateValueHandler(storage *repository.MemStorage, sugar *zap.SugaredLogger) http.HandlerFunc {
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
			sugar.Debugw("Set gauge metric", "name", nameMetric, "value", valueGauge)
		case "counter":
			valueCounter, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(rw, "Invalid type of value", http.StatusBadRequest)
				return
			}
			storage.SetCounter(nameMetric, repository.Counter(valueCounter))
			sugar.Debugw("Set counter metric", "name", nameMetric, "value", valueCounter)
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
