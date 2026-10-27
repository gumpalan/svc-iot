package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gumpalan/svc-iot/internal/config"
	"github.com/gumpalan/svc-iot/internal/core"
	"github.com/gumpalan/svc-iot/internal/store"
)

type Handler struct {
	registry *core.DeviceRegistry
	hub      *core.StateHub
	store    *store.Store
}

func NewHandler(registry *core.DeviceRegistry, hub *core.StateHub, st *store.Store) *Handler {
	return &Handler{
		registry: registry,
		hub:      hub,
		store:    st,
	}
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
