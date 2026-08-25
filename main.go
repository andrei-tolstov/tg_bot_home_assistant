package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tg_bot_home_assistant/bot"
	"tg_bot_home_assistant/config"
	"tg_bot_home_assistant/ha"
)

func main() {
	// Настройка стандартного логгера
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("Запуск бота Home Assistant...")

	// Загрузка конфигурации из .env и переменных окружения
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	// Инициализация клиента Home Assistant API
	haClient := ha.NewClient(cfg.HAURL, cfg.HAToken)

	// Инициализация Telegram-бота
	tgBot, err := bot.NewBot(cfg, haClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации Telegram бота: %v", err)
	}

	// Настройка graceful shutdown по сигналам SIGINT / SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Получен сигнал завершения (%v), останавливаем бота...", sig)
		cancel()
	}()

	// Запуск бота
	tgBot.Start(ctx)
	log.Println("Бот успешно остановлен.")
}
