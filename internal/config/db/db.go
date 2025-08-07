package db

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func DataBaseConnection(ctx context.Context, cfgAddrDB string) error {
	db, err := sql.Open("pgx", cfgAddrDB)
	if err != nil {
		return err
	}
	defer db.Close()

	err = db.PingContext(ctx)
	if err != nil {
		return err
	}

	return nil
}
