package database

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Dialect adalah nama mesin database yang didukung.
type Dialect string

const (
	DialectSQLite  Dialect = "sqlite"
	DialectMySQL   Dialect = "mysql"
	DialectMariaDB Dialect = "mariadb"
	DialectPgSQL   Dialect = "pgsql"
)

// Config menyimpan seluruh parameter koneksi database.
type Config struct {
	Connection  Dialect
	Database    string
	Host        string
	Port        int
	Username    string
	Password    string
	SSLMode     string
	URL         string
	AutoMigrate bool
}

// LoadConfig membaca environment variable dan berkas .env opsional.
// Nilai environment asli selalu diutamakan dibanding isi file .env.
func LoadConfig(defaultDBName string) (*Config, error) {
	_ = godotenv.Load()

	conn := strings.ToLower(getEnv("DB_CONNECTION", "sqlite"))
	var dialect Dialect
	switch conn {
	case "sqlite":
		dialect = DialectSQLite
	case "mysql":
		dialect = DialectMySQL
	case "mariadb":
		dialect = DialectMariaDB
	case "pgsql", "postgres", "postgresql":
		dialect = DialectPgSQL
	default:
		return nil, fmt.Errorf("DB_CONNECTION tidak didukung: %s (pilihan: sqlite, mysql, mariadb, pgsql)", conn)
	}

	defaultPort := 3306
	if dialect == DialectPgSQL {
		defaultPort = 5432
	}

	port := defaultPort
	if portStr := os.Getenv("DB_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	defaultDB := fmt.Sprintf("./data/%s.db", defaultDBName)
	if dialect != DialectSQLite {
		defaultDB = defaultDBName
	}

	autoMigrate := true
	if amStr := os.Getenv("DB_AUTO_MIGRATE"); amStr != "" {
		autoMigrate = strings.ToLower(amStr) == "true" || amStr == "1"
	}

	cfg := &Config{
		Connection:  dialect,
		Database:    getEnv("DB_DATABASE", defaultDB),
		Host:        getEnv("DB_HOST", "127.0.0.1"),
		Port:        port,
		Username:    os.Getenv("DB_USERNAME"),
		Password:    os.Getenv("DB_PASSWORD"),
		SSLMode:     getEnv("DB_SSLMODE", "disable"),
		URL:         os.Getenv("DB_URL"),
		AutoMigrate: autoMigrate,
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
