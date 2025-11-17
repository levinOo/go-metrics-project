package config

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
)

type ConfigStruct struct {
	Addr          string `json:"address"`
	Key           string `json:"key"`
	PollInterval  int    `json:"poll_interval"`
	ReqInterval   int    `json:"req_intervaal"`
	RateLimit     int    `json:"rate_limit"`
	CryptoKeyPath string `json:"crypto_key"`
}

type Config struct {
	Addr          string `env:"ADDRESS"`
	Key           string `env:"KEY"`
	PollInterval  int    `env:"POLL_INTERVAL"`
	ReqInterval   int    `env:"REPORT_INTERVAL"`
	RateLimit     int    `env:"RATE_LIMIT"`
	CryptoKeyPath string `env:"CRYPTO_KEY"`
}

func NewConfigStruct() *ConfigStruct {
	return &ConfigStruct{}
}

func NewConfig() *Config {
	return &Config{}
}

func GetAgentConfig(cfg *Config) error {
	configStruct := NewConfigStruct()

	addr := flag.String("a", "localhost:8080", "Адрес сервера")
	key := flag.String("k", "hello", "Ключ шифрования")
	configPathFlag := flag.String("config", "../internal/agent/config/config_example.json", "path to config file")
	cryptoKey := flag.String("c", "../keys/public.pem", "Публичный ключ шифрования")
	pollInterval := flag.String("p", "2", "Значение интервала обновления метрик в секундах")
	reqInterval := flag.String("r", "10", "Значение интервала отпрвки в секундах")
	rateLimit := flag.String("l", "1", "Значение Rate Limit")

	flag.Parse()

	configPath := getConfigPath(*configPathFlag, os.Getenv("CONFIG"))
	data, err := os.Open(configPath)
	if err != nil {
		log.Printf("Не удалось открыть файл: %v", err)
		return err
	}

	json.NewDecoder(data).Decode(configStruct)

	cfg.Addr = getString(os.Getenv("ADDRESS"), *addr, configStruct.Addr)
	cfg.Key = getString(os.Getenv("KEY"), *key, configStruct.Key)
	cfg.CryptoKeyPath = getString(os.Getenv("CRYPTO_KEY"), *cryptoKey, configStruct.CryptoKeyPath)
	cfg.PollInterval = getInt(os.Getenv("POLL_INTERVAL"), *pollInterval, configStruct.PollInterval)
	cfg.ReqInterval = getInt(os.Getenv("REPORT_INTERVAL"), *reqInterval, configStruct.ReqInterval)
	cfg.RateLimit = getInt(os.Getenv("RATE_LIMIT"), *rateLimit, configStruct.RateLimit)

	return nil
}

func getString(envValue, flagValue, configValue string) string {
	if envValue != "" {
		return envValue
	} else if flagValue != "" {
		return flagValue
	}

	return configValue
}

func getInt(envValue, flagValue string, configValue int) int {
	if envValue != "" {
		if v, err := strconv.Atoi(envValue); err == nil {
			return v
		}
	} else if flagValue != "" {
		v, _ := strconv.Atoi(flagValue)
		return v
	}

	return configValue
}

func getConfigPath(flagValue, envValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return envValue
}
