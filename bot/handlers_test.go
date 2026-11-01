package bot

import (
	"strings"
	"testing"
	"unicode/utf8"

	"tg_bot_home_assistant/ha"
)

func TestExtractDomainFromEntity(t *testing.T) {
	tests := []struct {
		entityID       string
		expectedDomain string
	}{
		{"automation.vkliuchitteplyipol", "automation"},
		{"light.living_room", "light"},
		{"switch.heater_relay", "switch"},
		{"input_boolean.vacation_mode", "input_boolean"},
		{"no_dot_identifier", "no_dot_identifier"},
		{"", ""},
	}

	for _, tt := range tests {
		domain := extractDomainFromEntity(tt.entityID)
		if domain != tt.expectedDomain {
			t.Errorf("extractDomainFromEntity(%q) = %q, expected %q", tt.entityID, domain, tt.expectedDomain)
		}
	}
}

func TestParseDomainToggleCallback(t *testing.T) {
	tests := []struct {
		raw            string
		expectedDomain string
		expectedRef    string
		expectedPage   int
	}{
		{
			raw:            "automation.vkliuchitteplyipol:0",
			expectedDomain: "",
			expectedRef:    "automation.vkliuchitteplyipol",
			expectedPage:   0,
		},
		{
			raw:            "h_1234abcd:2",
			expectedDomain: "",
			expectedRef:    "h_1234abcd",
			expectedPage:   2,
		},
		{
			raw:            "automation:h_1234abcd:3",
			expectedDomain: "automation",
			expectedRef:    "h_1234abcd",
			expectedPage:   3,
		},
		{
			raw:            "switch.socket",
			expectedDomain: "",
			expectedRef:    "switch.socket",
			expectedPage:   0,
		},
	}

	for _, tt := range tests {
		domain, ref, page := parseDomainToggleCallback(tt.raw)
		if domain != tt.expectedDomain || ref != tt.expectedRef || page != tt.expectedPage {
			t.Errorf("parseDomainToggleCallback(%q) = (%q, %q, %d), expected (%q, %q, %d)",
				tt.raw, domain, ref, page, tt.expectedDomain, tt.expectedRef, tt.expectedPage)
		}
	}
}

func TestFormatOperationStatus(t *testing.T) {
	// 1. nil stateObj
	status := formatOperationStatus("automation.test", nil)
	if status != "✅ Состояние переключено" {
		t.Errorf("expected default status for nil stateObj, got %q", status)
	}

	// 2. on with FriendlyName
	statusOn := formatOperationStatus("automation.vkliuchitteplyipol", &ha.EntityState{
		EntityID: "automation.vkliuchitteplyipol",
		State:    "on",
		Attributes: map[string]interface{}{
			"friendly_name": "Включить теплый пол",
		},
	})
	expectedOn := "🟢 Включить теплый пол: ВКЛ"
	if statusOn != expectedOn {
		t.Errorf("expected %q, got %q", expectedOn, statusOn)
	}

	// 3. off without FriendlyName
	statusOff := formatOperationStatus("automation.vkliuchitteplyipol", &ha.EntityState{
		EntityID: "automation.vkliuchitteplyipol",
		State:    "off",
	})
	expectedOff := "⚪ automation.vkliuchitteplyipol: ВЫКЛ"
	if statusOff != expectedOff {
		t.Errorf("expected %q, got %q", expectedOff, statusOff)
	}

	// 4. lock states
	statusLocked := formatOperationStatus("lock.front_door", &ha.EntityState{
		EntityID: "lock.front_door",
		State:    "locked",
		Attributes: map[string]interface{}{
			"friendly_name": "Входная дверь",
		},
	})
	if statusLocked != "⚪ Входная дверь: LOCKED" {
		t.Errorf("expected locked status, got %q", statusLocked)
	}

	statusUnlocked := formatOperationStatus("lock.front_door", &ha.EntityState{
		EntityID: "lock.front_door",
		State:    "unlocked",
		Attributes: map[string]interface{}{
			"friendly_name": "Входная дверь",
		},
	})
	if statusUnlocked != "🟢 Входная дверь: UNLOCKED" {
		t.Errorf("expected unlocked status, got %q", statusUnlocked)
	}

	// 5. Very long friendly name truncation (UTF-8 safe <= 190 runes)
	longName := strings.Repeat("А", 250)
	statusLong := formatOperationStatus("switch.test", &ha.EntityState{
		EntityID: "switch.test",
		State:    "on",
		Attributes: map[string]interface{}{
			"friendly_name": longName,
		},
	})
	if !strings.HasSuffix(statusLong, "...") {
		t.Errorf("expected truncated string with ..., got %q", statusLong)
	}
	if runeCount := utf8.RuneCountInString(statusLong); runeCount > 190 {
		t.Errorf("expected rune count <= 190, got %d", runeCount)
	}
}

func TestResolveControlService(t *testing.T) {
	// automation
	dom, srv, data := resolveControlService("automation.vkliuchitteplyipol", "on")
	if dom != "automation" || srv != "toggle" || data["entity_id"] != "automation.vkliuchitteplyipol" {
		t.Errorf("unexpected automation service: %s/%s", dom, srv)
	}

	// scene
	dom, srv, _ = resolveControlService("scene.movie", "scening")
	if dom != "scene" || srv != "turn_on" {
		t.Errorf("expected scene/turn_on, got %s/%s", dom, srv)
	}

	// script
	dom, srv, _ = resolveControlService("script.morning", "off")
	if dom != "script" || srv != "turn_on" {
		t.Errorf("expected script/turn_on, got %s/%s", dom, srv)
	}

	// lock
	dom, srv, _ = resolveControlService("lock.door", "locked")
	if dom != "lock" || srv != "unlock" {
		t.Errorf("expected lock/unlock, got %s/%s", dom, srv)
	}
	dom, srv, _ = resolveControlService("lock.door", "unlocked")
	if dom != "lock" || srv != "lock" {
		t.Errorf("expected lock/lock, got %s/%s", dom, srv)
	}

	// vacuum
	dom, srv, _ = resolveControlService("vacuum.cleaner", "cleaning")
	if dom != "vacuum" || srv != "return_to_base" {
		t.Errorf("expected vacuum/return_to_base, got %s/%s", dom, srv)
	}
	dom, srv, _ = resolveControlService("vacuum.cleaner", "docked")
	if dom != "vacuum" || srv != "start" {
		t.Errorf("expected vacuum/start, got %s/%s", dom, srv)
	}
}

func TestGetAreaIcon(t *testing.T) {
	tests := []struct {
		areaID   string
		areaName string
		expected string
	}{
		{"kukhnia", "Кухня", "🍳"},
		{"spalnia", "Спальня", "🛏"},
		{"gostinaia", "Гостиная", "🛋"},
		{"kids_room", "Детская", "🧸"},
		{"vannaia", "Ванная комната", "🛁"},
		{"koridor", "Прихожая", "🚪"},
		{"kabinet", "Рабочий кабинет", "🖥"},
		{"balcony", "Балкон", "🪴"},
		{"no_area", "Без комнаты", "📦"},
		{"unknown", "Склад", "🏠"},
	}

	for _, tt := range tests {
		icon := GetAreaIcon(tt.areaID, tt.areaName)
		if icon != tt.expected {
			t.Errorf("GetAreaIcon(%q, %q) = %q, expected %q", tt.areaID, tt.areaName, icon, tt.expected)
		}
	}
}

