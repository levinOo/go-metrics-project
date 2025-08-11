package migrations

import (
	"fmt"

	"github.com/golang-migrate/migrate"

	_ "github.com/golang-migrate/migrate/v4/source/file"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
)

func RunMigrations(dbConnString, migrationsPath string) error {
	m, err := migrate.New(
		"file://"+migrationsPath,
		"postgres://"+dbConnString,
	)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}
	defer m.Close()

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}
