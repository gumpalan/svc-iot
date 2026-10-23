package tasmota

import (
	"fmt"
	"strings"

	"github.com/gumpalan/svc-iot/pkg/driver"
)

// Driver mengimplementasikan konvensi topik Tasmota:
// Perintah dikirim ke: cmnd/<topic>/<COMMAND>
// Status diterima dari: stat/<topic>/<COMMAND> atau tele/<topic>/STATE
type Driver struct{}

func New() *Driver {
	return &Driver{}
}

func (d *Driver) Name() string {
	return "tasmota"
}

func (d *Driver) Subscriptions(dev driver.Device) []string {
	topic := dev.Topic
	if topic == "" {
		topic = dev.ID
	}
	return []string{
		fmt.Sprintf("stat/%s/+", topic),
		fmt.Sprintf("tele/%s/STATE", topic),
	}
}

func (d *Driver) Command(dev driver.Device, capability string, value any) ([]driver.Message, error) {
	topic := dev.Topic
	if topic == "" {
		topic = dev.ID
	}

	switch capability {
	case "switch":
		payloadStr := "TOGGLE"
		if v, ok := value.(string); ok && v != "" {
			payloadStr = strings.ToUpper(v)
		} else if b, ok := value.(bool); ok {
			if b {
				payloadStr = "ON"
			} else {
				payloadStr = "OFF"
			}
		}

		return []driver.Message{
			{
				Topic:   fmt.Sprintf("cmnd/%s/POWER", topic),
				Payload: []byte(payloadStr),
			},
		}, nil

	default:
		return nil, fmt.Errorf("tasmota driver: kapabilitas tidak didukung: %s", capability)
	}
}

func (d *Driver) ParseState(topic string, payload []byte) (driver.State, bool) {
	// Contoh: stat/kipas1/POWER -> ON
	parts := strings.Split(topic, "/")
	if len(parts) >= 3 && parts[0] == "stat" && strings.HasPrefix(parts[2], "POWER") {
		return driver.State{
			Capability: "switch",
			Value:      strings.ToLower(string(payload)),
		}, true
	}
	return driver.State{}, false
}
