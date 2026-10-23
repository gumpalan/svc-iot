package database

import (
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// Migrate menjalankan migrasi database yang tersimpan di embedded filesystem.
// tableName menentukan tabel pelacak versi (contoh: auth_migrations, iot_migrations).
func Migrate(db *sql.DB, dialect Dialect, migrationsFS fs.FS, dir string, tableName string) error {
	goose.SetBaseFS(migrationsFS)
	goose.SetTableName(tableName)

	var gooseDialect goose.Dialect
	switch dialect {
	case DialectSQLite:
		gooseDialect = goose.DialectSQLite3
	case DialectMySQL, DialectMariaDB:
		gooseDialect = goose.DialectMySQL
	case DialectPgSQL:
		gooseDialect = goose.DialectPostgres
	default:
		return fmt.Errorf("dialek migrasi tidak didukung: %s", dialect)
	}

	if err := goose.SetDialect(string(gooseDialect)); err != nil {
		return fmt.Errorf("gagal mengatur dialek goose (%s): %w", gooseDialect, err)
	}

	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("gagal menjalankan migrasi up: %w", err)
	}

	return nil
}
