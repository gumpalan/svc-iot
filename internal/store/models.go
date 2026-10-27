package store

import "errors"

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
