package store

import "errors"

// SensorLog menyimpan satu pembacaan nilai sensor dari perangkat IoT.
type SensorLog struct {
	ID         string `json:"id"`
	DeviceID   string `json:"deviceId"`
	ChannelID  string `json:"channelId"`
	Value      string `json:"value"`
	RecordedAt int64  `json:"recordedAt"`
}

// ActuatorLog menyimpan satu eksekusi perintah ke aktuator beserta sumbernya.
// Source: "app" | "automation" | "schedule" | "device"
// ActorID: user_id jika source=app, rule_id jika source=automation/schedule, nil jika device
// ActorName: nama tampilan (denormalisasi) untuk kemudahan UI
type ActuatorLog struct {
	ID         string  `json:"id"`
	DeviceID   string  `json:"deviceId"`
	ChannelID  string  `json:"channelId"`
	Value      string  `json:"value"`
	Source     string  `json:"source"`
	ActorID    *string `json:"actorId,omitempty"`
	ActorName  *string `json:"actorName,omitempty"`
	RecordedAt int64   `json:"recordedAt"`
}

// HistoryQuery adalah parameter filter untuk query data historis.
type HistoryQuery struct {
	DeviceID  string
	ChannelID string // kosong = semua channel pada device
	From      int64  // unix timestamp inklusif, 0 = tidak dibatasi
	To        int64  // unix timestamp inklusif, 0 = tidak dibatasi
	Limit     int    // default 200, max 1000
}

var (
	ErrNotFound      = errors.New("record not found")
	ErrAccessDenied  = errors.New("access denied")
	ErrAlreadyExists = errors.New("record already exists")
)

type Dashboard struct {
	ID         string   `json:"id"`
	UserID     string   `json:"userId"`
	Name       string   `json:"name"`
	IsPublic   bool     `json:"isPublic"`
	LayoutJSON string   `json:"layoutJson"`
	DeviceIDs  []string `json:"deviceIds"`
	CreatedAt  int64    `json:"createdAt"`
	UpdatedAt  int64    `json:"updatedAt"`
}

type DevicePermission struct {
	ID        string  `json:"id"`
	DeviceID  string  `json:"deviceId"`
	RoleID    *string `json:"roleId,omitempty"`
	UserID    *string `json:"userId,omitempty"`
	CanRead   bool    `json:"canRead"`
	CanWrite  bool    `json:"canWrite"`
	CreatedAt int64   `json:"createdAt"`
}

// AutomationRule merepresentasikan skenario logika otomasi (trigger -> action)
type AutomationRule struct {
	ID      string `json:"id"`
	UserID  string `json:"userId"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`

	// Trigger info
	TriggerType      string  `json:"triggerType"` // "state" atau "schedule"
	TriggerDeviceID  *string `json:"triggerDeviceId,omitempty"`
	TriggerChannelID *string `json:"triggerChannelId,omitempty"`
	TriggerOperator  *string `json:"triggerOperator,omitempty"` // "==", "!=", ">", ">=", "<", "<="
	TriggerValue     *string `json:"triggerValue,omitempty"`
	TriggerCron      *string `json:"triggerCron,omitempty"`

	// Action info
	ActionDeviceID  string `json:"actionDeviceId"`
	ActionChannelID string `json:"actionChannelId"`
	ActionValue     string `json:"actionValue"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
	IsOwner   bool  `json:"isOwner"`
}
