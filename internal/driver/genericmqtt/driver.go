package genericmqtt

import (
	"fmt"
	"strings"

	"github.com/gumpalan/svc-iot/pkg/driver"
)

// Driver mengimplementasikan kendali MQTT generik yang seluruh topiknya
// dikonfigurasi melalui konfigurasi perangkat (command_topic, state_topic, payload_on, payload_off).
type Driver struct{}

func New() *Driver {
	return &Driver{}
}

func (d *Driver) Name() string {
	return "generic_mqtt"
}

func (d *Driver) Subscriptions(dev driver.Device) []string {
	var subs []string
	if st, ok := dev.Config["state_topic"]; ok && st != "" {
		subs = append(subs, st)
	}
	if dt, ok := dev.Config["dimmer_state_topic"]; ok && dt != "" {
		subs = append(subs, dt)
	}
	return subs
}

func (d *Driver) Command(dev driver.Device, capability string, value any) ([]driver.Message, error) {
	switch capability {
	case "switch":
		cmdTopic := dev.Config["command_topic"]
		if cmdTopic == "" {
			return nil, fmt.Errorf("generic_mqtt: 'command_topic' tidak dikonfigurasi")
		}

		payloadOn := dev.Config["payload_on"]
		if payloadOn == "" {
			payloadOn = "ON"
		}
		payloadOff := dev.Config["payload_off"]
		if payloadOff == "" {
			payloadOff = "OFF"
		}

		payload := payloadOn
		if v, ok := value.(string); ok {
			if strings.EqualFold(v, "off") || strings.EqualFold(v, payloadOff) {
				payload = payloadOff
			}
		} else if b, ok := value.(bool); ok && !b {
			payload = payloadOff
		}

		return []driver.Message{
			{
				Topic:   cmdTopic,
				Payload: []byte(payload),
			},
		}, nil

	case "dimmer":
		dimmerCmdTopic := dev.Config["dimmer_command_topic"]
		if dimmerCmdTopic == "" {
			return nil, fmt.Errorf("generic_mqtt: 'dimmer_command_topic' tidak dikonfigurasi")
		}

		payload := fmt.Sprintf("%v", value)
		return []driver.Message{
			{
				Topic:   dimmerCmdTopic,
				Payload: []byte(payload),
			},
		}, nil

	default:
		return nil, fmt.Errorf("generic_mqtt: kapabilitas tidak didukung: %s", capability)
	}
}

func (d *Driver) ParseState(topic string, payload []byte) (driver.State, bool) {
	// Parsing generik; pemetaan spesifik dilakukan oleh core dispatcher berdasarkan konfigurasi device
	return driver.State{
		Capability: "raw",
		Value:      string(payload),
	}, true
}
