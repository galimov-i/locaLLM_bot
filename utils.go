package main

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
