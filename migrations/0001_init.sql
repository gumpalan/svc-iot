-- +goose Up
-- source bernilai config atau discovery
CREATE TABLE iot_devices (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  driver VARCHAR(64) NOT NULL,
  source VARCHAR(16) NOT NULL,
  config_json TEXT,
  created_at BIGINT NOT NULL
);

-- +goose Down
DROP TABLE iot_devices;
