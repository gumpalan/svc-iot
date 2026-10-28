-- +goose Up
-- Tabel aturan otomasi IoT (trigger -> action)
CREATE TABLE iot_automation_rules (
  id          VARCHAR(64) PRIMARY KEY,
  user_id     VARCHAR(64) NOT NULL,
  name        VARCHAR(128) NOT NULL,
  enabled     BOOLEAN NOT NULL DEFAULT TRUE,

  -- Trigger: state channel sensor mencapai kondisi tertentu atau jadwal waktu
  trigger_type    VARCHAR(16) NOT NULL,  -- "state" | "schedule"

  -- Trigger state: monitor device/channel tertentu
  trigger_device_id   VARCHAR(64),
  trigger_channel_id  VARCHAR(64),
  trigger_operator    VARCHAR(8),   -- "==" | "!=" | ">" | ">=" | "<" | "<="
  trigger_value       TEXT,         -- nilai pembanding (disimpan string)

  -- Trigger schedule: cron expression (misal "0 23 * * *" = jam 23:00 setiap hari)
  trigger_cron        VARCHAR(64),

  -- Action: kirim command ke device/channel dengan nilai tertentu
  action_device_id    VARCHAR(64) NOT NULL,
  action_channel_id   VARCHAR(64) NOT NULL,
  action_value        TEXT NOT NULL,

  created_at  BIGINT NOT NULL,
  updated_at  BIGINT NOT NULL
);

CREATE INDEX idx_iot_rules_user ON iot_automation_rules (user_id);
CREATE INDEX idx_iot_rules_enabled ON iot_automation_rules (enabled);

-- +goose Down
DROP TABLE IF EXISTS iot_automation_rules;
