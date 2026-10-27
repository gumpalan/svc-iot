-- +goose Up
CREATE TABLE iot_dashboards (
  id VARCHAR(64) PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  is_public BOOLEAN NOT NULL DEFAULT FALSE,
  layout_json TEXT NOT NULL,
  device_ids TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE INDEX idx_iot_dashboards_user_id ON iot_dashboards (user_id);
CREATE INDEX idx_iot_dashboards_is_public ON iot_dashboards (is_public);

CREATE TABLE iot_device_permissions (
  id VARCHAR(64) PRIMARY KEY,
  device_id VARCHAR(64) NOT NULL,
  role_id VARCHAR(64) NULL,
  user_id VARCHAR(64) NULL,
  can_read BOOLEAN NOT NULL DEFAULT TRUE,
  can_write BOOLEAN NOT NULL DEFAULT TRUE,
  created_at BIGINT NOT NULL
);

CREATE INDEX idx_iot_dev_perm_device ON iot_device_permissions (device_id);
CREATE INDEX idx_iot_dev_perm_role ON iot_device_permissions (role_id);
CREATE INDEX idx_iot_dev_perm_user ON iot_device_permissions (user_id);

-- +goose Down
DROP TABLE IF EXISTS iot_device_permissions;
DROP TABLE IF EXISTS iot_dashboards;
