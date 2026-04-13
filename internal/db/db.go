package db

import (
	"fmt"

	"github.com/scout-kit/fine-print/internal/config"
	"github.com/jmoiron/sqlx"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

func Open(cfg config.DatabaseConfig) (*sqlx.DB, error) {
	switch cfg.Driver {
	case "sqlite":
		return openSQLite(cfg.SQLitePath)
	case "mysql":
		return openMySQL(cfg.MySQLDSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
}

func openSQLite(path string) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// Enable WAL mode for concurrent read/write safety.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	// SQLite works best with a single writer connection.
	db.SetMaxOpenConns(1)

	return db, nil
}

func openMySQL(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening mysql database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to mysql: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	return db, nil
}
