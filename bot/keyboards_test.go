package bot

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"tg_bot_home_assistant/ha"
)

func TestFormatDeviceButton(t *testing.T) {
	tests := []struct {
		name     string
		entity   ha.Entity
		contains []string
	}{
		{
			name: "light on",
			entity: ha.Entity{
				EntityID:     "light.kitchen",
				Domain:       "light",
				FriendlyName: "Кухня",
				State:        "on",
			},
			contains: []string{"💡", "Кухня", "🟢 ВКЛ"},
		},
		{
			name: "switch off",
			entity: ha.Entity{
				EntityID:     "switch.socket",
				Domain:       "switch",
				FriendlyName: "Розетка",
				State:        "off",
			},
			contains: []string{"🔌", "Розетка", "⚪ ВЫКЛ"},
		},
		{
			name: "cover open",
			entity: ha.Entity{
				EntityID:     "cover.bedroom",
				Domain:       "cover",
				FriendlyName: "Шторы в спальне",
				State:        "open",
			},
			contains: []string{"🪟", "🟢 OPEN"},
		},
		{
			name: "humidifier on",
			entity: ha.Entity{
				EntityID:     "humidifier.kukhnia_osushitel",
				Domain:       "humidifier",
				FriendlyName: "Осушитель",
				State:        "on",
			},
			contains: []string{"💧", "Осушитель", "🟢 ВКЛ"},
		},
		{
			name: "long friendly name with cyrillic",
			entity: ha.Entity{
				EntityID:     "light.very_long_name",
				Domain:       "light",
				FriendlyName: "Очень длинное русское название светильника",
				State:        "on",
			},
			contains: []string{"💡", "Очень длинное русское название светильника", "🟢 ВКЛ"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			btnText := FormatDeviceButton(tt.entity)
			for _, exp := range tt.contains {
				if !strings.Contains(btnText, exp) {
					t.Errorf("FormatDeviceButton() = %q, expected to contain %q", btnText, exp)
				}
			}

			// Ensure UTF-8 validity
			if !utf8.ValidString(btnText) {
				t.Errorf("FormatDeviceButton() produced invalid UTF-8 string: %q", btnText)
			}
		})
	}
}

func TestGetDevicesKeyboard_CallbackDataLimits(t *testing.T) {
	devices := []ha.Entity{
		{
			EntityID:     "light.living_room_ceiling_lamp_chandelier_main_light",
			Domain:       "light",
			FriendlyName: "Люстра",
			State:        "on",
		},
		{
			EntityID:     "automation.turn_off_all_lights_when_leaving_home_automation",
			Domain:       "automation",
			FriendlyName: "Выключить все при уходе",
			State:        "off",
		},
	}

	kb := GetDevicesKeyboard(devices, 0, 10)
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				continue
			}
			data := *btn.CallbackData
			if len([]byte(data)) > 64 {
				t.Errorf("CallbackData %q exceeds 64 bytes limit: length is %d", data, len([]byte(data)))
			}
		}
	}
}

func TestGetDevicesKeyboard_Pagination(t *testing.T) {
	var devices []ha.Entity
	for i := 0; i < 25; i++ {
		devices = append(devices, ha.Entity{
			EntityID:     fmt.Sprintf("switch.socket_%d", i),
			Domain:       "switch",
			FriendlyName: fmt.Sprintf("Розетка %d", i),
			State:        "on",
		})
	}

	// Page 0 (items 0..9)
	kb0 := GetDevicesKeyboard(devices, 0, 10)
	// 10 device rows + 1 nav row + 1 refresh row = 12 rows
	if len(kb0.InlineKeyboard) != 12 {
		t.Fatalf("expected 12 rows on page 0, got %d", len(kb0.InlineKeyboard))
	}

	navRow0 := kb0.InlineKeyboard[10]
	if len(navRow0) != 2 {
		t.Fatalf("expected 2 buttons in nav row (counter and forward), got %d", len(navRow0))
	}
	if *navRow0[1].CallbackData != "pa:1" {
		t.Errorf("expected forward button pa:1, got %s", *navRow0[1].CallbackData)
	}

	// Page 1 (items 10..19)
	kb1 := GetDevicesKeyboard(devices, 1, 10)
	navRow1 := kb1.InlineKeyboard[10]
	if len(navRow1) != 3 {
		t.Fatalf("expected 3 buttons in nav row (back, counter, forward), got %d", len(navRow1))
	}
	if *navRow1[0].CallbackData != "pa:0" {
		t.Errorf("expected back button pa:0, got %s", *navRow1[0].CallbackData)
	}
	if *navRow1[2].CallbackData != "pa:2" {
		t.Errorf("expected forward button pa:2, got %s", *navRow1[2].CallbackData)
	}
}

func TestGetDomainsKeyboard_AdaptiveLayout(t *testing.T) {
	grouped := map[string][]ha.Entity{
		"light": {
			{EntityID: "light.1", Domain: "light", FriendlyName: "Свет 1", State: "on"},
		},
		"humidifier": {
			{EntityID: "humidifier.1", Domain: "humidifier", FriendlyName: "Осушитель", State: "on"},
		},
		"climate": {
			{EntityID: "climate.1", Domain: "climate", FriendlyName: "Кондиционер", State: "on"},
		},
	}

	kb := GetDomainsKeyboard(grouped)
	if len(kb.InlineKeyboard) == 0 {
		t.Fatal("expected keyboard rows, got 0")
	}

	// humidifier ("💧 Осушители и увлажнители (1)") has length > 18, so it must be on its own row with 1 button
	var foundHumidifierRow bool
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if strings.Contains(btn.Text, "Осушители и увлажнители") {
				if len(row) != 1 {
					t.Errorf("expected long humidifier button to occupy full row (len=1), got len=%d", len(row))
				}
				foundHumidifierRow = true
			}
		}
	}

	if !foundHumidifierRow {
		t.Error("humidifier button not found in keyboard")
	}
}

func TestGetDomainsKeyboard_HidesEmptyDomains(t *testing.T) {
	grouped := map[string][]ha.Entity{
		"light": {
			{EntityID: "light.1", Domain: "light", FriendlyName: "Свет 1", State: "on"},
		},
		"switch": {
			{EntityID: "switch.1", Domain: "switch", FriendlyName: "Розетка 1", State: "off"},
		},
		"cover":        {}, // empty (0)
		"siren":        {}, // empty (0)
		"water_heater": {}, // empty (0)
	}

	kb := GetDomainsKeyboard(grouped)

	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if strings.Contains(btn.Text, "Шторы") || strings.Contains(btn.Text, "Сирены") || strings.Contains(btn.Text, "Водонагреватели") {
				t.Errorf("expected empty categories to be hidden, but found button: %s", btn.Text)
			}
		}
	}
}

func TestFormatConfigEntityButton(t *testing.T) {
	tests := []struct {
		name     string
		entity   ha.Entity
		contains []string
	}{
		{
			name: "switch config on",
			entity: ha.Entity{
				EntityID:     "switch.backlight",
				Domain:       "switch",
				FriendlyName: "Подсветка клавиш",
				State:        "on",
				Category:     "config",
			},
			contains: []string{"🔌", "Подсветка клавиш", "🟢 ВКЛ"},
		},
		{
			name: "button config",
			entity: ha.Entity{
				EntityID:     "button.restart",
				Domain:       "button",
				FriendlyName: "Перезагрузка",
				Category:     "config",
			},
			contains: []string{"🔘", "Перезагрузка", "Нажать"},
		},
		{
			name: "select config",
			entity: ha.Entity{
				EntityID:     "select.power_on_mode",
				Domain:       "select",
				FriendlyName: "Режим включения",
				State:        "previous",
				Category:     "config",
			},
			contains: []string{"📋", "Режим включения", "previous"},
		},
		{
			name: "number config with unit",
			entity: ha.Entity{
				EntityID:     "number.auto_off_timer",
				Domain:       "number",
				FriendlyName: "Таймер автовыключения",
				State:        "15",
				Category:     "config",
				Attributes: map[string]interface{}{
					"unit_of_measurement": "мин",
				},
			},
			contains: []string{"⏱️", "Таймер автовыключения", "15 мин"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := FormatConfigEntityButton(tt.entity)
			for _, exp := range tt.contains {
				if !strings.Contains(res, exp) {
					t.Errorf("FormatConfigEntityButton() = %q, expected to contain %q", res, exp)
				}
			}
		})
	}
}

func TestGetDeviceSettingsKeyboard_CallbackDataLimits(t *testing.T) {
	dev := ha.DeviceConfigGroup{
		DeviceID:   "dev_long_identifier_1234567890abcdef",
		DeviceName: "Выключатель двухклавишный на кухне",
		Domain:     "switch",
		ConfigEntities: []ha.Entity{
			{
				EntityID:     "switch.kitchen_switch_button_led_indicator_backlight_mode",
				Domain:       "switch",
				FriendlyName: "Подсветка",
				State:        "on",
				Category:     "config",
			},
			{
				EntityID:     "number.kitchen_switch_button_auto_turn_off_timer_countdown",
				Domain:       "number",
				FriendlyName: "Таймер выключения",
				State:        "5",
				Category:     "config",
			},
			{
				EntityID:     "select.kitchen_switch_relay_power_on_behavior_mode_state",
				Domain:       "select",
				FriendlyName: "Поведение при подаче питания",
				State:        "on",
				Category:     "config",
			},
			{
				EntityID:     "button.kitchen_switch_restart_controller_hardware",
				Domain:       "button",
				FriendlyName: "Перезагрузка",
				Category:     "config",
			},
		},
	}

	kb := GetDeviceSettingsKeyboard(dev, "switch")
	if len(kb.InlineKeyboard) == 0 {
		t.Fatal("expected keyboard rows, got 0")
	}

	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				continue
			}
			cbData := *btn.CallbackData
			if len([]byte(cbData)) > 64 {
				t.Errorf("CallbackData %q exceeds 64 bytes limit: len is %d", cbData, len([]byte(cbData)))
			}
		}
	}
}

func TestGetConfigDomainsKeyboard(t *testing.T) {
	grouped := map[string][]ha.DeviceConfigGroup{
		"switch": {
			{
				DeviceID:   "dev_1",
				DeviceName: "Розетка на кухне",
				Domain:     "switch",
				ConfigEntities: []ha.Entity{
					{EntityID: "switch.dev1_backlight", FriendlyName: "Подсветка", State: "on", Category: "config"},
				},
			},
		},
		"light": {
			{
				DeviceID:   "dev_2",
				DeviceName: "Люстра",
				Domain:     "light",
				ConfigEntities: []ha.Entity{
					{EntityID: "number.dev2_timer", FriendlyName: "Таймер", State: "10", Category: "config"},
				},
			},
		},
		"climate": {}, // пустая категория
	}

	kb := GetConfigDomainsKeyboard(grouped)
	if len(kb.InlineKeyboard) == 0 {
		t.Fatal("expected non-empty keyboard")
	}

	var foundSwitch, foundLight, foundClimate bool
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && len([]byte(*btn.CallbackData)) > 64 {
				t.Errorf("CallbackData %q exceeds 64 bytes", *btn.CallbackData)
			}
			if strings.Contains(btn.Text, "Розетки") {
				foundSwitch = true
			}
			if strings.Contains(btn.Text, "Освещение") {
				foundLight = true
			}
			if strings.Contains(btn.Text, "Климат") {
				foundClimate = true
			}
		}
	}

	if !foundSwitch {
		t.Error("expected switch category to be present")
	}
	if !foundLight {
		t.Error("expected light category to be present")
	}
	if foundClimate {
		t.Error("expected empty climate category to be omitted")
	}
}

func TestGetConfigDevicesKeyboard_Pagination(t *testing.T) {
	var devices []ha.DeviceConfigGroup
	for i := 0; i < 25; i++ {
		devices = append(devices, ha.DeviceConfigGroup{
			DeviceID:   fmt.Sprintf("dev_id_%d", i),
			DeviceName: fmt.Sprintf("Устройство %d", i),
			Domain:     "switch",
			ConfigEntities: []ha.Entity{
				{EntityID: fmt.Sprintf("switch.dev_%d_led", i), FriendlyName: "LED", State: "on", Category: "config"},
			},
		})
	}

	kb := GetConfigDevicesKeyboard("switch", devices, 0, 10)
	if len(kb.InlineKeyboard) != 12 { // 10 devices + 1 nav + 1 bottom
		t.Fatalf("expected 12 rows, got %d", len(kb.InlineKeyboard))
	}

	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && len([]byte(*btn.CallbackData)) > 64 {
				t.Errorf("CallbackData %q exceeds 64 bytes limit", *btn.CallbackData)
			}
		}
	}
}

func TestResolveEntityRef(t *testing.T) {
	shortID := "switch.kitchen"
	ref1 := GetEntityRef(shortID)
	if ref1 != shortID {
		t.Errorf("expected short ID %q to remain unchanged, got %q", shortID, ref1)
	}
	if resolved := ResolveEntityRef(ref1); resolved != shortID {
		t.Errorf("expected resolved %q, got %q", shortID, resolved)
	}

	longID := "switch.living_room_super_long_entity_name_that_exceeds_forty_characters_limit"
	ref2 := GetEntityRef(longID)
	if !strings.HasPrefix(ref2, "h_") {
		t.Errorf("expected long ID to be hashed with prefix 'h_', got %q", ref2)
	}
	if len(ref2) > 12 {
		t.Errorf("expected compact hash length <= 12, got %d (%q)", len(ref2), ref2)
	}
	if resolved := ResolveEntityRef(ref2); resolved != longID {
		t.Errorf("expected resolved %q, got %q", longID, resolved)
	}
}


