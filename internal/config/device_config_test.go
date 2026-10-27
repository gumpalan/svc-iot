package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDevicesFile(t *testing.T) {
	yamlContent := `
devices:
  - id: "temp-sensor"
    name: "Sensor Kamar"
    driver: "custom"
    channels:
      - id: "temperature"
        name: "Suhu"
        direction: "input"
        topic: "home/kamar/temp"
        type: "number"
        unit: "°C"
  - id: "saklar-utama"
    name: "Saklar Utama"
    driver: "tasmota"
`
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "devices.yaml")
	if err := os.WriteFile(filePath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}

	cfg, err := LoadDevicesFile(filePath)
	if err != nil {
		t.Fatalf("LoadDevicesFile failed: %v", err)
	}

	if len(cfg.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(cfg.Devices))
	}

	// Cek normalisasi tasmota
	dev2 := cfg.Devices[1]
	if len(dev2.Channels) != 1 {
		t.Fatalf("expected 1 auto channel for tasmota, got %d", len(dev2.Channels))
	}
	if dev2.Channels[0].ID != "power" {
		t.Errorf("expected channel id 'power', got %s", dev2.Channels[0].ID)
	}
	if dev2.Channels[0].CommandTopic != "cmnd/saklar-utama/POWER" {
		t.Errorf("unexpected command topic: %s", dev2.Channels[0].CommandTopic)
	}
}
