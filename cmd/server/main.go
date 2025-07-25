package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/levinOo/go-metrics-project/internal/handler"
	"github.com/levinOo/go-metrics-project/internal/handler/config"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("ошибка парсинга ENV: %w", err)
	}

	store := repository.NewMemStorage()

	if cfg.Restore {
		file, err := os.ReadFile(cfg.FileStorage)
		if err != nil {
			return fmt.Errorf("error reading file: %v", err)
		}

		var metrics []models.Metrics
		if err := json.Unmarshal(file, &metrics); err != nil {
			return fmt.Errorf("error unmarshaling metrics: %v", err)
		}

		for _, m := range metrics {
			switch m.MType {
			case "gauge":
				if m.Value != nil {
					store.SetGauge(m.ID, repository.Gauge(*m.Value))
				}
			case "counter":
				if m.Delta != nil {
					store.SetCounter(m.ID, repository.Counter(*m.Delta))
				}
			default:
				log.Printf("unknown metric type: %s", m.MType)
			}
		}
	}

	return handler.Serve(store, cfg)

}
