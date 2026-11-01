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
		case "rooms", "areas":
			b.showAreasMenu(msg.Chat.ID, 0, false)
		case "domains", "categories":
			b.showDomainsMenu(msg.Chat.ID, 0, false)
		case "devices":
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
	case "📁 Устройства по доменам":
		b.showDomainsMenu(msg.Chat.ID, 0, false)
	case "🏠 Устройства по комнатам", "🏠 Комнаты":
		b.showAreasMenu(msg.Chat.ID, 0, false)
	case "💡 Все устройства", "💡 Доступные устройства":
		b.showDomainsMenu(msg.Chat.ID, 0, false)
	case "⚙️ Конфигурация устройств", "⚙️ Конфигурация", "⚙️ Настройки":
		b.showConfigDomainsMenu(msg.Chat.ID, 0, false)
	}
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	text := "👋 Привет! Я бот для управления твоим Home Assistant.\n\n" +
		"Вы можете использовать быстрые кнопки внизу экрана:\n" +
		"📊 <b>Статус HA</b> — проверка связи с сервером\n" +
		"📁 <b>Устройства по доменам</b> — устройства по категориям (свет, розетки и др.)\n" +
		"🏠 <b>Устройства по комнатам</b> — устройства, сгруппированные по комнатам\n\n" +
		"Команды:\n" +
		"• <code>/status</code> — проверить подключение к HA\n" +
		"• <code>/domains</code> — категории устройств\n" +
		"• <code>/rooms</code> — устройства по комнатам\n" +
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
	domain := extractDomainFromEntity(entityID)
	if !ha.IsControllableDomain(domain) {
		return false, "Это устройство является датчиком и не поддерживает включение/выключение."
	}
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
			"%s <b>Категория: %s</b> (%d шт., стр. %d/%d)\nВыберите устройство для управления:",
			domainIcon,
			escapedDomainName,
			len(devices),
			pageClamped+1,
			totalPages,
		)
	} else {
		text = fmt.Sprintf(
			"%s <b>Категория: %s</b> (%d шт.)\nВыберите устройство для управления:",
			domainIcon,
			escapedDomainName,
			len(devices),
		)
	}

	keyboard := GetDomainDevicesKeyboard(domain, devices, pageClamped, pageSize)
	b.editHTML(chatID, messageID, text, &keyboard)
}

func (b *Bot) showAreasMenu(chatID int64, messageID int, edit bool) {
	ok, grouped, areaNames, errMsg := b.haClient.GetEntitiesGroupedByArea()

	if !ok {
		text := fmt.Sprintf("❌ Не удалось получить список комнат:\n%s", html.EscapeString(errMsg))
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	totalDevices := 0
	activeAreas := 0
	for _, items := range grouped {
		count := len(items)
		totalDevices += count
		if count > 0 {
			activeAreas++
		}
	}

	if activeAreas == 0 {
		text := "🏠 <b>Комнаты Home Assistant:</b>\n\nℹ️ В Home Assistant пока не найдено активных комнат с устройствами."
		if edit {
			b.editHTML(chatID, messageID, text, nil)
		} else {
			_, _ = b.sendHTML(chatID, text, nil)
		}
		return
	}

	text := fmt.Sprintf(
		"🏠 <b>Устройства по комнатам:</b>\nВсего комнат: %d, активных устройств: %d\n\nВыберите комнату для просмотра устройств:",
		activeAreas,
		totalDevices,
	)
	keyboard := GetAreasKeyboard(grouped, areaNames)

	if edit {
		b.editHTML(chatID, messageID, text, &keyboard)
	} else {
		_, _ = b.sendHTML(chatID, text, keyboard)
	}
}

func (b *Bot) showAreaDevicesMenu(cbID string, chatID int64, messageID int, areaID string, page int) {
	ok, grouped, areaNames, errMsg := b.haClient.GetEntitiesGroupedByArea()

	if !ok {
		callbackAlert := tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg))
		_, _ = b.api.Request(callbackAlert)
		return
	}

	devices := grouped[areaID]
	areaName := areaNames[areaID]
	if areaName == "" {
		areaName = areaID
	}
	escapedAreaName := html.EscapeString(areaName)
	areaIcon := GetAreaIcon(areaID, areaName)

	pageSize := DefaultPageSize
	totalPages, pageClamped, _, _ := paginate(len(devices), page, pageSize)

	var text string
	if len(devices) == 0 {
		text = fmt.Sprintf(
			"%s <b>Комната: %s</b> (0 шт.)\n\nℹ️ В этой комнате пока нет подключенных устройств в Home Assistant.",
			areaIcon,
			escapedAreaName,
		)
	} else if totalPages > 1 {
		text = fmt.Sprintf(
			"%s <b>Комната: %s</b> (%d шт., стр. %d/%d)\nВыберите устройство для управления:",
			areaIcon,
			escapedAreaName,
			len(devices),
			pageClamped+1,
			totalPages,
		)
	} else {
		text = fmt.Sprintf(
			"%s <b>Комната: %s</b> (%d шт.)\nВыберите устройство для управления:",
			areaIcon,
			escapedAreaName,
			len(devices),
		)
	}

	keyboard := GetAreaDevicesKeyboard(areaID, areaName, devices, pageClamped, pageSize)
	b.editHTML(chatID, messageID, text, &keyboard)
}

func (b *Bot) showDeviceMenu(cbID string, chatID int64, messageID int, entityID string, srcType, srcID string, page int) {
	ok, allEntities, errMsg := b.haClient.GetAllEntities()
	if !ok {
		if cbID != "" {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg)))
		}
		return
	}

	var targetEntity *ha.Entity
	for i := range allEntities {
		if allEntities[i].EntityID == entityID {
			targetEntity = &allEntities[i]
			break
		}
	}

	if targetEntity == nil {
		st, err := b.haClient.GetEntityState(entityID)
		if err != nil || st == nil {
			if cbID != "" {
				_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cbID, "❌ Устройство не найдено в Home Assistant"))
			}
			return
		}
		dotIdx := strings.Index(entityID, ".")
		domain := ""
		if dotIdx != -1 {
			domain = entityID[:dotIdx]
		}
		fn := entityID
		if st.Attributes != nil {
			if f, ok := st.Attributes["friendly_name"].(string); ok && f != "" {
				fn = f
			}
		}
		targetEntity = &ha.Entity{
			EntityID:     entityID,
			Domain:       domain,
			FriendlyName: fn,
			State:        st.State,
			Attributes:   st.Attributes,
		}
	}

	// Если сущность привязана к физическому устройству, находим все его управляемые реле
	if targetEntity.DeviceID != "" {
		controllable := ha.FilterDeviceControllableEntities(allEntities, targetEntity.DeviceID)
		if len(controllable) > 1 {
			targetEntity.ChildEntities = controllable
			if targetEntity.DeviceName != "" {
				targetEntity.FriendlyName = targetEntity.DeviceName
			}
			hasOn := false
			for _, child := range controllable {
				st := strings.ToLower(child.State)
				if st == "on" || st == "open" || st == "unlocked" {
					hasOn = true
					break
				}
			}
			if hasOn {
				targetEntity.State = "on"
			} else {
				targetEntity.State = "off"
			}
		}
	}

	sensors := ha.FilterDeviceSensors(allEntities, targetEntity.DeviceID, targetEntity.EntityID)
	hasSensors := len(sensors) > 0

	icon := DomainIcons[targetEntity.Domain]
	if targetEntity.Domain == "sensor" || targetEntity.Domain == "binary_sensor" {
		sIcon := GetSensorIcon(*targetEntity)
		if sIcon != "" {
			icon = sIcon
		}
	}
	if icon == "" {
		icon = "⚙️"
	}

	areaDisplay := targetEntity.AreaName
	if areaDisplay == "" {
		areaDisplay = "Без комнаты"
	}

	domainName := DomainNames[targetEntity.Domain]
	if domainName == "" {
		domainName = targetEntity.Domain
	}

	statusLabel := "📊 <b>Состояние:</b>"
	statusDisplay := targetEntity.State
	if len(targetEntity.ChildEntities) > 1 {
		statusLabel = "📊 <b>Реле и переключатели:</b>"
		var lines []string
		for _, child := range targetEntity.ChildEntities {
			childName := FormatChildEntityName(child.FriendlyName, targetEntity.DeviceName)
			cIcon := DomainIcons[child.Domain]
			if cIcon == "" {
				cIcon = "🔌"
			}
			st := child.State
			switch strings.ToLower(st) {
			case "on":
				st = "🟢 ВКЛ"
			case "off":
				st = "⚪ ВЫКЛ"
			case "open", "unlocked":
				st = fmt.Sprintf("🟢 %s", strings.ToUpper(st))
			case "closed", "locked":
				st = fmt.Sprintf("⚪ %s", strings.ToUpper(st))
			default:
				st = fmt.Sprintf("[%s]", st)
			}
			lines = append(lines, fmt.Sprintf("  • %s %s: <b>%s</b>", cIcon, html.EscapeString(childName), st))
		}
		statusDisplay = "\n" + strings.Join(lines, "\n")
	} else if targetEntity.Domain == "sensor" {
		unit := ""
		if targetEntity.Attributes != nil {
			if u, ok := targetEntity.Attributes["unit_of_measurement"].(string); ok && u != "" {
				unit = " " + u
			}
		}
		statusDisplay = FormatSensorValue(targetEntity.State) + unit
	} else if targetEntity.Domain == "binary_sensor" {
		valLower := strings.ToLower(targetEntity.State)
		if valLower == "on" {
			statusDisplay = "🔴 ВКЛ (on)"
		} else if valLower == "off" {
			statusDisplay = "⚪ ВЫКЛ (off)"
		}
	} else {
		switch targetEntity.State {
		case "on":
			statusDisplay = "🟢 ВКЛ"
		case "off":
			statusDisplay = "⚪ ВЫКЛ"
		case "open", "unlocked":
			statusDisplay = fmt.Sprintf("🟢 %s", strings.ToUpper(targetEntity.State))
		case "closed", "locked":
			statusDisplay = fmt.Sprintf("⚪ %s", strings.ToUpper(targetEntity.State))
		}
	}

	var entityField string
	if len(targetEntity.ChildEntities) > 1 {
		entityField = fmt.Sprintf("🔌 <b>Устройство:</b> <code>%s</code>\n", html.EscapeString(targetEntity.FriendlyName))
	} else {
		entityField = fmt.Sprintf("🏷 <b>Сущность:</b> <code>%s</code>\n", html.EscapeString(targetEntity.EntityID))
	}

	text := fmt.Sprintf(
		"%s <b>%s</b>\n\n"+
			"%s"+
			"🏠 <b>Комната:</b> %s\n"+
			"📁 <b>Категория:</b> %s\n"+
			"%s %s",
		icon,
		html.EscapeString(targetEntity.FriendlyName),
		entityField,
		html.EscapeString(areaDisplay),
		html.EscapeString(domainName),
		statusLabel,
		statusDisplay,
	)

	keyboard := GetDeviceMenuKeyboard(*targetEntity, hasSensors, srcType, srcID, page)
	b.editHTML(chatID, messageID, text, &keyboard)
}

func (b *Bot) showDeviceSensors(cbID string, chatID int64, messageID int, entityID string, srcType, srcID string, page int) {
	ok, allEntities, errMsg := b.haClient.GetAllEntities()
	if !ok {
		if cbID != "" {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg)))
		}
		return
	}

	var targetEntity *ha.Entity
	for i := range allEntities {
		if allEntities[i].EntityID == entityID {
			targetEntity = &allEntities[i]
			break
		}
	}

	if targetEntity == nil {
		if cbID != "" {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cbID, "❌ Устройство не найдено"))
		}
		return
	}

	sensors := ha.FilterDeviceSensors(allEntities, targetEntity.DeviceID, targetEntity.EntityID)
	if len(sensors) == 0 {
		if cbID != "" {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cbID, "ℹ️ У этого устройства нет датчиков"))
		}
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 <b>Показания датчиков устройства:</b>\n<b>%s</b>\n", html.EscapeString(targetEntity.FriendlyName)))
	if targetEntity.AreaName != "" {
		sb.WriteString(fmt.Sprintf("🏠 <b>Комната:</b> %s\n", html.EscapeString(targetEntity.AreaName)))
	}
	sb.WriteString("\n")

	for _, s := range sensors {
		sb.WriteString(FormatSensorDisplay(s) + "\n")
	}

	keyboard := GetDeviceSensorsKeyboard(srcType, srcID, GetEntityRef(entityID), page)
	b.editHTML(chatID, messageID, sb.String(), &keyboard)
}

func (b *Bot) showDeviceConfigMenu(cbID string, chatID int64, messageID int, entityID string, srcType, srcID string, page int) {
	ok, allEntities, errMsg := b.haClient.GetAllEntities()
	if !ok {
		if cbID != "" {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cbID, fmt.Sprintf("Ошибка: %s", errMsg)))
		}
		return
	}

	deviceID := ""
	for _, e := range allEntities {
		if e.EntityID == entityID {
			deviceID = e.DeviceID
			break
		}
	}

	lookupKey := deviceID
	if lookupKey == "" {
		lookupKey = entityID
	}

	okCfg, deviceCfg, _ := b.haClient.GetDeviceConfig(lookupKey)
	if !okCfg || deviceCfg == nil || len(deviceCfg.ConfigEntities) == 0 {
		if cbID != "" {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cbID, "ℹ️ У этого устройства нет настраиваемых параметров."))
		}
		return
	}

	icon := DomainIcons[deviceCfg.Domain]
	if icon == "" {
		icon = "⚙️"
	}

	text := fmt.Sprintf(
		"⚙️ %s <b>Конфигурация устройства: %s</b>\n\nНажмите на параметр для изменения:",
		icon,
		html.EscapeString(deviceCfg.DeviceName),
	)

	keyboard := GetDeviceConfigKeyboard(*deviceCfg, srcType, srcID, GetEntityRef(entityID), page)
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

	// --- Обработка комнат ---
	if cb.Data == "b_rms" || cb.Data == "r_rms" {
		if cb.Data == "r_rms" {
			b.haClient.InvalidateCache()
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Загружаю комнаты..."))
		b.showAreasMenu(chatID, messageID, true)
		return
	}

	if strings.HasPrefix(cb.Data, "rm:") {
		areaID := b.resolveEntityRef(strings.TrimPrefix(cb.Data, "rm:"))
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showAreaDevicesMenu(cb.ID, chatID, messageID, areaID, 0)
		return
	}

	if strings.HasPrefix(cb.Data, "r_rm:") {
		b.haClient.InvalidateCache()
		raw := strings.TrimPrefix(cb.Data, "r_rm:")
		parts := strings.Split(raw, ":")
		areaID := b.resolveEntityRef(parts[0])
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю комнату..."))
		b.showAreaDevicesMenu(cb.ID, chatID, messageID, areaID, page)
		return
	}

	if strings.HasPrefix(cb.Data, "prm:") {
		raw := strings.TrimPrefix(cb.Data, "prm:")
		parts := strings.Split(raw, ":")
		areaID := b.resolveEntityRef(parts[0])
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.showAreaDevicesMenu(cb.ID, chatID, messageID, areaID, page)
		return
	}

	// --- Выбор устройства и контекстное меню ---
	if strings.HasPrefix(cb.Data, "dv_sel:") {
		raw := strings.TrimPrefix(cb.Data, "dv_sel:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			srcType := parts[0]
			srcID := parts[1]
			ref := parts[2]
			page, _ := strconv.Atoi(parts[3])
			entityID := b.resolveEntityRef(ref)
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
			b.showDeviceMenu(cb.ID, chatID, messageID, entityID, srcType, srcID, page)
			return
		}
	}

	if strings.HasPrefix(cb.Data, "d_tog:") {
		raw := strings.TrimPrefix(cb.Data, "d_tog:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			srcType := parts[0]
			srcID := parts[1]
			ref := parts[2]
			page, _ := strconv.Atoi(parts[3])
			entityID := b.resolveEntityRef(ref)

			// Защита: датчики и неуправляемые сущности не поддерживают переключение
			dotIdx := strings.Index(entityID, ".")
			domain := ""
			if dotIdx != -1 {
				domain = entityID[:dotIdx]
			}
			if !ha.IsControllableDomain(domain) {
				_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, "ℹ️ Это устройство является датчиком и не поддерживает включение/выключение."))
				return
			}

			success, msg := b.toggleEntity(entityID)
			if !success {
				_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
				return
			}

			stateObj, _ := b.haClient.GetEntityState(entityID)
			statusText := formatOperationStatus(entityID, stateObj)
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, statusText))

			b.showDeviceMenu(cb.ID, chatID, messageID, entityID, srcType, srcID, page)
			return
		}
	}

	if strings.HasPrefix(cb.Data, "d_cfg:") {
		raw := strings.TrimPrefix(cb.Data, "d_cfg:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			srcType := parts[0]
			srcID := parts[1]
			ref := parts[2]
			page, _ := strconv.Atoi(parts[3])
			entityID := b.resolveEntityRef(ref)
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
			b.showDeviceConfigMenu(cb.ID, chatID, messageID, entityID, srcType, srcID, page)
			return
		}
	}

	if strings.HasPrefix(cb.Data, "d_sen:") || strings.HasPrefix(cb.Data, "d_sen_r:") {
		isRefresh := strings.HasPrefix(cb.Data, "d_sen_r:")
		raw := strings.TrimPrefix(cb.Data, "d_sen:")
		if isRefresh {
			raw = strings.TrimPrefix(cb.Data, "d_sen_r:")
			b.haClient.InvalidateCache()
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю датчики..."))
		} else {
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		}
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			srcType := parts[0]
			srcID := parts[1]
			ref := parts[2]
			page, _ := strconv.Atoi(parts[3])
			entityID := b.resolveEntityRef(ref)
			b.showDeviceSensors(cb.ID, chatID, messageID, entityID, srcType, srcID, page)
			return
		}
	}

	if strings.HasPrefix(cb.Data, "d_ref:") {
		b.haClient.InvalidateCache()
		raw := strings.TrimPrefix(cb.Data, "d_ref:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			srcType := parts[0]
			srcID := parts[1]
			ref := parts[2]
			page, _ := strconv.Atoi(parts[3])
			entityID := b.resolveEntityRef(ref)
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю..."))
			b.showDeviceMenu(cb.ID, chatID, messageID, entityID, srcType, srcID, page)
			return
		}
	}

	if strings.HasPrefix(cb.Data, "d_bak:") {
		raw := strings.TrimPrefix(cb.Data, "d_bak:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			srcType := parts[0]
			srcID := parts[1]
			ref := parts[2]
			page, _ := strconv.Atoi(parts[3])
			entityID := b.resolveEntityRef(ref)
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
			b.showDeviceMenu(cb.ID, chatID, messageID, entityID, srcType, srcID, page)
			return
		}
	}

	if strings.HasPrefix(cb.Data, "d_lst:") {
		raw := strings.TrimPrefix(cb.Data, "d_lst:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 3 {
			srcType := parts[0]
			srcID := b.resolveEntityRef(parts[1])
			page, _ := strconv.Atoi(parts[2])
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
			if srcType == "r" {
				b.showAreaDevicesMenu(cb.ID, chatID, messageID, srcID, page)
			} else {
				b.showDomainDevicesMenu(cb.ID, chatID, messageID, srcID, page)
			}
			return
		}
	}

	if strings.HasPrefix(cb.Data, "r_cfg:") {
		b.haClient.InvalidateCache()
		raw := strings.TrimPrefix(cb.Data, "r_cfg:")
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			srcType := parts[0]
			srcID := parts[1]
			ref := parts[2]
			page, _ := strconv.Atoi(parts[3])
			entityID := b.resolveEntityRef(ref)
			_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Обновляю настройки..."))
			b.showDeviceConfigMenu(cb.ID, chatID, messageID, entityID, srcType, srcID, page)
			return
		}
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
		raw := strings.TrimPrefix(cb.Data, "cft:")
		parts := strings.Split(raw, ":")
		entityID := b.resolveEntityRef(parts[0])
		domain := strings.Split(entityID, ".")[0]
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Переключаю..."))
		ok, msg := b.haClient.CallService(domain, "toggle", map[string]interface{}{"entity_id": entityID})
		if !ok {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}
		if len(parts) >= 5 {
			srcType := parts[1]
			srcID := parts[2]
			devEntityID := b.resolveEntityRef(parts[3])
			page, _ := strconv.Atoi(parts[4])
			b.showDeviceConfigMenu(cb.ID, chatID, messageID, devEntityID, srcType, srcID, page)
			return
		}
		b.refreshDeviceSettingsForEntity(cb.ID, chatID, messageID, entityID, 0)
		return
	}

	if strings.HasPrefix(cb.Data, "cfp:") {
		raw := strings.TrimPrefix(cb.Data, "cfp:")
		parts := strings.Split(raw, ":")
		entityID := b.resolveEntityRef(parts[0])
		domain := strings.Split(entityID, ".")[0]
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Выполняю..."))
		ok, msg := b.haClient.CallService(domain, "press", map[string]interface{}{"entity_id": entityID})
		if !ok {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}
		if len(parts) >= 5 {
			srcType := parts[1]
			srcID := parts[2]
			devEntityID := b.resolveEntityRef(parts[3])
			page, _ := strconv.Atoi(parts[4])
			b.showDeviceConfigMenu(cb.ID, chatID, messageID, devEntityID, srcType, srcID, page)
			return
		}
		b.refreshDeviceSettingsForEntity(cb.ID, chatID, messageID, entityID, 0)
		return
	}

	if strings.HasPrefix(cb.Data, "cfs:") {
		raw := strings.TrimPrefix(cb.Data, "cfs:")
		parts := strings.Split(raw, ":")
		entityID := b.resolveEntityRef(parts[0])
		domain := strings.Split(entityID, ".")[0]
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Переключаю режим..."))
		ok, msg := b.haClient.CallService(domain, "select_next", map[string]interface{}{"entity_id": entityID, "cycle": true})
		if !ok {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}
		if len(parts) >= 5 {
			srcType := parts[1]
			srcID := parts[2]
			devEntityID := b.resolveEntityRef(parts[3])
			page, _ := strconv.Atoi(parts[4])
			b.showDeviceConfigMenu(cb.ID, chatID, messageID, devEntityID, srcType, srcID, page)
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

			if len(parts) >= 6 {
				srcType := parts[2]
				srcID := parts[3]
				devEntityID := b.resolveEntityRef(parts[4])
				page, _ := strconv.Atoi(parts[5])
				b.showDeviceConfigMenu(cb.ID, chatID, messageID, devEntityID, srcType, srcID, page)
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

