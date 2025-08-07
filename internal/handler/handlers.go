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
	"github.com/levinOo/go-metrics-project/internal/config"
	"github.com/levinOo/go-metrics-project/internal/config/db"
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
)

type ServerComponents struct {
	server *http.Server
	store  *repository.MemStorage
	logger *zap.SugaredLogger
}

type PeriodicSaver struct {
	store    *repository.MemStorage
	interval time.Duration
	filePath string
	logger   *zap.SugaredLogger
	stopCh   chan struct{}
	done     chan struct{}
}

func NewPeriodicSaver(store *repository.MemStorage, filePath string, interval time.Duration, logger *zap.SugaredLogger) *PeriodicSaver {
	return &PeriodicSaver{
		store:    store,
		interval: interval,
		filePath: filePath,
		logger:   logger,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (ps *PeriodicSaver) Start() {
	go func() {
		defer close(ps.done)
		ticker := time.NewTicker(ps.interval)
		defer ticker.Stop()

		ps.logger.Infow("Starting periodic save", "interval", ps.interval, "file", ps.filePath)

		for {
			select {
			case <-ticker.C:
				ps.logger.Debugw("Periodic save triggered")
				if err := saveToFile(ps.store, ps.filePath, ps.logger); err != nil {
					ps.logger.Errorw("Failed to save metrics", "error", err)
				} else {
					ps.logger.Debugw("Metrics saved successfully", "file", ps.filePath)
				}
			case <-ps.stopCh:
				ps.logger.Debugw("Stopping periodic save")
				return
			}
		}
	}()
}

func (ps *PeriodicSaver) Stop() {
	if ps.stopCh != nil {
		close(ps.stopCh)
		<-ps.done
	}
}

func Serve(cfg config.Config) error {
	sugar := logger.NewLogger()
	server := setupServer(cfg, sugar)
	saver := setupPeriodicSaver(cfg, server.store, sugar)

	return runServerWithGracefulShutdown(server, saver, cfg)
}

func setupServer(cfg config.Config, sugar *zap.SugaredLogger) *ServerComponents {
	sugar.Infow("Starting server with config",
		"address", cfg.Addr,
		"storeInterval", cfg.StoreInterval,
		"fileStorage", cfg.FileStorage,
		"restore", cfg.Restore,
	)

	storage := repository.NewMemStorage()

	if cfg.Restore {
		if err := loadFromFile(storage, cfg.FileStorage, sugar); err != nil {
			sugar.Errorw("Failed to load metrics from file", "error", err)
		}
	}

	router := newRouter(storage, sugar, cfg.AddrDB)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	return &ServerComponents{
		server: srv,
		store:  storage,
		logger: sugar,
	}
}

func setupPeriodicSaver(cfg config.Config, storage *repository.MemStorage, sugar *zap.SugaredLogger) *PeriodicSaver {
	if cfg.StoreInterval <= 0 {
		sugar.Infow("Periodic save disabled", "storeInterval", cfg.StoreInterval)
		return nil
	}

	saver := NewPeriodicSaver(storage, cfg.FileStorage, time.Duration(cfg.StoreInterval)*time.Second, sugar)
	saver.Start()

	return saver
}

func runServerWithGracefulShutdown(components *ServerComponents, saver *PeriodicSaver, cfg config.Config) error {
	server := components.server
	storage := components.store
	sugar := components.logger

	serverErr := make(chan error, 1)

	go func() {
		sugar.Infow("HTTP server started", "address", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		if err != nil {
			sugar.Errorw("Server error", "error", err)
			if saver != nil {
				saver.Stop()
			}
			return fmt.Errorf("server error: %w", err)
		}
	case <-quit:
		sugar.Infoln("Shutting down server...")
	}

	return gracefulShutdown(cfg, sugar, storage, server, saver)
}

func gracefulShutdown(cfg config.Config, sugar *zap.SugaredLogger, store *repository.MemStorage, srv *http.Server, saver *PeriodicSaver) error {
	if saver != nil {
		saver.Stop()
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
}

func loadFromFile(store *repository.MemStorage, fileName string, sugar *zap.SugaredLogger) error {
	if fileName == "" {
		return nil
	}

	data, err := readFile(fileName, sugar)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		sugar.Infow("Metrics file is empty, starting with empty storage", "file", fileName)
		return nil
	}

	metrics, err := deserializeMetrics(data, fileName)
	if err != nil {
		return err
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

func readFile(fileName string, sugar *zap.SugaredLogger) ([]byte, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			sugar.Infow("Metrics file does not exist, starting with empty storage", "file", fileName)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read metrics file %s: %w", fileName, err)
	}
	return data, nil
}

func deserializeMetrics(data []byte, fileName string) ([]models.Metrics, error) {
	var metrics []models.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metrics from %s: %w", fileName, err)
	}
	return metrics, nil
}

func saveToFile(store *repository.MemStorage, fileName string, sugar *zap.SugaredLogger) error {
	if fileName == "" {
		sugar.Debugw("Save skipped - no filename specified")
		return nil
	}

	sugar.Debugw("Starting save to file", "file", fileName)

	allMetrics := store.GetAll()
	sugar.Debugw("Retrieved metrics from storage", "count", len(allMetrics))

	data, err := serializeMetrics(allMetrics)
	if err != nil {
		return fmt.Errorf("failed to serialize metrics: %w", err)
	}

	if err := writeFile(fileName, data); err != nil {
		return fmt.Errorf("failed to write file %s: %w", fileName, err)
	}

	sugar.Debugw("Successfully saved metrics", "file", fileName, "size", len(data))
	return nil
}

func serializeMetrics(metrics []models.Metrics) ([]byte, error) {
	return json.MarshalIndent(metrics, "", "  ")
}

func writeFile(fileName string, data []byte) error {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

func newRouter(storage *repository.MemStorage, sugar *zap.SugaredLogger, cfgAddrDB string) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/", LoggerFuncServer(GetListHandler(storage), sugar))
	r.Get("/ping", LoggerFuncServer(PingHandler(cfgAddrDB), sugar))

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

func PingHandler(cfgAddrDB string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := db.DataBaseConnection(ctx, cfgAddrDB)
		if err != nil {
			http.Error(rw, "No conection with Database", http.StatusInternalServerError)
			return
		}

		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("Database is reachable"))
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
