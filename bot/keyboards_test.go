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
