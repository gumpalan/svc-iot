package database

import "testing"

func TestRebind_NonPgSQL(t *testing.T) {
	query := "SELECT id, name FROM auth_devices WHERE id = ? AND status = ?"

	resSQLite := Rebind(DialectSQLite, query)
	if resSQLite != query {
		t.Errorf("SQLite seharusnya tidak diubah: %s", resSQLite)
	}

	resMySQL := Rebind(DialectMySQL, query)
	if resMySQL != query {
		t.Errorf("MySQL seharusnya tidak diubah: %s", resMySQL)
	}
}

func TestRebind_PgSQL(t *testing.T) {
	query := "INSERT INTO auth_devices (id, name, status) VALUES (?, ?, ?)"
	expected := "INSERT INTO auth_devices (id, name, status) VALUES ($1, $2, $3)"

	res := Rebind(DialectPgSQL, query)
	if res != expected {
		t.Errorf("ekspektasi: %s, dapat: %s", expected, res)
	}
}

func TestRebind_PgSQL_StringLiteralWithQuestionMark(t *testing.T) {
	query := "SELECT * FROM t WHERE name = 'apa kabar?' AND id = ?"
	expected := "SELECT * FROM t WHERE name = 'apa kabar?' AND id = $1"

	res := Rebind(DialectPgSQL, query)
	if res != expected {
		t.Errorf("ekspektasi: %s, dapat: %s", expected, res)
	}
}
