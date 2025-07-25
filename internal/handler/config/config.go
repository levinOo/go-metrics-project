package config

import (
	"flag"
	"strconv"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Addr          string `env:"ADDRESS"`
	StoreInterval int    `env:"STORE_INTERVAL"`
	FileStorage   string `env:"FILE_STORAGE_PATH"`
	Restore       bool   `env:"RESTORE"`
}

func GetConfig() (Config, error) {
	// 1. Сначала парсим переменные окружения
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}

	addrFlag := flag.String("a", "localhost:8080", "address of HTTP server")
	intervalFlag := flag.Int("i", 300, "time interval in seconds for saving data")
	fileFlag := flag.String("f", "storage.txt", "path to storage file")
	restoreFlag := flag.String("r", "false", "restore metrics from file on startup (true/false)")

	flag.Parse()

	if *addrFlag != "" {
		cfg.Addr = *addrFlag
	}
	if *intervalFlag >= 0 {
		cfg.StoreInterval = *intervalFlag
	}
	if *fileFlag != "" {
		cfg.FileStorage = *fileFlag
	}
	if *restoreFlag != "" {
		if val, err := strconv.ParseBool(*restoreFlag); err == nil {
			cfg.Restore = val
		}
	}

	if cfg.Addr == "" {
		cfg.Addr = "localhost:8080"
	}
	if cfg.StoreInterval == 0 {
		cfg.StoreInterval = 300
	}
	if cfg.FileStorage == "" {
		cfg.FileStorage = "storage.txt"
	}

	return cfg, nil
}
