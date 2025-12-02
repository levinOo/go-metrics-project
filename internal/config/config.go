// Package config предоставляет функциональность для управления конфигурацией приложения.
// Поддерживает загрузку настроек из переменных окружения и флагов командной строки,
// с приоритетом переменных окружения над флагами.
package config

//go:generate go run ../../cmd/reset/main.go

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
)

// Config содержит все параметры конфигурации сервера метрик.
// Значения загружаются из переменных окружения (указаны в тегах env)
// или из флагов командной строки, если переменные окружения не установлены.
type ConfigStruct struct {
	Addr          string `json:"address"`
	StoreInterval int    `json:"store_interval"`
	FileStorage   string `json:"file_storage_path"`
	Restore       bool   `json:"restore"`
	AddrDB        string `json:"database_dsn"`
	Key           string `json:"key"`
	CryptoKeyPath string `json:"crypto_key"`
	AuditFile     string `json:"audit_file"`
	AuditURL      string `json:"audit_url"`
}

// generate:reset
type Config struct {
	// Addr задает адрес и порт HTTP-сервера (например, "localhost:8080").
	Addr string `env:"ADDRESS"`

	// StoreInterval определяет интервал в секундах между автоматическими сохранениями метрик на диск.
	// Значение 0 отключает периодическое сохранение.
	StoreInterval int `env:"STORE_INTERVAL"`

	// FileStorage указывает путь к файлу для хранения метрик на диске.
	FileStorage string `env:"FILE_STORAGE_PATH"`

	ConfigFilePath string `env:"CONFIG"`

	// Restore определяет, нужно ли восстанавливать метрики из файла при запуске сервера.
	Restore bool `env:"RESTORE"`

	// AddrDB содержит строку подключения к базе данных PostgreSQL (DSN).
	// Если не указано, используется хранилище в памяти.
	AddrDB string `env:"DATABASE_DSN"`

	// Key содержит секретный ключ для подписи запросов HMAC SHA256.
	// Пустое значение отключает проверку подписей.
	Key string `env:"KEY"`

	CryptoKeyPath string `env:"CRYPTO_KEY"`

	// AuditFile указывает путь к файлу для записи аудит-логов.
	AuditFile string `env:"AUDIT_FILE"`

	// AuditURL содержит URL для отправки аудит-событий на внешний сервис.
	AuditURL string `env:"AUDIT_URL"`
}

func NewConfigStruct() *ConfigStruct {
	return &ConfigStruct{}
}

// GetConfig загружает и возвращает конфигурацию приложения.
// Сначала обрабатываются флаги командной строки, затем переменные окружения.
// Переменные окружения имеют приоритет над флагами.
//
// Поддерживаемые флаги:
//
//	-a: адрес сервера (по умолчанию "localhost:8080")
//	-i: интервал сохранения в секундах (по умолчанию "300")
//	-f: путь к файлу хранилища (по умолчанию "storage.json")
//	-r: восстанавливать ли метрики при запуске (по умолчанию "false")
//	-d: строка подключения к базе данных (по умолчанию "")
//	-k: ключ для HMAC (по умолчанию "")
//	-p: путь к файлу аудита (по умолчанию "./audit.json")
//	-u: URL для аудита (по умолчанию "")
//
// Соответствующие переменные окружения:
//
//	ADDRESS, STORE_INTERVAL, FILE_STORAGE_PATH, RESTORE,
//	DATABASE_DSN, KEY, AUDIT_FILE, AUDIT_URL
func GetConfig() (Config, error) {
	configStruct := NewConfigStruct()

	addrFlag := flag.String("a", "localhost:8080", "HTTP server address")
	storeIntFlag := flag.String("i", "300", "store interval in seconds")
	fileFlag := flag.String("f", "storage.json", "path to storage file")
	configPathFlag := flag.String("config", "../internal/config/config_example.json", "path to config file")
	restoreFlag := flag.String("r", "false", "restore metrics from file on startup (true/false)")
	addrDBFlag := flag.String("d", "", "Database address")
	key := flag.String("k", "hello", "Hash key")
	cryptoKeyPath := flag.String("c", "../keys/private.pem", "crypto key")
	auditFile := flag.String("p", "./audit.json", "audit file path")
	auditURL := flag.String("u", "", "audit url")

	flag.Parse()

	configPath := getConfigPath(*configPathFlag, os.Getenv("CONFIG"))

	data, err := os.Open(configPath)
	if err != nil {
		log.Printf("Не удалось открыть файл: %v", err)
		return Config{}, err
	}

	json.NewDecoder(data).Decode(configStruct)

	cfg := Config{
		Addr:          getString(os.Getenv("ADDRESS"), *addrFlag, configStruct.Addr),
		FileStorage:   getString(os.Getenv("FILE_STORAGE_PATH"), *fileFlag, configStruct.FileStorage),
		StoreInterval: getInt(os.Getenv("STORE_INTERVAL"), *storeIntFlag, configStruct.StoreInterval),
		Restore:       getBool(os.Getenv("RESTORE"), *restoreFlag, configStruct.Restore),
		AddrDB:        getString(os.Getenv("DATABASE_DSN"), *addrDBFlag, configStruct.AddrDB),
		Key:           getString(os.Getenv("KEY"), *key, configStruct.Key),
		CryptoKeyPath: getString(os.Getenv("CRYPTO_KEY"), *cryptoKeyPath, configStruct.CryptoKeyPath),
		AuditFile:     getString(os.Getenv("AUDIT_FILE"), *auditFile, configStruct.AuditFile),
		AuditURL:      getString(os.Getenv("AUDIT_URL"), *auditURL, configStruct.AuditURL),
	}

	return cfg, nil
}

// getString возвращает значение переменной окружения, если она установлена,
// иначе возвращает значение флага командной строки.
func getString(envValue, flagValue, configValue string) string {
	if envValue != "" {
		return envValue
	} else if flagValue != "" {
		return flagValue
	}

	return configValue
}

// getInt преобразует строковое значение переменной окружения или флага в целое число.
// Приоритет отдается переменной окружения. При ошибке преобразования возвращает 0.
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

// getBool преобразует строковое значение переменной окружения или флага в булево значение.
// Приоритет отдается переменной окружения. При ошибке преобразования возвращает false.
// Принимаются значения: "1", "t", "T", "true", "TRUE", "True", "0", "f", "F", "false", "FALSE", "False".
func getBool(envValue, flagValue string, configValue bool) bool {
	if envValue != "" {
		if v, err := strconv.ParseBool(envValue); err == nil {
			return v
		}
	} else if flagValue != "" {
		v, _ := strconv.ParseBool(flagValue)
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
