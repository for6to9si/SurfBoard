package main

import (
	"SurfBoard/benchmarkMode"
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"SurfBoard/installer"
	"SurfBoard/locale"
	"SurfBoard/service"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/nicksnyder/go-i18n/v2/i18n"
)

func getLang() string {
	lang := os.Getenv("LANG") // e.g., "ru_RU.UTF-8"
	if lang[:2] == "ru" {
		return "ru"
	}
	return "en"
}

// Version specifies the current version of the application.
var Version = "1.3.0"

func main() {
	locale.InitI18n() // 📌 Инициализация i18n

	locale.Loc = locale.Getlocalizer(getLang()) // Установка локализатора

	// Локализация описания флага --config
	configFlagDesc, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
		MessageID: "config_flag_description",
	})

	// Локализация описания флага --version
	versionFlagDesc, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
		MessageID: "version_flag_description",
	})

	// Регистрируем флаг с локализованным описанием
	flagConfigPath := flag.String("c", "", configFlagDesc)
	flag.StringVar(flagConfigPath, "config", "", configFlagDesc)
	versionFlag := flag.Bool("version", false, versionFlagDesc)
	flag.Parse()

	// Обработка флага --version
	if *versionFlag {
		fmt.Printf("Version: %s\n", Version)
		os.Exit(0)
	}

	//export SF_LOCATION_CONFDIR=/opt/etc/surfboard/conf.json
	envConfigPath := os.Getenv("SF_LOCATION_CONFDIR")

	// Определяем финальный путь к конфигу
	finalConfigPath := ""
	if *flagConfigPath != "" {
		finalConfigPath = *flagConfigPath
	} else if envConfigPath != "" {
		finalConfigPath = envConfigPath
	} else {
		msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
			MessageID: "config_path_required",
		})
		fmt.Println(msg)
		os.Exit(1)
	}

	err := conf.InitConfig(finalConfigPath)

	if err != nil {
		msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
			MessageID: "config_load_failed",
			TemplateData: map[string]string{
				"Error": err.Error(),
			},
		})

		fmt.Println(msg)
		os.Exit(1)
	}

	config := conf.GetConfig()

	// Пример ручной очистки кэша
	err = installer.ClearCache()
	if err != nil {
		fmt.Println("Ошибка очистки:", err)
	}

	// Конфигурация для первого xray-сервера
	xraygRpcclient, err := grpcClient.NewGRpcClient(config.XwayConf.Grpc)

	if err != nil {
		log.Fatalf("Ошибка создания первого XrayClient: %v", err)
	}

	defer func() {
		if err := xraygRpcclient.Close(); err != nil {
			log.Printf("Ошибка закрытия первого XrayClient: %v", err)
		}
	}()

	// Конфигурация для первого xray-сервера
	benchmarkclient, err := grpcClient.NewGRpcClient(config.BenchmarkSettings.Grpc)

	if err != nil {
		log.Fatalf("Ошибка создания первого XrayClient: %v", err)
	}

	defer func() {
		if err := benchmarkclient.Close(); err != nil {
			log.Printf("Ошибка закрытия первого XrayClient: %v", err)
		}
	}()

	benchmarkMode.Init(config.BenchmarkSettings)

	ctx := context.Background()
	service.RunTgBot(ctx, &config, xraygRpcclient, benchmarkclient)
}
