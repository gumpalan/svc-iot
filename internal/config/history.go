package config

import (
	"os"
	"strconv"
	"strings"
)

// HistoryMode mengontrol jenis data historis yang disimpan ke database.
type HistoryMode string

const (
	HistoryNone     HistoryMode = "none"
	HistorySensor   HistoryMode = "sensor"
	HistoryActuator HistoryMode = "actuator"
	HistoryAll      HistoryMode = "all"
)

// HistoryConfig menyimpan konfigurasi penyimpanan data historis IoT.
type HistoryConfig struct {
	Mode          HistoryMode
	RetentionDays int // 0 = simpan selamanya
}

// LoadHistoryConfig membaca konfigurasi dari environment variable.
// HISTORY_MODE: none | sensor | actuator | all (default: none)
// HISTORY_RETENTION_DAYS: jumlah hari retensi data, 0 = selamanya (default: 0)
func LoadHistoryConfig() HistoryConfig {
	mode := HistoryMode(strings.ToLower(strings.TrimSpace(os.Getenv("HISTORY_MODE"))))
	switch mode {
	case HistorySensor, HistoryActuator, HistoryAll:
		// valid
	default:
		mode = HistoryNone
	}

	retention := 0
	if v := strings.TrimSpace(os.Getenv("HISTORY_RETENTION_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			retention = n
		}
	}

	return HistoryConfig{
		Mode:          mode,
		RetentionDays: retention,
	}
}

// Enabled mengembalikan true jika mode bukan "none".
func (c HistoryConfig) Enabled() bool {
	return c.Mode != HistoryNone
}

// LogSensor mengembalikan true jika mode menyimpan data sensor.
func (c HistoryConfig) LogSensor() bool {
	return c.Mode == HistorySensor || c.Mode == HistoryAll
}

// LogActuator mengembalikan true jika mode menyimpan data aktuator.
func (c HistoryConfig) LogActuator() bool {
	return c.Mode == HistoryActuator || c.Mode == HistoryAll
}
