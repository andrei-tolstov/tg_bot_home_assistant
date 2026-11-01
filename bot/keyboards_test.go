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
		{
			name: "temperature sensor with unit",
			entity: ha.Entity{
				EntityID:     "sensor.kukhnia_ik_pulk_i_datchik_temperature",
				Domain:       "sensor",
				FriendlyName: "Кухня ИК Пульт и датчик Temperature",
				State:        "24.5",
				Attributes: map[string]interface{}{
					"unit_of_measurement": "°C",
					"device_class":        "temperature",
				},
			},
			contains: []string{"🌡", "Кухня ИК Пульт и датчик Temperature", "24.5 °C"},
		},
		{
			name: "humidity sensor with unit",
			entity: ha.Entity{
				EntityID:     "sensor.humidity",
				Domain:       "sensor",
				FriendlyName: "Влажность воздуха",
				State:        "45",
				Attributes: map[string]interface{}{
					"unit_of_measurement": "%",
					"device_class":        "humidity",
				},
			},
			contains: []string{"💧", "Влажность воздуха", "45 %"},
		},
		{
			name: "binary sensor motion on",
			entity: ha.Entity{
				EntityID:     "binary_sensor.motion_sensor",
				Domain:       "binary_sensor",
				FriendlyName: "Датчик движения",
				State:        "on",
				Attributes: map[string]interface{}{
					"device_class": "motion",
				},
			},
			contains: []string{"🚶", "Датчик движения", "🔴 ВКЛ"},
		},
		{
			name: "binary sensor door off",
			entity: ha.Entity{
				EntityID:     "binary_sensor.front_door",
				Domain:       "binary_sensor",
				FriendlyName: "Входная дверь",
				State:        "off",
				Attributes: map[string]interface{}{
					"device_class": "door",
				},
			},
			contains: []string{"🚪", "Входная дверь", "⚪ ВЫКЛ"},
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

func TestWarmupEntityRefs_And_HasEntityRef(t *testing.T) {
	longEntity := "light.corridor_main_smart_dimmable_light_switch_controller"
	longDevice := "dev_very_long_physical_device_identifier_string_xyz"

	// До прогрева HasEntityRef для неизвестных хэшей возвращает false
	fakeHash := "h_1234abcd"
	if HasEntityRef(fakeHash) {
		t.Errorf("expected false for unknown hash %s", fakeHash)
	}

	// Прогреваем
	WarmupEntityRefs([]ha.Entity{
		{
			EntityID: longEntity,
			DeviceID: longDevice,
		},
	})

	refEnt := GetEntityRef(longEntity)
	if !HasEntityRef(refEnt) {
		t.Errorf("expected HasEntityRef(%s) to be true after warmup", refEnt)
	}
	if ResolveEntityRef(refEnt) != longEntity {
		t.Errorf("expected ResolveEntityRef to restore %s", longEntity)
	}

	refDev := GetDeviceRef(longDevice)
	if !HasEntityRef(refDev) {
		t.Errorf("expected HasEntityRef(%s) to be true after warmup", refDev)
	}
	if ResolveEntityRef(refDev) != longDevice {
		t.Errorf("expected ResolveEntityRef to restore %s", longDevice)
	}
}

func TestGetDeviceSettingsKeyboard_DynamicStep(t *testing.T) {
	dev := ha.DeviceConfigGroup{
		DeviceID:   "dev_climate_thermostat",
		DeviceName: "Термостат",
		Domain:     "climate",
		ConfigEntities: []ha.Entity{
			{
				EntityID:     "number.thermostat_temperature_step",
				Domain:       "number",
				FriendlyName: "Температура",
				State:        "21.5",
				Category:     "config",
				Attributes: map[string]interface{}{
					"step": 0.5,
				},
			},
		},
	}

	kb := GetDeviceSettingsKeyboard(dev, "climate", 1)

	var foundMinusStep, foundMinusLarge, foundPlusStep, foundPlusLarge bool
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.Text == "➖ 2.5" {
				foundMinusLarge = true
			}
			if btn.Text == "➖ 0.5" {
				foundMinusStep = true
			}
			if btn.Text == "➕ 0.5" {
				foundPlusStep = true
			}
			if btn.Text == "➕ 2.5" {
				foundPlusLarge = true
			}
			if btn.Text == "🔙 К устройствам" && *btn.CallbackData != "b_cdev:climate:1" {
				t.Errorf("expected back button to preserve page 1, got %s", *btn.CallbackData)
			}
		}
	}

	if !foundMinusLarge || !foundMinusStep || !foundPlusStep || !foundPlusLarge {
		t.Errorf("expected dynamic step buttons [➖ 2.5, ➖ 0.5, ➕ 0.5, ➕ 2.5] not found")
	}
}

func TestGetMainKeyboard_Buttons(t *testing.T) {
	kb := GetMainKeyboard()

	var allButtons []string
	for _, row := range kb.Keyboard {
		for _, btn := range row {
			allButtons = append(allButtons, btn.Text)
		}
	}

	for _, text := range allButtons {
		if strings.Contains(text, "Все устройства") {
			t.Errorf("unexpected 'Все устройства' button in main keyboard")
		}
		if strings.Contains(text, "Конфигурация устройств") {
			t.Errorf("unexpected 'Конфигурация устройств' button in main keyboard")
		}
	}

	foundRooms := false
	foundDomains := false
	foundStatus := false
	for _, text := range allButtons {
		if strings.Contains(text, "Устройства по комнатам") {
			foundRooms = true
		}
		if strings.Contains(text, "Устройства по доменам") {
			foundDomains = true
		}
		if strings.Contains(text, "Статус HA") {
			foundStatus = true
		}
	}

	if !foundRooms {
		t.Errorf("expected 'Устройства по комнатам' button in main keyboard")
	}
	if !foundDomains {
		t.Errorf("expected 'Устройства по доменам' button in main keyboard")
	}
	if !foundStatus {
		t.Errorf("expected 'Статус HA' button in main keyboard")
	}
}

func TestGetAreasKeyboard_AdaptiveLayoutAndEmpty(t *testing.T) {
	grouped := map[string][]ha.Entity{
		"kukhnia": {
			{EntityID: "light.kukhnia", Domain: "light", FriendlyName: "Свет"},
		},
		"spalnia": {
			{EntityID: "light.spalnia", Domain: "light", FriendlyName: "Свет"},
		},
		"pustaia_komnata": {}, // 0 устройств, должна скрываться
		"no_area": {
			{EntityID: "automation.test", Domain: "automation", FriendlyName: "Автоматизация"},
		},
	}
	areaNames := map[string]string{
		"kukhnia":         "Кухня",
		"spalnia":         "Очень длинное название комнаты для теста",
		"pustaia_komnata": "Пустая комната",
		"no_area":         "Без комнаты",
	}

	kb := GetAreasKeyboard(grouped, areaNames)

	var foundEmpty bool
	var buttons []string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			buttons = append(buttons, btn.Text)
			if strings.Contains(btn.Text, "Пустая комната") {
				foundEmpty = true
			}
			if btn.CallbackData != nil && len([]byte(*btn.CallbackData)) > 64 {
				t.Errorf("CallbackData %q exceeds 64 bytes", *btn.CallbackData)
			}
		}
	}

	if foundEmpty {
		t.Errorf("empty area should be hidden from areas keyboard")
	}

	// no_area должен быть последней комнатой
	if len(buttons) >= 2 {
		lastAreaBtn := buttons[len(buttons)-2] // перед кнопкой обновления
		if !strings.Contains(lastAreaBtn, "Без комнаты") {
			t.Errorf("expected 'Без комнаты' to be the last room, got %s", lastAreaBtn)
		}
	}
}

func TestGetAreaDevicesKeyboard_PaginationAndSelection(t *testing.T) {
	var devices []ha.Entity
	for i := 0; i < 15; i++ {
		devices = append(devices, ha.Entity{
			EntityID:     fmt.Sprintf("light.lamp_%d", i),
			Domain:       "light",
			FriendlyName: fmt.Sprintf("Лампа %d", i),
			State:        "on",
		})
	}

	kb := GetAreaDevicesKeyboard("kukhnia", "Кухня", devices, 0, 10)

	// Проверяем формат callback_data для первого устройства
	firstBtn := kb.InlineKeyboard[0][0]
	if !strings.HasPrefix(*firstBtn.CallbackData, "dv_sel:r:kukhnia:") {
		t.Errorf("expected dv_sel:r:kukhnia:... callback prefix, got %s", *firstBtn.CallbackData)
	}

	// Проверяем ограничение 64 байт
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && len([]byte(*btn.CallbackData)) > 64 {
				t.Errorf("CallbackData %q exceeds 64 bytes", *btn.CallbackData)
			}
		}
	}
}

func TestGetDomainDevicesKeyboard_SelectionCallback(t *testing.T) {
	devices := []ha.Entity{
		{
			EntityID:     "switch.relay_1",
			Domain:       "switch",
			FriendlyName: "Реле 1",
			State:        "off",
		},
	}

	kb := GetDomainDevicesKeyboard("switch", devices, 0, 10)
	firstBtn := kb.InlineKeyboard[0][0]
	if !strings.HasPrefix(*firstBtn.CallbackData, "dv_sel:d:switch:") {
		t.Errorf("expected dv_sel:d:switch:... callback prefix, got %s", *firstBtn.CallbackData)
	}
}

func TestGetDeviceMenuKeyboard_ThreeButtons(t *testing.T) {
	entity := ha.Entity{
		EntityID:     "humidifier.kukhnia_osushitel",
		Domain:       "humidifier",
		FriendlyName: "Осушитель",
		State:        "on",
	}

	// 1. С датчиками (hasSensors = true)
	kbWithSensors := GetDeviceMenuKeyboard(entity, true, "r", "kukhnia", 0)

	var textsWith []string
	for _, row := range kbWithSensors.InlineKeyboard {
		for _, btn := range row {
			textsWith = append(textsWith, btn.Text)
			if btn.CallbackData != nil && len([]byte(*btn.CallbackData)) > 64 {
				t.Errorf("CallbackData %q exceeds 64 bytes", *btn.CallbackData)
			}
		}
	}

	hasToggle := false
	hasConfig := false
	hasSensorsBtn := false
	for _, txt := range textsWith {
		if strings.Contains(txt, "Выключить") || strings.Contains(txt, "Вкл / Выкл") {
			hasToggle = true
		}
		if strings.Contains(txt, "Конфигурация") {
			hasConfig = true
		}
		if strings.Contains(txt, "Показания датчиков") {
			hasSensorsBtn = true
		}
	}

	if !hasToggle || !hasConfig || !hasSensorsBtn {
		t.Errorf("expected 3 buttons (Toggle, Config, Sensors), got: %v", textsWith)
	}

	// 2. Без датчиков (hasSensors = false)
	kbNoSensors := GetDeviceMenuKeyboard(entity, false, "r", "kukhnia", 0)
	for _, row := range kbNoSensors.InlineKeyboard {
		for _, btn := range row {
			if strings.Contains(btn.Text, "Показания датчиков") {
				t.Errorf("unexpected Sensors button when hasSensors is false")
			}
		}
	}

	// 3. Для устройства-датчика (sensor): кнопка Вкл/Выкл должна быть СКРЫТА
	sensorEntity := ha.Entity{
		EntityID:     "sensor.kukhnia_ik_pulk_i_datchik_temperature",
		Domain:       "sensor",
		FriendlyName: "Кухня ИК Пульт и датчик Temperature",
		State:        "24.5",
	}
	kbSensorDevice := GetDeviceMenuKeyboard(sensorEntity, true, "r", "kukhnia", 0)
	for _, row := range kbSensorDevice.InlineKeyboard {
		for _, btn := range row {
			if strings.Contains(btn.Text, "Выключить") || strings.Contains(btn.Text, "Включить") || strings.Contains(btn.Text, "Вкл / Выкл") {
				t.Errorf("unexpected Toggle button for sensor entity: %s", btn.Text)
			}
		}
	}

	// Проверяем наличие кнопок Конфигурация и Показания датчиков для sensorEntity
	hasSensorConfig := false
	hasSensorReadings := false
	for _, row := range kbSensorDevice.InlineKeyboard {
		for _, btn := range row {
			if strings.Contains(btn.Text, "Конфигурация") {
				hasSensorConfig = true
			}
			if strings.Contains(btn.Text, "Показания датчиков") {
				hasSensorReadings = true
			}
		}
	}
	if !hasSensorConfig {
		t.Errorf("expected Configuration button for sensor entity")
	}
	if !hasSensorReadings {
		t.Errorf("expected Sensors button for sensor entity with hasSensors=true")
	}
}

func TestGetSensorIcon(t *testing.T) {
	tests := []struct {
		name     string
		entity   ha.Entity
		expected string
	}{
		{
			name: "temperature by unit",
			entity: ha.Entity{
				Domain:     "sensor",
				Attributes: map[string]interface{}{"unit_of_measurement": "°C"},
			},
			expected: "🌡",
		},
		{
			name: "humidity by device_class",
			entity: ha.Entity{
				Domain:     "sensor",
				Attributes: map[string]interface{}{"device_class": "humidity", "unit_of_measurement": "%"},
			},
			expected: "💧",
		},
		{
			name: "battery by name and unit",
			entity: ha.Entity{
				Domain:       "sensor",
				FriendlyName: "Заряд батареи",
				Attributes:   map[string]interface{}{"unit_of_measurement": "%"},
			},
			expected: "🔋",
		},
		{
			name: "power by unit",
			entity: ha.Entity{
				Domain:     "sensor",
				Attributes: map[string]interface{}{"unit_of_measurement": "W"},
			},
			expected: "⚡",
		},
		{
			name: "leak by device_class",
			entity: ha.Entity{
				Domain:     "binary_sensor",
				Attributes: map[string]interface{}{"device_class": "moisture"},
			},
			expected: "🌊",
		},
		{
			name: "door by device_class",
			entity: ha.Entity{
				Domain:     "binary_sensor",
				Attributes: map[string]interface{}{"device_class": "door"},
			},
			expected: "🚪",
		},
		{
			name: "motion by device_class",
			entity: ha.Entity{
				Domain:     "binary_sensor",
				Attributes: map[string]interface{}{"device_class": "motion"},
			},
			expected: "🚶",
		},
		{
			name: "smoke by name",
			entity: ha.Entity{
				Domain:       "binary_sensor",
				FriendlyName: "Датчик дыма в коридоре",
			},
			expected: "🚨",
		},
		{
			name: "pressure hPa does not trigger current icon",
			entity: ha.Entity{
				Domain:       "sensor",
				FriendlyName: "Атмосферное давление",
				Attributes:   map[string]interface{}{"unit_of_measurement": "hPa"},
			},
			expected: "⏲",
		},
		{
			name: "word with tok substring does not trigger current icon",
			entity: ha.Entity{
				Domain:       "sensor",
				FriendlyName: "Backup Последнее автоматического копирования",
			},
			expected: "📊",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSensorIcon(tt.entity)
			if got != tt.expected {
				t.Errorf("GetSensorIcon() = %s, expected %s", got, tt.expected)
			}
		})
	}
}

func TestFormatSensorDisplay(t *testing.T) {
	tempSensor := ha.Entity{
		EntityID:     "sensor.living_room_temperature",
		Domain:       "sensor",
		FriendlyName: "Температура",
		State:        "23.5",
		Attributes: map[string]interface{}{
			"unit_of_measurement": "°C",
		},
	}
	outTemp := FormatSensorDisplay(tempSensor)
	if !strings.Contains(outTemp, "🌡") || !strings.Contains(outTemp, "23.5 °C") {
		t.Errorf("unexpected temperature formatting: %s", outTemp)
	}

	binSensorOn := ha.Entity{
		EntityID:     "binary_sensor.leak",
		Domain:       "binary_sensor",
		FriendlyName: "Датчик протечки",
		State:        "on",
	}
	outBin := FormatSensorDisplay(binSensorOn)
	if !strings.Contains(outBin, "🔴 ДА") {
		t.Errorf("unexpected binary sensor formatting for on: %s", outBin)
	}

	binSensorOff := ha.Entity{
		EntityID:     "binary_sensor.leak",
		Domain:       "binary_sensor",
		FriendlyName: "Датчик протечки",
		State:        "off",
	}
	outBinOff := FormatSensorDisplay(binSensorOff)
	if !strings.Contains(outBinOff, "⚪ Нет") {
		t.Errorf("unexpected binary sensor formatting for off: %s", outBinOff)
	}
}

func TestFormatSensorValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"26.3000011444092", "26.3"},
		{"24.5", "24.5"},
		{"24.55", "24.55"},
		{"100", "100"},
		{"idle", "idle"},
		{"0.05", "0.05"},
		{"24.00001", "24"},
	}

	for _, tt := range tests {
		got := FormatSensorValue(tt.input)
		if got != tt.expected {
			t.Errorf("FormatSensorValue(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestGetDeviceConfigKeyboard_CallbackDataLimits(t *testing.T) {
	devGroup := ha.DeviceConfigGroup{
		DeviceID:   "dev_very_long_device_id_string_1234567890",
		DeviceName: "Терморегулятор теплого пола",
		Domain:     "climate",
		ConfigEntities: []ha.Entity{
			{
				EntityID:     "switch.living_room_thermostat_child_lock_protection",
				Domain:       "switch",
				FriendlyName: "Защита от детей",
				State:        "on",
			},
			{
				EntityID:     "number.living_room_thermostat_temperature_calibration_offset",
				Domain:       "number",
				FriendlyName: "Калибровка датчика температуры",
				State:        "-1.5",
				Attributes: map[string]interface{}{
					"step": 0.5,
				},
			},
			{
				EntityID:     "select.living_room_thermostat_sensor_working_mode",
				Domain:       "select",
				FriendlyName: "Режим датчика",
				State:        "floor_only",
			},
			{
				EntityID:     "button.living_room_thermostat_factory_reset_action",
				Domain:       "button",
				FriendlyName: "Сброс настроек",
			},
		},
	}

	longAreaID := "komnata_otdykha_na_vtorom_etazhe_bolshaia"
	longPrimaryEntity := "climate.living_room_floor_heating_thermostat_main"

	kb := GetDeviceConfigKeyboard(devGroup, "r", longAreaID, GetEntityRef(longPrimaryEntity), 12)
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				continue
			}
			cbData := *btn.CallbackData
			if len([]byte(cbData)) > 64 {
				t.Errorf("GetDeviceConfigKeyboard CallbackData %q exceeds 64 bytes limit: length is %d", cbData, len([]byte(cbData)))
			}
		}
	}
}


