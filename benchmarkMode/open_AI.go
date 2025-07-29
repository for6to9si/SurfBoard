package benchmarkMode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/net/proxy"
)

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func main() {
	// Проверка прямого доступа к OpenAI API
	fmt.Println("Проверка прямого доступа к OpenAI API...")
	if err := checkOpenAI("https://api.openai.com/v1/chat/completions", os.Getenv("OPENAI_API_KEY"), "gpt-3.5-turbo", false); err != nil {
		fmt.Printf("Ошибка при подключении к OpenAI: %v\n", err)
	} else {
		fmt.Println("Успешное подключение к OpenAI!")
	}

	// Проверка через DigitalOcean Serverless Inference
	fmt.Println("\nПроверка через DigitalOcean Serverless Inference...")
	if err := checkOpenAI("https://inference.do-ai.run/v1/chat/completions", os.Getenv("DIGITAL_OCEAN_MODEL_ACCESS_KEY"), "gpt-4o", false); err != nil {
		fmt.Printf("Ошибка при подключении к DigitalOcean Inference: %v\n", err)
	} else {
		fmt.Println("Успешное подключение к DigitalOcean Inference!")
	}

	// Проверка через SOCKS5-прокси
	fmt.Println("\nПроверка через SOCKS5-прокси (127.0.0.1:1080)...")
	if err := checkOpenAI("https://api.openai.com/v1/chat/completions", os.Getenv("OPENAI_API_KEY"), "gpt-3.5-turbo", true); err != nil {
		fmt.Printf("Ошибка при подключении через прокси: %v\n", err)
	} else {
		fmt.Println("Успешное подключение через SOCKS5-прокси!")
	}
}

func checkOpenAI(url, apiKey, model string, useProxy bool) error {
	var client *http.Client
	if useProxy {
		// Настройка SOCKS5-прокси
		dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1080", nil, proxy.Direct)
		if err != nil {
			return fmt.Errorf("ошибка настройки прокси: %v", err)
		}
		transport := &http.Transport{
			Dial: dialer.Dial,
		}
		client = &http.Client{
			Transport: transport,
		}
	} else {
		client = &http.Client{}
	}

	// Подготовка тестового запроса
	reqBody := ChatRequest{
		Model: model,
		Messages: []Message{
			{Role: "user", Content: "Test"},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("ошибка при сериализации JSON: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("ошибка при создании запроса: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка при выполнении запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		switch resp.StatusCode {
		case 403:
			return fmt.Errorf("ошибка 403: Страна, регион или территория не поддерживается (тело ответа: %s)", string(bodyBytes))
		case 429:
			return fmt.Errorf("ошибка 429: Превышен лимит запросов, проверьте ваш тарифный план и данные биллинга (тело ответа: %s)", string(bodyBytes))
		default:
			return fmt.Errorf("неуспешный статус код: %d, тело ответа: %s", resp.StatusCode, string(bodyBytes))
		}
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return fmt.Errorf("ошибка при декодировании ответа: %v", err)
	}

	if len(chatResp.Choices) > 0 {
		fmt.Println("Ответ от API:", chatResp.Choices[0].Message.Content)
	}
	return nil
}
