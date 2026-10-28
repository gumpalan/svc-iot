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

func (s *Store) CreatePermission(ctx context.Context, p *DevicePermission) error {
	p.CreatedAt = time.Now().Unix()
	query := database.Rebind(s.dialect, `
		INSERT INTO iot_device_permissions (id, device_id, role_id, user_id, can_read, can_write, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	_, err := s.db.ExecContext(ctx, query, p.ID, p.DeviceID, p.RoleID, p.UserID, p.CanRead, p.CanWrite, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("create permission: %w", err)
	}
	return nil
}

func (s *Store) ListPermissions(ctx context.Context, deviceID string) ([]*DevicePermission, error) {
	var query string
	var args []any
	if deviceID != "" {
		query = database.Rebind(s.dialect, `
			SELECT id, device_id, role_id, user_id, can_read, can_write, created_at
			FROM iot_device_permissions
			WHERE device_id = ?
			ORDER BY created_at DESC
		`)
		args = append(args, deviceID)
	} else {
		query = database.Rebind(s.dialect, `
			SELECT id, device_id, role_id, user_id, can_read, can_write, created_at
			FROM iot_device_permissions
			ORDER BY created_at DESC
		`)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var list []*DevicePermission
	for rows.Next() {
		var p DevicePermission
		var roleID, userID sql.NullString
		if err := rows.Scan(&p.ID, &p.DeviceID, &roleID, &userID, &p.CanRead, &p.CanWrite, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		if roleID.Valid {
			p.RoleID = &roleID.String
		}
		if userID.Valid {
			p.UserID = &userID.String
		}
		list = append(list, &p)
	}
	return list, nil
}

func (s *Store) DeletePermission(ctx context.Context, id string) error {
	query := database.Rebind(s.dialect, `
		DELETE FROM iot_device_permissions
		WHERE id = ?
	`)
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete permission: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------------
// Automation Rules CRUD
// ----------------------------------------------------------------------

func (s *Store) CreateAutomationRule(ctx context.Context, r *AutomationRule) error {
	now := time.Now().Unix()
	r.CreatedAt = now
	r.UpdatedAt = now

	query := database.Rebind(s.dialect, `
		INSERT INTO iot_automation_rules (
			id, user_id, name, enabled,
			trigger_type, trigger_device_id, trigger_channel_id, trigger_operator, trigger_value, trigger_cron,
			action_device_id, action_channel_id, action_value,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)

	_, err := s.db.ExecContext(ctx, query,
		r.ID, r.UserID, r.Name, r.Enabled,
		r.TriggerType, r.TriggerDeviceID, r.TriggerChannelID, r.TriggerOperator, r.TriggerValue, r.TriggerCron,
		r.ActionDeviceID, r.ActionChannelID, r.ActionValue,
		r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create automation rule: %w", err)
	}
	return nil
}

func (s *Store) GetAutomationRule(ctx context.Context, id string) (*AutomationRule, error) {
	query := database.Rebind(s.dialect, `
		SELECT id, user_id, name, enabled,
		       trigger_type, trigger_device_id, trigger_channel_id, trigger_operator, trigger_value, trigger_cron,
		       action_device_id, action_channel_id, action_value,
		       created_at, updated_at
		FROM iot_automation_rules
		WHERE id = ?
	`)

	row := s.db.QueryRowContext(ctx, query, id)
	var r AutomationRule
	var trDev, trCh, trOp, trVal, trCron sql.NullString
	err := row.Scan(
		&r.ID, &r.UserID, &r.Name, &r.Enabled,
		&r.TriggerType, &trDev, &trCh, &trOp, &trVal, &trCron,
		&r.ActionDeviceID, &r.ActionChannelID, &r.ActionValue,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get automation rule: %w", err)
	}

	if trDev.Valid {
		r.TriggerDeviceID = &trDev.String
	}
	if trCh.Valid {
		r.TriggerChannelID = &trCh.String
	}
	if trOp.Valid {
		r.TriggerOperator = &trOp.String
	}
	if trVal.Valid {
		r.TriggerValue = &trVal.String
	}
	if trCron.Valid {
		r.TriggerCron = &trCron.String
	}

	return &r, nil
}

func (s *Store) ListAutomationRules(ctx context.Context, userID string) ([]*AutomationRule, error) {
	var query string
	var args []any
	if userID != "" {
		query = database.Rebind(s.dialect, `
			SELECT id, user_id, name, enabled,
			       trigger_type, trigger_device_id, trigger_channel_id, trigger_operator, trigger_value, trigger_cron,
			       action_device_id, action_channel_id, action_value,
			       created_at, updated_at
			FROM iot_automation_rules
			WHERE user_id = ?
			ORDER BY created_at DESC
		`)
		args = append(args, userID)
	} else {
		query = database.Rebind(s.dialect, `
			SELECT id, user_id, name, enabled,
			       trigger_type, trigger_device_id, trigger_channel_id, trigger_operator, trigger_value, trigger_cron,
			       action_device_id, action_channel_id, action_value,
			       created_at, updated_at
			FROM iot_automation_rules
			ORDER BY created_at DESC
		`)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	defer rows.Close()

	var list []*AutomationRule
	for rows.Next() {
		var r AutomationRule
		var trDev, trCh, trOp, trVal, trCron sql.NullString
		err := rows.Scan(
			&r.ID, &r.UserID, &r.Name, &r.Enabled,
			&r.TriggerType, &trDev, &trCh, &trOp, &trVal, &trCron,
			&r.ActionDeviceID, &r.ActionChannelID, &r.ActionValue,
			&r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan automation rule: %w", err)
		}
		if trDev.Valid {
			r.TriggerDeviceID = &trDev.String
		}
		if trCh.Valid {
			r.TriggerChannelID = &trCh.String
		}
		if trOp.Valid {
			r.TriggerOperator = &trOp.String
		}
		if trVal.Valid {
			r.TriggerValue = &trVal.String
		}
		if trCron.Valid {
			r.TriggerCron = &trCron.String
		}
		list = append(list, &r)
	}
	return list, nil
}

func (s *Store) ListActiveAutomationRules(ctx context.Context) ([]*AutomationRule, error) {
	query := database.Rebind(s.dialect, `
		SELECT id, user_id, name, enabled,
		       trigger_type, trigger_device_id, trigger_channel_id, trigger_operator, trigger_value, trigger_cron,
		       action_device_id, action_channel_id, action_value,
		       created_at, updated_at
		FROM iot_automation_rules
		WHERE enabled = TRUE
		ORDER BY created_at ASC
	`)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active automation rules: %w", err)
	}
	defer rows.Close()

	var list []*AutomationRule
	for rows.Next() {
		var r AutomationRule
		var trDev, trCh, trOp, trVal, trCron sql.NullString
		err := rows.Scan(
			&r.ID, &r.UserID, &r.Name, &r.Enabled,
			&r.TriggerType, &trDev, &trCh, &trOp, &trVal, &trCron,
			&r.ActionDeviceID, &r.ActionChannelID, &r.ActionValue,
			&r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan active rule: %w", err)
		}
		if trDev.Valid {
			r.TriggerDeviceID = &trDev.String
		}
		if trCh.Valid {
			r.TriggerChannelID = &trCh.String
		}
		if trOp.Valid {
			r.TriggerOperator = &trOp.String
		}
		if trVal.Valid {
			r.TriggerValue = &trVal.String
		}
		if trCron.Valid {
			r.TriggerCron = &trCron.String
		}
		list = append(list, &r)
	}
	return list, nil
}

func (s *Store) UpdateAutomationRule(ctx context.Context, r *AutomationRule) error {
	r.UpdatedAt = time.Now().Unix()

	query := database.Rebind(s.dialect, `
		UPDATE iot_automation_rules
		SET name = ?, enabled = ?,
		    trigger_type = ?, trigger_device_id = ?, trigger_channel_id = ?, trigger_operator = ?, trigger_value = ?, trigger_cron = ?,
		    action_device_id = ?, action_channel_id = ?, action_value = ?,
		    updated_at = ?
		WHERE id = ? AND user_id = ?
	`)

	res, err := s.db.ExecContext(ctx, query,
		r.Name, r.Enabled,
		r.TriggerType, r.TriggerDeviceID, r.TriggerChannelID, r.TriggerOperator, r.TriggerValue, r.TriggerCron,
		r.ActionDeviceID, r.ActionChannelID, r.ActionValue,
		r.UpdatedAt,
		r.ID, r.UserID,
	)
	if err != nil {
		return fmt.Errorf("update automation rule: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ToggleAutomationRule(ctx context.Context, id, userID string, enabled bool) error {
	query := database.Rebind(s.dialect, `
		UPDATE iot_automation_rules
		SET enabled = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`)

	res, err := s.db.ExecContext(ctx, query, enabled, time.Now().Unix(), id, userID)
	if err != nil {
		return fmt.Errorf("toggle automation rule: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAutomationRule(ctx context.Context, id, userID string) error {
	query := database.Rebind(s.dialect, `
		DELETE FROM iot_automation_rules
		WHERE id = ? AND user_id = ?
	`)

	res, err := s.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("delete automation rule: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
