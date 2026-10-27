package core

import (
	"testing"

	"github.com/gumpalan/svc-iot/internal/config"
)

type mockPublisher struct {
	lastTopic   string
	lastPayload []byte
}

func (m *mockPublisher) PublishCommand(topic string, payload []byte) error {
	m.lastTopic = topic
	m.lastPayload = payload
	return nil
}

func TestStateHub(t *testing.T) {
	cfg := &config.Config{
		Devices: []config.DeviceConfig{
			{
				ID:     "lamp1",
				Name:   "Lampu Ruang",
				Driver: "custom",
				Channels: []config.ChannelConfig{
					{
						ID:           "switch",
						Name:         "Saklar",
						Direction:    config.DirectionBidirectional,
						CommandTopic: "home/lamp1/set",
						StateTopic:   "home/lamp1/stat",
						Type:         config.DataTypeBoolean,
					},
					{
						ID:         "lux",
						Name:       "Sensor Cahaya",
						Direction:  config.DirectionInput,
						Topic:      "home/lamp1/lux",
						Type:       config.DataTypeNumber,
						Unit:       "lx",
					},
				},
			},
		},
	}

	reg := NewDeviceRegistry(cfg)
	mockPub := &mockPublisher{}
	hub := NewStateHub(reg, mockPub)

	// 1. Inbound MQTT simulation
	hub.HandleInboundMQTT("home/lamp1/stat", []byte("true"))
	states := hub.GetDeviceStates("lamp1")
	if states["switch"] != true {
		t.Fatalf("expected switch state true, got %v", states["switch"])
	}

	hub.HandleInboundMQTT("home/lamp1/lux", []byte("152.5"))
	states = hub.GetDeviceStates("lamp1")
	if states["lux"] != 152.5 {
		t.Fatalf("expected lux state 152.5, got %v", states["lux"])
	}

	// 2. Command execution
	if err := hub.ExecuteCommand("lamp1", "switch", false); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if mockPub.lastTopic != "home/lamp1/set" {
		t.Fatalf("unexpected topic published: %s", mockPub.lastTopic)
	}
	if string(mockPub.lastPayload) != "false" {
		t.Fatalf("unexpected payload published: %s", string(mockPub.lastPayload))
	}
	if hub.GetDeviceStates("lamp1")["switch"] != false {
		t.Fatalf("expected switch state false after command")
	}

	// 3. Command on input-only channel should fail
	err := hub.ExecuteCommand("lamp1", "lux", 200)
	if err == nil {
		t.Fatalf("expected error executing command on input-only channel")
	}
}
