package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gumpalan/svc-iot/internal/database"
)

type Store struct {
	db      *sql.DB
	dialect database.Dialect
}

func New(db *sql.DB, dialect database.Dialect) *Store {
	return &Store{
		db:      db,
		dialect: dialect,
	}
}

func (s *Store) CreateDashboard(ctx context.Context, d *Dashboard) error {
	now := time.Now().Unix()
	d.CreatedAt = now
	d.UpdatedAt = now

	devIDsBytes, err := json.Marshal(d.DeviceIDs)
	if err != nil {
		return fmt.Errorf("marshal device_ids: %w", err)
	}

	query := database.Rebind(s.dialect, `
		INSERT INTO iot_dashboards (id, user_id, name, is_public, layout_json, device_ids, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)

	_, err = s.db.ExecContext(ctx, query, d.ID, d.UserID, d.Name, d.IsPublic, d.LayoutJSON, string(devIDsBytes), d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert dashboard: %w", err)
	}
	return nil
}

func (s *Store) GetDashboard(ctx context.Context, id string) (*Dashboard, error) {
	query := database.Rebind(s.dialect, `
		SELECT id, user_id, name, is_public, layout_json, device_ids, created_at, updated_at
		FROM iot_dashboards
		WHERE id = ?
	`)

	row := s.db.QueryRowContext(ctx, query, id)
	var d Dashboard
	var devIDsStr string
	err := row.Scan(&d.ID, &d.UserID, &d.Name, &d.IsPublic, &d.LayoutJSON, &devIDsStr, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get dashboard: %w", err)
	}

	_ = json.Unmarshal([]byte(devIDsStr), &d.DeviceIDs)
	if d.DeviceIDs == nil {
		d.DeviceIDs = []string{}
	}
	return &d, nil
}

func (s *Store) ListUserDashboards(ctx context.Context, userID string) ([]*Dashboard, error) {
	query := database.Rebind(s.dialect, `
		SELECT id, user_id, name, is_public, layout_json, device_ids, created_at, updated_at
		FROM iot_dashboards
		WHERE user_id = ?
		ORDER BY updated_at DESC
	`)

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list user dashboards: %w", err)
	}
	defer rows.Close()

	var list []*Dashboard
	for rows.Next() {
		var d Dashboard
		var devIDsStr string
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.IsPublic, &d.LayoutJSON, &devIDsStr, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan dashboard: %w", err)
		}
		_ = json.Unmarshal([]byte(devIDsStr), &d.DeviceIDs)
		if d.DeviceIDs == nil {
			d.DeviceIDs = []string{}
		}
		list = append(list, &d)
	}
	return list, nil
}

func (s *Store) UpdateDashboard(ctx context.Context, d *Dashboard) error {
	d.UpdatedAt = time.Now().Unix()
	devIDsBytes, err := json.Marshal(d.DeviceIDs)
	if err != nil {
		return fmt.Errorf("marshal device_ids: %w", err)
	}

	query := database.Rebind(s.dialect, `
		UPDATE iot_dashboards
		SET name = ?, is_public = ?, layout_json = ?, device_ids = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`)

	res, err := s.db.ExecContext(ctx, query, d.Name, d.IsPublic, d.LayoutJSON, string(devIDsBytes), d.UpdatedAt, d.ID, d.UserID)
	if err != nil {
		return fmt.Errorf("update dashboard: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteDashboard(ctx context.Context, id, userID string) error {
	query := database.Rebind(s.dialect, `
		DELETE FROM iot_dashboards
		WHERE id = ? AND user_id = ?
	`)

	res, err := s.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("delete dashboard: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListExploreDashboards(ctx context.Context, excludeUserID string) ([]*Dashboard, error) {
	query := database.Rebind(s.dialect, `
		SELECT id, user_id, name, is_public, layout_json, device_ids, created_at, updated_at
		FROM iot_dashboards
		WHERE is_public = true AND user_id != ?
		ORDER BY updated_at DESC
	`)

	rows, err := s.db.QueryContext(ctx, query, excludeUserID)
	if err != nil {
		return nil, fmt.Errorf("list explore dashboards: %w", err)
	}
	defer rows.Close()

	var list []*Dashboard
	for rows.Next() {
		var d Dashboard
		var devIDsStr string
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.IsPublic, &d.LayoutJSON, &devIDsStr, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan explore dashboard: %w", err)
		}
		_ = json.Unmarshal([]byte(devIDsStr), &d.DeviceIDs)
		if d.DeviceIDs == nil {
			d.DeviceIDs = []string{}
		}
		list = append(list, &d)
	}
	return list, nil
}

// CheckDeviceAccess mengecek apakah user / role diizinkan membaca atau menulis device
func (s *Store) CheckDeviceAccess(ctx context.Context, deviceID, userID, role string, requireWrite bool) (bool, error) {
	if role == "admin" {
		return true, nil
	}

	// Cek apakah ada record permission spesifik untuk device ini
	countQuery := database.Rebind(s.dialect, `
		SELECT COUNT(*) FROM iot_device_permissions WHERE device_id = ?
	`)
	var count int
	err := s.db.QueryRowContext(ctx, countQuery, deviceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check permission count: %w", err)
	}

	// Jika tidak ada batasan khusus di tabel, izinkan akses default
	if count == 0 {
		return true, nil
	}

	// Jika ada batasan, cari permission matching user atau role
	var checks []string
	var args []any
	checks = append(checks, "device_id = ?")
	args = append(args, deviceID)

	var userRoleConds []string
	if userID != "" {
		userRoleConds = append(userRoleConds, "user_id = ?")
		args = append(args, userID)
	}
	if role != "" {
		userRoleConds = append(userRoleConds, "role_id = ?")
		args = append(args, role)
	}

	if len(userRoleConds) == 0 {
		return false, nil
	}

	queryStr := fmt.Sprintf(`
		SELECT can_read, can_write FROM iot_device_permissions
		WHERE device_id = ? AND (%s)
	`, strings.Join(userRoleConds, " OR "))

	query := database.Rebind(s.dialect, queryStr)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("query device permission: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var canRead, canWrite bool
		if err := rows.Scan(&canRead, &canWrite); err != nil {
			return false, err
		}
		if requireWrite && canWrite {
			return true, nil
		}
		if !requireWrite && canRead {
			return true, nil
		}
	}

	return false, nil
}
