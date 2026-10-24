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
