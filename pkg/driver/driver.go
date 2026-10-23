package driver

// Capability merepresentasikan satu kemampuan atomik dari perangkat IoT (switch, dimmer, sensor, dll).
type Capability struct {
	Type  string `json:"type" yaml:"type"`
	State any    `json:"state,omitempty" yaml:"state,omitempty"`
	Value any    `json:"value,omitempty" yaml:"value,omitempty"`
}

// Device menyimpan atribut deskriptif perangkat beserta daftar kapabilitas dan konfigurasinya.
type Device struct {
	ID           string            `json:"id" yaml:"id"`
	Name         string            `json:"name" yaml:"name"`
	Driver       string            `json:"driver" yaml:"driver"`
	Capabilities []Capability      `json:"capabilities" yaml:"capabilities"`
	Topic        string            `json:"topic,omitempty" yaml:"topic,omitempty"`
	Config       map[string]string `json:"config,omitempty" yaml:"config,omitempty"`
}

// Message merepresentasikan pesan MQTT yang perlu dipublikasikan ke broker.
type Message struct {
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
}

// State menyimpan hasil parsing payload state dari topik MQTT.
type State struct {
	Capability string `json:"capability"`
	Value      any    `json:"value"`
}

// Driver adalah kontrak murni untuk penerjemah protokol perangkat IoT.
// Driver tidak melakukan koneksi MQTT langsung sehingga dapat diuji secara independen.
type Driver interface {
	// Name mengembalikan identifier unik jenis driver (contoh: "tasmota", "generic_mqtt").
	Name() string

	// Subscriptions mengembalikan daftar topik MQTT yang perlu didengarkan untuk perangkat ini.
	Subscriptions(dev Device) []string

	// Command menerjemahkan permintaan aksi aplikasi menjadi daftar pesan MQTT yang perlu dipublikasikan.
	Command(dev Device, capability string, value any) ([]Message, error)

	// ParseState mengekstrak state perangkat saat ada pesan MQTT masuk pada topik yang dilanggani.
	ParseState(topic string, payload []byte) (State, bool)
}
