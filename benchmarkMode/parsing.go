package benchmarkMode

import (
	"encoding/json"
	"fmt"
	"github.com/for6to9si/vpnparser/pkgs/outbound"
	"net/url"
	"os"
	"strings"
	"unicode"
)

// replaceInvalidChars заменяет недопустимые символы в имени файла на подчёркивания
func replaceInvalidChars(name string) string {
	// Недопустимые символы для имён файлов в большинстве ОС
	invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\n", "\r", "\t"}
	// Заменяем недопустимые символы
	for _, char := range invalidChars {
		name = strings.ReplaceAll(name, char, "_")
	}
	// Удаляем или заменяем непечатаемые символы и пробелы в начале/конце
	var result strings.Builder
	for _, r := range name {
		if unicode.IsPrint(r) && !unicode.IsControl(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	// Удаляем пробелы в начале и конце
	return strings.TrimSpace(result.String())
}

func extractComment(input string) string {
	// Разделяем строку по символу '#'
	parts := strings.SplitN(input, "#", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func createFile(filename string) error {
	// Очищаем имя файла от недопустимых символов
	cleanedFilename := replaceInvalidChars(filename)
	if cleanedFilename == "" {
		return fmt.Errorf("имя файла пустое после очистки")
	}

	// Проверяем длину имени файла
	if len(cleanedFilename) > 255 {
		cleanedFilename = cleanedFilename[:255]
	}

	// Создаём файл
	file, err := os.Create(cleanedFilename)
	if err != nil {
		return fmt.Errorf("ошибка при создании файла '%s': %v", cleanedFilename, err)
	}
	defer file.Close()
	return nil
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

// Go function parses VLESS URIs and returns formatted JSON strings
func Go(vlessURI []string) []string {

	for i, input := range vlessURI {

		// Check if config is already a JSON string
		var jsonData []byte
		var err error

		// Обработка каждой строки

		comment := extractComment(input)
		if comment == "" {
			fmt.Printf("Строка %d: Пропущена (нет комментария после #)\n", i+1)
			continue
		}

		// Декодируем URL-кодированную строку
		decodedComment, err := decodeURLComment(comment)
		if err != nil {
			fmt.Printf("Строка %d: Ошибка декодирования: %v\n", i+1, err)
			continue
		}

		// Initialize and parse outbound configuration
		ob := outbound.GetOutbound(outbound.XrayCore, input)
		ob.Parse(input)

		// Get the outbound configuration
		config := ob.GetOutboundStr()

		// Check if config is already a JSON string
		//var jsonData []byte
		//var err error

		// Try to treat config as a JSON string first
		var temp map[string]interface{}
		if err := json.Unmarshal([]byte(config), &temp); err == nil {
			// If config is a valid JSON string, re-serialize it with proper formattin
			temp["tag"] = decodedComment
			jsonData, err = json.MarshalIndent(temp, "", "  ")
		} else {
			temp["tag"] = decodedComment
			// If config is not a JSON string, assume it's a struct and serialize it
			jsonData, err = json.MarshalIndent(config, "", "  ")
		}

		if err != nil {
			fmt.Printf("Error serializing to JSON: %v\n", err)
			return
		}

		// Print the formatted JSON to console
		fmt.Println(string(jsonData))

		fmt.Printf("Строка %d: %s\n", i+1, decodedComment)
		err = createFile(decodedComment)
		if err != nil {
			fmt.Printf("Ошибка при создании файла для строки %d: %v\n", i+1, err)
		} else {
			fmt.Printf("Файл '%s' успешно создан\n", replaceInvalidChars(decodedComment))
		}

		// Save the formatted JSON to a file for verification
		err = os.WriteFile(decodedComment, jsonData, 0644)
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			return
		}
		fmt.Println("JSON configuration saved to %v\n", decodedComment)
	}
}
