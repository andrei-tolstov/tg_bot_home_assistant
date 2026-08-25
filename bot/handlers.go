package bot

import (
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	}
}

func (b *Bot) handleStart(msg *tgbotapi.Message) {
	text := "👋 Привет! Я бот для управления твоим Home Assistant.\n\n" +
		"Вы можете использовать быстрые кнопки внизу экрана:\n" +
		"📊 <b>Статус HA</b> — проверка связи с сервером\n" +
		"💡 <b>Все устройства</b> — общий список всех устройств с кнопками\n" +
		"📁 <b>Устройства по доменам</b> — устройства по категориям (свет, розетки, автоматизации и др.)\n\n" +
		"Команды:\n" +
		"• <code>/status</code> — проверить подключение к HA\n" +
		"• <code>/devices</code> — общий список устройств\n" +
		"• <code>/domains</code> — категории устройств\n" +
		"• <code>/toggle &lt;entity_id&gt;</code> — ручное переключение по ID"

	replyMsg := tgbotapi.NewMessage(msg.Chat.ID, text)
	replyMsg.ParseMode = tgbotapi.ModeHTML
	replyMsg.ReplyMarkup = GetMainKeyboard()

	if _, err := b.api.Send(replyMsg); err != nil {
		log.Printf("Ошибка отправки start-сообщения: %v", err)
	}
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
	b.editMessage(editMsg)
}

func (b *Bot) showAllDevicesMenu(chatID int64, messageID int, edit bool, page int) {
	ok, devices, errMsg := b.haClient.GetControllableEntities()

	if !ok {
		text := fmt.Sprintf("❌ Не удалось получить список устройств:\n%s", html.EscapeString(errMsg))
		if edit {
			b.editMessage(tgbotapi.NewEditMessageText(chatID, messageID, text))
		} else {
			msg := tgbotapi.NewMessage(chatID, text)
			if _, err := b.api.Send(msg); err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
		}
		return
	}

	if len(devices) == 0 {
		text := "ℹ️ В Home Assistant пока не найдено управляемых устройств."
		if edit {
			b.editMessage(tgbotapi.NewEditMessageText(chatID, messageID, text))
		} else {
			msg := tgbotapi.NewMessage(chatID, text)
			if _, err := b.api.Send(msg); err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
		}
		return
	}

	pageSize := DefaultPageSize
	totalPages := (len(devices) + pageSize - 1) / pageSize
	if totalPages <= 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	var text string
	if totalPages > 1 {
		text = fmt.Sprintf(
			"💡 <b>Все доступные устройства:</b> (всего %d шт., стр. %d/%d)\nНажмите на устройство для переключения:",
			len(devices),
			page+1,
			totalPages,
		)
	} else {
		text = fmt.Sprintf(
			"💡 <b>Все доступные устройства:</b> (%d шт.)\nНажмите на устройство для переключения:",
			len(devices),
		)
	}

	keyboard := GetDevicesKeyboard(devices, page, pageSize)

	if edit {
		editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, keyboard)
		editMsg.ParseMode = tgbotapi.ModeHTML
		b.editMessage(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyMarkup = keyboard
		if _, err := b.api.Send(msg); err != nil {
			log.Printf("Ошибка отправки списка устройств: %v", err)
		}
	}
}

func (b *Bot) showDomainsMenu(chatID int64, messageID int, edit bool) {
	ok, grouped, errMsg := b.haClient.GetEntitiesGroupedByDomain()

	if !ok {
		text := fmt.Sprintf("❌ Не удалось получить категории устройств:\n%s", html.EscapeString(errMsg))
		if edit {
			b.editMessage(tgbotapi.NewEditMessageText(chatID, messageID, text))
		} else {
			msg := tgbotapi.NewMessage(chatID, text)
			if _, err := b.api.Send(msg); err != nil {
				log.Printf("Ошибка отправки категорий: %v", err)
			}
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
			b.editMessage(tgbotapi.NewEditMessageText(chatID, messageID, text))
		} else {
			msg := tgbotapi.NewMessage(chatID, text)
			if _, err := b.api.Send(msg); err != nil {
				log.Printf("Ошибка отправки категорий: %v", err)
			}
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
		editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, keyboard)
		editMsg.ParseMode = tgbotapi.ModeHTML
		b.editMessage(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyMarkup = keyboard
		if _, err := b.api.Send(msg); err != nil {
			log.Printf("Ошибка отправки категорий: %v", err)
		}
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
	totalPages := (len(devices) + pageSize - 1) / pageSize
	if totalPages <= 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

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
			page+1,
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

	keyboard := GetDomainDevicesKeyboard(domain, devices, page, pageSize)
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, keyboard)
	editMsg.ParseMode = tgbotapi.ModeHTML
	b.editMessage(editMsg)
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
		entityID := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}

		domain := strings.Split(entityID, ".")[0]

		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Переключаю..."))
		success, msg := b.haClient.CallService(domain, "toggle", map[string]interface{}{"entity_id": entityID})
		if !success {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}

		b.showAllDevicesMenu(chatID, messageID, true, page)
		return
	}

	if cb.Data == "b_dom" || cb.Data == "r_dom" {
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
		b.showDomainDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}

	if strings.HasPrefix(cb.Data, "dr:") {
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
		parts := strings.Split(raw, ":")
		entityID := parts[0]
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}

		domain := strings.Split(entityID, ".")[0]

		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "⏳ Переключаю..."))
		success, msg := b.haClient.CallService(domain, "toggle", map[string]interface{}{"entity_id": entityID})
		if !success {
			_, _ = b.api.Request(tgbotapi.NewCallbackWithAlert(cb.ID, fmt.Sprintf("❌ Ошибка: %s", msg)))
			return
		}

		b.showDomainDevicesMenu(cb.ID, chatID, messageID, domain, page)
		return
	}
}

func (b *Bot) handleToggle(msg *tgbotapi.Message) {
	args := strings.Fields(msg.CommandArguments())
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "ℹ️ Пожалуйста, укажите entity_id.\nПример: <code>/toggle light.living_room</code>")
		reply.ParseMode = tgbotapi.ModeHTML
		if _, err := b.api.Send(reply); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}
		return
	}

	entityID := strings.TrimSpace(args[0])
	if !strings.Contains(entityID, ".") {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "⚠️ Неверный формат entity_id. Идентификатор должен быть вида <code>domain.object_id</code> (например, <code>switch.relay</code>).")
		reply.ParseMode = tgbotapi.ModeHTML
		if _, err := b.api.Send(reply); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}
		return
	}

	domain := strings.Split(entityID, ".")[0]
	success, errMsg := b.haClient.CallService(domain, "toggle", map[string]interface{}{"entity_id": entityID})

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
		reply := tgbotapi.NewMessage(msg.Chat.ID, replyText)
		reply.ParseMode = tgbotapi.ModeHTML
		if _, err := b.api.Send(reply); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}
	} else {
		replyText := fmt.Sprintf(
			"❌ Не удалось переключить <code>%s</code>.\nПричина: %s",
			html.EscapeString(entityID),
			html.EscapeString(errMsg),
		)
		reply := tgbotapi.NewMessage(msg.Chat.ID, replyText)
		reply.ParseMode = tgbotapi.ModeHTML
		if _, err := b.api.Send(reply); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}
	}
}
