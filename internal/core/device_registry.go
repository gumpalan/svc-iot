package core

import (
	"sync"

	"github.com/gumpalan/svc-iot/internal/config"
)

type DeviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]config.DeviceConfig
}

func NewDeviceRegistry(cfg *config.Config) *DeviceRegistry {
	reg := &DeviceRegistry{
		devices: make(map[string]config.DeviceConfig),
	}
	if cfg != nil {
		for _, dev := range cfg.Devices {
			reg.devices[dev.ID] = dev
		}
	}
	return reg
}

func (r *DeviceRegistry) SetConfig(cfg *config.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.devices = make(map[string]config.DeviceConfig)
	for _, dev := range cfg.Devices {
		r.devices[dev.ID] = dev
	}
}

func (r *DeviceRegistry) GetDevice(id string) (config.DeviceConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	dev, ok := r.devices[id]
	return dev, ok
}

func (r *DeviceRegistry) ListDevices() []config.DeviceConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]config.DeviceConfig, 0, len(r.devices))
	for _, dev := range r.devices {
		list = append(list, dev)
	}
	return list
}

// FindChannelByStateTopic mencari device dan channel yang memiliki state/telemetry topic yang cocok
func (r *DeviceRegistry) FindChannelByStateTopic(topic string) (config.DeviceConfig, config.ChannelConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, dev := range r.devices {
		for _, ch := range dev.Channels {
			if ch.StateTopic == topic || ch.Topic == topic {
				return dev, ch, true
			}
		}
	}
	return config.DeviceConfig{}, config.ChannelConfig{}, false
}

// AllSubscriptionTopics mengumpulkan semua topik MQTT yang harus dilanggani (state & telemetry topics)
func (r *DeviceRegistry) AllSubscriptionTopics() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var topics []string

	for _, dev := range r.devices {
		for _, ch := range dev.Channels {
			t := ch.StateTopic
			if t == "" {
				t = ch.Topic
			}
			if t != "" && !seen[t] {
				seen[t] = true
				topics = append(topics, t)
			}
		}
	}
	return topics
}
