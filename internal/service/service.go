// Package service предоставляет основной функционал сервера для системы сбора метрик.
// Пакет управляет жизненным циклом HTTP-сервера, периодическим сохранением метрик
// и корректным завершением работы при получении системных сигналов.
package service

//go:generate go run ../../cmd/reset/main.go

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/levinOo/go-metrics-project/internal/config"
	"github.com/levinOo/go-metrics-project/internal/config/db"
	"github.com/levinOo/go-metrics-project/internal/handler"
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// ServerComponents содержит все компоненты, необходимые для работы сервера метрик.
// Включает HTTP-сервер, хранилище данных, логгер и опциональное подключение к базе данных.

// generate:reset
type ServerComponents struct {
	server *http.Server
	store  repository.Storage
	logger *zap.SugaredLogger
	dbConn *sql.DB
}

// PeriodicSaver управляет автоматическим периодическим сохранением метрик на диск.
// Запускает фоновую горутину, которая сохраняет метрики через заданные интервалы времени.

// generate:reset
type PeriodicSaver struct {
	store    repository.Storage
	interval time.Duration
	filePath string
	logger   *zap.SugaredLogger
	stopCh   chan struct{}
	done     chan struct{}
}

// Serve инициализирует и запускает сервер метрик с указанной конфигурацией.
// Настраивает хранилище (в памяти или база данных), запускает периодическое сохранение,
// включает профилирование pprof и обрабатывает корректное завершение работы по SIGINT/SIGTERM.
//
// Возвращает ошибку, если запуск или завершение сервера завершились неудачей.
func Serve(cfg config.Config) error {
	sugar := logger.NewLogger()
	server := setupServer(cfg, sugar)
	saver := setupPeriodicSaver(cfg, server.store, sugar)

	return runServerWithGracefulShutdown(server, saver, cfg)
}

func setupServer(cfg config.Config, sugar *zap.SugaredLogger) *ServerComponents {
	sugar.Infow("Starting server with config", "address", cfg.Addr, "storeInterval", cfg.StoreInterval, "fileStorage", cfg.FileStorage, "restore", cfg.Restore, "addressDB", cfg.AddrDB, "hash key", cfg.Key)

	var storage repository.Storage
	var dbConn *sql.DB

	if cfg.AddrDB != "" {
		dbConn, err := db.ConnectDB(cfg.AddrDB, sugar)
		if err != nil {
			sugar.Errorw("Failed to connect to DB", "error", err)
			return nil
		}

		if err := db.RunMigrations(cfg.AddrDB); err != nil {
			sugar.Fatalw("Failed to run migrations", "error", err)
		}

		storage = repository.NewDBStorage(dbConn)
	} else {
		storage = repository.NewMemStorage()
	}

	if cfg.Restore {
		if err := loadFromFile(storage, cfg.FileStorage, sugar); err != nil {
			sugar.Errorw("Failed to load metrics from file", "error", err)
		}
	}

	router := handler.NewRouter(storage, sugar, cfg)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	return &ServerComponents{
		server: srv,
		store:  storage,
		logger: sugar,
		dbConn: dbConn,
	}
}

func setupPeriodicSaver(cfg config.Config, storage repository.Storage, sugar *zap.SugaredLogger) *PeriodicSaver {
	if cfg.StoreInterval <= 0 {
		sugar.Infow("Periodic save disabled", "storeInterval", cfg.StoreInterval)
		return nil
	}

	saver := NewPeriodicSaver(storage, cfg.FileStorage, time.Duration(cfg.StoreInterval)*time.Second, sugar)
	saver.Start()

	return saver
}

// NewPeriodicSaver создает новый экземпляр PeriodicSaver, который будет сохранять метрики
// в указанный файл с заданным интервалом. Сохранение необходимо запустить методом Start
// и остановить методом Stop когда оно больше не требуется.
func NewPeriodicSaver(store repository.Storage, filePath string, interval time.Duration, logger *zap.SugaredLogger) *PeriodicSaver {
	return &PeriodicSaver{
		store:    store,
		interval: interval,
		filePath: filePath,
		logger:   logger,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start запускает операцию периодического сохранения в фоновой горутине.
// Метрики будут сохраняться на диск с настроенным интервалом до вызова Stop.
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

// Stop корректно останавливает операцию периодического сохранения и ожидает
// завершения фоновой горутины.
func (ps *PeriodicSaver) Stop() {
	if ps.stopCh != nil {
		close(ps.stopCh)
		<-ps.done
	}
}

func runServerWithGracefulShutdown(components *ServerComponents, saver *PeriodicSaver, cfg config.Config) error {
	server := components.server
	storage := components.store
	sugar := components.logger

	go func() {
		pprofAddr := "localhost:6060"
		sugar.Infow("pprof server started", "address", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			sugar.Errorw("pprof server error", "error", err)
		}
	}()

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

	return gracefulShutdown(cfg, sugar, storage, server, saver, components.dbConn)
}

func gracefulShutdown(cfg config.Config, sugar *zap.SugaredLogger, store repository.Storage, srv *http.Server, saver *PeriodicSaver, dbConn *sql.DB) error {
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

	if dbConn != nil {
		sugar.Infow("Closing database connection")
		if err := dbConn.Close(); err != nil {
			sugar.Errorw("Error closing database connection", "error", err)
		}
	}

	sugar.Infoln("Metrics saved and server stopped gracefully")
	return nil
}

func saveToFile(store repository.Storage, fileName string, sugar *zap.SugaredLogger) error {
	if fileName == "" {
		sugar.Debugw("Save skipped - no filename specified")
		return nil
	}

	sugar.Debugw("Starting save to file", "file", fileName)

	allMetrics, err := store.GetAll()
	if err != nil {
		return fmt.Errorf("failed to get all metrics: %w", err)
	}
	sugar.Debugw("Retrieved metrics from storage", "count", len(allMetrics.List))

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

func loadFromFile(store repository.Storage, fileName string, sugar *zap.SugaredLogger) error {
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
	for _, m := range metrics.List {
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

func writeFile(fileName string, data []byte) error {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write  %w", err)
	}

	return nil
}

func deserializeMetrics(data []byte, fileName string) (*models.ListMetrics, error) {
	var metrics models.ListMetrics

	if err := metrics.UnmarshalJSON(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metric from %s: %w", fileName, err)
	}
	return &metrics, nil
}

func serializeMetrics(metrics *models.ListMetrics) ([]byte, error) {
	return metrics.MarshalJSON()
}
