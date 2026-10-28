package core

import (
	"context"
	"testing"
	"time"

	"github.com/gumpalan/svc-iot/internal/store"
)

type mockStore struct {
	rules []*store.AutomationRule
}

func (m *mockStore) ListActiveAutomationRules(ctx context.Context) ([]*store.AutomationRule, error) {
	return m.rules, nil
}

type mockExecutor struct {
	commands []mockCmd
}

type mockCmd struct {
	deviceID  string
	channelID string
	value     any
}

func (m *mockExecutor) ExecuteCommand(deviceID, channelID string, value any) error {
	m.commands = append(m.commands, mockCmd{deviceID, channelID, value})
	return nil
}

func TestAutomationEngine_StateTrigger(t *testing.T) {
	devID := "sensor-1"
	chID := "temperature"
	op := ">="
	targetVal := "30.0"

	rules := []*store.AutomationRule{
		{
			ID:               "rule-temp",
			UserID:           "u1",
			Name:             "Nyalakan AC",
			Enabled:          true,
			TriggerType:      "state",
			TriggerDeviceID:  &devID,
			TriggerChannelID: &chID,
			TriggerOperator:  &op,
			TriggerValue:     &targetVal,
			ActionDeviceID:   "ac-1",
			ActionChannelID:  "power",
			ActionValue:      "true",
		},
	}

	st := &mockStore{rules: rules}
	exec := &mockExecutor{}

	engine := NewAutomationEngine(st, exec)
	if err := engine.ReloadRules(context.Background()); err != nil {
		t.Fatalf("ReloadRules failed: %v", err)
	}

	// 1. Suhu 28 -> tidak trigger
	engine.EvaluateStateChange("sensor-1", "temperature", 28.0)
	time.Sleep(50 * time.Millisecond)
	if len(exec.commands) != 0 {
		t.Fatalf("expected 0 commands, got %d", len(exec.commands))
	}

	// 2. Suhu 31.5 -> harus trigger
	engine.EvaluateStateChange("sensor-1", "temperature", 31.5)
	time.Sleep(50 * time.Millisecond)
	if len(exec.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(exec.commands))
	}
	cmd := exec.commands[0]
	if cmd.deviceID != "ac-1" || cmd.channelID != "power" || cmd.value != true {
		t.Errorf("unexpected command: %+v", cmd)
	}
}

func TestMatchCondition(t *testing.T) {
	tests := []struct {
		cur  any
		op   string
		tgt  string
		want bool
	}{
		{35.0, ">", "30.0", true},
		{25.0, ">", "30.0", false},
		{30.0, ">=", "30.0", true},
		{true, "==", "true", true},
		{false, "==", "true", false},
		{"ON", "==", "on", true},
		{1, "==", "1", true},
		{0, "==", "false", true},
	}

	for _, tt := range tests {
		got := matchCondition(tt.cur, tt.op, tt.tgt)
		if got != tt.want {
			t.Errorf("matchCondition(%v, %q, %q) = %v; want %v", tt.cur, tt.op, tt.tgt, got, tt.want)
		}
	}
}
