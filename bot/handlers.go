package bot

import (
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tg_bot_home_assistant/ha"
)

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	if msg.IsCommand() {
		log.Printf("Получена команда /%s от %d (%s)", msg.Command(), msg.From.ID, msg.From.UserName)
		switch msg.Command() {
		case "start":
			b.handleStart(msg)
		case "status":
			b.handleStatus(msg)
		case "devices":
			b.showAllDevicesMenu(msg.Chat.ID, 0, false, 0)
		case "domains", "categories":
			b.showDomainsMenu(msg.Chat.ID, 0, false)
		case "config", "settings":
			b.showConfigDomainsMenu(msg.Chat.ID, 0, false)
		case "toggle":
			b.handleToggle(msg)
		}
		return
	}

	log.Printf("Получено текстовое сообщение %q от %d (%s)", msg.Text, msg.From.ID, msg.From.UserName)
	switch msg.Text {
	case "📊 Статус HA":
		b.handleStatus(msg)
	case "💡 Все устройства", "💡 Доступные устройства":
		b.showAllDevicesMenu(msg.Chat.ID, 0, false, 0)
	case "📁 Устройства по доменам":
		b.showDomainsMenu(msg.Chat.ID, 0, false)
	case "⚙️ Конфигурация устройств", "⚙️ Конфигурация", "⚙️ Настройки":
		b.showConfigDomainsMenu(msg.Chat.ID, 0, false)
	}
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	text := "👋 Привет! Я бот для управления твоим Home Assistant.\n\n" +
		"Вы можете использовать быстрые кнопки внизу экрана:\n" +
		"📊 <b>Статус HA</b> — проверка связи с сервером\n" +
		"💡 <b>Все устройства</b> — общий список устройств управления\n" +
		"📁 <b>Устройства по доменам</b> — устройства по категориям (свет, розетки и др.)\n" +
		"⚙️ <b>Конфигурация устройств</b> — настройки и параметры (подсветка, таймеры и др.)\n\n" +
		"Команды:\n" +
		"• <code>/status</code> — проверить подключение к HA\n" +
		"• <code>/devices</code> — список устройств управления\n" +
		"• <code>/domains</code> — категории устройств\n" +
		"• <code>/config</code> — меню конфигурации и параметров\n" +
		"• <code>/toggle &lt;entity_id&gt;</code> — ручное переключение по ID"

	replyMsg := tgbotapi.NewMessage(msg.Chat.ID, text)
	replyMsg.ParseMode = tgbotapi.ModeHTML
	replyMsg.ReplyMarkup = GetMainKeyboard()

	if _, err := b.api.Send(replyMsg); err != nil {
		log.Printf("Ошибка отправки start-сообщения: %v", err)
	}
}


// extractDomainFromEntity возвращает домен сущности из entity_id.
func extractDomainFromEntity(entityID string) string {
	if dotIdx := strings.Index(entityID, "."); dotIdx != -1 {
		return entityID[:dotIdx]
	}
	return entityID
}

// parseDomainToggleCallback разбирает параметры переключения устройства в категории.
// Поддерживает как короткий формат (ref:page), так и полный (domain:ref:page).
func parseDomainToggleCallback(raw string) (domain string, ref string, page int) {
	parts := strings.Split(raw, ":")
	if len(parts) >= 3 {
		domain = parts[0]
		ref = parts[1]
		page, _ = strconv.Atoi(parts[2])
	} else if len(parts) == 2 {
		ref = parts[0]
		page, _ = strconv.Atoi(parts[1])
	} else if len(parts) == 1 {
		ref = parts[0]
	}
	return domain, ref, page
}

// formatOperationStatus формирует краткий текст статуса операции для всплывающего уведомления в Telegram.
func formatOperationStatus(entityID string, stateObj *ha.EntityState) string {
	if stateObj == nil {
		return "✅ Состояние переключено"
	}
	name := entityID
	if stateObj.Attributes != nil {
		if fn, ok := stateObj.Attributes["friendly_name"].(string); ok && fn != "" {
			name = fn
		}
	}
	var text string
	switch stateObj.State {
	case "on":
		text = fmt.Sprintf("🟢 %s: ВКЛ", name)
	case "off":
		text = fmt.Sprintf("⚪ %s: ВЫКЛ", name)
	case "open", "unlocked":
		text = fmt.Sprintf("🟢 %s: %s", name, strings.ToUpper(stateObj.State))
	case "closed", "locked":
		text = fmt.Sprintf("⚪ %s: %s", name, strings.ToUpper(stateObj.State))
	default:
		text = fmt.Sprintf("✅ %s: %s", name, stateObj.State)
	}

	runes := []rune(text)
	if len(runes) > 190 {
		return string(runes[:187]) + "..."
	}
	return text
}

// resolveControlService определяет домен, метод сервиса и параметры для сущности на основе ее типа и состояния.
func resolveControlService(entityID, currentState string) (string, string, map[string]interface{}) {
	domain := strings.Split(entityID, ".")[0]
	data := map[string]interface{}{"entity_id": entityID}
	switch domain {
	case "scene", "script":
		return domain, "turn_on", data
	case "lock":
		if currentState == "locked" {
			return domain, "unlock", data
		}
		return domain, "lock", data
	case "vacuum":
		if currentState == "cleaning" {
			return domain, "return_to_base", data
		}
		return domain, "start", data
	default:
		return domain, "toggle", data
	}
}

// toggleEntity безопасно вызывает сервис переключения для указанной сущности.
func (b *Bot) toggleEntity(entityID string) (bool, string) {
	stateObj, _ := b.haClient.GetEntityState(entityID)
	currentState := ""
	if stateObj != nil {
		currentState = stateObj.State
	}
	domain, service, serviceData := resolveControlService(entityID, currentState)
	return b.haClient.CallService(domain, service, serviceData)
}

func (b *Bot) handleStatus(msg *tgbotapi.Message) {
	tempMsg := tgbotapi.NewMessage(msg.Chat.ID, "🔄 Проверяю подключение к Home Assistant...")
	sentMsg, err := b.api.Send(tempMsg)
	if err != nil {
		log.Printf("Ошибка отправки статуса: %v", err)
		return
	}

	_, responseText := b.haClient.CheckAPI()

	editMsg := tgbotapi.NewEditMessageText(msg.Chat.ID, sentMsg.MessageID, responseText)
	editMsg.ParseMode = tgbotapi.ModeHTML
	b.editMessage(editMsg)
}

func (b *Bot) showAllDevicesMenu(chatID int64, messageID int, edit bool, page int) {
	ok, devices, errMsg := b.haClient.GetControllableEntities()

	if !ok {
		text := fmt.Sprintf("❌ Не удалось получить список устройств:\n%s", html.EscapeString(errMsg))
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	if len(devices) == 0 {
		text := "ℹ️ В Home Assistant пока не найдено управляемых устройств."
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	pageSize := DefaultPageSize
	totalPages, pageClamped, _, _ := paginate(len(devices), page, pageSize)

	var text string
	if totalPages > 1 {
		text = fmt.Sprintf(
			"💡 <b>Все доступные устройства:</b> (всего %d шт., стр. %d/%d)\nНажмите на устройство для переключения:",
			len(devices),
			pageClamped+1,
			totalPages,
		)
	} else {
		text = fmt.Sprintf(
			"💡 <b>Все доступные устройства:</b> (%d шт.)\nНажмите на устройство для переключения:",
			len(devices),
		)
	}

	keyboard := GetDevicesKeyboard(devices, pageClamped, pageSize)

	if edit {
		b.editHTML(chatID, messageID, text, &keyboard)
	} else {
		_, _ = b.sendHTML(chatID, text, keyboard)
	}
}

func (b *Bot) showDomainsMenu(chatID int64, messageID int, edit bool) {
	ok, grouped, errMsg := b.haClient.GetEntitiesGroupedByDomain()

	if !ok {
		text := fmt.Sprintf("❌ Не удалось получить категории устройств:\n%s", html.EscapeString(errMsg))
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	totalDevices := 0
	activeCategories := 0
	for _, items := range grouped {
		count := len(items)
		totalDevices += count
		if count > 0 {
			activeCategories++
		}
	}

	if activeCategories == 0 {
		text := "📁 <b>Категории устройств Home Assistant:</b>\n\nℹ️ В Home Assistant пока не найдено активных управляемых устройств."
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	text := fmt.Sprintf(
		"📁 <b>Категории устройств Home Assistant:</b>\nВсего категорий: %d, активных устройств: %d\n\nВыберите категорию для просмотра и управления:",
		activeCategories,
		totalDevices,
	)
	keyboard := GetDomainsKeyboard(grouped)

	if edit {
		b.editHTML(chatID, messageID, text, &keyboard)
	} else {
		_, _ = b.sendHTML(chatID, text, keyboard)
	}
}

func (b *Bot) showDomainDevicesMenu(cbID string, chatID int64, messageID int, domain string, page int) {
	ok, devices, errMsg := b.haClient.GetEntitiesForDomain(domain)

	domainName := DomainNames[domain]
	if domainName == "" {
		domainName = domain
	}
	escapedDomainName := html.EscapeString(domainName)

	domainIcon := DomainIcons[domain]
	if domainIcon == "" {
		domainIcon = "📁"
	}

	if !ok {
		callbackAlert := tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg))
		_, _ = b.api.Request(callbackAlert)
		return
	}

	pageSize := DefaultPageSize
	totalPages, pageClamped, _, _ := paginate(len(devices), page, pageSize)

	var text string
	if len(devices) == 0 {
		text = fmt.Sprintf(
			"%s <b>Категория: %s</b> (0 шт.)\n\nℹ️ В этой категории пока нет подключенных устройств в Home Assistant.",
			domainIcon,
			escapedDomainName,
		)
	} else if totalPages > 1 {
		text = fmt.Sprintf(
			"%s <b>Категория: %s</b> (%d шт., стр. %d/%d)\nНажмите на устройство для переключения:",
			domainIcon,
			escapedDomainName,
			len(devices),
			pageClamped+1,
			totalPages,
		)
	} else {
		text = fmt.Sprintf(
			"%s <b>Категория: %s</b> (%d шт.)\nНажмите на устройство для переключения:",
			domainIcon,
			escapedDomainName,
			len(devices),
		)
	}

	keyboard := GetDomainDevicesKeyboard(domain, devices, pageClamped, pageSize)
	b.editHTML(chatID, messageID, text, &keyboard)
}

func (b *Bot) showConfigDomainsMenu(chatID int64, messageID int, edit bool) {
	ok, grouped, errMsg := b.haClient.GetConfigDevicesGroupedByDomain()

	if !ok {
		text := fmt.Sprintf("❌ Не удалось получить список настроек:\n%s", html.EscapeString(errMsg))
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	totalDevices := 0
	activeCategories := 0
	for _, items := range grouped {
		count := len(items)
		totalDevices += count
		if count > 0 {
			activeCategories++
		}
	}

	if activeCategories == 0 {
		text := "⚙️ <b>Конфигурация устройств:</b>\n\nℹ️ В Home Assistant пока не найдено устройств с доступными параметрами настроек."
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	text := fmt.Sprintf(
		"⚙️ <b>Конфигурация устройств Home Assistant:</b>\nВсего категорий: %d, настраиваемых устройств: %d\n\nВыберите категорию для настройки параметров:",
		activeCategories,
		totalDevices,
	)
	keyboard := GetConfigDomainsKeyboard(grouped)

	if edit {
		b.editHTML(chatID, messageID, text, &keyboard)
	} else {
		_, _ = b.sendHTML(chatID, text, keyboard)
	}
}

func (b *Bot) showConfigDevicesMenu(cbID string, chatID int64, messageID int, domain string, page int) {
	ok, devices, errMsg := b.haClient.GetConfigDevicesForDomain(domain)

	domainName := DomainNames[domain]
	if domainName == "" {
		domainName = domain
	}
	escapedDomainName := html.EscapeString(domainName)

	domainIcon := DomainIcons[domain]
	if domainIcon == "" {
		domainIcon = "⚙️"
	}

	if !ok {
		callbackAlert := tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg))
		_, _ = b.api.Request(callbackAlert)
		return
	}

	pageSize := DefaultPageSize
	totalPages, pageClamped, _, _ := paginate(len(devices), page, pageSize)

	var text string
	if len(devices) == 0 {
		text = fmt.Sprintf(
			"⚙️ %s <b>Настройка: %s</b> (0 устройств)\n\nℹ️ В этой категории нет устройств с настраиваемыми параметрами.",
			domainIcon,
			escapedDomainName,
		)
	} else if totalPages > 1 {
		text = fmt.Sprintf(
			"⚙️ %s <b>Настройка: %s</b> (%d устройств, стр. %d/%d)\nВыберите устройство для изменения параметров:",
			domainIcon,
			escapedDomainName,
			len(devices),
			pageClamped+1,
			totalPages,
		)
	} else {
		text = fmt.Sprintf(
			"⚙️ %s <b>Настройка: %s</b> (%d устройств)\nВыберите устройство для изменения параметров:",
			domainIcon,
			escapedDomainName,
			len(devices),
		)
	}

	keyboard := GetConfigDevicesKeyboard(domain, devices, pageClamped, pageSize)
	b.editHTML(chatID, messageID, text, &keyboard)
}

func (b *Bot) renderDeviceSettings(cbID string, chatID int64, messageID int, device ha.DeviceConfigGroup, domain string, page int) {
	icon := DomainIcons[device.Domain]
	if icon == "" {
		icon = "⚙️"
	}

	text := fmt.Sprintf(
		"⚙️ %s <b>Параметры устройства: %s</b>\n\nНажмите на параметр для изменения:",
		icon,
		html.EscapeString(device.DeviceName),
	)

	keyboard := GetDeviceSettingsKeyboard(device, domain, page)
	b.editHTML(chatID, messageID, text, &keyboard)
}

func (b *Bot) showDeviceSettingsMenu(cbID string, chatID int64, messageID int, deviceID string, domain string, page ...int) {
	currPage := 0
	if len(page) > 0 {
		currPage = page[0]
	}

	ok, device, errMsg := b.haClient.GetDeviceConfig(deviceID)
	if !ok || device == nil {
		if cbID != "" {
			callbackAlert := tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg))
			_, _ = b.api.Request(callbackAlert)
		}
		return
	}

	b.renderDeviceSettings(cbID, chatID, messageID, *device, domain, currPage)
}

// refreshDeviceSettingsForEntity находит устройство и обновляет меню параметров за один проход по реестру.
func (b *Bot) refreshDeviceSettingsForEntity(cbID string, chatID int64, messageID int, entityID string, page int) {
	ok, grouped, errMsg := b.haClient.GetConfigDevicesGroupedByDomain()
	if !ok {
		if cbID != "" {
			callbackAlert := tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg))
			_, _ = b.api.Request(callbackAlert)
		}
		return
	}
	for domain, devices := range grouped {
		for _, dev := range devices {
			for _, cfg := range dev.ConfigEntities {
				if cfg.EntityID == entityID {
					b.renderDeviceSettings(cbID, chatID, messageID, dev, domain, page)
					return
				}
			}
		}
	}
}

func (b *Bot) handleCallbackQuery(cb *tgbotapi.CallbackQuery) {
	if cb.Message == nil {
		return
	}

	log.Printf("Получен callback %q от %d (%s)", cb.Data, cb.From.ID, cb.From.UserName)

	chatID := cb.Message.Chat.ID
	messageID := cb.Message.MessageID

	if cb.Data == "noop" {
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		return
	}

	if cb.Data == "r_all" || strings.HasPrefix(cb.Data, "r_all:") {
		b.haClient.InvalidateCache()
		page := 0
		if strings.HasPrefix(cb.Data, "r_all:") {
			page, _ = strconv.Atoi(strings.TrimPrefix(cb.Data, "r_all:"))
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю список..."))
		b.showAllDevicesMenu(chatID, messageID, true, page)
		return
	}

	if strings.HasPrefix(cb.Data, "pa:") {
		page, _ := strconv.Atoi(strings.TrimPrefix(cb.Data, "pa:"))
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showAllDevicesMenu(chatID, messageID, true, page)
		return
	}

	if strings.HasPrefix(cb.Data, "t:") {
		raw := strings.TrimPrefix(cb.Data, "t:")
		parts := strings.Split(raw, ":")
		entityID := b.resolveEntityRef(parts[0])
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}

		success, msg := b.toggleEntity(entityID)
		if !success {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}

		stateObj, _ := b.haClient.GetEntityState(entityID)
		statusText := formatOperationStatus(entityID, stateObj)
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, statusText))

		b.showAllDevicesMenu(chatID, messageID, true, page)
		return
	}

	if cb.Data == "b_dom" || cb.Data == "r_dom" {
		if cb.Data == "r_dom" {
			b.haClient.InvalidateCache()
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Загружаю категории..."))
		b.showDomainsMenu(chatID, messageID, true)
		return
	}

	if strings.HasPrefix(cb.Data, "dv:") {
		raw := strings.TrimPrefix(cb.Data, "dv:")
		parts := strings.Split(raw, ":")
		domain := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showDomainDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "dr:") {
		b.haClient.InvalidateCache()
		raw := strings.TrimPrefix(cb.Data, "dr:")
		parts := strings.Split(raw, ":")
		domain := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю категорию..."))
		b.showDomainDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "pd:") {
		raw := strings.TrimPrefix(cb.Data, "pd:")
		parts := strings.Split(raw, ":")
		domain := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showDomainDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "dt:") {
		raw := strings.TrimPrefix(cb.Data, "dt:")
		domain, ref, page := parseDomainToggleCallback(raw)
		entityID := b.resolveEntityRef(ref)
		if domain == "" {
			domain = extractDomainFromEntity(entityID)
		}

		success, msg := b.toggleEntity(entityID)
		if !success {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}

		stateObj, _ := b.haClient.GetEntityState(entityID)
		statusText := formatOperationStatus(entityID, stateObj)
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, statusText))

		b.showDomainDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	// --- Обработка меню конфигурации устройств ---

	if cb.Data == "b_cdm" || cb.Data == "r_cdm" {
		if cb.Data == "r_cdm" {
			b.haClient.InvalidateCache()
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Загружаю настройки..."))
		b.showConfigDomainsMenu(chatID, messageID, true)
		return
	}

	if strings.HasPrefix(cb.Data, "cdm:") {
		domain := strings.TrimPrefix(cb.Data, "cdm:")
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showConfigDevicesMenu(cb.ID, chatID, messageID, domain, 0)
		return
	}

	if strings.HasPrefix(cb.Data, "rdc:") {
		b.haClient.InvalidateCache()
		raw := strings.TrimPrefix(cb.Data, "rdc:")
		parts := strings.Split(raw, ":")
		domain := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю..."))
		b.showConfigDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "pdc:") {
		raw := strings.TrimPrefix(cb.Data, "pdc:")
		parts := strings.Split(raw, ":")
		domain := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showConfigDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "cds:") {
		raw := strings.TrimPrefix(cb.Data, "cds:")
		parts := strings.Split(raw, ":")
		deviceID := b.resolveEntityRef(parts[0])
		domain := ""
		if len(parts) > 1 {
			domain = parts[1]
		}
		page := 0
		if len(parts) > 2 {
			page, _ = strconv.Atoi(parts[2])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showDeviceSettingsMenu(cb.ID, chatID, messageID, deviceID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "r_cds:") {
		b.haClient.InvalidateCache()
		raw := strings.TrimPrefix(cb.Data, "r_cds:")
		parts := strings.Split(raw, ":")
		deviceID := b.resolveEntityRef(parts[0])
		domain := ""
		if len(parts) > 1 {
			domain = parts[1]
		}
		page := 0
		if len(parts) > 2 {
			page, _ = strconv.Atoi(parts[2])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю параметры..."))
		b.showDeviceSettingsMenu(cb.ID, chatID, messageID, deviceID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "b_cdev:") {
		raw := strings.TrimPrefix(cb.Data, "b_cdev:")
		parts := strings.Split(raw, ":")
		domain := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showConfigDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "cft:") {
		entityID := b.resolveEntityRef(strings.TrimPrefix(cb.Data, "cft:"))
		domain := strings.Split(entityID, ".")[0]
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Переключаю..."))
		ok, msg := b.haClient.CallService(domain, "toggle", map[string]interface{}{"entity_id": entityID})
		if !ok {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}
		b.refreshDeviceSettingsForEntity(cb.ID, chatID, messageID, entityID, 0)
		return
	}

	if strings.HasPrefix(cb.Data, "cfp:") {
		entityID := b.resolveEntityRef(strings.TrimPrefix(cb.Data, "cfp:"))
		domain := strings.Split(entityID, ".")[0]
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Выполняю..."))
		ok, msg := b.haClient.CallService(domain, "press", map[string]interface{}{"entity_id": entityID})
		if !ok {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}
		b.refreshDeviceSettingsForEntity(cb.ID, chatID, messageID, entityID, 0)
		return
	}

	if strings.HasPrefix(cb.Data, "cfs:") {
		entityID := b.resolveEntityRef(strings.TrimPrefix(cb.Data, "cfs:"))
		domain := strings.Split(entityID, ".")[0]
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Переключаю режим..."))
		ok, msg := b.haClient.CallService(domain, "select_next", map[string]interface{}{"entity_id": entityID, "cycle": true})
		if !ok {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}
		b.refreshDeviceSettingsForEntity(cb.ID, chatID, messageID, entityID, 0)
		return
	}

	if strings.HasPrefix(cb.Data, "cfn:") {
		raw := strings.TrimPrefix(cb.Data, "cfn:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 2 {
			entityID := b.resolveEntityRef(parts[0])
			delta, _ := strconv.ParseFloat(parts[1], 64)

			stateObj, err := b.haClient.GetEntityState(entityID)
			if err != nil || stateObj == nil {
				_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, "❌ Не удалось получить состояние"))
				return
			}

			currentVal, err := strconv.ParseFloat(stateObj.State, 64)
			if err != nil {
				currentVal = 0
			}

			newVal := currentVal + delta
			if stateObj.Attributes != nil {
				if minVal, ok := stateObj.Attributes["min"].(float64); ok && newVal < minVal {
					newVal = minVal
				}
				if maxVal, ok := stateObj.Attributes["max"].(float64); ok && newVal > maxVal {
					newVal = maxVal
				}
			}

			// Устраняем артефакты точности float64 (IEEE 754)
			newVal, _ = strconv.ParseFloat(fmt.Sprintf("%.4f", newVal), 64)

			domain := strings.Split(entityID, ".")[0]
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Устанавливаю значение..."))
			ok, msg := b.haClient.CallService(domain, "set_value", map[string]interface{}{
				"entity_id": entityID,
				"value":     newVal,
			})
			if !ok {
				_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
				return
			}

			b.refreshDeviceSettingsForEntity(cb.ID, chatID, messageID, entityID, 0)
			return
		}
	}
}

func (b *Bot) handleToggle(msg *tgbotapi.Message) {
	args := strings.Fields(msg.CommandArguments())
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		replyText := "ℹ️ Пожалуйста, укажите entity_id.\nПример: <code>/toggle light.living_room</code>"
		_, _ = b.sendHTML(msg.Chat.ID, replyText, nil)
		return
	}

	entityID := strings.TrimSpace(args[0])
	if !strings.Contains(entityID, ".") {
		replyText := "⚠️ Неверный формат entity_id. Идентификатор должен быть вида <code>domain.object_id</code> (например, <code>switch.relay</code>)."
		_, _ = b.sendHTML(msg.Chat.ID, replyText, nil)
		return
	}

	success, errMsg := b.toggleEntity(entityID)

	if success {
		stateObj, _ := b.haClient.GetEntityState(entityID)
		currentState := "неизвестно"
		if stateObj != nil && stateObj.State != "" {
			currentState = stateObj.State
		}

		replyText := fmt.Sprintf(
			"✅ Устройство <code>%s</code> переключено.\nТекущее состояние: <b>%s</b>",
			html.EscapeString(entityID),
			html.EscapeString(currentState),
		)
		_, _ = b.sendHTML(msg.Chat.ID, replyText, nil)
	} else {
		replyText := fmt.Sprintf(
			"❌ Не удалось переключить <code>%s</code>.\nПричина: %s",
			html.EscapeString(entityID),
			html.EscapeString(errMsg),
		)
		_, _ = b.sendHTML(msg.Chat.ID, replyText, nil)
	}
}

