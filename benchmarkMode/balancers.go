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
		DomainStrategy string `json:"domainStrategy,omitempty"`
		Balancers      []struct {
			Tag      string   `json:"tag"`
			Selector []string `json:"selector"`
			Fallback string   `json:"fallbackTag"`
			Strategy struct {
				Type string `json:"type"`
			} `json:"strategy"`
		} `json:"balancers"`
		Rules []map[string]interface{} `json:"rules,omitempty"`
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

// Функция очистки всех узлов
func clearNodes(cfg *Config) {
	cfg.BurstObservatory.SubjectSelector = []string{}
	for i := range cfg.Routing.Balancers {
		cfg.Routing.Balancers[i].Selector = []string{}
	}
}

// Функция добавления доменов в правило с xwave-domain = true
func addDomainsToRules(cfg *Config, newDomains []string) ([]string, error) {
	var results []string

	// Если массив пустой — просто логируем
	if len(newDomains) == 0 {
		results = append(results, "⚠️  Список добавляемых доменов пуст.")
		return results, nil
	}

	foundXwaveRule := false
	foundXwaveKey := false // ← встречался ли ключ вообще

	for i, rule := range cfg.Routing.Rules {

		// Проверяем наличие "xwave-domain"
		flag, exists := rule["xwave-domain"]

		if exists {
			foundXwaveKey = true // ключ существует хотя бы в одном правиле
		}

		// Если ключа нет — продолжаем
		if !exists {
			continue
		}

		// Если "xwave-domain": "false" — просто игнорируем
		if flag == "false" {
			continue
		}

		// Работаем только с "xwave-domain": "true"
		if flag != "true" {
			continue
		}

		// Нашли нужное правило
		foundXwaveRule = true

		// --- Обработка доменов ---
		if domains, ok := rule["domain"].([]interface{}); ok {

			// проверяем дубликаты
			existsMap := map[string]bool{}
			for _, d := range domains {
				existsMap[d.(string)] = true
			}

			for _, nd := range newDomains {
				if !existsMap[nd] {
					domains = append(domains, nd)
					results = append(results, fmt.Sprintf("Добавлен домен: %s", nd))
				}
			}

			cfg.Routing.Rules[i]["domain"] = domains

		} else {
			// если доменов не было — создаём новое поле
			domains := []interface{}{}
			for _, nd := range newDomains {
				domains = append(domains, nd)
				results = append(results, fmt.Sprintf("Добавлен домен: %s", nd))
			}
			cfg.Routing.Rules[i]["domain"] = domains
		}
	}

	// --- ОШИБКИ ---

	// Если ключ "xwave-domain" вообще отсутствует в правилах
	if !foundXwaveKey {
		return results, fmt.Errorf("❌ В конфигурации json ни одно правило не содержит ключа \"xwave-domain\"")
	}

	// Если ключ есть, но нет ни одного "true"
	if !foundXwaveRule {
		return results, fmt.Errorf("⚠️  Ключ \"xwave-domain\" найден, но значение \"true\" отсутствует — домены не добавлены")
	}

	return results, nil
}

func ModifyBalancerJson(template string, filename string, vpns []string) []string {

	var results []string
	// Читаем файл temp_config.json
	data, err := os.ReadFile(template)
	if err != nil {
		results = append(results, fmt.Sprintf("не удалось открыть %s", template))
		return results
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		results = append(results, fmt.Sprintf("ошибка парсинга JSON: %s", err))
	}

	//Если нужно полностью очистить узлы:
	//	clearNodes(&cfg)

	// Добавляем все новые узлы
	for _, node := range vpns {
		cfg.BurstObservatory.SubjectSelector = addUnique(cfg.BurstObservatory.SubjectSelector, node)
		if len(cfg.Routing.Balancers) > 0 {
			cfg.Routing.Balancers[0].Selector = addUnique(cfg.Routing.Balancers[0].Selector, node)
		}
	}

	// Удаляем ненужный узел
	//removeNodes := []string{"test-vless"}
	//for _, node := range removeNodes {
	//	cfg.BurstObservatory.SubjectSelector = remove(cfg.BurstObservatory.SubjectSelector, node)
	//if len(cfg.Routing.Balancers) > 0 {
	//	cfg.Routing.Balancers[0].Selector = remove(cfg.Routing.Balancers[0].Selector, node)
	//}
	//}

	// Добавляем новые домены в rules

	// Конвертируем обратно в JSON
	output, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		results = append(results, fmt.Sprintf("ошибка сериализации JSON: %s", err))
	}

	// Сохраняем результат в routing-settings.generated.json
	if err := os.WriteFile(filename, output, 0644); err != nil {
		results = append(results, fmt.Sprintf("не удалось записать %s", filename))
	}

	results = append(results, fmt.Sprintf("✅ Сгенерирован новый %s", filename))

	return results
}

func ModifyDomainsJson(template string, newDomains []string) []string {

	var results []string

	// Читаем template.json
	data, err := os.ReadFile(template)
	if err != nil {
		results = append(results, fmt.Sprintf("не удалось открыть %s", template))
		return results
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		results = append(results, fmt.Sprintf("ошибка парсинга JSON: %s", err))
		return results
	}

	// Добавляем новые домены
	added, err := addDomainsToRules(&cfg, newDomains)

	if err != nil {
		return []string{err.Error()}
	}

	// объединяем два массива
	results = append(results, added...)

	// Конвертируем обратно в JSON
	output, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		results = append(results, fmt.Sprintf("ошибка сериализации JSON: %s", err))
		return results
	}

	// Перезаписываем исходный template-файл
	if err := os.WriteFile(template, output, 0644); err != nil {
		results = append(results, fmt.Sprintf("не удалось записать %s", template))
		return results
	}

	results = append(results, fmt.Sprintf("♻️ Файл %s успешно обновлён", template))

	return results
}
