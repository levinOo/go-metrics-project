package config

import (
	"flag"
	"os"
	"strconv"
)

type Config struct {
	Addr          string
	StoreInterval int
	FileStorage   string
	Restore       bool
}

func GetConfig() (Config, error) {
	defaultAddr := "localhost:8080"
	defaultStoreInterval := 300
	defaultFileStorage := "storage.json"
	defaultRestore := true

	addr := flag.String("a", defaultAddr, "address of HTTP server")
	storeInterval := flag.Int("i", defaultStoreInterval, "time interval in seconds for saving data (0 = synchronous)")
	fileStorage := flag.String("f", defaultFileStorage, "path to storage file")
	restore := flag.String("r", strconv.FormatBool(defaultRestore), "restore metrics from file on startup (true/false)")

	flag.Parse()

	cfg := Config{}

	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		cfg.Addr = envAddr
	} else {
		cfg.Addr = *addr
	}

	if envInterval := os.Getenv("STORE_INTERVAL"); envInterval != "" {
		if val, err := strconv.Atoi(envInterval); err == nil {
			cfg.StoreInterval = val
		} else {
			cfg.StoreInterval = *storeInterval
		}
	} else {
		cfg.StoreInterval = *storeInterval
	}

	if envFile := os.Getenv("FILE_STORAGE_PATH"); envFile != "" {
		cfg.FileStorage = envFile
	} else {
		cfg.FileStorage = *fileStorage
	}

	if envRestore := os.Getenv("RESTORE"); envRestore != "" {
		if val, err := strconv.ParseBool(envRestore); err == nil {
			cfg.Restore = val
		} else {
			if val, err := strconv.ParseBool(*restore); err == nil {
				cfg.Restore = val
			} else {
				cfg.Restore = defaultRestore
			}
		}
	} else {
		if val, err := strconv.ParseBool(*restore); err == nil {
			cfg.Restore = val
		} else {
			cfg.Restore = defaultRestore
		}
	}

	return cfg, nil
}
