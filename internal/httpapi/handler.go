package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gumpalan/svc-iot/internal/config"
	"github.com/gumpalan/svc-iot/internal/core"
	"github.com/gumpalan/svc-iot/internal/store"
)

type Handler struct {
	registry   *core.DeviceRegistry
	hub        *core.StateHub
	store      *store.Store
	automation *core.AutomationEngine
}

func NewHandler(registry *core.DeviceRegistry, hub *core.StateHub, st *store.Store) *Handler {
	return &Handler{
		registry: registry,
		hub:      hub,
		store:    st,
	}
}

func (h *Handler) SetAutomationEngine(auto *core.AutomationEngine) {
	h.automation = auto
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.HandleFunc("GET /iot/v1/healthz", h.handleHealthz)

	// WebSocket real-time
	mux.HandleFunc("/iot/v1/ws", h.hub.HandleWebSocket)

	// Devices & Commands
	mux.HandleFunc("GET /iot/v1/devices", h.handleListDevices)
	mux.HandleFunc("GET /iot/v1/devices/{id}", h.handleGetDevice)
	mux.HandleFunc("POST /iot/v1/devices/{id}/command", h.handleDeviceCommand)

	// User Dashboards
	mux.HandleFunc("GET /iot/v1/dashboards", h.handleListDashboards)
	mux.HandleFunc("POST /iot/v1/dashboards", h.handleCreateDashboard)
	mux.HandleFunc("GET /iot/v1/dashboards/explore", h.handleExploreDashboards)
	mux.HandleFunc("GET /iot/v1/dashboards/{id}", h.handleGetDashboard)
	mux.HandleFunc("PUT /iot/v1/dashboards/{id}", h.handleUpdateDashboard)
	mux.HandleFunc("DELETE /iot/v1/dashboards/{id}", h.handleDeleteDashboard)
	mux.HandleFunc("POST /iot/v1/dashboards/{id}/clone", h.handleCloneDashboard)

	// Device Permissions (Admin only)
	mux.HandleFunc("GET /iot/v1/permissions", h.handleListPermissions)
	mux.HandleFunc("POST /iot/v1/permissions", h.handleCreatePermission)
	mux.HandleFunc("DELETE /iot/v1/permissions/{id}", h.handleDeletePermission)

	// Automation Rules
	mux.HandleFunc("GET /iot/v1/rules", h.handleListRules)
	mux.HandleFunc("POST /iot/v1/rules", h.handleCreateRule)
	mux.HandleFunc("GET /iot/v1/rules/{id}", h.handleGetRule)
	mux.HandleFunc("PUT /iot/v1/rules/{id}", h.handleUpdateRule)
	mux.HandleFunc("PATCH /iot/v1/rules/{id}/toggle", h.handleToggleRule)
	mux.HandleFunc("DELETE /iot/v1/rules/{id}", h.handleDeleteRule)
	mux.HandleFunc("POST /iot/v1/rules/{id}/test", h.handleTestRule)
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"svc-iot"}`))
}

type DeviceResponse struct {
	config.DeviceConfig
	States map[string]any `json:"states"`
}

func (h *Handler) handleListDevices(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	ctx := r.Context()

	allDevs := h.registry.ListDevices()
	res := make([]DeviceResponse, 0, len(allDevs))

	for _, dev := range allDevs {
		if h.store != nil {
			allowed, err := h.store.CheckDeviceAccess(ctx, dev.ID, userID, role, false)
			if err != nil || !allowed {
				continue
			}
		}
		states := h.hub.GetDeviceStates(dev.ID)
		res = append(res, DeviceResponse{
			DeviceConfig: dev,
			States:       states,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *Handler) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, ok := h.registry.GetDevice(id)
	if !ok {
		http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
		return
	}

	userID, role := extractUserContext(r)
	if h.store != nil {
		allowed, err := h.store.CheckDeviceAccess(r.Context(), id, userID, role, false)
		if err != nil || !allowed {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	states := h.hub.GetDeviceStates(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeviceResponse{
		DeviceConfig: dev,
		States:       states,
	})
}

type CommandRequest struct {
	Channel    string `json:"channel"`
	Capability string `json:"capability"` // kompatibel fallback ke openapi
	Value      any    `json:"value"`
}

func (h *Handler) handleDeviceCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID, role := extractUserContext(r)

	if h.store != nil {
		allowed, err := h.store.CheckDeviceAccess(r.Context(), id, userID, role, true)
		if err != nil || !allowed {
			http.Error(w, `{"error":"forbidden: no write access"}`, http.StatusForbidden)
			return
		}
	}

	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	channelID := req.Channel
	if channelID == "" {
		channelID = req.Capability
	}
	if channelID == "" {
		http.Error(w, `{"error":"channel is required"}`, http.StatusBadRequest)
		return
	}

	if err := h.hub.ExecuteCommand(id, channelID, req.Value); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

type DashboardPayload struct {
	Name       string   `json:"name"`
	IsPublic   bool     `json:"isPublic"`
	LayoutJSON string   `json:"layoutJson"`
	DeviceIDs  []string `json:"deviceIds"`
}

func (h *Handler) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	userID, _ := extractUserContext(r)
	if h.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	list, err := h.store.ListUserDashboards(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"failed to list dashboards"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*store.Dashboard{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *Handler) handleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	userID, _ := extractUserContext(r)
	if h.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	var payload DashboardPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.Name) == "" {
		http.Error(w, `{"error":"name and valid layout required"}`, http.StatusBadRequest)
		return
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	dashID := "dash_" + hex.EncodeToString(b)

	dash := &store.Dashboard{
		ID:         dashID,
		UserID:     userID,
		Name:       payload.Name,
		IsPublic:   payload.IsPublic,
		LayoutJSON: payload.LayoutJSON,
		DeviceIDs:  payload.DeviceIDs,
	}

	if err := h.store.CreateDashboard(r.Context(), dash); err != nil {
		http.Error(w, `{"error":"failed to create dashboard"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dash)
}

func (h *Handler) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if h.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	dash, err := h.store.GetDashboard(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"dashboard not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dash)
}

func (h *Handler) handleUpdateDashboard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID, _ := extractUserContext(r)
	if h.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	var payload DashboardPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.Name) == "" {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	dash := &store.Dashboard{
		ID:         id,
		UserID:     userID,
		Name:       payload.Name,
		IsPublic:   payload.IsPublic,
		LayoutJSON: payload.LayoutJSON,
		DeviceIDs:  payload.DeviceIDs,
	}

	if err := h.store.UpdateDashboard(r.Context(), dash); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"dashboard not found or unauthorized"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to update dashboard"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dash)
}

func (h *Handler) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID, _ := extractUserContext(r)
	if h.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	if err := h.store.DeleteDashboard(r.Context(), id, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"dashboard not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to delete dashboard"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

func (h *Handler) handleExploreDashboards(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	if h.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	exploreList, err := h.store.ListExploreDashboards(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch explore dashboards"}`, http.StatusInternalServerError)
		return
	}

	// Filter: hanya kembalikan dashboard yang perangkat di dalamnya dapat diakses oleh user saat ini
	ctx := r.Context()
	matched := make([]*store.Dashboard, 0, len(exploreList))
	for _, d := range exploreList {
		allAccessible := true
		for _, devID := range d.DeviceIDs {
			allowed, err := h.store.CheckDeviceAccess(ctx, devID, userID, role, false)
			if err != nil || !allowed {
				allAccessible = false
				break
			}
		}
		if allAccessible {
			matched = append(matched, d)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(matched)
}

func (h *Handler) handleCloneDashboard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID, _ := extractUserContext(r)
	if h.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusInternalServerError)
		return
	}

	source, err := h.store.GetDashboard(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"source dashboard not found"}`, http.StatusNotFound)
		return
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	newID := "dash_" + hex.EncodeToString(b)

	clone := &store.Dashboard{
		ID:         newID,
		UserID:     userID,
		Name:       source.Name + " (Salinan)",
		IsPublic:   false,
		LayoutJSON: source.LayoutJSON,
		DeviceIDs:  source.DeviceIDs,
	}

	if err := h.store.CreateDashboard(r.Context(), clone); err != nil {
		http.Error(w, `{"error":"failed to clone dashboard"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(clone)
}

func (h *Handler) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	_, role := extractUserContext(r)
	if role != "admin" {
		http.Error(w, `{"error":"forbidden: admin only"}`, http.StatusForbidden)
		return
	}
	deviceID := r.URL.Query().Get("deviceId")
	list, err := h.store.ListPermissions(r.Context(), deviceID)
	if err != nil {
		http.Error(w, `{"error":"failed to list permissions"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*store.DevicePermission{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

type createPermRequest struct {
	DeviceID string  `json:"deviceId"`
	RoleID   *string `json:"roleId,omitempty"`
	UserID   *string `json:"userId,omitempty"`
	CanRead  *bool   `json:"canRead,omitempty"`
	CanWrite *bool   `json:"canWrite,omitempty"`
}

func (h *Handler) handleCreatePermission(w http.ResponseWriter, r *http.Request) {
	_, role := extractUserContext(r)
	if role != "admin" {
		http.Error(w, `{"error":"forbidden: admin only"}`, http.StatusForbidden)
		return
	}

	var req createPermRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" {
		http.Error(w, `{"error":"deviceId is required"}`, http.StatusBadRequest)
		return
	}

	canRead := true
	if req.CanRead != nil {
		canRead = *req.CanRead
	}
	canWrite := true
	if req.CanWrite != nil {
		canWrite = *req.CanWrite
	}

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	permID := "perm_" + hex.EncodeToString(b)

	p := &store.DevicePermission{
		ID:       permID,
		DeviceID: req.DeviceID,
		RoleID:   req.RoleID,
		UserID:   req.UserID,
		CanRead:  canRead,
		CanWrite: canWrite,
	}

	if err := h.store.CreatePermission(r.Context(), p); err != nil {
		http.Error(w, `{"error":"failed to create permission"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (h *Handler) handleDeletePermission(w http.ResponseWriter, r *http.Request) {
	_, role := extractUserContext(r)
	if role != "admin" {
		http.Error(w, `{"error":"forbidden: admin only"}`, http.StatusForbidden)
		return
	}

	id := r.PathValue("id")
	if err := h.store.DeletePermission(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"permission not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to delete permission"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

// ----------------------------------------------------------------------
// Automation Rules Handlers
// ----------------------------------------------------------------------

func (h *Handler) handleListRules(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	filterUser := userID
	if role == "admin" && r.URL.Query().Get("all") == "true" {
		filterUser = ""
	}

	rules, err := h.store.ListAutomationRules(r.Context(), filterUser)
	if err != nil {
		http.Error(w, `{"error":"failed to list automation rules"}`, http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []*store.AutomationRule{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rules)
}

func (h *Handler) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	userID, _ := extractUserContext(r)

	var req store.AutomationRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.TriggerType == "" || req.ActionDeviceID == "" || req.ActionChannelID == "" {
		http.Error(w, `{"error":"missing required fields: name, triggerType, actionDeviceId, actionChannelId"}`, http.StatusBadRequest)
		return
	}

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	req.ID = "rule_" + hex.EncodeToString(b)
	req.UserID = userID

	if err := h.store.CreateAutomationRule(r.Context(), &req); err != nil {
		http.Error(w, `{"error":"failed to create automation rule"}`, http.StatusInternalServerError)
		return
	}

	if h.automation != nil {
		_ = h.automation.ReloadRules(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(req)
}

func (h *Handler) handleGetRule(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	id := r.PathValue("id")

	rule, err := h.store.GetAutomationRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"rule not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get rule"}`, http.StatusInternalServerError)
		return
	}

	if role != "admin" && rule.UserID != userID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rule)
}

func (h *Handler) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	id := r.PathValue("id")

	existing, err := h.store.GetAutomationRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"rule not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get rule"}`, http.StatusInternalServerError)
		return
	}

	if role != "admin" && existing.UserID != userID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var req store.AutomationRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	req.ID = id
	req.UserID = existing.UserID // Pertahankan pemilik asli

	if err := h.store.UpdateAutomationRule(r.Context(), &req); err != nil {
		http.Error(w, `{"error":"failed to update rule"}`, http.StatusInternalServerError)
		return
	}

	if h.automation != nil {
		_ = h.automation.ReloadRules(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

func (h *Handler) handleToggleRule(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	id := r.PathValue("id")

	existing, err := h.store.GetAutomationRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"rule not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get rule"}`, http.StatusInternalServerError)
		return
	}

	if role != "admin" && existing.UserID != userID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// default flip jika tidak ada payload
		body.Enabled = !existing.Enabled
	}

	targetUser := existing.UserID
	if err := h.store.ToggleAutomationRule(r.Context(), id, targetUser, body.Enabled); err != nil {
		http.Error(w, `{"error":"failed to toggle rule"}`, http.StatusInternalServerError)
		return
	}

	if h.automation != nil {
		_ = h.automation.ReloadRules(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      id,
		"enabled": body.Enabled,
	})
}

func (h *Handler) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	id := r.PathValue("id")

	existing, err := h.store.GetAutomationRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"rule not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get rule"}`, http.StatusInternalServerError)
		return
	}

	if role != "admin" && existing.UserID != userID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	if err := h.store.DeleteAutomationRule(r.Context(), id, existing.UserID); err != nil {
		http.Error(w, `{"error":"failed to delete rule"}`, http.StatusInternalServerError)
		return
	}

	if h.automation != nil {
		_ = h.automation.ReloadRules(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}

func (h *Handler) handleTestRule(w http.ResponseWriter, r *http.Request) {
	userID, role := extractUserContext(r)
	id := r.PathValue("id")

	rule, err := h.store.GetAutomationRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"rule not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get rule"}`, http.StatusInternalServerError)
		return
	}

	if role != "admin" && rule.UserID != userID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	// Eksekusi aksi rule secara instan
	var actionVal any = rule.ActionValue
	low := strings.ToLower(rule.ActionValue)
	if low == "true" || low == "on" {
		actionVal = true
	} else if low == "false" || low == "off" {
		actionVal = false
	}

	if err := h.hub.ExecuteCommand(rule.ActionDeviceID, rule.ActionChannelID, actionVal); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to fire action: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"message":"rule action executed successfully"}`))
}

func extractUserContext(r *http.Request) (userID, role string) {
	userID = r.Header.Get("X-User-Id")
	role = r.Header.Get("X-User-Role")
	if userID == "" {
		userID = "default_user"
	}
	if role == "" {
		role = "user"
	}
	return userID, role
}
