package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gumpalan/svc-iot/internal/config"
	"github.com/gumpalan/svc-iot/internal/core"
	"github.com/gumpalan/svc-iot/internal/database"
	"github.com/gumpalan/svc-iot/internal/store"
	"github.com/gumpalan/svc-iot/migrations"
	_ "modernc.org/sqlite"
)

type mockPub struct{}

func (m *mockPub) PublishCommand(topic string, payload []byte) error {
	return nil
}

func setupTestServer(t *testing.T) (*httptest.Server, *store.Store, *core.StateHub) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := database.Migrate(db, database.DialectSQLite, migrations.FS, ".", "iot_migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	st := store.New(db, database.DialectSQLite)

	cfg := &config.Config{
		Devices: []config.DeviceConfig{
			{
				ID:     "lamp1",
				Name:   "Lampu Kamar",
				Driver: "custom",
				Channels: []config.ChannelConfig{
					{
						ID:           "power",
						Name:         "Power",
						Direction:    config.DirectionBidirectional,
						CommandTopic: "home/lamp1/set",
						StateTopic:   "home/lamp1/stat",
						Type:         config.DataTypeBoolean,
					},
				},
			},
		},
	}

	reg := core.NewDeviceRegistry(cfg)
	hub := core.NewStateHub(reg, &mockPub{})

	handler := NewHandler(reg, hub, st)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return httptest.NewServer(mux), st, hub
}

func TestHTTPAPI_DevicesAndCommand(t *testing.T) {
	ts, _, hub := setupTestServer(t)
	defer ts.Close()

	// 1. GET /iot/v1/devices
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/iot/v1/devices", nil)
	req.Header.Set("X-User-Id", "u1")
	req.Header.Set("X-User-Role", "admin")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get devices failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var devs []DeviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&devs); err != nil {
		t.Fatalf("decode devices: %v", err)
	}
	if len(devs) != 1 || devs[0].ID != "lamp1" {
		t.Fatalf("unexpected devices: %+v", devs)
	}

	// 2. POST /iot/v1/devices/lamp1/command
	cmdPayload := []byte(`{"channel":"power","value":true}`)
	cmdReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/iot/v1/devices/lamp1/command", bytes.NewReader(cmdPayload))
	cmdReq.Header.Set("X-User-Id", "u1")
	cmdReq.Header.Set("X-User-Role", "admin")

	cmdResp, err := http.DefaultClient.Do(cmdReq)
	if err != nil {
		t.Fatalf("post command: %v", err)
	}
	defer cmdResp.Body.Close()

	if cmdResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", cmdResp.StatusCode)
	}

	if hub.GetDeviceStates("lamp1")["power"] != true {
		t.Fatalf("expected hub state true after command")
	}
}

func TestHTTPAPI_DashboardsCRUDAndClone(t *testing.T) {
	ts, _, _ := setupTestServer(t)
	defer ts.Close()

	// 1. Create Dashboard for User 1
	body := []byte(`{
		"name": "Kamar Utama",
		"isPublic": true,
		"layoutJson": "[{\"widget\":\"toggle\"}]",
		"deviceIds": ["lamp1"]
	}`)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/iot/v1/dashboards", bytes.NewReader(body))
	createReq.Header.Set("X-User-Id", "user_1")
	resp, err := http.DefaultClient.Do(createReq)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create failed, status: %d, err: %v", resp.StatusCode, err)
	}
	var created store.Dashboard
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// 2. List Dashboards for User 1
	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/iot/v1/dashboards", nil)
	listReq.Header.Set("X-User-Id", "user_1")
	resp2, _ := http.DefaultClient.Do(listReq)
	var list []store.Dashboard
	_ = json.NewDecoder(resp2.Body).Decode(&list)
	resp2.Body.Close()
	if len(list) != 1 {
		t.Fatalf("expected 1 dashboard, got %d", len(list))
	}

	// 3. Explore Dashboards as User 2 (should see user 1's public dashboard)
	expReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/iot/v1/dashboards/explore", nil)
	expReq.Header.Set("X-User-Id", "user_2")
	resp3, _ := http.DefaultClient.Do(expReq)
	var exploreList []store.Dashboard
	_ = json.NewDecoder(resp3.Body).Decode(&exploreList)
	resp3.Body.Close()
	if len(exploreList) != 1 {
		t.Fatalf("expected 1 explore dashboard for user 2, got %d", len(exploreList))
	}

	// 4. Clone User 1's dashboard for User 2
	cloneReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/iot/v1/dashboards/"+created.ID+"/clone", nil)
	cloneReq.Header.Set("X-User-Id", "user_2")
	resp4, _ := http.DefaultClient.Do(cloneReq)
	if resp4.StatusCode != http.StatusCreated {
		t.Fatalf("clone failed, status: %d", resp4.StatusCode)
	}
	var cloned store.Dashboard
	_ = json.NewDecoder(resp4.Body).Decode(&cloned)
	resp4.Body.Close()

	if cloned.UserID != "user_2" || cloned.Name != "Kamar Utama (Salinan)" {
		t.Fatalf("cloned dashboard mismatch: %+v", cloned)
	}
}

func TestHTTPAPI_AutomationRules(t *testing.T) {
	ts, _, _ := setupTestServer(t)
	defer ts.Close()

	// 1. Create Rule
	devID := "lamp1"
	chID := "power"
	op := "=="
	val := "true"
	payload := store.AutomationRule{
		Name:             "Matikan Lampu Saat Siang",
		Enabled:          true,
		TriggerType:      "schedule",
		TriggerCron:      &val,
		ActionDeviceID:   devID,
		ActionChannelID:  chID,
		ActionValue:      "false",
		TriggerDeviceID:  &devID,
		TriggerChannelID: &chID,
		TriggerOperator:  &op,
		TriggerValue:     &val,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/iot/v1/rules", bytes.NewReader(body))
	req.Header.Set("X-User-Id", "user_1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rule failed: status %d, err %v", resp.StatusCode, err)
	}
	var created store.AutomationRule
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	if created.ID == "" || created.UserID != "user_1" {
		t.Fatalf("unexpected created rule: %+v", created)
	}

	// 2. List Rules
	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/iot/v1/rules", nil)
	listReq.Header.Set("X-User-Id", "user_1")
	listResp, _ := http.DefaultClient.Do(listReq)
	var list []store.AutomationRule
	_ = json.NewDecoder(listResp.Body).Decode(&list)
	listResp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(list))
	}

	// 3. Toggle Rule
	toggleReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/iot/v1/rules/"+created.ID+"/toggle", bytes.NewReader([]byte(`{"enabled":false}`)))
	toggleReq.Header.Set("X-User-Id", "user_1")
	toggleResp, _ := http.DefaultClient.Do(toggleReq)
	if toggleResp.StatusCode != http.StatusOK {
		t.Fatalf("toggle failed: status %d", toggleResp.StatusCode)
	}
	toggleResp.Body.Close()

	// 4. Test Fire Action
	testReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/iot/v1/rules/"+created.ID+"/test", nil)
	testReq.Header.Set("X-User-Id", "user_1")
	testResp, _ := http.DefaultClient.Do(testReq)
	if testResp.StatusCode != http.StatusOK {
		t.Fatalf("test-fire failed: status %d", testResp.StatusCode)
	}
	testResp.Body.Close()

	// 5. Delete Rule
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/iot/v1/rules/"+created.ID, nil)
	delReq.Header.Set("X-User-Id", "user_1")
	delResp, _ := http.DefaultClient.Do(delReq)
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("delete rule failed: status %d", delResp.StatusCode)
	}
	delResp.Body.Close()
}
