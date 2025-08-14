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

func DataBaseConnection(cfgAddrDB string) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfgAddrDB)
	if err != nil {
		return nil, err
	}

	return db, nil
}

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
