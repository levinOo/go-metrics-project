package db_test

import (
	"context"
	"fmt"
	"log"

	"github.com/levinOo/go-metrics-project/internal/config/db"
	"github.com/levinOo/go-metrics-project/internal/logger"
)

// Example_databaseConnection демонстрирует базовое подключение к базе данных.
func Example_databaseConnection() {
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.DataBaseConnection(dsn)
	if err != nil {
		log.Printf("Connection failed: %v", err)
		return
	}
	defer conn.Close()

	// Проверяем соединение
	if err := conn.Ping(); err != nil {
		log.Printf("Ping failed: %v", err)
		return
	}

	fmt.Println("Database connected successfully")
	// Output: Database connected successfully
}

// Example_connectWithRetry демонстрирует подключение с автоматическими повторными попытками.
func Example_connectWithRetry() {
	sugar := logger.NewLogger()
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.ConnectDB(dsn, sugar)
	if err != nil {
		log.Printf("Failed to connect after retries: %v", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected with retry logic")
	// Output: Connected with retry logic
}

// Example_runMigrations демонстрирует выполнение миграций базы данных.
func Example_runMigrations() {
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	// Запускаем миграции
	err := db.RunMigrations(dsn)
	if err != nil {
		log.Printf("Migration failed: %v", err)
		return
	}

	fmt.Println("Migrations applied successfully")
	// Output: Migrations applied successfully
}

// Example_checkConnection демонстрирует проверку активного соединения.
func Example_checkConnection() {
	sugar := logger.NewLogger()
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.ConnectDB(dsn, sugar)
	if err != nil {
		log.Printf("Connection error: %v", err)
		return
	}
	defer conn.Close()

	// Настраиваем пул соединений
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(0)

	// Проверяем соединение
	if err := conn.Ping(); err != nil {
		fmt.Println("Database unavailable")
		return
	}

	fmt.Println("Database available")
	// Output: Database available
}

// Example_connectionPoolSettings демонстрирует настройку пула соединений.
func Example_connectionPoolSettings() {
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.DataBaseConnection(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Оптимальные настройки для продакшена
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(10)
	conn.SetConnMaxLifetime(0)
	conn.SetConnMaxIdleTime(0)

	stats := conn.Stats()
	fmt.Printf("Max Open Connections: %d\n", stats.MaxOpenConnections)
	// Output: Max Open Connections: 25
}

// Example_transactionUsage демонстрирует использование транзакций.
func Example_transactionUsage() {
	sugar := logger.NewLogger()
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.ConnectDB(dsn, sugar)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Начинаем транзакцию
	tx, err := conn.Begin()
	if err != nil {
		log.Fatal(err)
	}

	// Выполняем операции
	_, err = tx.Exec("INSERT INTO metrics (name, value, type) VALUES ($1, $2, $3)",
		"test_metric", 42.5, "gauge")
	if err != nil {
		tx.Rollback()
		log.Fatal(err)
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Transaction completed")
	// Output: Transaction completed
}

// Example_preparedStatement демонстрирует использование подготовленных выражений.
func Example_preparedStatement() {
	sugar := logger.NewLogger()
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.ConnectDB(dsn, sugar)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Создаем подготовленное выражение
	stmt, err := conn.Prepare(`
		INSERT INTO metrics (name, value, type) 
		VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	// Используем многократно
	_, err = stmt.Exec("cpu_usage", 45.5, "gauge")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Prepared statement executed")
	// Output: Prepared statement executed
}

// Example_connectionWithContext демонстрирует использование контекста для таймаутов.
func Example_connectionWithContext() {
	sugar := logger.NewLogger()
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.ConnectDB(dsn, sugar)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Используем контекст с таймаутом
	ctx := context.Background()

	err = conn.PingContext(ctx)
	if err != nil {
		fmt.Println("Ping failed")
		return
	}

	fmt.Println("Ping with context succeeded")
	// Output: Ping with context succeeded
}

// Example_batchInsert демонстрирует пакетную вставку данных.
func Example_batchInsert() {
	sugar := logger.NewLogger()
	dsn := "postgres://user:password@localhost:5432/metrics?sslmode=disable"

	conn, err := db.ConnectDB(dsn, sugar)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Начинаем транзакцию для пакетной вставки
	tx, err := conn.Begin()
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO metrics (name, value, type) 
		VALUES ($1, $2, $3)
	`)
	if err != nil {
		tx.Rollback()
		log.Fatal(err)
	}
	defer stmt.Close()

	// Вставляем несколько записей
	metrics := []struct {
		name  string
		value float64
		mtype string
	}{
		{"cpu", 45.5, "gauge"},
		{"memory", 78.2, "gauge"},
		{"disk", 92.1, "gauge"},
	}

	for _, m := range metrics {
		_, err = stmt.Exec(m.name, m.value, m.mtype)
		if err != nil {
			tx.Rollback()
			log.Fatal(err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Inserted %d metrics\n", len(metrics))
	// Output: Inserted 3 metrics
}
