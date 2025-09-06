package benchmarkMode

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/for6to9si/vpnparser/pkgs/outbound"
)

// replaceInvalidChars заменяет недопустимые символы в имени файла на подчёркивания
func replaceInvalidChars(name string) (string, error) {
	// Рекурсивно декодируем URL-кодированные символы
	decodedName := name
	for {
		newDecoded, err := url.QueryUnescape(decodedName)
		if err != nil || newDecoded == decodedName {
			break
		}
		decodedName = newDecoded
	}

	// Недопустимые символы для имён файлов в большинстве ОС
	invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\n", "\r", "\t"}

	// Заменяем недопустимые символы
	for _, char := range invalidChars {
		decodedName = strings.ReplaceAll(decodedName, char, "_")
	}

	// Удаляем или заменяем непечатаемые символы и эмодзи
	var result strings.Builder
	for _, r := range decodedName {
		if unicode.IsPrint(r) && !unicode.IsControl(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}

	// Удаляем пробелы в начале и конце
	cleaned := strings.TrimSpace(result.String())

	// Удаляем повторяющиеся подчёркивания
	cleaned = strings.ReplaceAll(cleaned, "__", "_")
	cleaned = strings.ReplaceAll(cleaned, "__", "_") // Дважды на случай тройных

	// Ограничиваем длину имени файла
	if len(cleaned) > 100 {
		cleaned = cleaned[:100]
	}

	return cleaned, nil
}

func extractComment(input string) string {
	// Разделяем строку по символу '#'
	parts := strings.SplitN(input, "#", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// decodeURLComment декодирует URL-кодированную строку
func decodeURLComment(comment string) (string, error) {
	// Декодируем URL-кодированные последовательности
	decoded, err := url.QueryUnescape(comment)
	if err != nil {
		return "", fmt.Errorf("ошибка декодирования URL: %v", err)
	}
	return decoded, nil
}

// Parses function parses VLESS URIs and returns formatted JSON strings
func Parses(vlessURI []string, path string) []string {
	var results []string
	var tags []string

	for i, input := range vlessURI {

		// Обработка каждой строки

		comment := extractComment(input)
		if comment == "" {
			results = append(results, fmt.Sprintf("Строка %d: Пропущена (нет комментария после #)", i+1))
			comment = fmt.Sprintf("config_%d", i+1)
			timestamp := time.Now().Format("20060102_150405") // ГГГГММДД_ЧЧММСС
			comment = fmt.Sprintf("%s", timestamp)
		}

		// Декодируем URL-кодированную строку
		decodedComment, err := replaceInvalidChars(comment)
		if err != nil {
			results = append(results, fmt.Sprintf("Строка %d: Ошибка декодирования: %v", i+1, err))
			continue
		}

		// Инициализируем и парсим конфигурацию (с обработкой ошибок)
		ob := outbound.GetOutbound(outbound.XrayCore, input)
		if ob == nil {
			results = append(results, fmt.Sprintf("Строка %d: Неподдерживаемый протокол: %s\n", i+1, input))
			continue
		}
		// Парсим с обработкой паники
		func() {
			defer func() {
				if r := recover(); r != nil {
					results = append(results, fmt.Sprintf("Строка %d: Ошибка парсинга (panic recovered): %v\n", i+1, r))
				}
			}()
			ob.Parse(input)
		}()

		// Get the outbound configuration
		config := ob.GetOutboundStr()

		if config == "" {
			results = append(results, fmt.Sprintf("Строка %d: Не удалось распарсить конфигурацию\n", i+1))
			continue
		}

		// Check if config is already a JSON string
		var jsonData []byte
		//var err error

		// Try to treat config as a JSON string first
		var temp map[string]interface{}
		if err := json.Unmarshal([]byte(config), &temp); err == nil {
			// If config is a valid JSON string, re-serialize it with proper formattin

			decodedComment = fmt.Sprintf("%s_%s_%d", decodedComment, temp["protocol"], i+1)
			temp["tag"] = decodedComment

			// Создаем структуру с outbounds
			outboundWrapper := map[string]interface{}{
				"outbounds": []interface{}{temp},
			}

			jsonData, err = json.MarshalIndent(outboundWrapper, "", "  ")
		} else {
			// Если config не JSON, создаем новый объект
			temp = map[string]interface{}{
				"config": config,
				"tag":    decodedComment,
			}

			// Обертываем в outbounds
			outboundWrapper := map[string]interface{}{
				"outbounds": []interface{}{temp},
			}
			// If config is not a JSON string, assume it's a struct and serialize it
			jsonData, err = json.MarshalIndent(outboundWrapper, "", "  ")
		}

		// Print the formatted JSON to console
		results = append(results, string(jsonData))
		results = append(results, fmt.Sprintf("Строка %d: %s", i+1, decodedComment))

		// сохраняем комментарий
		tags = append(tags, decodedComment)

		//err = createFile(decodedComment)
		//if err != nil {
		//	fmt.Printf("Ошибка при создании файла для строки %d: %v\n", i+1, err)
		//} else {
		//	fmt.Printf("Файл '%s' успешно создан\n", decodedComment)
		//}

		// Создаем файл с очищенным именем
		cleanedName, err := replaceInvalidChars(decodedComment)
		if cleanedName == "" {
			cleanedName = fmt.Sprintf("config_%d", i+1)
		}

		// Формируем полный путь к файлу
		fullpath := filepath.Join(path, "!vpn"+cleanedName+".json")

		// Save the formatted JSON to a file for verification
		err = os.WriteFile(fullpath, jsonData, 0644)
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
		}
		fmt.Println("JSON configuration saved to %v\n", decodedComment)
	}

	return tags
}

func GetTags(path string) []string {
	// директория, в которой ищем
	dir := path

	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	var result []string
	for _, f := range files {
		if f.IsDir() {
			continue
		}

		name := f.Name()

		if strings.HasPrefix(name, "!vpn") && strings.HasSuffix(name, ".json") {
			// убираем префикс и суффикс
			trimmed := strings.TrimPrefix(name, "!vpn")
			trimmed = strings.TrimSuffix(trimmed, ".json")
			result = append(result, trimmed)
		}
	}

	return result
}
