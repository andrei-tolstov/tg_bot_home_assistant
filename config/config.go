package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config содержит параметры конфигурации бота и подключения к Home Assistant.
type Config struct {
	TelegramBotToken string
	AllowedUserID    int64
	HAURL            string
	HAToken          string
	NotifyOnStartup  bool
}

// LoadConfig загружает настройки из файла .env и переменных окружения.
func LoadConfig() (*Config, error) {
	// Пытаемся загрузить .env, если он существует (не считаем критической ошибкой, если файла нет, но переменные заданы в окружении)
	_ = godotenv.Load()

	tgToken := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	allowedUserStr := strings.TrimSpace(os.Getenv("ALLOWED_USER_ID"))
	haURL := strings.TrimSpace(os.Getenv("HA_URL"))
	haToken := strings.TrimSpace(os.Getenv("HA_TOKEN"))

	var missing []string
	if tgToken == "" {
		missing = append(missing, "TELEGRAM_BOT_TOKEN")
	}
	if allowedUserStr == "" {
		missing = append(missing, "ALLOWED_USER_ID")
	}
	if haURL == "" {
		missing = append(missing, "HA_URL")
	}
	if haToken == "" {
		missing = append(missing, "HA_TOKEN")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("не все обязательные переменные окружения заданы в .env файле: %s", strings.Join(missing, ", "))
	}

	allowedUserID, err := strconv.ParseInt(allowedUserStr, 10, 64)
	if err != nil {
		return nil, errors.New("ALLOWED_USER_ID должен быть числом")
	}

	notifyStartupStr := strings.ToLower(strings.TrimSpace(os.Getenv("NOTIFY_ON_STARTUP")))
	notifyOnStartup := true
	if notifyStartupStr == "false" || notifyStartupStr == "0" || notifyStartupStr == "no" {
		notifyOnStartup = false
	}

	return &Config{
		TelegramBotToken: tgToken,
		AllowedUserID:    allowedUserID,
		HAURL:            strings.TrimRight(haURL, "/"),
		HAToken:          haToken,
		NotifyOnStartup:  notifyOnStartup,
	}, nil
}
