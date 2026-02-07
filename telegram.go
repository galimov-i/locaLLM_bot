package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// maxResponseSize ограничивает размер ответа от API (10 МБ)
	maxResponseSize = 10 * 1024 * 1024
)

// Telegram API структуры
type Update struct {
	UpdateID int64    `json:"update_id"`
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
	OK          bool        `json:"ok"`
	Result      interface{} `json:"result,omitempty"`
	Description string      `json:"description,omitempty"`
}

// TelegramBot структура для работы с Telegram Bot API
type TelegramBot struct {
	Token        string
	APIURL       string
	Ollama       *OllamaClient
	LastUpdate   int64
	AllowedUsers map[int64]bool
	rateLimiter  map[int64][]time.Time
	mu           sync.Mutex
	maxRequests  int
	rateWindow   time.Duration
	maxPromptLen int
}

// NewTelegramBot создает новый экземпляр бота
func NewTelegramBot(token string) *TelegramBot {
	// Парсинг списка разрешённых пользователей
	allowedUsers := make(map[int64]bool)
	if ids := os.Getenv("ALLOWED_USER_IDS"); ids != "" {
		for _, idStr := range strings.Split(ids, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				allowedUsers[id] = true
			}
		}
		log.Printf("Настроен список разрешённых пользователей: %d пользователь(ей)", len(allowedUsers))
	} else {
		log.Println("ВНИМАНИЕ: ALLOWED_USER_IDS не задан — бот доступен всем пользователям")
	}

	// Настройка rate limiting
	maxReq := 10
	if v := os.Getenv("RATE_LIMIT_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxReq = n
		}
	}

	rateWindow := time.Minute
	if v := os.Getenv("RATE_LIMIT_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			rateWindow = d
		}
	}

	// Настройка максимальной длины промпта
	maxPromptLen := 4096
	if v := os.Getenv("MAX_PROMPT_LENGTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPromptLen = n
		}
	}

	return &TelegramBot{
		Token:        token,
		APIURL:       "https://api.telegram.org/bot" + token,
		Ollama:       NewOllamaClient(),
		LastUpdate:   0,
		AllowedUsers: allowedUsers,
		rateLimiter:  make(map[int64][]time.Time),
		maxRequests:  maxReq,
		rateWindow:   rateWindow,
		maxPromptLen: maxPromptLen,
	}
}

// sanitizeError удаляет токен бота из сообщений об ошибках,
// чтобы токен не утёк в логи или к пользователям
func (bot *TelegramBot) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	sanitized := strings.ReplaceAll(err.Error(), bot.Token, "[REDACTED]")
	return fmt.Errorf("%s", sanitized)
}

// isUserAllowed проверяет, разрешён ли пользователь.
// Если ALLOWED_USER_IDS не задан (список пуст), доступ разрешён всем.
func (bot *TelegramBot) isUserAllowed(message *Message) bool {
	if len(bot.AllowedUsers) == 0 {
		return true
	}
	if message.From != nil {
		return bot.AllowedUsers[message.From.ID]
	}
	return bot.AllowedUsers[message.Chat.ID]
}

// checkRateLimit проверяет, не превышен ли лимит запросов для пользователя.
// Использует алгоритм скользящего окна.
func (bot *TelegramBot) checkRateLimit(userID int64) bool {
	bot.mu.Lock()
	defer bot.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-bot.rateWindow)

	// Фильтруем записи старше окна
	var recent []time.Time
	for _, t := range bot.rateLimiter[userID] {
		if t.After(windowStart) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= bot.maxRequests {
		bot.rateLimiter[userID] = recent
		return false
	}

	bot.rateLimiter[userID] = append(recent, now)
	return true
}

// GetUpdates получает обновления от Telegram через long polling
func (bot *TelegramBot) GetUpdates() ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=30", bot.APIURL, bot.LastUpdate+1)

	// Используем клиент с таймаутом: 30с long polling + 10с запас
	client := &http.Client{
		Timeout: 40 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к Telegram API: %w", bot.sanitizeError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return nil, fmt.Errorf("Telegram API вернул статус %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
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

	// Проверяем, что result не nil
	if telegramResp.Result == nil {
		return []Update{}, nil
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
		return fmt.Errorf("ошибка создания HTTP запроса: %w", bot.sanitizeError(err))
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения HTTP запроса: %w", bot.sanitizeError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return fmt.Errorf("Telegram API вернул статус %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
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

	// Проверка авторизации пользователя
	if !bot.isUserAllowed(message) {
		log.Printf("Отклонён запрос от неавторизованного пользователя (chat_id: %d)", message.Chat.ID)
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
			"Просто отправь мне сообщение, и я передам его модели для генерации ответа.\n\n" +
			"Используй /help для получения справки."
		if err := bot.SendMessage(chatID, msg); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", bot.sanitizeError(err))
		}

	case "/help":
		msg := "Доступные команды:\n\n" +
			"/start - приветственное сообщение\n" +
			"/help - эта справка\n\n" +
			"Любое другое сообщение будет отправлено в Ollama для генерации ответа."
		if err := bot.SendMessage(chatID, msg); err != nil {
			log.Printf("Ошибка отправки сообщения: %v", bot.sanitizeError(err))
		}

	default:
		// Неизвестная команда - обрабатываем как обычный текст
		bot.handleTextMessage(chatID, command)
	}
}

// handleTextMessage обрабатывает текстовые сообщения
func (bot *TelegramBot) handleTextMessage(chatID int64, text string) {
	// Проверка rate limit
	if !bot.checkRateLimit(chatID) {
		bot.SendMessage(chatID, "Слишком много запросов. Пожалуйста, подождите немного.")
		return
	}

	// Проверка длины промпта
	if len(text) > bot.maxPromptLen {
		bot.SendMessage(chatID, fmt.Sprintf("Сообщение слишком длинное. Максимальная длина: %d символов.", bot.maxPromptLen))
		return
	}

	// Отправляем сообщение о том, что запрос обрабатывается
	bot.SendMessage(chatID, "Обрабатываю запрос...")

	// Отправляем запрос в Ollama
	response, err := bot.Ollama.SendPrompt(text)
	if err != nil {
		// Логируем полную ошибку на сервере, пользователю — общее сообщение
		log.Printf("Ошибка от Ollama для chat %d: %v", chatID, err)
		if sendErr := bot.SendMessage(chatID, "Произошла ошибка при обработке запроса. Попробуйте позже."); sendErr != nil {
			log.Printf("Ошибка отправки сообщения об ошибке: %v", bot.sanitizeError(sendErr))
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
			log.Printf("Ошибка отправки части сообщения: %v", bot.sanitizeError(err))
		}
		// Небольшая задержка между сообщениями, чтобы не превысить rate limit
		if i < len(parts)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
}
