package db

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"path"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

//go:embed migrations/mysql/*.sql
var mysqlMigrations embed.FS

func Migrate(db *sqlx.DB, driver string) error {
	if err := ensureMigrationsTable(db, driver); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	var migFS embed.FS
	var dir string
	switch driver {
	case "sqlite":
		migFS = sqliteMigrations
		dir = "migrations/sqlite"
	case "mysql":
		migFS = mysqlMigrations
		dir = "migrations/mysql"
	default:
		return fmt.Errorf("unsupported driver for migrations: %s", driver)
	}

	entries, err := fs.ReadDir(migFS, dir)
	if err != nil {
		return fmt.Errorf("reading migration directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	applied, err := appliedMigrations(db)
	if err != nil {
		return fmt.Errorf("reading applied migrations: %w", err)
	}

	for _, file := range files {
		if applied[file] {
			continue
		}

		data, err := fs.ReadFile(migFS, path.Join(dir, file))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", file, err)
		}

		log.Printf("Applying migration: %s", file)

		// Run all statements in a transaction so partial failures roll back
		tx, err := db.Beginx()
		if err != nil {
			return fmt.Errorf("starting transaction for %s: %w", file, err)
		}

		statements := splitStatements(string(data))
		migErr := func() error {
			for _, stmt := range statements {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" {
					continue
				}
				if _, err := tx.Exec(stmt); err != nil {
					return fmt.Errorf("executing migration %s: %w\nStatement: %s", file, err, stmt)
				}
			}
			if _, err := tx.Exec("INSERT INTO schema_migrations (filename) VALUES (?)", file); err != nil {
				return fmt.Errorf("recording migration %s: %w", file, err)
			}
			return nil
		}()

		if migErr != nil {
			tx.Rollback()
			return migErr
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", file, err)
		}

		log.Printf("Migration applied: %s", file)
	}

	return nil
}

func ensureMigrationsTable(db *sqlx.DB, driver string) error {
	var query string
	switch driver {
	case "sqlite":
		query = `CREATE TABLE IF NOT EXISTS schema_migrations (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			filename  TEXT NOT NULL UNIQUE,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`
	case "mysql":
		query = `CREATE TABLE IF NOT EXISTS schema_migrations (
			id         INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			filename   VARCHAR(255) NOT NULL UNIQUE,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`
	}
	_, err := db.Exec(query)
	return err
}

func appliedMigrations(db *sqlx.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT filename FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

// splitStatements splits SQL text on semicolons, handling basic cases.
// This doesn't handle semicolons inside string literals, but our migrations
// are simple DDL/DML that don't contain embedded semicolons in values.
func splitStatements(sql string) []string {
	return strings.Split(sql, ";")
}
