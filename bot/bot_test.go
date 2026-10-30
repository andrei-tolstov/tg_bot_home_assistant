package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"tg_bot_home_assistant/config"
)

func TestFormatStartupMessage(t *testing.T) {
	fixedTime := time.Date(2026, time.October, 30, 23, 45, 12, 0, time.UTC)
	msg := formatStartupMessage(fixedTime)

	expectedTimeStr := "30.10.2026 23:45:12"
	if !strings.Contains(msg, expectedTimeStr) {
		t.Errorf("expected msg to contain %q, got:\n%s", expectedTimeStr, msg)
	}

	if !strings.Contains(msg, "⚡️ <b>Бот Home Assistant запущен!</b>") {
		t.Errorf("expected msg to contain header, got:\n%s", msg)
	}

	if !strings.Contains(msg, "Электричество включено") {
		t.Errorf("expected msg to mention electricity, got:\n%s", msg)
	}
}

func TestSendStartupNotification_Disabled(t *testing.T) {
	// Если уведомления отключены в конфигурации, метод не должен выполнять вызовов к api
	b := &Bot{
		cfg: &config.Config{
			AllowedUserID:   12345,
			NotifyOnStartup: false,
		},
		api: nil, // Если бы был вызов api, произошла бы nil-паника
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Не должно вызывать панику или сетевые запросы
	b.sendStartupNotification(ctx)
}

func TestSendStartupNotification_NoUser(t *testing.T) {
	// Если AllowedUserID == 0, метод не должен выполнять вызовов к api
	b := &Bot{
		cfg: &config.Config{
			AllowedUserID:   0,
			NotifyOnStartup: true,
		},
		api: nil, // Если бы был вызов api, произошла бы nil-паника
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Не должно вызывать панику или сетевые запросы
	b.sendStartupNotification(ctx)
}
