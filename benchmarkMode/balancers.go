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

// Функция добавления доменов в правило с xwave = true
func addDomainsToRules(cfg *Config, newDomains []string) {

	// Если массив пустой — ничего не делаем
	if len(newDomains) == 0 {
		return
	}

	for i, rule := range cfg.Routing.Rules {

		// Мы ищем только правила где присутствует "xwave": "true"
		flag, ok := rule["xwave"]
		if !ok || flag != "true" {
			continue
		}

		// Проверяем наличие поля domain
		if domains, ok := rule["domain"].([]interface{}); ok {
			exist := map[string]bool{}
			for _, d := range domains {
				exist[d.(string)] = true
			}

			// Добавляем новые домены без дубликатов
			for _, nd := range newDomains {
				if !exist[nd] {
					domains = append(domains, nd)
				}
			}

			cfg.Routing.Rules[i]["domain"] = domains

		} else {
			// Поля domain нет → создаём новый массив
			domains := []interface{}{}
			for _, nd := range newDomains {
				domains = append(domains, nd)
			}
			cfg.Routing.Rules[i]["domain"] = domains
		}
	}
}

func ModifyBalancerJson(template string, filename string, vpns []string, newDomains []string) []string {

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

	addDomainsToRules(&cfg, newDomains)

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
