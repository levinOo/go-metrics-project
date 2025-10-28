// Package db предоставляет функциональность для работы с базой данных PostgreSQL.
// Включает подключение к БД с повторными попытками, проверку ошибок соединения
// и выполнение миграций схемы базы данных.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DataBaseConnection устанавливает соединение с базой данных PostgreSQL.
// Использует драйвер pgx для подключения.
//
// Параметры:
//
//	cfgAddrDB: строка подключения в формате PostgreSQL DSN
//	           (например, "postgres://user:password@localhost:5432/dbname?sslmode=disable")
//
// Возвращает открытое соединение *sql.DB или ошибку при неудаче.
// Не выполняет проверку доступности базы данных - только открывает соединение.
func DataBaseConnection(cfgAddrDB string) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfgAddrDB)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// ConnectDB устанавливает соединение с базой данных с автоматическими повторными попытками.
// При ошибках подключения выполняет до 3 попыток с экспоненциальными задержками:
// 1 секунда, 3 секунды, 5 секунд.
//
// Повторные попытки выполняются только для ошибок подключения PostgreSQL (класс 08)
// и системных ошибок соединения (ECONNREFUSED).
//
// Параметры:
//
//	cfgAddrDB: строка подключения PostgreSQL DSN
//	sugar: логгер для записи информации о попытках подключения
//
// Возвращает установленное соединение или ошибку после всех неудачных попыток.
func ConnectDB(cfgAddrDB string, sugar *zap.SugaredLogger) (*sql.DB, error) {
	var dbConn *sql.DB
	intervals := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

	dbConn, err := DataBaseConnection(cfgAddrDB)

	if isPostgreSQLConnectionError(err) {
		for i := 0; i < 3; i++ {
			sugar.Infow("Database connection retry", "attempt", i+1, "error", err)
			time.Sleep(intervals[i])

			dbConn, err = DataBaseConnection(cfgAddrDB)
			if err == nil {
				sugar.Infow("Database connected after retries", "attempts", i+1)
				break
			}

			if !isPostgreSQLConnectionError(err) {
				break
			}
		}
	}

	if err != nil {
		if dbConn != nil {
			dbConn.Close()
		}
		sugar.Errorw("Failed to connect to the database after retries", "error", err)
		return nil, err
	}

	return dbConn, nil
}

// isPostgreSQLConnectionError проверяет, является ли ошибка проблемой соединения с PostgreSQL.
// Определяет следующие типы ошибок подключения:
//   - Ошибки PostgreSQL класса 08 (Connection Exception)
//   - Системная ошибка ECONNREFUSED (соединение отклонено)
//   - Строковые ошибки, содержащие "connection refused", "dial tcp", "connect:"
//
// Возвращает true, если ошибка связана с подключением и имеет смысл повторить попытку.
// Возвращает false для nil или других типов ошибок.
func isPostgreSQLConnectionError(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code[:2] == "08"
	}

	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "connect:")
}

// RunMigrations выполняет миграции базы данных из директории migrations/.
// Использует библиотеку golang-migrate для применения SQL-миграций.
//
// Миграции должны находиться в директории "./migrations" относительно рабочей директории.
// Файлы миграций должны следовать формату: {version}_{name}.up.sql и {version}_{name}.down.sql
//
// Параметры:
//
//	dbConnString: строка подключения PostgreSQL DSN
//
// Возвращает nil при успешном применении миграций или если миграции уже применены.
// Возвращает ошибку при проблемах с созданием экземпляра migrate или применением миграций.
//
// Пример структуры директории migrations:
//
//	migrations/
//	  001_create_metrics_table.up.sql
//	  001_create_metrics_table.down.sql
//	  002_add_indexes.up.sql
//	  002_add_indexes.down.sql
func RunMigrations(dbConnString string) error {
	migrationsPath := "file://migrations"
	m, err := migrate.New(
		migrationsPath,
		dbConnString,
	)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}
