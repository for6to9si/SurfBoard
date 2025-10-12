package conf

import (
	"SurfBoard/locale"
	"encoding/json"
	"fmt"
	"os"

	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// Структуры конфигурации сделал автоматом https://mholt.github.io/json-to-go/
type Config struct {
	XwayConf          XwayConf          `json:"xwayConf"`
	SwayConf          SwayConf          `json:"swayConf"`
	BenchmarkSettings BenchmarkSettings `json:"benchmarkSettings"`
	TgBot             TgBot             `json:"tgBot"`
	Github            Github            `json:"github"`
	Installer         Installer         `json:"installer"`
}
type Paths struct {
	XrayExecutable    string `json:"xrayExecutable"`
	XrayRestart       string `json:"xrayRestart"`
	XrayStart         string `json:"xrayStart"`
	XrayStop          string `json:"xrayStop"`
	XrayStatus        string `json:"xrayStatus"`
	XraylockFile      string `json:"xraylockFile"`
	SingboxExecutable string `json:"singboxExecutable"`
	SingboxRestart    string `json:"singboxRestart"`
	SingboxStart      string `json:"singboxStart"`
	SingboxStop       string `json:"singboxStop"`
	SingboxStatus     string `json:"singboxStatus"`
	SingboxlockFile   string `json:"singboxlockFile"`
}
type Env struct {
	XrayLocationAsset       string `json:"XRAY_LOCATION_ASSET"`
	XrayLocationConfdir     string `json:"XRAY_LOCATION_CONFDIR"`
	XrayLocationTemplatedir string `json:"XRAY_LOCATION_TEMPLATEDIR"`
	XrayRayBufferSize       int    `json:"XRAY_RAY_BUFFER_SIZE"`
	SslCertDir              string `json:"SSL_CERT_DIR"`
	SingboxLocationConfdir  string `json:"SINGBOX_LOCATION_CONFDIR"`
}
type Target struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}
type Grpc struct {
	Target Target `json:"target"`
}
type XwayConf struct {
	IsEnabled bool  `json:"isEnabled"`
	Paths     Paths `json:"paths"`
	Env       Env   `json:"env"`
	Grpc      Grpc  `json:"grpc"`
}

type SwayConf struct {
	IsEnabled bool  `json:"isEnabled"`
	Paths     Paths `json:"paths"`
	Env       Env   `json:"env"`
}
type ProxySOCKS5 struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}
type StatusCodes struct {
	GeoBlocked      int   `json:"geoBlocked"`
	RetryableErrors []int `json:"retryableErrors"`
	Allowed         []int `json:"allowed"`
}
type OpenAI struct {
	IsEnabled   bool        `json:"isEnabled"`
	Apikey      string      `json:"apikey"`
	URL         string      `json:"url"`
	StatusCodes StatusCodes `json:"statusCodes"`
}
type BenchmarkSettings struct {
	IsEnabled   bool        `json:"isEnabled"`
	Autostart   bool        `json:"autostart"`
	Paths       Paths       `json:"paths"`
	Env         Env         `json:"env"`
	Grpc        Grpc        `json:"grpc"`
	ProxySOCKS5 ProxySOCKS5 `json:"proxySOCKS5"`
	OpenAI      OpenAI      `json:"openAI"`
}
type TgBot struct {
	Token    string  `json:"TOKEN"`
	AdminIds []int64 `json:"adminIds"`
}

type Github struct {
	Token string `json:"TOKEN"`
}
type Programm struct {
	ExecutablePath string   `json:"executablePath"`
	Args           []string `json:"args"`
	UpdateURL      string   `json:"update_url"`
	IsEnabled      bool     `json:"isEnabled"`
}

type Installer struct {
	IsEnabled bool                `json:"isEnabled"`
	Programs  map[string]Programm `json:"programs"`
}

var surConfig Config

// LoadConfig загружает конфигурацию из JSON-файла
func InitConfig(path string) error {
	file, err := os.Open(path)
	if err != nil {
		msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
			MessageID: "error_opening_file",
			TemplateData: map[string]string{
				"Path": path,
			},
		})
		return fmt.Errorf("%s: %w", msg, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
				MessageID: "error_closing_file",
				TemplateData: map[string]string{
					"Path": path,
				},
			})
			fmt.Printf("%s: %v\n", msg, err)
		}
	}()

	if err := json.NewDecoder(file).Decode(&surConfig); err != nil {
		msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
			MessageID: "error_decoding_json",
			TemplateData: map[string]string{
				"Path": path,
			},
		})
		return fmt.Errorf("%s: %w", msg, err)
	}

	return nil
}

// GetConfig возвращает копию структуры конфигурации
func GetConfig() Config {
	return surConfig // возвращаем копию (безопасно!)
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
