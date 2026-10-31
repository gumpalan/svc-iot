package core

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gumpalan/svc-iot/internal/config"
	"github.com/gumpalan/svc-iot/internal/store"
)

type AutomationStore interface {
	ListActiveAutomationRules(ctx context.Context) ([]*store.AutomationRule, error)
}

// HistoryStore adalah subset store.Store untuk pencatatan actuator log dari automation engine.
type HistoryStore interface {
	CreateActuatorLog(ctx context.Context, l *store.ActuatorLog) error
}

type CommandExecutor interface {
	ExecuteCommand(deviceID, channelID string, value any) error
	ExecuteCommandFrom(deviceID, channelID string, value any, logEntry *store.ActuatorLog) error
}

type AutomationEngine struct {
	store       AutomationStore
	histStore   HistoryStore
	executor    CommandExecutor
	historyCfg  config.HistoryConfig

	mu        sync.RWMutex
	rules     []*store.AutomationRule
	lastFired map[string]int64 // ruleID -> timestamp detik terakhir dieksekusi

	stopCh chan struct{}
}

func NewAutomationEngine(st AutomationStore, exec CommandExecutor) *AutomationEngine {
	return &AutomationEngine{
		store:     st,
		executor:  exec,
		rules:     make([]*store.AutomationRule, 0),
		lastFired: make(map[string]int64),
		stopCh:    make(chan struct{}),
	}
}

// SetHistoryStore menetapkan store dan konfigurasi untuk pencatatan actuator log.
func (e *AutomationEngine) SetHistoryStore(hs HistoryStore, cfg config.HistoryConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.histStore = hs
	e.historyCfg = cfg
}

// ReloadRules membaca ulang seluruh aturan aktif dari database ke memori
func (e *AutomationEngine) ReloadRules(ctx context.Context) error {
	if e.store == nil {
		return nil
	}
	active, err := e.store.ListActiveAutomationRules(ctx)
	if err != nil {
		return fmt.Errorf("reload rules: %w", err)
	}

	e.mu.Lock()
	e.rules = active
	e.mu.Unlock()

	log.Printf("[Automation] Memuat %d aturan otomasi aktif ke engine.", len(active))
	return nil
}

// EvaluateStateChange memeriksa apakah ada aturan tipe 'state' yang terpenuhi
func (e *AutomationEngine) EvaluateStateChange(deviceID, channelID string, newVal any) {
	e.mu.RLock()
	rules := make([]*store.AutomationRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	for _, r := range rules {
		if !r.Enabled || r.TriggerType != "state" {
			continue
		}
		if r.TriggerDeviceID == nil || *r.TriggerDeviceID != deviceID {
			continue
		}
		if r.TriggerChannelID == nil || *r.TriggerChannelID != channelID {
			continue
		}

		op := "=="
		if r.TriggerOperator != nil && *r.TriggerOperator != "" {
			op = *r.TriggerOperator
		}
		targetVal := ""
		if r.TriggerValue != nil {
			targetVal = *r.TriggerValue
		}

		if matchCondition(newVal, op, targetVal) {
			// Cegah trigger beruntun dalam interval 2 detik untuk rule yang sama
			now := time.Now().Unix()
			e.mu.Lock()
			last := e.lastFired[r.ID]
			if now-last < 2 {
				e.mu.Unlock()
				continue
			}
			e.lastFired[r.ID] = now
			e.mu.Unlock()

			log.Printf("[Automation] Aturan '%s' (%s) terpenuhi! Menjalankan aksi -> %s/%s = %s",
				r.Name, r.ID, r.ActionDeviceID, r.ActionChannelID, r.ActionValue)

			go func(rule *store.AutomationRule) {
				parsedActionVal := parseFlexibleValue(rule.ActionValue)
				logEntry := (*store.ActuatorLog)(nil)

				e.mu.RLock()
				hs := e.histStore
				hcfg := e.historyCfg
				e.mu.RUnlock()

				if hs != nil && hcfg.LogActuator() {
					ruleID := rule.ID
					ruleName := rule.Name
					logEntry = &store.ActuatorLog{
						Source:    "automation",
						ActorID:   &ruleID,
						ActorName: &ruleName,
					}
				}

				if err := e.executor.ExecuteCommandFrom(rule.ActionDeviceID, rule.ActionChannelID, parsedActionVal, logEntry); err != nil {
					log.Printf("[Automation] Gagal menjalankan aksi aturan '%s': %v", rule.Name, err)
				}
			}(r)
		}
	}
}

// StartScheduler menjalankan pengecekan aturan jadwal waktu (schedule/timer) setiap menit
func (e *AutomationEngine) StartScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				e.evaluateSchedules()
			case <-e.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (e *AutomationEngine) Stop() {
	close(e.stopCh)
}

func (e *AutomationEngine) evaluateSchedules() {
	now := time.Now()
	nowMinute := now.Format("15:04") // "HH:MM"
	nowUnixMin := now.Unix() / 60

	e.mu.RLock()
	rules := make([]*store.AutomationRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	for _, r := range rules {
		if !r.Enabled || r.TriggerType != "schedule" || r.TriggerCron == nil {
			continue
		}

		cronExpr := strings.TrimSpace(*r.TriggerCron)
		matched := false

		// Format sederhana "HH:MM" (misal "23:00")
		if len(cronExpr) == 5 && strings.Contains(cronExpr, ":") {
			if cronExpr == nowMinute {
				matched = true
			}
		} else {
			// Format cron 5 bagian: "min hour dom mon dow"
			parts := strings.Fields(cronExpr)
			if len(parts) == 5 {
				minMatch := parts[0] == "*" || parts[0] == fmt.Sprintf("%d", now.Minute())
				hourMatch := parts[1] == "*" || parts[1] == fmt.Sprintf("%d", now.Hour())
				matched = minMatch && hourMatch
			}
		}

		if matched {
			e.mu.Lock()
			lastMin := e.lastFired[r.ID]
			if lastMin == nowUnixMin {
				e.mu.Unlock()
				continue
			}
			e.lastFired[r.ID] = nowUnixMin
			e.mu.Unlock()

			log.Printf("[Automation-Schedule] Aturan '%s' (%s) dipicu jadwal (%s)! Aksi -> %s/%s = %s",
				r.Name, r.ID, cronExpr, r.ActionDeviceID, r.ActionChannelID, r.ActionValue)

			go func(rule *store.AutomationRule) {
				parsedActionVal := parseFlexibleValue(rule.ActionValue)
				logEntry := (*store.ActuatorLog)(nil)

				e.mu.RLock()
				hs := e.histStore
				hcfg := e.historyCfg
				e.mu.RUnlock()

				if hs != nil && hcfg.LogActuator() {
					ruleID := rule.ID
					ruleName := rule.Name
					logEntry = &store.ActuatorLog{
						Source:    "schedule",
						ActorID:   &ruleID,
						ActorName: &ruleName,
					}
				}

				if err := e.executor.ExecuteCommandFrom(rule.ActionDeviceID, rule.ActionChannelID, parsedActionVal, logEntry); err != nil {
					log.Printf("[Automation] Gagal menjalankan aksi jadwal '%s': %v", rule.Name, err)
				}
			}(r)
		}
	}
}

// matchCondition membandingkan nilai aktual dengan operator dan target
func matchCondition(current any, op, target string) bool {
	// 1. Coba perbandingan numerik jika target adalah angka
	if targetNum, err := strconv.ParseFloat(target, 64); err == nil {
		var curNum float64
		switch v := current.(type) {
		case float64:
			curNum = v
		case int:
			curNum = float64(v)
		case string:
			if parsed, err2 := strconv.ParseFloat(v, 64); err2 == nil {
				curNum = parsed
			} else {
				return compareString(v, op, target)
			}
		default:
			return compareString(fmt.Sprintf("%v", current), op, target)
		}

		switch op {
		case ">":
			return curNum > targetNum
		case ">=":
			return curNum >= targetNum
		case "<":
			return curNum < targetNum
		case "<=":
			return curNum <= targetNum
		case "==":
			return curNum == targetNum
		case "!=":
			return curNum != targetNum
		}
	}

	// 2. Coba perbandingan boolean
	if targetBool, err := parseBool(target); err == nil {
		curBool, err2 := parseBool(fmt.Sprintf("%v", current))
		if err2 == nil {
			if op == "!=" {
				return curBool != targetBool
			}
			return curBool == targetBool
		}
	}

	// 3. Fallback perbandingan string
	curStr := fmt.Sprintf("%v", current)
	return compareString(curStr, op, target)
}

func compareString(cur, op, target string) bool {
	c := strings.ToLower(strings.TrimSpace(cur))
	t := strings.ToLower(strings.TrimSpace(target))
	switch op {
	case "!=":
		return c != t
	default: // "=="
		return c == t
	}
}

func parseBool(s string) (bool, error) {
	low := strings.ToLower(strings.TrimSpace(s))
	switch low {
	case "true", "1", "on", "yes":
		return true, nil
	case "false", "0", "off", "no":
		return false, nil
	}
	return false, fmt.Errorf("not bool: %s", s)
}

func parseFlexibleValue(s string) any {
	s = strings.TrimSpace(s)
	if b, err := parseBool(s); err == nil {
		return b
	}
	if num, err := strconv.ParseFloat(s, 64); err == nil {
		if float64(int(num)) == num {
			return int(num)
		}
		return num
	}
	return s
}
