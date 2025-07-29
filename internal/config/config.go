package config

import (
	"flag"
	"strconv"
)

type Config struct {
	Addr          string `env:"ADDRESS"`
	StoreInterval int    `env:"STORE_INTERVAL"`
	FileStorage   string `env:"FILE_STORAGE_PATH"`
	Restore       bool   `env:"RESTORE"`
}

func GetConfig() (Config, error) {
	addrFlag := flag.String("a", "localhost:8080", "HTTP server address")
	storeIntFlag := flag.String("i", "300", "store interval in seconds")
	fileFlag := flag.String("f", "storage.json", "path to storage file")
	restoreFlag := flag.String("r", "true", "restore metrics from file on startup (true/false)")

	flag.Parse()

	cfg := Config{
		Addr:          getString("ADDRESS", *addrFlag),
		FileStorage:   getString("FILE_STORAGE_PATH", *fileFlag),
		StoreInterval: getInt("STORE_INTERVAL", *storeIntFlag),
		Restore:       getBool("RESTORE", *restoreFlag),
	}

	return cfg, nil
}

func getString(envValue, flagValue string) string {
	if envValue != "" {
		return envValue
	}
	return flagValue
}

func getInt(envValue, flagValue string) int {
	if envValue != "" {
		if v, err := strconv.Atoi(envValue); err == nil {
			return v
		}
	}
	v, _ := strconv.Atoi(flagValue)
	return v
}

func getBool(envValue, flagValue string) bool {
	if envValue != "" {
		if v, err := strconv.ParseBool(envValue); err == nil {
			return v
		}
	}
	v, _ := strconv.ParseBool(flagValue)
	return v
}
