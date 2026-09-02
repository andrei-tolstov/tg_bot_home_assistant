package bot

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tg_bot_home_assistant/ha"
)

var (
	entityMapMutex sync.RWMutex
	entityHashMap  = make(map[string]string)
)

// GetEntityRef возвращает безопасный идентификатор для callback_data (<= 64 байт).
// Если длина entity_id превышает 40 символов, генерируется короткий хэш.
func GetEntityRef(entityID string) string {
	if len(entityID) <= 40 {
		return entityID
	}
	hash := fmt.Sprintf("h_%08x", crc32.ChecksumIEEE([]byte(entityID)))
	entityMapMutex.Lock()
	entityHashMap[hash] = entityID
	entityMapMutex.Unlock()
	return hash
}

// GetDeviceRef возвращает безопасный идентификатор устройства для callback_data.
func GetDeviceRef(deviceID string) string {
	if len(deviceID) <= 32 {
		return deviceID
	}
	hash := fmt.Sprintf("d_%08x", crc32.ChecksumIEEE([]byte(deviceID)))
	entityMapMutex.Lock()
	entityHashMap[hash] = deviceID
	entityMapMutex.Unlock()
	return hash
}

// ResolveEntityRef восстанавливает исходный ID сущности или устройства по хэшу.
func ResolveEntityRef(ref string) string {
	if strings.HasPrefix(ref, "h_") || strings.HasPrefix(ref, "d_") {
		entityMapMutex.RLock()
		defer entityMapMutex.RUnlock()
		if full, ok := entityHashMap[ref]; ok {
			return full
		}
	}
	return ref
}


// DomainIcons содержит сопоставление доменов Home Assistant с эмодзи-иконками.
var DomainIcons = map[string]string{
	"light":         "💡",
	"switch":        "🔌",
	"fan":           "🌀",
	"humidifier":    "💧",
	"cover":         "🪟",
	"climate":       "❄️",
	"lock":          "🔒",
	"media_player":  "📺",
	"vacuum":        "🧹",
	"water_heater":  "🚿",
	"valve":         "🚰",
	"siren":         "🚨",
	"input_boolean": "🔘",
	"automation":    "⚡",
	"scene":         "🎬",
	"script":        "📜",
	"number":        "🔢",
	"input_number":  "🔢",
	"select":        "📋",
	"input_select":  "📋",
	"button":        "🔘",
	"input_button":  "🔘",
	"time":          "⏱️",
	"text":          "📝",
	"sensor":        "📊",
}

// DomainNames содержит русскоязычные названия доменов.
var DomainNames = map[string]string{
	"light":         "Освещение",
	"switch":        "Розетки и реле",
	"fan":           "Вентиляторы",
	"humidifier":    "Осушители и увлажнители",
	"cover":         "Шторы и жалюзи",
	"climate":       "Климат",
	"lock":          "Замки",
	"media_player":  "Медиаплееры",
	"vacuum":        "Пылесосы",
	"water_heater":  "Водонагреватели",
	"valve":         "Краны и клапаны",
	"siren":         "Сирены",
	"input_boolean": "Переключатели",
	"automation":    "Автоматизации",
	"scene":         "Сцены",
	"script":        "Скрипты",
	"number":        "Числовые настройки",
	"input_number":  "Числовые настройки",
	"select":        "Выбор режима",
	"input_select":  "Выбор режима",
	"button":        "Кнопки",
	"input_button":  "Кнопки",
	"time":          "Таймеры и время",
}

// GetMainKeyboard возвращает постоянную клавиатуру с кнопками внизу экрана.
func GetMainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Статус HA"),
			tgbotapi.NewKeyboardButton("💡 Все устройства"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📁 Устройства по доменам"),
			tgbotapi.NewKeyboardButton("⚙️ Конфигурация устройств"),
		),
	)
	keyboard.ResizeKeyboard = true
	keyboard.InputFieldPlaceholder = "Выберите действие..."
	return keyboard
}


// DefaultPageSize задает количество устройств на одной странице инлайн-клавиатуры.
const DefaultPageSize = 10

// FormatDeviceButton форматирует текст для кнопки устройства с иконкой и текущим состоянием.
func FormatDeviceButton(item ha.Entity) string {
	icon, ok := DomainIcons[item.Domain]
	if !ok {
		icon = "⚙️"
	}

	var statusBadge string
	switch item.State {
	case "on":
		statusBadge = "🟢 ВКЛ"
	case "off":
		statusBadge = "⚪ ВЫКЛ"
	case "open", "unlocked":
		statusBadge = fmt.Sprintf("🟢 %s", strings.ToUpper(item.State))
	case "closed", "locked":
		statusBadge = fmt.Sprintf("⚪ %s", strings.ToUpper(item.State))
	default:
		statusBadge = fmt.Sprintf("[%s]", item.State)
	}

	return fmt.Sprintf("%s %s: %s", icon, item.FriendlyName, statusBadge)
}

// GetDevicesKeyboard возвращает инлайн-клавиатуру со списком всех устройств с поддержкой пагинации.
func GetDevicesKeyboard(devices []ha.Entity, page, pageSize int) tgbotapi.InlineKeyboardMarkup {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	total := len(devices)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages <= 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	if start < total {
		for _, item := range devices[start:end] {
			btnText := FormatDeviceButton(item)
			cbData := fmt.Sprintf("t:%s:%d", GetEntityRef(item.EntityID), page)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))
		}
	}

	if totalPages > 1 {
		var navRow []tgbotapi.InlineKeyboardButton
		if page > 0 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("pa:%d", page-1)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, totalPages), "noop"))
		if page < totalPages-1 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Вперед ➡️", fmt.Sprintf("pa:%d", page+1)))
		}
		rows = append(rows, navRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить список устройств", fmt.Sprintf("r_all:%d", page)),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// GetDomainsKeyboard возвращает инлайн-клавиатуру со списком категорий (доменов) и количеством устройств.
// Использует адаптивную компоновку: длинные названия (> 18 символов) занимают отдельный ряд на всю ширину экрана.
func GetDomainsKeyboard(groupedDevices map[string][]ha.Entity) tgbotapi.InlineKeyboardMarkup {
	domainPriority := map[string]int{
		"light":         1,
		"switch":        2,
		"fan":           3,
		"humidifier":    4,
		"climate":       5,
		"cover":         6,
		"water_heater":  7,
		"valve":         8,
		"siren":         9,
		"lock":          10,
		"media_player":  11,
		"vacuum":        12,
		"input_boolean": 13,
		"automation":    14,
		"scene":         15,
		"script":        16,
	}

	domains := make([]string, 0, len(groupedDevices))
	for d := range groupedDevices {
		domains = append(domains, d)
	}

	sort.Slice(domains, func(i, j int) bool {
		d1, d2 := domains[i], domains[j]
		c1, c2 := len(groupedDevices[d1]), len(groupedDevices[d2])

		// Сначала категории с устройствами (> 0), затем пустые (0)
		if (c1 > 0) != (c2 > 0) {
			return c1 > 0
		}

		p1, ok1 := domainPriority[d1]
		if !ok1 {
			p1 = 20
		}
		p2, ok2 := domainPriority[d2]
		if !ok2 {
			p2 = 20
		}

		if p1 != p2 {
			return p1 < p2
		}

		n1, ok1 := DomainNames[d1]
		if !ok1 {
			n1 = d1
		}
		n2, ok2 := DomainNames[d2]
		if !ok2 {
			n2 = d2
		}

		return n1 < n2
	})

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton

	for _, domain := range domains {
		items := groupedDevices[domain]
		count := len(items)
		if count == 0 {
			continue
		}

		icon, ok := DomainIcons[domain]
		if !ok {
			icon = "📁"
		}
		name, ok := DomainNames[domain]
		if !ok {
			name = capitalize(domain)
		}

		btnText := fmt.Sprintf("%s %s (%d)", icon, name, count)
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("dv:%s", domain))
		textLen := utf8.RuneCountInString(btnText)

		// В Telegram клиентах инлайн-кнопки однострочные (nowrap).
		// Длинные названия (> 18 символов) выделяем в отдельный ряд на всю ширину,
		// чтобы они никогда не обрезались многоточием "...".
		if textLen > 18 {
			if len(currentRow) > 0 {
				rows = append(rows, currentRow)
				currentRow = []tgbotapi.InlineKeyboardButton{}
			}
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
		} else {
			currentRow = append(currentRow, btn)
			if len(currentRow) == 2 {
				rows = append(rows, currentRow)
				currentRow = []tgbotapi.InlineKeyboardButton{}
			}
		}
	}

	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить список категорий", "r_dom"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetDomainDevicesKeyboard возвращает инлайн-клавиатуру устройств конкретного домена с поддержкой пагинации.
func GetDomainDevicesKeyboard(domain string, devices []ha.Entity, page, pageSize int) tgbotapi.InlineKeyboardMarkup {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	total := len(devices)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages <= 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	if start < total {
		for _, item := range devices[start:end] {
			btnText := FormatDeviceButton(item)
			cbData := fmt.Sprintf("dt:%s:%d", GetEntityRef(item.EntityID), page)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))
		}
	}

	if totalPages > 1 {
		var navRow []tgbotapi.InlineKeyboardButton
		if page > 0 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("pd:%s:%d", domain, page-1)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, totalPages), "noop"))
		if page < totalPages-1 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Вперед ➡️", fmt.Sprintf("pd:%s:%d", domain, page+1)))
		}
		rows = append(rows, navRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("dr:%s:%d", domain, page)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 Все категории", "b_dom"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// FormatConfigEntityButton форматирует текст кнопки для конфигурационной сущности с текущим состоянием.
func FormatConfigEntityButton(item ha.Entity) string {
	icon, ok := DomainIcons[item.Domain]
	if !ok {
		icon = "⚙️"
	}

	switch item.Domain {
	case "switch", "input_boolean", "light":
		var statusBadge string
		switch item.State {
		case "on":
			statusBadge = "🟢 ВКЛ"
		case "off":
			statusBadge = "⚪ ВЫКЛ"
		default:
			statusBadge = fmt.Sprintf("[%s]", item.State)
		}
		return fmt.Sprintf("%s %s: %s", icon, item.FriendlyName, statusBadge)

	case "button", "input_button":
		return fmt.Sprintf("🔘 %s (Нажать)", item.FriendlyName)

	case "select", "input_select":
		return fmt.Sprintf("📋 %s: [%s] ❯", item.FriendlyName, item.State)

	case "number", "input_number":
		unit := ""
		if item.Attributes != nil {
			if u, ok := item.Attributes["unit_of_measurement"].(string); ok && u != "" {
				unit = " " + u
			}
		}
		return fmt.Sprintf("⏱️ %s: %s%s", item.FriendlyName, item.State, unit)

	default:
		return fmt.Sprintf("%s %s: %s", icon, item.FriendlyName, item.State)
	}
}

// GetConfigDomainsKeyboard возвращает инлайн-клавиатуру выбора категории (домена) для настройки устройств.
func GetConfigDomainsKeyboard(groupedDevices map[string][]ha.DeviceConfigGroup) tgbotapi.InlineKeyboardMarkup {
	domainPriority := map[string]int{
		"light":         1,
		"switch":        2,
		"fan":           3,
		"humidifier":    4,
		"climate":       5,
		"cover":         6,
		"water_heater":  7,
		"valve":         8,
		"siren":         9,
		"lock":          10,
		"media_player":  11,
		"vacuum":        12,
		"number":        13,
		"select":        14,
		"input_boolean": 15,
		"automation":    16,
		"scene":         17,
		"script":        18,
	}

	domains := make([]string, 0, len(groupedDevices))
	for d := range groupedDevices {
		domains = append(domains, d)
	}

	sort.Slice(domains, func(i, j int) bool {
		d1, d2 := domains[i], domains[j]
		c1, c2 := len(groupedDevices[d1]), len(groupedDevices[d2])

		if (c1 > 0) != (c2 > 0) {
			return c1 > 0
		}

		p1, ok1 := domainPriority[d1]
		if !ok1 {
			p1 = 20
		}
		p2, ok2 := domainPriority[d2]
		if !ok2 {
			p2 = 20
		}

		if p1 != p2 {
			return p1 < p2
		}

		n1, ok1 := DomainNames[d1]
		if !ok1 {
			n1 = d1
		}
		n2, ok2 := DomainNames[d2]
		if !ok2 {
			n2 = d2
		}

		return n1 < n2
	})

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton

	for _, domain := range domains {
		items := groupedDevices[domain]
		count := len(items)
		if count == 0 {
			continue
		}

		icon, ok := DomainIcons[domain]
		if !ok {
			icon = "📁"
		}
		name, ok := DomainNames[domain]
		if !ok {
			name = capitalize(domain)
		}

		btnText := fmt.Sprintf("%s %s (%d)", icon, name, count)
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("cdm:%s", domain))
		textLen := utf8.RuneCountInString(btnText)

		if textLen > 18 {
			if len(currentRow) > 0 {
				rows = append(rows, currentRow)
				currentRow = []tgbotapi.InlineKeyboardButton{}
			}
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
		} else {
			currentRow = append(currentRow, btn)
			if len(currentRow) == 2 {
				rows = append(rows, currentRow)
				currentRow = []tgbotapi.InlineKeyboardButton{}
			}
		}
	}

	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить список настроек", "r_cdm"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetConfigDevicesKeyboard возвращает список устройств конкретного домена, имеющих конфигурацию.
func GetConfigDevicesKeyboard(domain string, devices []ha.DeviceConfigGroup, page, pageSize int) tgbotapi.InlineKeyboardMarkup {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	total := len(devices)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages <= 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	if start < total {
		for _, item := range devices[start:end] {
			icon, ok := DomainIcons[domain]
			if !ok {
				icon = "⚙️"
			}
			btnText := fmt.Sprintf("%s %s (%d настр.)", icon, item.DeviceName, len(item.ConfigEntities))
			cbData := fmt.Sprintf("cds:%s:%s", GetDeviceRef(item.DeviceID), domain)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))
		}
	}

	if totalPages > 1 {
		var navRow []tgbotapi.InlineKeyboardButton
		if page > 0 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("pdc:%s:%d", domain, page-1)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, totalPages), "noop"))
		if page < totalPages-1 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Вперед ➡️", fmt.Sprintf("pdc:%s:%d", domain, page+1)))
		}
		rows = append(rows, navRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("rdc:%s:%d", domain, page)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 Все категории", "b_cdm"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetDeviceSettingsKeyboard возвращает клавиатуру со списком параметров и кнопок управления для устройства.
func GetDeviceSettingsKeyboard(device ha.DeviceConfigGroup, domain string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, ent := range device.ConfigEntities {
		entRef := GetEntityRef(ent.EntityID)
		switch ent.Domain {
		case "switch", "input_boolean", "light":
			btnText := FormatConfigEntityButton(ent)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("cft:%s", entRef)),
			))

		case "button", "input_button":
			btnText := FormatConfigEntityButton(ent)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("cfp:%s", entRef)),
			))

		case "select", "input_select":
			btnText := FormatConfigEntityButton(ent)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("cfs:%s", entRef)),
			))

		case "number", "input_number":
			btnText := FormatConfigEntityButton(ent)
			// Верхняя строка с текущим значением
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, "noop"),
			))
			// Нижняя строка с быстрыми кнопками регулировки: [-5] [-1] [+1] [+5]
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("➖ 5", fmt.Sprintf("cfn:%s:-5", entRef)),
				tgbotapi.NewInlineKeyboardButtonData("➖ 1", fmt.Sprintf("cfn:%s:-1", entRef)),
				tgbotapi.NewInlineKeyboardButtonData("➕ 1", fmt.Sprintf("cfn:%s:1", entRef)),
				tgbotapi.NewInlineKeyboardButtonData("➕ 5", fmt.Sprintf("cfn:%s:5", entRef)),
			))

		default:
			btnText := FormatConfigEntityButton(ent)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, "noop"),
			))
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("r_cds:%s:%s", GetDeviceRef(device.DeviceID), domain)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 К устройствам", fmt.Sprintf("b_cdev:%s", domain)),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}


