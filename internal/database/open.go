package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Open membuka koneksi ke database sesuai Config yang diberikan.
// Mengembalikan instance *sql.DB dan dialek yang aktif.
func Open(cfg *Config) (*sql.DB, Dialect, error) {
	var driverName string
	var dsn string

	switch cfg.Connection {
	case DialectSQLite:
		driverName = "sqlite"
		if cfg.URL != "" {
			dsn = cfg.URL
		} else {
			if cfg.Database != ":memory:" {
				dir := filepath.Dir(cfg.Database)
				if dir != "." && dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						return nil, cfg.Connection, fmt.Errorf("gagal membuat direktori database sqlite: %w", err)
					}
				}
			}
			// WAL mode, busy timeout 5000ms, foreign keys aktif
			dsn = fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", cfg.Database)
		}

	case DialectMySQL, DialectMariaDB:
		driverName = "mysql"
		if cfg.URL != "" {
			dsn = cfg.URL
		} else {
			// parseTime=true wajib untuk scan TIME/DATETIME/TIMESTAMP ke time.Time
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
				cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
		}

	case DialectPgSQL:
		driverName = "pgx"
		if cfg.URL != "" {
			dsn = cfg.URL
		} else {
			sslmode := cfg.SSLMode
			if sslmode == "" {
				sslmode = "disable"
			}
			userInfo := ""
			if cfg.Username != "" {
				if cfg.Password != "" {
					userInfo = fmt.Sprintf("%s:%s@", url.QueryEscape(cfg.Username), url.QueryEscape(cfg.Password))
				} else {
					userInfo = fmt.Sprintf("%s@", url.QueryEscape(cfg.Username))
				}
			}
			dsn = fmt.Sprintf("postgres://%s%s:%d/%s?sslmode=%s",
				userInfo, cfg.Host, cfg.Port, cfg.Database, sslmode)
		}

	default:
		return nil, cfg.Connection, fmt.Errorf("driver tidak dikenal: %s", cfg.Connection)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, cfg.Connection, fmt.Errorf("gagal membuka koneksi database (%s): %w", cfg.Connection, err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, cfg.Connection, fmt.Errorf("gagal ping database (%s): %w", cfg.Connection, err)
	}

	return db, cfg.Connection, nil
}
