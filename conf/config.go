package conf

import (
	"SurfBoard/locale"
	"encoding/json"
	"fmt"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"os"
)

// Структуры конфигурации
type Config struct {
	GRPC GRPCConfig `json:"grpc"`
}

type GRPCConfig struct {
	Target TargetConfig `json:"target"`
}

type TargetConfig struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// LoadConfig загружает конфигурацию из JSON-файла
func LoadConfig(path string) (*Config, error) {

	file, err := os.Open(path)
	if err != nil {
		msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
			MessageID: "error_opening_file",
			TemplateData: map[string]string{
				"Path": path,
			},
		})
		return nil, fmt.Errorf("%s: %w", msg, err)
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
			MessageID: "error_decoding_json",
			TemplateData: map[string]string{
				"Path": path,
			},
		})
		return nil, fmt.Errorf("%s: %w", msg, err)
	}

	return &config, nil
}

func getLang() string {
	lang := os.Getenv("LANG") // e.g., "ru_RU.UTF-8"
	if lang[:2] == "ru" {
		return "ru"
	}
	return "en"
}

func GetLang() string {
	loc := locale.Getlocalizer(getLang()) // язык из среды или логики

	arguments := os.Args
	if len(arguments) == 1 {
		msg, _ := loc.Localize(&i18n.LocalizeConfig{
			MessageID: "no_filename",
		})
		return msg
	}

	filename := arguments[1]
	msg, _ := loc.Localize(&i18n.LocalizeConfig{
		MessageID: "file_provided",
		TemplateData: map[string]string{
			"Filename": filename,
		},
	})
	return msg
}
