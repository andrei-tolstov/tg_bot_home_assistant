package bot

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"tg_bot_home_assistant/config"
	"tg_bot_home_assistant/ha"
)

// Bot представляет экземпляр Telegram-бота для управления Home Assistant.
type Bot struct {
	cfg      *config.Config
	haClient *ha.Client
	api      *tgbotapi.BotAPI
}

// NewBot создает и инициализирует новый экземпляр Bot.
func NewBot(cfg *config.Config, haClient *ha.Client) (*Bot, error) {
	// Явный таймаут для Telegram API: опрос Long Polling длится до 30с,
	// поэтому таймаут 45с гарантирует разрыв мертвого TCP-сокета при сетевых сбоях.
	httpClient := &http.Client{
		Timeout: 45 * time.Second,
	}
	api, err := tgbotapi.NewBotAPIWithClient(cfg.TelegramBotToken, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания экземпляра Telegram Bot API: %w", err)
	}

	log.Printf("Авторизован как Telegram-бот @%s (ID: %d)", api.Self.UserName, api.Self.ID)

	return &Bot{
		cfg:      cfg,
		haClient: haClient,
		api:      api,
	}, nil
}

// Start запускает опрос обновлений Telegram (Long Polling) с поддержкой graceful shutdown.
func (b *Bot) Start(ctx context.Context) {
	// Отправка уведомления о запуске бота пользователю (для фиксации включения электричества)
	b.sendStartupNotification(ctx)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := b.api.GetUpdatesChan(u)

	log.Println("Telegram-бот успешно запущен и слушает обновления...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Останавливаем обработку обновлений бота...")
			b.api.StopReceivingUpdates()
			return
		case update, ok := <-updates:
			if !ok {
				log.Println("Канал обновлений Telegram закрыт.")
				return
			}
			go b.safeHandleUpdate(update)
		}
	}
}

// formatStartupMessage формирует текст сообщения при запуске бота.
func formatStartupMessage(t time.Time) string {
	return fmt.Sprintf("⚡️ <b>Бот Home Assistant запущен!</b>\n\n🕒 <b>Время запуска:</b> <code>%s</code>\n💡 <i>Электричество включено, бот успешно возобновил работу.</i>", t.Format("02.01.2006 15:04:05"))
}

// sendStartupNotification отправляет уведомление о старте бота авторизованному пользователю.
func (b *Bot) sendStartupNotification(ctx context.Context) {
	if !b.cfg.NotifyOnStartup || b.cfg.AllowedUserID == 0 {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Паника при отправке уведомления о запуске: %v\nСтек:\n%s", r, debug.Stack())
			}
		}()

		text := formatStartupMessage(time.Now())
		msg := tgbotapi.NewMessage(b.cfg.AllowedUserID, text)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyMarkup = GetMainKeyboard()

		const maxAttempts = 3
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if _, err := b.api.Send(msg); err != nil {
				log.Printf("Попытка %d/%d: ошибка отправки уведомления о запуске (chat_id=%d): %v", attempt, maxAttempts, b.cfg.AllowedUserID, err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			log.Printf("Уведомление о запуске бота успешно отправлено пользователю %d", b.cfg.AllowedUserID)
			return
		}
		log.Printf("Не удалось отправить уведомление о запуске бота после %d попыток", maxAttempts)
	}()
}

// safeHandleUpdate безопасно обрабатывает обновление в отдельной горутине с защитой от паники и проверкой прав доступа.
func (b *Bot) safeHandleUpdate(update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Паника при обработке обновления: %v\nСтек:\n%s", r, debug.Stack())
		}
	}()

	user := update.SentFrom()
	if user == nil {
		return
	}

	// Проверка прав доступа: разрешен только пользователь из ALLOWED_USER_ID
	if user.ID != b.cfg.AllowedUserID {
		log.Printf("Доступ запрещен для пользователя id=%d (@%s)", user.ID, user.UserName)
		if update.CallbackQuery != nil {
			alert := tgbotapi.NewCallbackWithAlert(update.CallbackQuery.ID, "⛔ Доступ запрещен.")
			_, _ = b.api.Request(alert)
		}
		return
	}

	if update.Message != nil {
		b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		b.handleCallbackQuery(update.CallbackQuery)
	}
}

// editMessage безопасно редактирует сообщение, игнорируя ошибку 'message is not modified'.
func (b *Bot) editMessage(msg tgbotapi.Chattable) {
	_, err := b.api.Request(msg)
	if err != nil {
		if !strings.Contains(err.Error(), "message is not modified") {
			log.Printf("Ошибка редактирования сообщения: %v", err)
		}
	}
}
