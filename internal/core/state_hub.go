package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gumpalan/svc-iot/internal/config"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Diizinkan untuk mobile app dan dev proxy
	},
}

type CommandPublisher interface {
	PublishCommand(topic string, payload []byte) error
}

type Event struct {
	Type      string `json:"type"` // "init", "state_change", "command", "error"
	DeviceID  string `json:"deviceId,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
	Value     any    `json:"value,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Data      any    `json:"data,omitempty"`
}

type StateHub struct {
	registry   *DeviceRegistry
	publisher  CommandPublisher
	automation *AutomationEngine

	mu         sync.RWMutex
	stateCache map[string]map[string]any // deviceID -> channelID -> value

	wsMu    sync.RWMutex
	clients map[*websocket.Conn]bool
}

func NewStateHub(registry *DeviceRegistry, pub CommandPublisher) *StateHub {
	return &StateHub{
		registry:   registry,
		publisher:  pub,
		stateCache: make(map[string]map[string]any),
		clients:    make(map[*websocket.Conn]bool),
	}
}

func (h *StateHub) SetPublisher(pub CommandPublisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.publisher = pub
}

func (h *StateHub) SetAutomationEngine(engine *AutomationEngine) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.automation = engine
}

// GetDeviceStates mengembalikan semua state terkini untuk satu device
func (h *StateHub) GetDeviceStates(deviceID string) map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]any)
	if m, ok := h.stateCache[deviceID]; ok {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}

// GetAllStates mengembalikan snapshot seluruh state perangkat
func (h *StateHub) GetAllStates() map[string]map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]map[string]any)
	for devID, m := range h.stateCache {
		res[devID] = make(map[string]any)
		for k, v := range m {
			res[devID][k] = v
		}
	}
	return res
}

// UpdateState memperbarui cache dan menyiarkan perubahan ke seluruh client WebSocket
func (h *StateHub) UpdateState(deviceID, channelID string, value any) {
	h.mu.Lock()
	if _, ok := h.stateCache[deviceID]; !ok {
		h.stateCache[deviceID] = make(map[string]any)
	}
	h.stateCache[deviceID][channelID] = value
	h.mu.Unlock()

	ev := Event{
		Type:      "state_change",
		DeviceID:  deviceID,
		ChannelID: channelID,
		Value:     value,
		Timestamp: time.Now().Unix(),
	}
	h.Broadcast(ev)

	// Evaluasi aturan otomasi
	h.mu.RLock()
	auto := h.automation
	h.mu.RUnlock()
	if auto != nil {
		auto.EvaluateStateChange(deviceID, channelID, value)
	}
}

// HandleInboundMQTT dipanggil saat ada payload masuk dari topik MQTT yang dilanggani
func (h *StateHub) HandleInboundMQTT(topic string, payload []byte) {
	rawStr := strings.TrimSpace(string(payload))
	log.Printf("[MQTT Inbound] Topik: %s, Payload: %s", topic, rawStr)

	dev, ch, found := h.registry.FindChannelByStateTopic(topic)
	if !found {
		return
	}

	var parsedVal any = rawStr

	switch ch.Type {
	case config.DataTypeBoolean:
		low := strings.ToLower(rawStr)
		if low == "1" || low == "true" || low == "on" {
			parsedVal = true
		} else if low == "0" || low == "false" || low == "off" {
			parsedVal = false
		}
	case config.DataTypeNumber:
		// Sanitasi karakter satuan jika ada, misal "24.0 C" atau "40.0 %"
		cleaned := strings.TrimRight(rawStr, " °Cc%Ff")
		if num, err := strconv.ParseFloat(cleaned, 64); err == nil {
			parsedVal = num
		} else if num, err := strconv.ParseFloat(rawStr, 64); err == nil {
			parsedVal = num
		} else {
			log.Printf("[MQTT Inbound] Gagal parse angka pada channel %s.%s: %s (%v)", dev.ID, ch.ID, rawStr, err)
		}
	}

	h.UpdateState(dev.ID, ch.ID, parsedVal)
}

// ExecuteCommand memproses perintah ke perangkat dan meneruskan ke MQTT broker
func (h *StateHub) ExecuteCommand(deviceID, channelID string, value any) error {
	dev, ok := h.registry.GetDevice(deviceID)
	if !ok {
		return fmt.Errorf("device '%s' not found", deviceID)
	}

	var targetChannel *config.ChannelConfig
	for i := range dev.Channels {
		if dev.Channels[i].ID == channelID {
			targetChannel = &dev.Channels[i]
			break
		}
	}

	if targetChannel == nil {
		return fmt.Errorf("channel '%s' not found in device '%s'", channelID, deviceID)
	}

	if targetChannel.Direction == config.DirectionInput {
		return fmt.Errorf("channel '%s' is input-only", channelID)
	}

	cmdTopic := targetChannel.CommandTopic
	if cmdTopic == "" {
		cmdTopic = targetChannel.Topic
	}
	if cmdTopic == "" {
		return fmt.Errorf("channel '%s' has no command topic defined", channelID)
	}

	// Normalisasi dan konversi nilai jika tipe channel boolean
	if targetChannel.Type == config.DataTypeBoolean {
		switch v := value.(type) {
		case string:
			low := strings.ToLower(strings.TrimSpace(v))
			if low == "press" || low == "toggle" {
				// Toggle berdasarkan state terkini
				currState := false
				h.mu.RLock()
				if m, ok := h.stateCache[deviceID]; ok {
					if b, ok := m[channelID].(bool); ok {
						currState = b
					} else if s, ok := m[channelID].(string); ok {
						sLow := strings.ToLower(s)
						currState = (sLow == "1" || sLow == "true" || sLow == "on")
					}
				}
				h.mu.RUnlock()
				value = !currState
			} else if low == "1" || low == "true" || low == "on" {
				value = true
			} else if low == "0" || low == "false" || low == "off" {
				value = false
			}
		case int:
			value = (v != 0)
		case float64:
			value = (v != 0)
		}
	}

	// Format payload sesuai tipe data dan driver bawaan
	var payload []byte
	switch v := value.(type) {
	case bool:
		if dev.Driver == "tasmota" {
			if v {
				payload = []byte("ON")
			} else {
				payload = []byte("OFF")
			}
		} else {
			if v {
				payload = []byte("true")
			} else {
				payload = []byte("false")
			}
		}
	case string:
		payload = []byte(v)
	case float64:
		payload = []byte(strconv.FormatFloat(v, 'f', -1, 64))
	case int:
		payload = []byte(strconv.Itoa(v))
	default:
		payload = []byte(fmt.Sprintf("%v", v))
	}

	// Terbitkan ke MQTT jika publisher tersedia
	h.mu.RLock()
	pub := h.publisher
	h.mu.RUnlock()

	if pub != nil {
		if err := pub.PublishCommand(cmdTopic, payload); err != nil {
			return fmt.Errorf("mqtt publish failed: %w", err)
		}
		log.Printf("[MQTT Command] Publish ke %s: %s (value: %v)", cmdTopic, string(payload), value)
	}

	// Perbarui state lokal seketika
	h.UpdateState(deviceID, channelID, value)
	return nil
}

// Broadcast mengirim event ke seluruh koneksi WebSocket yang aktif
func (h *StateHub) Broadcast(ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}

	h.wsMu.Lock()
	defer h.wsMu.Unlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

// HandleWebSocket mengelola koneksi WebSocket client (mobile app)
func (h *StateHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	h.wsMu.Lock()
	h.clients[conn] = true
	h.wsMu.Unlock()

	// Kirim pesan inisialisasi state awal
	initEv := Event{
		Type:      "init",
		Timestamp: time.Now().Unix(),
		Data: map[string]any{
			"devices": h.registry.ListDevices(),
			"states":  h.GetAllStates(),
		},
	}
	_ = conn.WriteJSON(initEv)

	go func() {
		defer func() {
			h.wsMu.Lock()
			delete(h.clients, conn)
			h.wsMu.Unlock()
			conn.Close()
		}()

		for {
			var msg struct {
				Type      string `json:"type"`
				DeviceID  string `json:"deviceId"`
				ChannelID string `json:"channelId"`
				Value     any    `json:"value"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				break
			}

			if msg.Type == "ping" {
				_ = conn.WriteJSON(Event{Type: "pong", Timestamp: time.Now().Unix()})
				continue
			}

			if msg.Type == "command" {
				if err := h.ExecuteCommand(msg.DeviceID, msg.ChannelID, msg.Value); err != nil {
					_ = conn.WriteJSON(Event{
						Type:      "error",
						DeviceID:  msg.DeviceID,
						ChannelID: msg.ChannelID,
						Value:     err.Error(),
						Timestamp: time.Now().Unix(),
					})
				}
			}
		}
	}()
}
