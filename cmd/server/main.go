package main

import (
	"fmt"
	"log"

	"github.com/levinOo/go-metrics-project/internal/handler"
	"github.com/levinOo/go-metrics-project/internal/handler/config"
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

	return handler.Serve(store, cfg)

}
