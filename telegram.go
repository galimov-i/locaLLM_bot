package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Telegram API структуры
type Update struct {
	UpdateID int64   `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text,omitempty"`
	Date      int64  `json:"date"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	FirstName string `json:"first_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type SendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type TelegramResponse struct {
	OK     bool   `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Description string `json:"description,omitempty"`
}

// TelegramBot структура для работы с Telegram Bot API
type TelegramBot struct {
	Token      string
	APIURL     string
	Ollama     *OllamaClient
	LastUpdate int64
}

// NewTelegramBot создает новый экземпляр бота
func NewTelegramBot(token string) *TelegramBot {
	return &TelegramBot{
		Token:      token,
		APIURL:     "https://api.telegram.org/bot" + token,
		Ollama:     NewOllamaClient(),
		LastUpdate: 0,
	}
}

// GetUpdates получает обновления от Telegram через long polling
func (bot *TelegramBot) GetUpdates() ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=30", bot.APIURL, bot.LastUpdate+1)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к Telegram API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Telegram API вернул статус %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var telegramResp TelegramResponse
	if err := json.Unmarshal(body, &telegramResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	if !telegramResp.OK {
		return nil, fmt.Errorf("Telegram API вернул ошибку: %s", telegramResp.Description)
	}

	// Парсим result как массив обновлений
	resultBytes, err := json.Marshal(telegramResp.Result)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации result: %w", err)
	}

	var updates []Update
	if err := json.Unmarshal(resultBytes, &updates); err != nil {
		return nil, fmt.Errorf("ошибка парсинга обновлений: %w", err)
	}

	return updates, nil
}

// SendMessage отправляет сообщение в чат
func (bot *TelegramBot) SendMessage(chatID int64, text string) error {
	reqBody := SendMessageRequest{
		ChatID: chatID,
		Text:   text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("ошибка сериализации запроса: %w", err)
	}

	url := bot.APIURL + "/sendMessage"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения HTTP запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Telegram API вернул статус %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var telegramResp TelegramResponse
	if err := json.Unmarshal(body, &telegramResp); err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	if !telegramResp.OK {
		return fmt.Errorf("Telegram API вернул ошибку: %s", telegramResp.Description)
	}

	return nil
}

// HandleMessage обрабатывает входящее сообщение
func (bot *TelegramBot) HandleMessage(message *Message) {
	if message == nil || message.Chat == nil {
		return
	}

	chatID := message.Chat.ID
	text := message.Text

	// Обработка команд
	if len(text) > 0 && text[0] == '/' {
		bot.handleCommand(chatID, text)
		return
	}

	// Обработка обычных сообщений
	if text != "" {
		bot.handleTextMessage(chatID, text)
	}
}

// handleCommand обрабатывает команды бота
func (bot *TelegramBot) handleCommand(chatID int64, command string) {
	switch command {
	case "/start":
		msg := "Привет! Я бот для работы с Ollama LLM.\n\n" +
			"Просто отправь мне сообщение, и я передам его модели gemma3:1b для генерации ответа.\n\n" +
			"Используй /help для получения справки."
		if err := bot.SendMessage(chatID, msg); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}

	case "/help":
		msg := "Доступные команды:\n\n" +
			"/start - приветственное сообщение\n" +
			"/help - эта справка\n\n" +
			"Любое другое сообщение будет отправлено в Ollama для генерации ответа."
		if err := bot.SendMessage(chatID, msg); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", err)
		}

	default:
		// Неизвестная команда - обрабатываем как обычный текст
		bot.handleTextMessage(chatID, command)
	}
}

// handleTextMessage обрабатывает текстовые сообщения
func (bot *TelegramBot) handleTextMessage(chatID int64, text string) {
	// Отправляем сообщение о том, что запрос обрабатывается
	bot.SendMessage(chatID, "Обрабатываю запрос...")

	// Отправляем запрос в Ollama
	response, err := bot.Ollama.SendPrompt(text)
	if err != nil {
		errorMsg := fmt.Sprintf("Произошла ошибка при обращении к Ollama:\n\n%s", err.Error())
		if err := bot.SendMessage(chatID, errorMsg); err != nil {
			log.Printf("Ошибка отправки сообщения об ошибке: %v", err)
		}
		return
	}

	// Разбиваем длинные ответы на части
	parts := SplitMessage(response, 4000)
	
	// Отправляем каждую часть
	for i, part := range parts {
		if i == 0 && len(parts) > 1 {
			// Первая часть с указанием, что будет продолжение
			part = part + "\n\n[Продолжение следует...]"
		}
		if err := bot.SendMessage(chatID, part); err != nil {
			log.Printf("Ошибка отправки части сообщения: %v", err)
		}
		// Небольшая задержка между сообщениями, чтобы не превысить rate limit
		if i < len(parts)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
}
