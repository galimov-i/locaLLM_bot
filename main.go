package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
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

	// Канал для остановки основного цикла
	done := make(chan bool)

	// Запускаем основной цикл в горутине
	go func() {
		for {
			updates, err := bot.GetUpdates()
			if err != nil {
				log.Printf("Ошибка получения обновлений: %v", err)
				// Продолжаем работу после ошибки
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
	}()

	// Ожидаем сигнал завершения
	<-sigChan
	log.Println("Получен сигнал завершения. Останавливаю бота...")
	close(done)
	log.Println("Бот остановлен.")
}
