package bot

import (
	"fmt"
	"hash/crc32"
	"html"
	"sort"
	"strconv"
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
// Если длина entity_id превышает 28 символов, генерируется короткий хэш.
func GetEntityRef(entityID string) string {
	if len(entityID) <= 28 {
		return entityID
	}
	hash := fmt.Sprintf("h_%08x", crc32.ChecksumIEEE([]byte(entityID)))
	entityMapMutex.Lock()
	entityHashMap[hash] = entityID
	entityMapMutex.Unlock()
	return hash
}

// GetCompactEntityRef возвращает гарантированно компактный идентификатор (<= 14 байт)
// для составных callback_data, содержащих несколько идентификаторов одновременно.
func GetCompactEntityRef(entityID string) string {
	if strings.HasPrefix(entityID, "h_") || len(entityID) <= 14 {
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
	if len(deviceID) <= 24 {
		return deviceID
	}
	hash := fmt.Sprintf("d_%08x", crc32.ChecksumIEEE([]byte(deviceID)))
	entityMapMutex.Lock()
	entityHashMap[hash] = deviceID
	entityMapMutex.Unlock()
	return hash
}

// GetAreaRef возвращает безопасный идентификатор комнаты для callback_data (<= 16 байт).
func GetAreaRef(areaID string) string {
	if strings.HasPrefix(areaID, "a_") || len(areaID) <= 16 {
		return areaID
	}
	hash := fmt.Sprintf("a_%08x", crc32.ChecksumIEEE([]byte(areaID)))
	entityMapMutex.Lock()
	entityHashMap[hash] = areaID
	entityMapMutex.Unlock()
	return hash
}

// ResolveEntityRef восстанавливает исходный ID сущности, устройства или комнаты по хэшу.
func ResolveEntityRef(ref string) string {
	if strings.HasPrefix(ref, "h_") || strings.HasPrefix(ref, "d_") || strings.HasPrefix(ref, "a_") {
		entityMapMutex.RLock()
		defer entityMapMutex.RUnlock()
		if full, ok := entityHashMap[ref]; ok {
			return full
		}
	}
	return ref
}

// HasEntityRef проверяет, известен ли хэш в таблице соответствия.
func HasEntityRef(ref string) bool {
	if !strings.HasPrefix(ref, "h_") && !strings.HasPrefix(ref, "d_") && !strings.HasPrefix(ref, "a_") {
		return true
	}
	entityMapMutex.RLock()
	defer entityMapMutex.RUnlock()
	_, ok := entityHashMap[ref]
	return ok
}

// WarmupEntityRefs заполняет таблицу хэшей для всех идентификаторов сущностей, устройств и комнат.
func WarmupEntityRefs(entities []ha.Entity) {
	entityMapMutex.Lock()
	defer entityMapMutex.Unlock()
	for _, ent := range entities {
		if ent.EntityID != "" {
			hash := fmt.Sprintf("h_%08x", crc32.ChecksumIEEE([]byte(ent.EntityID)))
			entityHashMap[hash] = ent.EntityID
		}
		if ent.DeviceID != "" {
			hash := fmt.Sprintf("d_%08x", crc32.ChecksumIEEE([]byte(ent.DeviceID)))
			entityHashMap[hash] = ent.DeviceID
		}
		if ent.AreaID != "" {
			hash := fmt.Sprintf("a_%08x", crc32.ChecksumIEEE([]byte(ent.AreaID)))
			entityHashMap[hash] = ent.AreaID
		}
	}
}

// paginate вычисляет параметры пагинации: totalPages, pageClamped, start, end.
func paginate(total, page, pageSize int) (totalPages, pageClamped, start, end int) {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	totalPages = (total + pageSize - 1) / pageSize
	if totalPages <= 0 {
		totalPages = 1
	}
	pageClamped = page
	if pageClamped < 0 {
		pageClamped = 0
	}
	if pageClamped >= totalPages {
		pageClamped = totalPages - 1
	}
	start = pageClamped * pageSize
	end = start + pageSize
	if end > total {
		end = total
	}
	return totalPages, pageClamped, start, end
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
	"binary_sensor": "🔔",
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
	"sensor":        "Датчики",
	"binary_sensor": "Бинарные датчики",
}

// GetMainKeyboard возвращает постоянную клавиатуру с кнопками внизу экрана.
func GetMainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Статус HA"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📁 Устройства по доменам"),
			tgbotapi.NewKeyboardButton("🏠 Устройства по комнатам"),
		),
	)
	keyboard.ResizeKeyboard = true
	keyboard.InputFieldPlaceholder = "Выберите действие..."
	return keyboard
}


// DefaultPageSize задает количество устройств на одной странице инлайн-клавиатуры.
const DefaultPageSize = 10

// FormatSensorValue аккуратно форматирует числовое значение датчика, убирая длинные хвосты IEEE-754 float.
func FormatSensorValue(raw string) string {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}
	parts := strings.Split(raw, ".")
	if len(parts) == 2 && len(parts[1]) > 2 {
		formatted := fmt.Sprintf("%.2f", f)
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
		return formatted
	}
	return raw
}

// FormatDeviceButton форматирует текст для кнопки устройства с иконкой и текущим состоянием.
func FormatDeviceButton(item ha.Entity) string {
	icon, ok := DomainIcons[item.Domain]
	if !ok {
		icon = "⚙️"
	}

	var statusBadge string
	if item.Domain == "sensor" {
		sensorIcon := GetSensorIcon(item)
		if sensorIcon != "" {
			icon = sensorIcon
		}
		unit := ""
		if item.Attributes != nil {
			if u, ok := item.Attributes["unit_of_measurement"].(string); ok && u != "" {
				unit = " " + u
			}
		}
		statusBadge = FormatSensorValue(item.State) + unit
	} else if item.Domain == "binary_sensor" {
		sensorIcon := GetSensorIcon(item)
		if sensorIcon != "" {
			icon = sensorIcon
		}
		valLower := strings.ToLower(item.State)
		if valLower == "on" {
			statusBadge = "🔴 ВКЛ"
		} else if valLower == "off" {
			statusBadge = "⚪ ВЫКЛ"
		} else {
			statusBadge = fmt.Sprintf("[%s]", item.State)
		}
	} else {
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
	}

	return fmt.Sprintf("%s %s: %s", icon, item.FriendlyName, statusBadge)
}

// GetDevicesKeyboard возвращает инлайн-клавиатуру со списком всех устройств с поддержкой пагинации.
func GetDevicesKeyboard(devices []ha.Entity, page, pageSize int) tgbotapi.InlineKeyboardMarkup {
	totalPages, pageClamped, start, end := paginate(len(devices), page, pageSize)

	var rows [][]tgbotapi.InlineKeyboardButton
	if start < len(devices) {
		for _, item := range devices[start:end] {
			btnText := FormatDeviceButton(item)
			cbData := fmt.Sprintf("t:%s:%d", GetEntityRef(item.EntityID), pageClamped)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))
		}
	}

	if totalPages > 1 {
		var navRow []tgbotapi.InlineKeyboardButton
		if pageClamped > 0 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("pa:%d", pageClamped-1)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", pageClamped+1, totalPages), "noop"))
		if pageClamped < totalPages-1 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Вперед ➡️", fmt.Sprintf("pa:%d", pageClamped+1)))
		}
		rows = append(rows, navRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить список устройств", fmt.Sprintf("r_all:%d", pageClamped)),
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

		p1 := ha.GetDomainPriority(d1)
		p2 := ha.GetDomainPriority(d2)

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
// Нажатие на устройство открывает контекстное меню управления этим устройством.
func GetDomainDevicesKeyboard(domain string, devices []ha.Entity, page, pageSize int) tgbotapi.InlineKeyboardMarkup {
	totalPages, pageClamped, start, end := paginate(len(devices), page, pageSize)

	var rows [][]tgbotapi.InlineKeyboardButton
	if start < len(devices) {
		for _, item := range devices[start:end] {
			btnText := FormatDeviceButton(item)
			cbData := fmt.Sprintf("dv_sel:d:%s:%s:%d", domain, GetEntityRef(item.EntityID), pageClamped)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))
		}
	}

	if totalPages > 1 {
		var navRow []tgbotapi.InlineKeyboardButton
		if pageClamped > 0 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("pd:%s:%d", domain, pageClamped-1)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", pageClamped+1, totalPages), "noop"))
		if pageClamped < totalPages-1 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Вперед ➡️", fmt.Sprintf("pd:%s:%d", domain, pageClamped+1)))
		}
		rows = append(rows, navRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("dr:%s:%d", domain, pageClamped)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 Все категории", "b_dom"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetAreaIcon возвращает эмодзи-иконку для комнаты по ее названию или идентификатору.
func GetAreaIcon(areaID, areaName string) string {
	combined := strings.ToLower(areaID + " " + areaName)
	switch {
	case strings.Contains(combined, "кухн") || strings.Contains(combined, "kukh") || strings.Contains(combined, "kitchen"):
		return "🍳"
	case strings.Contains(combined, "спальн") || strings.Contains(combined, "spaln") || strings.Contains(combined, "bed"):
		return "🛏"
	case strings.Contains(combined, "гостин") || strings.Contains(combined, "gostin") || strings.Contains(combined, "living"):
		return "🛋"
	case strings.Contains(combined, "детск") || strings.Contains(combined, "child") || strings.Contains(combined, "kids"):
		return "🧸"
	case strings.Contains(combined, "ванн") || strings.Contains(combined, "санузел") || strings.Contains(combined, "vann") || strings.Contains(combined, "bath"):
		return "🛁"
	case strings.Contains(combined, "коридор") || strings.Contains(combined, "прихож") || strings.Contains(combined, "koridor") || strings.Contains(combined, "hall"):
		return "🚪"
	case strings.Contains(combined, "кабинет") || strings.Contains(combined, "офис") || strings.Contains(combined, "office") || strings.Contains(combined, "work"):
		return "🖥"
	case strings.Contains(combined, "балкон") || strings.Contains(combined, "лоджи") || strings.Contains(combined, "balcon"):
		return "🪴"
	case strings.Contains(combined, "без комнаты") || strings.Contains(combined, "no_area"):
		return "📦"
	default:
		return "🏠"
	}
}

// GetAreasKeyboard возвращает инлайн-клавиатуру со списком комнат и количеством устройств.
// Применяется адаптивная сетка: длинные названия (> 18 символов) занимают отдельный ряд.
func GetAreasKeyboard(grouped map[string][]ha.Entity, areaNames map[string]string) tgbotapi.InlineKeyboardMarkup {
	areaIDs := make([]string, 0, len(grouped))
	for aID := range grouped {
		if len(grouped[aID]) > 0 {
			areaIDs = append(areaIDs, aID)
		}
	}

	sort.Slice(areaIDs, func(i, j int) bool {
		id1, id2 := areaIDs[i], areaIDs[j]
		if (id1 == "no_area") != (id2 == "no_area") {
			return id2 == "no_area"
		}
		name1 := areaNames[id1]
		if name1 == "" {
			name1 = id1
		}
		name2 := areaNames[id2]
		if name2 == "" {
			name2 = id2
		}
		return strings.ToLower(name1) < strings.ToLower(name2)
	})

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton

	for _, aID := range areaIDs {
		name := areaNames[aID]
		if name == "" {
			name = aID
		}
		count := len(grouped[aID])
		icon := GetAreaIcon(aID, name)

		btnText := fmt.Sprintf("%s %s (%d)", icon, name, count)
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("rm:%s", GetAreaRef(aID)))
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
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить список комнат", "r_rms"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetAreaDevicesKeyboard возвращает инлайн-клавиатуру устройств конкретной комнаты с поддержкой пагинации.
// Нажатие на устройство открывает контекстное меню управления этим устройством.
func GetAreaDevicesKeyboard(areaID, areaName string, devices []ha.Entity, page, pageSize int) tgbotapi.InlineKeyboardMarkup {
	totalPages, pageClamped, start, end := paginate(len(devices), page, pageSize)
	areaRef := GetAreaRef(areaID)

	var rows [][]tgbotapi.InlineKeyboardButton
	if start < len(devices) {
		for _, item := range devices[start:end] {
			btnText := FormatDeviceButton(item)
			cbData := fmt.Sprintf("dv_sel:r:%s:%s:%d", areaRef, GetEntityRef(item.EntityID), pageClamped)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))
		}
	}

	if totalPages > 1 {
		var navRow []tgbotapi.InlineKeyboardButton
		if pageClamped > 0 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("prm:%s:%d", areaRef, pageClamped-1)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", pageClamped+1, totalPages), "noop"))
		if pageClamped < totalPages-1 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Вперед ➡️", fmt.Sprintf("prm:%s:%d", areaRef, pageClamped+1)))
		}
		rows = append(rows, navRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("r_rm:%s:%d", areaRef, pageClamped)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 Все комнаты", "b_rms"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetDeviceMenuKeyboard формирует контекстное меню управления выбранным устройством с 3 кнопками:
// 1. Вкл\Выкл (только если устройство управляемое: ha.IsControllableDomain(entity.Domain))
// 2. Конфигурация
// 3. Показания датчиков (только если hasSensors == true)
// а также навигационными кнопками «Обновить» и «Назад к списку».
func GetDeviceMenuKeyboard(entity ha.Entity, hasSensors bool, srcType, srcID string, page int) tgbotapi.InlineKeyboardMarkup {
	entRef := GetEntityRef(entity.EntityID)
	srcRef := GetAreaRef(srcID)

	var rows [][]tgbotapi.InlineKeyboardButton

	// 1. Кнопка Вкл\Выкл (только для управляемых устройств)
	if ha.IsControllableDomain(entity.Domain) {
		toggleText := "⚡ Вкл / Выкл"
		switch entity.State {
		case "on":
			toggleText = "⚪ Выключить (ВЫКЛ)"
		case "off":
			toggleText = "🟢 Включить (ВКЛ)"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleText, fmt.Sprintf("d_tog:%s:%s:%s:%d", srcType, srcRef, entRef, page)),
		))
	}

	// 2. Кнопка Конфигурация
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⚙️ Конфигурация", fmt.Sprintf("d_cfg:%s:%s:%s:%d", srcType, srcRef, entRef, page)),
	))

	// 3. Кнопка Показания датчиков (отображается только если у устройства есть хотя бы один датчик)
	if hasSensors {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Показания датчиков", fmt.Sprintf("d_sen:%s:%s:%s:%d", srcType, srcRef, entRef, page)),
		))
	}

	// 4. Навигация
	backText := "🔙 Назад к списку"
	if srcType == "r" {
		backText = "🔙 К списку комнаты"
	} else if srcType == "d" {
		backText = "🔙 К списку категории"
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("d_ref:%s:%s:%s:%d", srcType, srcRef, entRef, page)),
		tgbotapi.NewInlineKeyboardButtonData(backText, fmt.Sprintf("d_lst:%s:%s:%d", srcType, srcRef, page)),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetDeviceSensorsKeyboard возвращает клавиатуру для экрана показаний датчиков устройства.
func GetDeviceSensorsKeyboard(srcType, srcID, entityRef string, page int) tgbotapi.InlineKeyboardMarkup {
	srcRef := GetAreaRef(srcID)
	entRef := GetEntityRef(ResolveEntityRef(entityRef))
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить показания", fmt.Sprintf("d_sen_r:%s:%s:%s:%d", srcType, srcRef, entRef, page)),
			tgbotapi.NewInlineKeyboardButtonData("🔙 К устройству", fmt.Sprintf("d_bak:%s:%s:%s:%d", srcType, srcRef, entRef, page)),
		),
	)
}

// GetDeviceConfigKeyboard возвращает клавиатуру параметров устройства с возвратом в меню устройства.
func GetDeviceConfigKeyboard(device ha.DeviceConfigGroup, srcType, srcID, entityRef string, page int) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	srcRef := GetAreaRef(srcID)
	compactDevRef := GetCompactEntityRef(ResolveEntityRef(entityRef))

	for _, ent := range device.ConfigEntities {
		entItemRef := GetCompactEntityRef(ent.EntityID)
		switch ent.Domain {
		case "switch", "input_boolean", "light":
			btnText := FormatConfigEntityButton(ent)
			cbData := fmt.Sprintf("cft:%s:%s:%s:%s:%d", entItemRef, srcType, srcRef, compactDevRef, page)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))

		case "button", "input_button":
			btnText := FormatConfigEntityButton(ent)
			cbData := fmt.Sprintf("cfp:%s:%s:%s:%s:%d", entItemRef, srcType, srcRef, compactDevRef, page)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))

		case "select", "input_select":
			btnText := FormatConfigEntityButton(ent)
			cbData := fmt.Sprintf("cfs:%s:%s:%s:%s:%d", entItemRef, srcType, srcRef, compactDevRef, page)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))

		case "number", "input_number":
			btnText := FormatConfigEntityButton(ent)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, "noop"),
			))

			step := 1.0
			if ent.Attributes != nil {
				if s, ok := ent.Attributes["step"].(float64); ok && s > 0 {
					step = s
				}
			}
			stepLarge := step * 5

			formatStep := func(val float64) string {
				return strconv.FormatFloat(val, 'f', -1, 64)
			}

			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("➖ "+formatStep(stepLarge), fmt.Sprintf("cfn:%s:%s:%s:%s:%s:%d", entItemRef, formatStep(-stepLarge), srcType, srcRef, compactDevRef, page)),
				tgbotapi.NewInlineKeyboardButtonData("➖ "+formatStep(step), fmt.Sprintf("cfn:%s:%s:%s:%s:%s:%d", entItemRef, formatStep(-step), srcType, srcRef, compactDevRef, page)),
				tgbotapi.NewInlineKeyboardButtonData("➕ "+formatStep(step), fmt.Sprintf("cfn:%s:%s:%s:%s:%s:%d", entItemRef, formatStep(step), srcType, srcRef, compactDevRef, page)),
				tgbotapi.NewInlineKeyboardButtonData("➕ "+formatStep(stepLarge), fmt.Sprintf("cfn:%s:%s:%s:%s:%s:%d", entItemRef, formatStep(stepLarge), srcType, srcRef, compactDevRef, page)),
			))

		default:
			btnText := FormatConfigEntityButton(ent)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, "noop"),
			))
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("r_cfg:%s:%s:%s:%d", srcType, srcRef, compactDevRef, page)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 К устройству", fmt.Sprintf("d_bak:%s:%s:%s:%d", srcType, srcRef, compactDevRef, page)),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetSensorIcon возвращает подходящую иконку для датчика на основе имени, домена и атрибутов.
func GetSensorIcon(sensor ha.Entity) string {
	name := sensor.FriendlyName
	if name == "" {
		name = sensor.EntityID
	}

	unit := ""
	deviceClass := ""
	if sensor.Attributes != nil {
		if u, ok := sensor.Attributes["unit_of_measurement"].(string); ok && u != "" {
			unit = strings.ToLower(strings.TrimSpace(u))
		}
		if dc, ok := sensor.Attributes["device_class"].(string); ok && dc != "" {
			deviceClass = strings.ToLower(strings.TrimSpace(dc))
		}
	}

	nameLower := strings.ToLower(name)
	isPowerUnit := unit == "w" || unit == "вт" || unit == "kw" || unit == "квт" || unit == "wh" || unit == "вт·ч" || unit == "kwh" || unit == "квт·ч" || unit == "квтч" || unit == "va" || unit == "ва"
	isVoltageUnit := unit == "v" || unit == "в" || unit == "mv" || unit == "мв" || unit == "kv" || unit == "кв"
	isCurrentUnit := unit == "a" || unit == "а" || unit == "ma" || unit == "ма"
	isPressureUnit := unit == "pa" || unit == "па" || unit == "hpa" || unit == "гпа" || unit == "kpa" || unit == "кпа" || unit == "bar" || unit == "бар" || unit == "mbar" || unit == "мбар" || unit == "mmhg" || unit == "мм рт. ст." || unit == "psi"

	switch {
	case deviceClass == "temperature" || strings.Contains(unit, "°") || strings.Contains(nameLower, "температур") || strings.Contains(nameLower, "temp"):
		return "🌡"
	case deviceClass == "humidity" || (unit == "%" && (strings.Contains(nameLower, "влажн") || strings.Contains(nameLower, "humid"))):
		return "💧"
	case deviceClass == "battery" || (unit == "%" && (strings.Contains(nameLower, "батар") || strings.Contains(nameLower, "battery"))):
		return "🔋"
	case deviceClass == "pressure" || deviceClass == "atmospheric_pressure" || isPressureUnit || strings.Contains(nameLower, "давлен") || strings.Contains(nameLower, "pressure"):
		return "⏲"
	case deviceClass == "power" || deviceClass == "energy" || isPowerUnit || strings.Contains(nameLower, "мощност") || strings.Contains(nameLower, "power") || strings.Contains(nameLower, "энерг"):
		return "⚡"
	case deviceClass == "voltage" || isVoltageUnit || strings.Contains(nameLower, "напряжен") || strings.Contains(nameLower, "voltage"):
		return "🔌"
	case deviceClass == "current" || isCurrentUnit || strings.Contains(nameLower, "сила тока") || strings.Contains(nameLower, "силы тока") || strings.Contains(nameLower, "current"):
		return "⚡"
	case deviceClass == "illuminance" || unit == "lx" || unit == "лк" || strings.Contains(nameLower, "люкс") || strings.Contains(nameLower, "освещен") || strings.Contains(nameLower, "illuminance"):
		return "☀️"
	case strings.Contains(unit, "μg") || strings.Contains(unit, "мкг") || strings.Contains(unit, "pm"):
		return "🌫"
	case deviceClass == "signal_strength" || strings.Contains(nameLower, "сигнал") || strings.Contains(nameLower, "rssi") || strings.Contains(unit, "db") || strings.Contains(unit, "дб"):
		return "📶"
	case deviceClass == "door" || deviceClass == "window" || deviceClass == "opening" || strings.Contains(nameLower, "двер") || strings.Contains(nameLower, "окно") || strings.Contains(nameLower, "door") || strings.Contains(nameLower, "window"):
		return "🚪"
	case deviceClass == "motion" || deviceClass == "occupancy" || strings.Contains(nameLower, "движен") || strings.Contains(nameLower, "motion") || strings.Contains(nameLower, "присутств"):
		return "🚶"
	case deviceClass == "moisture" || strings.Contains(nameLower, "протечк") || strings.Contains(nameLower, "leak") || strings.Contains(nameLower, "moisture"):
		return "🌊"
	case deviceClass == "smoke" || deviceClass == "gas" || strings.Contains(nameLower, "дым") || strings.Contains(nameLower, "газ") || strings.Contains(nameLower, "smoke") || strings.Contains(nameLower, "gas"):
		return "🚨"
	case deviceClass == "problem" || strings.Contains(nameLower, "проблем") || strings.Contains(nameLower, "problem"):
		return "⚠️"
	case strings.Contains(nameLower, "бак") || strings.Contains(nameLower, "tank"):
		return "🪣"
	}

	if sensor.Domain == "binary_sensor" {
		return "🔔"
	}
	return "📊"
}

// FormatSensorDisplay форматирует строку отображения одного датчика с иконкой и значением.
func FormatSensorDisplay(sensor ha.Entity) string {
	name := sensor.FriendlyName
	if name == "" {
		name = sensor.EntityID
	}

	icon := GetSensorIcon(sensor)
	unit := ""
	if sensor.Attributes != nil {
		if u, ok := sensor.Attributes["unit_of_measurement"].(string); ok && u != "" {
			unit = " " + u
		}
	}

	valLower := strings.ToLower(sensor.State)
	stateDisplay := FormatSensorValue(sensor.State)
	if sensor.Domain == "binary_sensor" {
		if valLower == "on" {
			stateDisplay = "🔴 ДА (on)"
		} else if valLower == "off" {
			stateDisplay = "⚪ Нет (off)"
		}
	} else {
		stateDisplay = stateDisplay + unit
	}

	return fmt.Sprintf("%s <b>%s:</b> <code>%s</code>", icon, html.EscapeString(name), html.EscapeString(stateDisplay))
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

		p1 := ha.GetDomainPriority(d1)
		p2 := ha.GetDomainPriority(d2)

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
	totalPages, pageClamped, start, end := paginate(len(devices), page, pageSize)

	var rows [][]tgbotapi.InlineKeyboardButton
	if start < len(devices) {
		for _, item := range devices[start:end] {
			icon, ok := DomainIcons[domain]
			if !ok {
				icon = "⚙️"
			}
			btnText := fmt.Sprintf("%s %s (%d настр.)", icon, item.DeviceName, len(item.ConfigEntities))
			cbData := fmt.Sprintf("cds:%s:%s:%d", GetDeviceRef(item.DeviceID), domain, pageClamped)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, cbData),
			))
		}
	}

	if totalPages > 1 {
		var navRow []tgbotapi.InlineKeyboardButton
		if pageClamped > 0 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("pdc:%s:%d", domain, pageClamped-1)))
		}
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", pageClamped+1, totalPages), "noop"))
		if pageClamped < totalPages-1 {
			navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Вперед ➡️", fmt.Sprintf("pdc:%s:%d", domain, pageClamped+1)))
		}
		rows = append(rows, navRow)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("rdc:%s:%d", domain, pageClamped)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 Все категории", "b_cdm"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GetDeviceSettingsKeyboard возвращает клавиатуру со списком параметров и кнопок управления для устройства.
func GetDeviceSettingsKeyboard(device ha.DeviceConfigGroup, domain string, page ...int) tgbotapi.InlineKeyboardMarkup {
	currPage := 0
	if len(page) > 0 && page[0] >= 0 {
		currPage = page[0]
	}

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

			// Определяем шаг регулировки (по умолчанию 1, либо из атрибутов HA)
			step := 1.0
			if ent.Attributes != nil {
				if s, ok := ent.Attributes["step"].(float64); ok && s > 0 {
					step = s
				}
			}
			stepLarge := step * 5

			formatStep := func(val float64) string {
				return strconv.FormatFloat(val, 'f', -1, 64)
			}

			// Нижняя строка с кнопками регулировки: [-5*step] [-step] [+step] [+5*step]
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("➖ "+formatStep(stepLarge), fmt.Sprintf("cfn:%s:%s", entRef, formatStep(-stepLarge))),
				tgbotapi.NewInlineKeyboardButtonData("➖ "+formatStep(step), fmt.Sprintf("cfn:%s:%s", entRef, formatStep(-step))),
				tgbotapi.NewInlineKeyboardButtonData("➕ "+formatStep(step), fmt.Sprintf("cfn:%s:%s", entRef, formatStep(step))),
				tgbotapi.NewInlineKeyboardButtonData("➕ "+formatStep(stepLarge), fmt.Sprintf("cfn:%s:%s", entRef, formatStep(stepLarge))),
			))

		default:
			btnText := FormatConfigEntityButton(ent)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(btnText, "noop"),
			))
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", fmt.Sprintf("r_cds:%s:%s:%d", GetDeviceRef(device.DeviceID), domain, currPage)),
		tgbotapi.NewInlineKeyboardButtonData("🔙 К устройствам", fmt.Sprintf("b_cdev:%s:%d", domain, currPage)),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}


