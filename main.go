package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Получаем токен из переменной окружения
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("Ошибка: переменная окружения TELEGRAM_BOT_TOKEN не установлена")
	}

	// Создаем экземпляр бота
	bot := NewTelegramBot(token)

	log.Println("Бот запущен. Ожидание сообщений...")

	// Настройка graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Создаем контекст для остановки горутины
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запускаем основной цикл в горутине
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				updates, err := bot.GetUpdates()
				if err != nil {
					log.Printf("Ошибка получения обновлений: %v", err)
					// Задержка перед повтором, чтобы не спамить запросами
					time.Sleep(5 * time.Second)
					continue
				}

				// Обрабатываем каждое обновление
				for _, update := range updates {
					if update.UpdateID > bot.LastUpdate {
						bot.LastUpdate = update.UpdateID
					}

					if update.Message != nil {
						bot.HandleMessage(update.Message)
					}
				}
			}
		}
	}()

	// Ожидаем сигнал завершения
	<-sigChan
	log.Println("Получен сигнал завершения. Останавливаю бота...")
	cancel()
	// Даем время горутине завершиться
	time.Sleep(1 * time.Second)
	log.Println("Бот остановлен.")
}
