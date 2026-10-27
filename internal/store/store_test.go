package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gumpalan/svc-iot/internal/database"
	"github.com/gumpalan/svc-iot/migrations"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (*Store, *sql.DB) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	if err := database.Migrate(db, database.DialectSQLite, migrations.FS, ".", "iot_migrations"); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	return New(db, database.DialectSQLite), db
}

func TestDashboardCRUD(t *testing.T) {
	s, db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	dash := &Dashboard{
		ID:         "dash-1",
		UserID:     "user-101",
		Name:       "Utama",
		IsPublic:   true,
		LayoutJSON: `[{"widget":"toggle","channelId":"power"}]`,
		DeviceIDs:  []string{"lampu-kamar"},
	}

	// Create
	if err := s.CreateDashboard(ctx, dash); err != nil {
		t.Fatalf("create dashboard failed: %v", err)
	}

	// Get
	got, err := s.GetDashboard(ctx, "dash-1")
	if err != nil {
		t.Fatalf("get dashboard failed: %v", err)
	}
	if got.Name != "Utama" || len(got.DeviceIDs) != 1 || got.DeviceIDs[0] != "lampu-kamar" {
		t.Fatalf("unexpected dashboard data: %+v", got)
	}

	// List User
	list, err := s.ListUserDashboards(ctx, "user-101")
	if err != nil {
		t.Fatalf("list user dashboards failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 dashboard, got %d", len(list))
	}

	// Update
	dash.Name = "Utama Diperbarui"
	if err := s.UpdateDashboard(ctx, dash); err != nil {
		t.Fatalf("update dashboard failed: %v", err)
	}
	gotUpdated, err := s.GetDashboard(ctx, "dash-1")
	if err != nil || gotUpdated.Name != "Utama Diperbarui" {
		t.Fatalf("expected updated name, got: %v", gotUpdated)
	}

	// List Explore from other user
	exploreList, err := s.ListExploreDashboards(ctx, "user-202")
	if err != nil {
		t.Fatalf("list explore failed: %v", err)
	}
	if len(exploreList) != 1 {
		t.Fatalf("expected 1 explore dashboard, got %d", len(exploreList))
	}

	// Delete
	if err := s.DeleteDashboard(ctx, "dash-1", "user-101"); err != nil {
		t.Fatalf("delete dashboard failed: %v", err)
	}
	_, err = s.GetDashboard(ctx, "dash-1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestCheckDeviceAccess(t *testing.T) {
	s, db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// 1. Device tanpa entri di iot_device_permissions -> diizinkan default
	allowed, err := s.CheckDeviceAccess(ctx, "unrestricted-dev", "user-1", "user", true)
	if err != nil || !allowed {
		t.Fatalf("expected unrestricted device to be allowed: %v", err)
	}

	// 2. Tambah batasan khusus untuk restricted-dev
	_, err = db.Exec(`
		INSERT INTO iot_device_permissions (id, device_id, role_id, user_id, can_read, can_write, created_at)
		VALUES
		  ('p1', 'restricted-dev', 'admin', NULL, 1, 1, 1000),
		  ('p2', 'restricted-dev', NULL, 'user-allowed', 1, 0, 1000)
	`)
	if err != nil {
		t.Fatalf("insert permissions: %v", err)
	}

	// Admin selalu diizinkan
	allowed, _ = s.CheckDeviceAccess(ctx, "restricted-dev", "random-admin", "admin", true)
	if !allowed {
		t.Errorf("admin should always be allowed")
	}

	// user-allowed boleh baca, tapi tidak boleh tulis
	canRead, _ := s.CheckDeviceAccess(ctx, "restricted-dev", "user-allowed", "user", false)
	if !canRead {
		t.Errorf("user-allowed should be able to read")
	}
	canWrite, _ := s.CheckDeviceAccess(ctx, "restricted-dev", "user-allowed", "user", true)
	if canWrite {
		t.Errorf("user-allowed should NOT be able to write")
	}

	// user lain ditolak
	allowed, _ = s.CheckDeviceAccess(ctx, "restricted-dev", "stranger", "user", false)
	if allowed {
		t.Errorf("stranger should not be allowed")
	}
}
