package config

import (
	"flag"
	"os"
	"strconv"
)

type Config struct {
	Addr          string `env:"ADDRESS"`
	StoreInterval int    `env:"STORE_INTERVAL"`
	FileStorage   string `env:"FILE_STORAGE_PATH"`
	Restore       bool   `env:"RESTORE"`
	AddrDB        string `env:"DATABASE_DSN"`
}

func GetConfig() (Config, error) {
	addrFlag := flag.String("a", "localhost:8080", "HTTP server address")
	storeIntFlag := flag.String("i", "300", "store interval in seconds")
	fileFlag := flag.String("f", "storage.json", "path to storage file")
	restoreFlag := flag.String("r", "false", "restore metrics from file on startup (true/false)")
	addrDBFlag := flag.String("d", "", "Database addres")

	flag.Parse()

	cfg := Config{
		Addr:          getString(os.Getenv("ADDRESS"), *addrFlag),
		FileStorage:   getString(os.Getenv("FILE_STORAGE_PATH"), *fileFlag),
		StoreInterval: getInt(os.Getenv("STORE_INTERVAL"), *storeIntFlag),
		Restore:       getBool(os.Getenv("RESTORE"), *restoreFlag),
		AddrDB:        getString(os.Getenv("DATABASE_DSN"), *addrDBFlag),
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
