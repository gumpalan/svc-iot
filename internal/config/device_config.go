package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Direction string

const (
	DirectionInput         Direction = "input"
	DirectionOutput        Direction = "output"
	DirectionBidirectional Direction = "bidirectional"
)

type DataType string

const (
	DataTypeBoolean DataType = "boolean"
	DataTypeNumber  DataType = "number"
	DataTypeString  DataType = "string"
	DataTypeEnum    DataType = "enum"
)

type ChannelConfig struct {
	ID           string    `json:"id" yaml:"id"`
	Name         string    `json:"name" yaml:"name"`
	Direction    Direction `json:"direction" yaml:"direction"`
	Topic        string    `json:"topic,omitempty" yaml:"topic"`
	CommandTopic string    `json:"command_topic,omitempty" yaml:"command_topic"`
	StateTopic   string    `json:"state_topic,omitempty" yaml:"state_topic"`
	Type         DataType  `json:"type" yaml:"type"`
	Unit         string    `json:"unit,omitempty" yaml:"unit"`
	Min          *float64  `json:"min,omitempty" yaml:"min"`
	Max          *float64  `json:"max,omitempty" yaml:"max"`
	Options      []string  `json:"options,omitempty" yaml:"options"`
	UIHint       string    `json:"ui_hint,omitempty" yaml:"ui_hint"`
}

type DeviceConfig struct {
	ID       string            `json:"id" yaml:"id"`
	Name     string            `json:"name" yaml:"name"`
	Driver   string            `json:"driver" yaml:"driver"`
	Topic    string            `json:"topic,omitempty" yaml:"topic"`
	Channels []ChannelConfig   `json:"channels" yaml:"channels"`
	Config   map[string]string `json:"config,omitempty" yaml:"config"`
}

type Config struct {
	Devices []DeviceConfig `json:"devices" yaml:"devices"`
}

// LoadDevicesFile membaca dan mem-parsing berkas YAML daftar perangkat
func LoadDevicesFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read devices config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Normalize mengisi topik otomatis untuk driver bawaan seperti tasmota
func (c *Config) Normalize() {
	for i := range c.Devices {
		dev := &c.Devices[i]
		if dev.Driver == "" {
			dev.Driver = "custom"
		}

		baseTopic := dev.Topic
		if baseTopic == "" {
			baseTopic = dev.ID
		}

		// Jika driver tasmota dan channels belum ada, buat channel default switch POWER
		if dev.Driver == "tasmota" && len(dev.Channels) == 0 {
			dev.Channels = []ChannelConfig{
				{
					ID:           "power",
					Name:         "Power",
					Direction:    DirectionBidirectional,
					CommandTopic: fmt.Sprintf("cmnd/%s/POWER", baseTopic),
					StateTopic:   fmt.Sprintf("stat/%s/POWER", baseTopic),
					Type:         DataTypeBoolean,
					UIHint:       "toggle",
				},
			}
		}

		for j := range dev.Channels {
			ch := &dev.Channels[j]
			if ch.Direction == "" {
				if ch.CommandTopic != "" && ch.StateTopic != "" {
					ch.Direction = DirectionBidirectional
				} else if ch.CommandTopic != "" {
					ch.Direction = DirectionOutput
				} else {
					ch.Direction = DirectionInput
				}
			}

			// Jika satu topic diisi pada channel bidirectional/input/output
			if ch.Topic != "" {
				if ch.StateTopic == "" && (ch.Direction == DirectionInput || ch.Direction == DirectionBidirectional) {
					ch.StateTopic = ch.Topic
				}
				if ch.CommandTopic == "" && (ch.Direction == DirectionOutput || ch.Direction == DirectionBidirectional) {
					ch.CommandTopic = ch.Topic
				}
			}

			// Default UI hint
			if ch.UIHint == "" {
				switch ch.Type {
				case DataTypeBoolean:
					ch.UIHint = "toggle"
				case DataTypeNumber:
					if ch.Direction == DirectionOutput || ch.Direction == DirectionBidirectional {
						ch.UIHint = "slider"
					} else {
						ch.UIHint = "gauge"
					}
				case DataTypeEnum:
					ch.UIHint = "dropdown"
				default:
					ch.UIHint = "value_display"
				}
			}
		}
	}
}

func (c *Config) Validate() error {
	seenDevice := make(map[string]bool)
	for _, dev := range c.Devices {
		if strings.TrimSpace(dev.ID) == "" {
			return fmt.Errorf("device id is required")
		}
		if seenDevice[dev.ID] {
			return fmt.Errorf("duplicate device id: %s", dev.ID)
		}
		seenDevice[dev.ID] = true

		seenChannel := make(map[string]bool)
		for _, ch := range dev.Channels {
			if strings.TrimSpace(ch.ID) == "" {
				return fmt.Errorf("device %s: channel id is required", dev.ID)
			}
			if seenChannel[ch.ID] {
				return fmt.Errorf("device %s: duplicate channel id: %s", dev.ID, ch.ID)
			}
			seenChannel[ch.ID] = true
		}
	}
	return nil
}
