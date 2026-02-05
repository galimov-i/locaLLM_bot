package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// OllamaRequest структура для запроса к Ollama API
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse структура для ответа от Ollama API
type OllamaResponse struct {
	Model     string    `json:"model"`
	CreatedAt string    `json:"created_at"`
	Response  string    `json:"response"`
	Done      bool      `json:"done"`
	Error     string    `json:"error,omitempty"`
	Context   []int     `json:"context,omitempty"`
}

// OllamaClient клиент для работы с Ollama API
type OllamaClient struct {
	URL   string
	Model string
}

// NewOllamaClient создает новый клиент Ollama с настройками из переменных окружения
func NewOllamaClient() *OllamaClient {
	url := os.Getenv("OLLAMA_URL")
	if url == "" {
		// Значение по умолчанию - localhost, так как бот обычно запускается на той же машине
		url = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "gemma3:1b"
	}

	return &OllamaClient{
		URL:   url,
		Model: model,
	}
}

// SendPrompt отправляет запрос к Ollama API и возвращает ответ
func (c *OllamaClient) SendPrompt(prompt string) (string, error) {
	reqBody := OllamaRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ошибка сериализации запроса: %w", err)
	}

	url := c.URL + "/api/generate"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 480 * time.Second, // Таймаут 8 минут для генерации
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка выполнения HTTP запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama API вернул статус %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("ошибка парсинга JSON ответа: %w", err)
	}

	if ollamaResp.Error != "" {
		return "", fmt.Errorf("ошибка от Ollama: %s", ollamaResp.Error)
	}

	if !ollamaResp.Done {
		return "", fmt.Errorf("ответ от Ollama не завершен")
	}

	return ollamaResp.Response, nil
}
