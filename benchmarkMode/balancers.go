package benchmarkMode

import (
	"encoding/json"
	"fmt"
	"os"
)

// Структуры для шаблона JSON
type Config struct {
	BurstObservatory struct {
		PingConfig struct {
			Connectivity string `json:"connectivity"`
			Destination  string `json:"destination"`
			Interval     string `json:"interval"`
			Sampling     int    `json:"sampling"`
			Timeout      string `json:"timeout"`
		} `json:"pingConfig"`
		SubjectSelector []string `json:"subjectSelector"`
	} `json:"burstObservatory"`

	Routing struct {
		Balancers []struct {
			Tag      string   `json:"tag"`
			Selector []string `json:"selector"`
			Fallback string   `json:"fallbackTag"`
			Strategy struct {
				Type string `json:"type"`
			} `json:"strategy"`
		} `json:"balancers"`
	} `json:"routing"`
}

// Функция добавления элемента (без дубликатов)
func addUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

// Функция удаления элемента
func remove(slice []string, val string) []string {
	result := []string{}
	for _, v := range slice {
		if v != val {
			result = append(result, v)
		}
	}
	return result
}

func ModifyBalancerJson(filename string) []string {

	var results []string
	// Читаем файл temp_config.json
	data, err := os.ReadFile(filename)
	if err != nil {
		results = append(results, fmt.Sprintf("не удалось открыть temp_config.json: %s", err))
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		results = append(results, fmt.Sprintf("ошибка парсинга JSON: %s", err))
	}

	// Добавляем новый узел
	newNode := "trojan-france"
	cfg.BurstObservatory.SubjectSelector = addUnique(cfg.BurstObservatory.SubjectSelector, newNode)
	cfg.Routing.Balancers[0].Selector = addUnique(cfg.Routing.Balancers[0].Selector, newNode)

	// Удаляем ненужный узел
	cfg.BurstObservatory.SubjectSelector = remove(cfg.BurstObservatory.SubjectSelector, "test-vless")
	cfg.Routing.Balancers[0].Selector = remove(cfg.Routing.Balancers[0].Selector, "test-vless")

	// Конвертируем обратно в JSON
	output, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		results = append(results, fmt.Sprintf("ошибка сериализации JSON: %s", err))
	}

	// Сохраняем результат в config.json
	if err := os.WriteFile("config.json", output, 0644); err != nil {
		results = append(results, fmt.Sprintf("не удалось записать config.json: %s", err))
	}

	results = append(results, fmt.Sprintf("✅ Сгенерирован новый config.json"))

	return results
}
