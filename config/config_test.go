package config

import (
	"os"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "test_token_123")
	os.Setenv("ALLOWED_USER_ID", "123456789")
	os.Setenv("HA_URL", "http://192.168.1.100:8123/")
	os.Setenv("HA_TOKEN", "test_ha_token")
	defer func() {
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("ALLOWED_USER_ID")
		os.Unsetenv("HA_URL")
		os.Unsetenv("HA_TOKEN")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.TelegramBotToken != "test_token_123" {
		t.Errorf("expected test_token_123, got %s", cfg.TelegramBotToken)
	}
	if cfg.AllowedUserID != 123456789 {
		t.Errorf("expected 123456789, got %d", cfg.AllowedUserID)
	}
	if cfg.HAURL != "http://192.168.1.100:8123" {
		t.Errorf("expected trailing slash removed, got %s", cfg.HAURL)
	}
	if cfg.HAToken != "test_ha_token" {
		t.Errorf("expected test_ha_token, got %s", cfg.HAToken)
	}
}

func TestLoadConfig_MissingEnv(t *testing.T) {
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	os.Unsetenv("ALLOWED_USER_ID")
	os.Unsetenv("HA_URL")
	os.Unsetenv("HA_TOKEN")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing environment variables, got nil")
	}
}

func TestLoadConfig_InvalidUserID(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "token")
	os.Setenv("ALLOWED_USER_ID", "not_a_number")
	os.Setenv("HA_URL", "http://localhost:8123")
	os.Setenv("HA_TOKEN", "token")
	defer func() {
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("ALLOWED_USER_ID")
		os.Unsetenv("HA_URL")
		os.Unsetenv("HA_TOKEN")
	}()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for non-numeric ALLOWED_USER_ID, got nil")
	}
}
