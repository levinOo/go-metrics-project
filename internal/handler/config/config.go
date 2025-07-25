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
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}

	addr := flag.String("a", "", "address of HTTP server")
	storeInterval := flag.Int("i", 300, "time interval in seconds for saving data")
	fileStorage := flag.String("f", "", "path to storage file")
	restore := flag.String("r", "false", "restore metrics from file on startup")

	flag.Parse()

	if *addr != "" {
		cfg.Addr = *addr
	}
	if *storeInterval >= 0 {
		cfg.StoreInterval = *storeInterval
	}
	if *fileStorage != "" {
		cfg.FileStorage = *fileStorage
	}
	if *restore != "" {
		if val, err := strconv.ParseBool(*restore); err == nil {
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
