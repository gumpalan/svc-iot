-- +goose Up
-- Tabel log data sensor (channel direction=input, dari MQTT inbound)
CREATE TABLE iot_sensor_logs (
  id          VARCHAR(64)  PRIMARY KEY,
  device_id   VARCHAR(64)  NOT NULL,
  channel_id  VARCHAR(64)  NOT NULL,
  value       TEXT         NOT NULL,
  recorded_at BIGINT       NOT NULL
);

CREATE INDEX idx_sensor_logs_lookup
  ON iot_sensor_logs (device_id, channel_id, recorded_at DESC);

-- Tabel log eksekusi aktuator (channel direction=output/bidirectional)
-- source: "app" | "automation" | "schedule" | "device"
-- actor_id: user_id jika source=app, rule_id jika source=automation/schedule, NULL jika device
-- actor_name: denormalisasi untuk keperluan tampilan (nama user atau nama rule)
CREATE TABLE iot_actuator_logs (
  id          VARCHAR(64)  PRIMARY KEY,
  device_id   VARCHAR(64)  NOT NULL,
  channel_id  VARCHAR(64)  NOT NULL,
  value       TEXT         NOT NULL,
  source      VARCHAR(16)  NOT NULL,
  actor_id    VARCHAR(64),
  actor_name  VARCHAR(128),
  recorded_at BIGINT       NOT NULL
);

CREATE INDEX idx_actuator_logs_lookup
  ON iot_actuator_logs (device_id, channel_id, recorded_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_actuator_logs_lookup;
DROP TABLE  IF EXISTS iot_actuator_logs;
DROP INDEX  IF EXISTS idx_sensor_logs_lookup;
DROP TABLE  IF EXISTS iot_sensor_logs;
