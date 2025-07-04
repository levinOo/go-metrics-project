package main

import (
	"log"

	"github.com/levinOo/go-metrics-project/internal/agent"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	errCh := agent.StartAgent()
	return <-errCh
}
