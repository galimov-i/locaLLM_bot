package main

// SplitMessage разбивает длинный текст на части по maxLen символов.
// Старается разбивать по переносам строк для лучшей читаемости.
func SplitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
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

		parts = append(parts, current[:splitPos])
		current = current[splitPos:]
	}

	if len(current) > 0 {
		parts = append(parts, current)
	}

	return parts
}
