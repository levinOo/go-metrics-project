package main

import (
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
	cfg := config.GetConfig()

	store := repository.NewMemStorage()

	return handler.Serve(store, cfg)

}
