package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// ipv4OnlyTransport — HTTP-транспорт, принудительно использующий только IPv4.
// Решает проблему таймаутов при подключении к Telegram API через IPv6.
var ipv4OnlyTransport = &http.Transport{
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		return dialer.DialContext(ctx, "tcp4", addr)
	},
	MaxIdleConns:        100,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

// useIPv4Only определяет, нужно ли принудительно использовать только IPv4.
// Управляется переменной окружения USE_IPV4_ONLY (по умолчанию true).
func useIPv4Only() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("USE_IPV4_ONLY")))
	// По умолчанию true — IPv4 only, так как IPv6 часто вызывает таймауты
	if v == "" || v == "true" || v == "1" || v == "yes" {
		return true
	}
	return false
}

// newHTTPClient создаёт HTTP-клиент с заданным таймаутом.
// Если USE_IPV4_ONLY=true (по умолчанию), использует только IPv4-соединения.
func newHTTPClient(timeout time.Duration) *http.Client {
	client := &http.Client{
		Timeout: timeout,
	}
	if useIPv4Only() {
		client.Transport = ipv4OnlyTransport
	}
	return client
}

func init() {
	if useIPv4Only() {
		log.Println("Режим IPv4-only включён (USE_IPV4_ONLY=true)")
	}
}

// SplitMessage разбивает длинный текст на части по maxLen символов.
// Старается разбивать по переносам строк для лучшей читаемости.
func SplitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	if maxLen <= 0 {
		maxLen = 4000 // Защита от некорректного значения
	}

	var parts []string
	current := text

	for len(current) > maxLen {
		// Ищем последний перенос строки в пределах maxLen
		splitPos := maxLen
		for i := maxLen; i > 0 && i > maxLen-100; i-- {
			if current[i-1] == '\n' {
				splitPos = i
				break
			}
		}

		// Если не нашли перенос строки, ищем пробел
		if splitPos == maxLen {
			for i := maxLen; i > 0 && i > maxLen-50; i-- {
				if current[i-1] == ' ' {
					splitPos = i
					break
				}
			}
		}

		// Если все еще не нашли подходящее место и текст слишком длинный,
		// принудительно разбиваем по maxLen, чтобы избежать бесконечного цикла
		if splitPos == maxLen && len(current) > maxLen {
			splitPos = maxLen
		}

		parts = append(parts, current[:splitPos])
		current = current[splitPos:]
		
		// Пропускаем начальные пробелы/переносы строк в следующей части
		for len(current) > 0 && (current[0] == ' ' || current[0] == '\n') {
			current = current[1:]
		}
	}

	if len(current) > 0 {
		parts = append(parts, current)
	}

	return parts
}
