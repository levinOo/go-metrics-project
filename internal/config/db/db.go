package db

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func DataBaseConnection(cfgAddrDB string) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfgAddrDB)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func CreateTableDB(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS metrics (
		name TEXT PRIMARY KEY,
		type TEXT,
		value DOUBLE PRECISION,
		delta BIGINT
	);`

	_, err := db.Exec(query)
	if err != nil {
		return err
	}
	return nil
}
