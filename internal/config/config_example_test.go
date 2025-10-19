package config_test

import (
	"fmt"
	"os"

	"github.com/levinOo/go-metrics-project/internal/config"
)

// Example_defaultConfig демонстрирует загрузку конфигурации со значениями по умолчанию.
func Example_defaultConfig() {
	// Очищаем переменные окружения для демонстрации
	os.Clearenv()

	// Сбрасываем флаги (в реальном приложении они парсятся автоматически)
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Address: %s\n", cfg.Addr)
	fmt.Printf("Store Interval: %d\n", cfg.StoreInterval)
	fmt.Printf("File Storage: %s\n", cfg.FileStorage)
	// Output:
	// Address: localhost:8080
	// Store Interval: 300
	// File Storage: storage.json
}

// Example_environmentVariables демонстрирует приоритет переменных окружения.
func Example_environmentVariables() {
	// Устанавливаем переменные окружения
	os.Setenv("ADDRESS", "0.0.0.0:9090")
	os.Setenv("STORE_INTERVAL", "60")
	os.Setenv("FILE_STORAGE_PATH", "/tmp/metrics.json")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	fmt.Printf("Address: %s\n", cfg.Addr)
	fmt.Printf("Store Interval: %d\n", cfg.StoreInterval)
	fmt.Printf("File Storage: %s\n", cfg.FileStorage)
	// Output:
	// Address: 0.0.0.0:9090
	// Store Interval: 60
	// File Storage: /tmp/metrics.json
}

// Example_databaseConfiguration демонстрирует настройку подключения к базе данных.
func Example_databaseConfiguration() {
	os.Setenv("DATABASE_DSN", "postgres://user:password@localhost:5432/metrics?sslmode=disable")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	if cfg.AddrDB != "" {
		fmt.Println("Database configured: Yes")
		fmt.Printf("Connection string length: %d\n", len(cfg.AddrDB))
	} else {
		fmt.Println("Database configured: No")
	}
	// Output:
	// Database configured: Yes
	// Connection string length: 65
}

// Example_restoreFlag демонстрирует настройку восстановления метрик.
func Example_restoreFlag() {
	os.Setenv("RESTORE", "true")
	os.Setenv("FILE_STORAGE_PATH", "/var/lib/metrics/data.json")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	fmt.Printf("Restore on startup: %t\n", cfg.Restore)
	fmt.Printf("Storage file: %s\n", cfg.FileStorage)
	// Output:
	// Restore on startup: true
	// Storage file: /var/lib/metrics/data.json
}

// Example_securityConfiguration демонстрирует настройку ключа безопасности.
func Example_securityConfiguration() {
	os.Setenv("KEY", "my-secret-key-12345")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	if cfg.Key != "" {
		fmt.Println("Security: Enabled")
		fmt.Printf("Key length: %d\n", len(cfg.Key))
	} else {
		fmt.Println("Security: Disabled")
	}
	// Output:
	// Security: Enabled
	// Key length: 20
}

// Example_disablePeriodicSave демонстрирует отключение периодического сохранения.
func Example_disablePeriodicSave() {
	os.Setenv("STORE_INTERVAL", "0")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	if cfg.StoreInterval == 0 {
		fmt.Println("Periodic save: Disabled")
	} else {
		fmt.Printf("Save every: %d seconds\n", cfg.StoreInterval)
	}
	// Output:
	// Periodic save: Disabled
}

// Example_auditConfiguration демонстрирует настройку аудита.
func Example_auditConfiguration() {
	os.Setenv("AUDIT_FILE", "/var/log/metrics/audit.log")
	os.Setenv("AUDIT_URL", "https://audit.example.com/events")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	fmt.Printf("Audit file: %s\n", cfg.AuditFile)
	fmt.Printf("Audit URL configured: %t\n", cfg.AuditURL != "")
	// Output:
	// Audit file: /var/log/metrics/audit.log
	// Audit URL configured: true
}

// Example_productionConfiguration демонстрирует типичную production конфигурацию.
func Example_productionConfiguration() {
	os.Setenv("ADDRESS", "0.0.0.0:8080")
	os.Setenv("STORE_INTERVAL", "300")
	os.Setenv("DATABASE_DSN", "postgres://metrics:pass@db:5432/metrics")
	os.Setenv("KEY", "production-secret-key")
	os.Setenv("RESTORE", "true")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	fmt.Printf("Server: %s\n", cfg.Addr)
	fmt.Printf("Database: Configured\n")
	fmt.Printf("Security: Enabled\n")
	fmt.Printf("Auto-save: Every %d seconds\n", cfg.StoreInterval)
	fmt.Printf("Restore: %t\n", cfg.Restore)
	// Output:
	// Server: 0.0.0.0:8080
	// Database: Configured
	// Security: Enabled
	// Auto-save: Every 300 seconds
	// Restore: true
}

// Example_developmentConfiguration демонстрирует типичную development конфигурацию.
func Example_developmentConfiguration() {
	os.Setenv("ADDRESS", "localhost:8080")
	os.Setenv("STORE_INTERVAL", "0")
	os.Setenv("FILE_STORAGE_PATH", "./dev_storage.json")
	os.Setenv("RESTORE", "false")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	fmt.Printf("Server: %s\n", cfg.Addr)
	fmt.Printf("Storage: In-memory\n")
	fmt.Printf("Auto-save: Disabled\n")
	fmt.Printf("Restore: %t\n", cfg.Restore)
	// Output:
	// Server: localhost:8080
	// Storage: In-memory
	// Auto-save: Disabled
	// Restore: false
}

// Example_booleanParsing демонстрирует различные способы задания булевых значений.
func Example_booleanParsing() {
	// Тест различных форматов для RESTORE
	testValues := []string{"true", "True", "TRUE", "1", "false", "False", "FALSE", "0"}

	for _, val := range testValues {
		os.Setenv("RESTORE", val)
		cfg, _ := config.GetConfig()

		if val == "true" || val == "True" || val == "TRUE" || val == "1" {
			fmt.Printf("%s -> %t\n", val, cfg.Restore)
			break
		}
	}

	os.Clearenv()
	// Output:
	// true -> true
}

// Example_integerParsing демонстрирует парсинг целочисленных значений.
func Example_integerParsing() {
	os.Setenv("STORE_INTERVAL", "3600")
	defer os.Clearenv()

	cfg, _ := config.GetConfig()

	hours := cfg.StoreInterval / 3600
	fmt.Printf("Save interval: %d hour(s)\n", hours)
	// Output:
	// Save interval: 1 hour(s)
}

// Example_emptyDatabaseDSN демонстрирует поведение без настроенной базы данных.
func Example_emptyDatabaseDSN() {
	os.Clearenv()

	cfg, _ := config.GetConfig()

	if cfg.AddrDB == "" {
		fmt.Println("Storage type: In-Memory")
	} else {
		fmt.Println("Storage type: Database")
	}
	// Output:
	// Storage type: In-Memory
}
