package database

import "testing"

func TestOpen_SQLiteInMemory(t *testing.T) {
	cfg := &Config{
		Connection:  DialectSQLite,
		Database:    ":memory:",
		AutoMigrate: false,
	}

	db, dialect, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open sqlite :memory: gagal: %v", err)
	}
	defer db.Close()

	if dialect != DialectSQLite {
		t.Errorf("ekspektasi dialek sqlite, dapat: %s", dialect)
	}

	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1 gagal: %v", err)
	}
	if one != 1 {
		t.Errorf("ekspektasi 1, dapat %d", one)
	}
}
