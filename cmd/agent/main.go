package main

import (
	"fmt"
	"log"

	"github.com/levinOo/go-metrics-project/internal/agent"
)

var (
	buildVersion string = "N/A"
	buildDate    string = "N/A"
	buildCommit  string = "N/A"
)

func main() {
	fmt.Printf("Build version: %s\n", buildVersion)
	fmt.Printf("Build date: %s\n", buildDate)
	fmt.Printf("Build commit: %s\n", buildCommit)

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	errCh := agent.StartAgent()
	return <-errCh
}
