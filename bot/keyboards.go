package bot

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tg_bot_home_assistant/ha"
)

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
			cbData := fmt.Sprintf("t:%s:%d", item.EntityID, page)
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
			cbData := fmt.Sprintf("dt:%s:%d", item.EntityID, page)
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
