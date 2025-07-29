package main

import (
	"fmt"
	"log"

	"github.com/levinOo/go-metrics-project/internal/config"
	"github.com/levinOo/go-metrics-project/internal/handler"
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

	return handler.Serve(cfg)

}
