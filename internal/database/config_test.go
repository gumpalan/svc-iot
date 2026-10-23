package database

import (
	"os"
	"testing"
)

func TestLoadConfig_DefaultSQLite(t *testing.T) {
	os.Clearenv()

	cfg, err := LoadConfig("testsvc")
	if err != nil {
		t.Fatalf("LoadConfig gagal: %v", err)
	}

	if cfg.Connection != DialectSQLite {
		t.Errorf("ekspektasi dialek sqlite, dapat: %s", cfg.Connection)
	}
	if cfg.Database != "./data/testsvc.db" {
		t.Errorf("ekspektasi default db ./data/testsvc.db, dapat: %s", cfg.Database)
	}
	if !cfg.AutoMigrate {
		t.Errorf("ekspektasi AutoMigrate true secara default")
	}
}

func TestLoadConfig_Postgres(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("DB_CONNECTION", "pgsql")
	_ = os.Setenv("DB_DATABASE", "mydb")
	_ = os.Setenv("DB_PORT", "5433")

	cfg, err := LoadConfig("testsvc")
	if err != nil {
		t.Fatalf("LoadConfig gagal: %v", err)
	}

	if cfg.Connection != DialectPgSQL {
		t.Errorf("ekspektasi pgsql, dapat: %s", cfg.Connection)
	}
	if cfg.Port != 5433 {
		t.Errorf("ekspektasi port 5433, dapat: %d", cfg.Port)
	}
	if cfg.Database != "mydb" {
		t.Errorf("ekspektasi database mydb, dapat: %s", cfg.Database)
	}
}

func TestLoadConfig_InvalidDialect(t *testing.T) {
	os.Clearenv()
	_ = os.Setenv("DB_CONNECTION", "oracle")

	_, err := LoadConfig("testsvc")
	if err == nil {
		t.Errorf("ekspektasi error untuk dialek tidak valid, namun sukses")
	}
}
