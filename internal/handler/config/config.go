package config

import (
	"flag"

	"github.com/caarlos0/env"
)

type Config struct {
	Addr          string `env:"ADDRESS"`
	StoreInterval int    `env:"STORE_INTERVAL"`
	FileStorage   string `env:"FILE_STORAGE_PATH"`
	Restore       bool   `env:"RESTORE"`
}

func GetConfig() (cfg Config, err error) {
	cfg = Config{}

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "address of HTTP server")
	flag.IntVar(&cfg.StoreInterval, "i", 10, "time interval in seconds for saving data")
	flag.StringVar(&cfg.FileStorage, "f", "storage.txt", "path to storage file")
	flag.BoolVar(&cfg.Restore, "r", false, "restore metrics from file on startup")

	flag.Parse()

	err = env.Parse(&cfg)

	return cfg, err
}
