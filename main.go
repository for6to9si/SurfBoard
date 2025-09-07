package main

import (
	"SurfBoard/benchmarkMode"
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"SurfBoard/locale"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

func getLang() string {
	lang := os.Getenv("LANG") // e.g., "ru_RU.UTF-8"
	if lang[:2] == "ru" {
		return "ru"
	}
	return "en"
}

func main() {
	locale.InitI18n() // 📌 Инициализация i18n

	locale.Loc = locale.Getlocalizer(getLang()) // Установка локализатора

	//export SF_LOCATION_CONFDIR=/opt/etc/xray/configs
	envConfigPath := os.Getenv("SF_LOCATION_CONFDIR")

	// Локализация описания флага
	configFlagDesc, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
		MessageID: "config_flag_description",
	})

	// Регистрируем флаг с локализованным описанием
	flagConfigPath := flag.String("c", "", configFlagDesc)
	flag.StringVar(flagConfigPath, "config", "", configFlagDesc)
	flag.Parse()

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

	config, err := conf.LoadConfig(finalConfigPath)
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
	//botToken := os.Getenv("TOKEN")

	bot, err := telego.NewBot(config.TgBot.Token, telego.WithDefaultDebugLogger())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// 1. Установка описания бота
	err = bot.SetMyDescription(ctx, &telego.SetMyDescriptionParams{
		Description: "Приветствую! Это бот для управления VPN.\n\n" +
			"📌 Основные функции:\n" +
			"- Настройка VPN подключений\n" +
			"- Тестирование скорости\n" +
			"- Управление серверами\n\n" +
			"Нажмите /start для начала работы",
	})
	if err != nil {
		log.Printf("Ошибка установки описания бота: %v", err)
	}

	// 2. Установка краткого описания (отображается в списке чатов)
	err = bot.SetMyShortDescription(ctx, &telego.SetMyShortDescriptionParams{
		ShortDescription: "Бот для управления VPN - настройка, тестирование, управление серверами",
	})
	if err != nil {
		log.Printf("Ошибка установки краткого описания бота: %v", err)
	}

	// 3. Установка команд меню (отображаются при вводе "/")
	err = bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
		Commands: []telego.BotCommand{
			{
				Command:     "start",
				Description: "Начать работу с ботом",
			},
			{
				Command:     "help",
				Description: "Помощь и инструкции",
			},
		},
	})
	if err != nil {
		log.Printf("Ошибка установки команд бота: %v", err)
	}

	// 2. Установка картинки бота (если нужно)
	// Для этого нужно сначала загрузить картинку на сервер Telegram

	// Helper function to check if user is authorized
	isUserAuthorized := func(userID int64) bool {
		for _, allowedID := range config.TgBot.AdminIds {
			if allowedID == userID {
				return true
			}
		}
		return false
	}

	updates, _ := bot.UpdatesViaLongPolling(ctx, nil)
	bh, _ := th.NewBotHandler(bot, updates)
	defer func() { _ = bh.Stop() }()

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {

		loc := locale.Getlocalizer(message.From.LanguageCode)

		// Check if user is authorized
		if !isUserAuthorized(message.From.ID) {

			accessDenied, _ := loc.Localize(&i18n.LocalizeConfig{
				MessageID: "access_denied",
				TemplateData: map[string]interface{}{
					"UserID": message.From.ID,
				},
			})

			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(message.Chat.ID),
				accessDenied,
			))
			return nil
		}

		welcome, _ := loc.Localize(&i18n.LocalizeConfig{
			MessageID: "welcome",
			TemplateData: map[string]string{
				"Name": message.From.FirstName,
			},
		})

		inlineKeyboard := greetUser(config)

		_, _ = bot.SendMessage(ctx, tu.Message(
			tu.ID(message.Chat.ID), welcome,
		).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))

		return nil
	}, th.CommandEqual("start"))

	type State uint

	const (
		StateDefault State = iota
		StateBenchmark
		StateXray
		StateSingBox
	)

	type User struct {
		State State
	}

	users := make(map[int64]User)
	// Since this is in-memory storage, we must use mutex
	lock := sync.RWMutex{}

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {

		loc := locale.Getlocalizer(message.From.LanguageCode)

		// Check if user is authorized
		if !isUserAuthorized(message.From.ID) {

			accessDenied, _ := loc.Localize(&i18n.LocalizeConfig{
				MessageID: "access_denied",
				TemplateData: map[string]interface{}{
					"UserID": message.From.ID,
				},
			})

			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(message.Chat.ID),
				accessDenied,
			))
			return nil
		}

		userID := message.From.ID

		lock.RLock()
		user := users[userID]
		lock.RUnlock()

		var text string
		// Фильтруем пустые строки
		var filteredLines []string

		var tags []string

		switch user.State {
		case StateDefault:
			text = "не выбрано"
		case StateBenchmark:
			text = "Thanks for your data!"
			lines := strings.Split(message.Text, "\n")

			// Проверяем, не команда ли это set X
			trimmedText := strings.TrimSpace(message.Text)
			if strings.HasPrefix(trimmedText, "set ") {
				parts := strings.SplitN(trimmedText, " ", 2)
				if len(parts) == 2 {
					indexStr := strings.TrimSpace(parts[1])
					x, err := strconv.Atoi(indexStr)
					if err != nil {
						_, _ = bot.SendMessage(ctx, tu.Message(
							message.Chat.ChatID(),
							fmt.Sprintf("Ошибка: `%s` не является числом", indexStr),
						))
						break
					}

					// Получаем все теги
					_, allTags := benchmarkclient.ListVPNStatuses()
					//allTags := benchmarkMode.GetTags(config.BenchmarkSettings.Env.XrayLocationConfdir)

					if x < 0 || x >= len(allTags) {
						_, _ = bot.SendMessage(ctx, tu.Message(
							message.Chat.ChatID(),
							fmt.Sprintf("Ошибка: индекс %d вне диапазона (0..%d)", x, len(allTags)-1),
						))
						break
					}

					// Запускаем OverrideBalancerTarget
					grpcClient.OverrideBalancerTarget(benchmarkclient, "bestVPN", allTags[x])

					_, _ = bot.SendMessage(ctx, tu.Message(
						message.Chat.ChatID(),
						fmt.Sprintf("Balancer переопределён на: %s", allTags[x]),
					))
					break
				}
			}

			// Если это не команда, то обрабатываем строки как раньше
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					filteredLines = append(filteredLines, trimmed)
				}
			}

			tags = benchmarkMode.Parses(filteredLines, config.BenchmarkSettings.Env.XrayLocationConfdir)

			for _, line := range tags {
				// Пропускаем пустые строки, если они есть
				if strings.TrimSpace(line) == "" {
					continue
				}

				_, _ = bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), line))
			}
		case StateXray:
			text = "StateXray!"
		case StateSingBox:
			text = "StateSingBox!"
		default:
			panic("unknown state")
		}

		lock.Lock()
		users[userID] = user
		lock.Unlock()

		_, _ = bot.SendMessage(ctx, tu.Message(message.Chat.ChatID(), text))
		return nil
	})

	bh.HandleCallbackQuery(func(ctx *th.Context, query telego.CallbackQuery) error {

		loc := locale.Getlocalizer(query.From.LanguageCode)

		// Check if user is authorized
		if !isUserAuthorized(query.From.ID) {

			accessDenied, _ := loc.Localize(&i18n.LocalizeConfig{
				MessageID: "access_denied",
				TemplateData: map[string]interface{}{
					"UserID": query.From.ID,
				},
			})
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				accessDenied,
			))
			_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))
			return nil
		}

		userID := query.From.ID

		lock.RLock()
		user := users[userID]
		lock.RUnlock()

		currentVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "current_vpn"})
		allVPNs, _ := loc.LocalizeMessage(&i18n.Message{ID: "all_vpns"})
		addVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "add_vpn"})
		done, _ := loc.LocalizeMessage(&i18n.Message{ID: "done"})
		underDevelopment, _ := loc.LocalizeMessage(&i18n.Message{ID: "under_development"})

		// Объявляем пустую клавиатуру
		//var inlineKeyboard [][]telego.InlineKeyboardButton

		switch query.Data {
		case "xray_vpn":
			// Отображаем скрытые кнопки
			user.State = StateXray
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Second VPN options:",
			).WithReplyMarkup(tu.InlineKeyboard(
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(currentVPN).WithCallbackData("xray_current_vpn")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(allVPNs).WithCallbackData("xray_all_vpns")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(addVPN).WithCallbackData("xray_add_vpn")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")),
			)))

		case "xray_current_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), getCurrentVPN(xraygRpcclient)))
		case "xray_all_vpns":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), listAllVPNs(xraygRpcclient)))
		case "xray_add_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), addNewVPN(xraygRpcclient)))

		case "benchmark_vpn":

			user.State = StateBenchmark
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Benchmark mode selected",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{
				InlineKeyboard: createBenchmarkKeyboard(loc, benchmarkMode.IsXrayRunning()),
			}))

		case "benchmark_vpn_on": //DO-TO Delete

			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), benchmarkMode.StartXray()))

			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Benchmark mode selected",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{
				InlineKeyboard: createBenchmarkKeyboard(loc, benchmarkMode.IsXrayRunning()),
			}))

		case "benchmark_vpn_off":
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				benchmarkMode.StopXray(),
			))
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Benchmark mode selected",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{
				InlineKeyboard: createBenchmarkKeyboard(loc, benchmarkMode.IsXrayRunning()),
			}))

		case "singbox_vpn":
			user.State = StateSingBox
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				underDevelopment,
			).WithReplyMarkup(tu.InlineKeyboard(
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")),
			)))

		case "back_to_main":
			// Возвращаемся к начальному меню
			user.State = StateDefault
			inlineKeyboard := greetUser(config)
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Вы вернулись в главное меню.",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))

		case "benchmark_start_xray":

			tags := benchmarkMode.GetTags(config.BenchmarkSettings.Env.XrayLocationConfdir)
			for _, line := range tags {
				// Пропускаем пустые строки, если они есть
				if strings.TrimSpace(line) == "" {
					continue
				}
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), line))
			}

			// Формируем полный путь к файлу
			fulltempdir := filepath.Join(config.BenchmarkSettings.Env.XrayLocationTemplatedir, "routing-settings.generated.json")
			fullpath := filepath.Join(config.BenchmarkSettings.Env.XrayLocationConfdir, "routing-settings.generated.json")

			results := benchmarkMode.ModifyBalancerJson(fulltempdir, fullpath, tags)

			for _, line := range results {
				// Пропускаем пустые строки, если они есть
				if strings.TrimSpace(line) == "" {
					continue
				}
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), line))
			}
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), benchmarkMode.StartXray()))

			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Benchmark mode selected",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{
				InlineKeyboard: createBenchmarkKeyboard(loc, benchmarkMode.IsXrayRunning()),
			}))

		case "benchmark_stop_xray":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), benchmarkMode.StopXray()))

			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Benchmark mode selected",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{
				InlineKeyboard: createBenchmarkKeyboard(loc, benchmarkMode.IsXrayRunning()),
			}))
		case "fast_vpn_test":
			if err := handleFastVPNTest(ctx, query, bot, allVPNs, addVPN); err != nil {
				return err
			}
		case "benchmark_current_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), getCurrentVPN(benchmarkclient)))
		case "benchmark_all_vpns":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), listAllVPNs(benchmarkclient)))
		case "benchmark_add_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), addNewVPN(benchmarkclient)))
		default:
			unknownCommand, _ := loc.LocalizeMessage(&i18n.Message{ID: "unknown_command"})
			inlineKeyboard := greetUser(config)
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				unknownCommand,
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))
		}

		lock.Lock()
		users[userID] = user
		lock.Unlock()

		_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID).WithText(done))
		return nil
	}, th.AnyCallbackQueryWithMessage())

	_ = bh.Start()
}

// Определяем функцию для создания клавиатуры для benchmark-режима
func createBenchmarkKeyboard(loc *i18n.Localizer, isXrayRunning bool) [][]telego.InlineKeyboardButton {

	allVPNs, _ := loc.LocalizeMessage(&i18n.Message{ID: "all_vpns"})
	addVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "add_vpn"})
	currentVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "current_vpn"})

	buttonText := "⏹️ Стоп"
	buttonData := "benchmark_vpn_off"
	if !isXrayRunning {
		buttonText = "▶️ Старт"
		buttonData = "benchmark_start_xray"
	}

	return [][]telego.InlineKeyboardButton{
		{tu.InlineKeyboardButton(buttonText).WithCallbackData(buttonData)},
		{tu.InlineKeyboardButton(allVPNs).WithCallbackData("benchmark_all_vpns")},
		{tu.InlineKeyboardButton(currentVPN).WithCallbackData("benchmark_current_vpn")},
		{tu.InlineKeyboardButton(addVPN).WithCallbackData("benchmark_add_vpn")},
		{tu.InlineKeyboardButton("fastVpnTest").WithCallbackData("fast_vpn_test")},
		{tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")},
	}
}

// Определяем функцию для обработки логики fast_vpn_test
func handleFastVPNTest(ctx *th.Context, query telego.CallbackQuery, bot *telego.Bot, allVPNs, addVPN string) error {
	inlineKeyboard := [][]telego.InlineKeyboardButton{
		{tu.InlineKeyboardButton("▶️ Cтарт").WithCallbackData("benchmark_start_xray")},
	}

	if benchmarkMode.IsXrayRunning() {
		inlineKeyboard[0] = []telego.InlineKeyboardButton{tu.InlineKeyboardButton("⏹️ Стоп").WithCallbackData("benchmark_stop_xray")}
		inlineKeyboard = append(inlineKeyboard,
			[]telego.InlineKeyboardButton{tu.InlineKeyboardButton(allVPNs).WithCallbackData("benchmark_all_vpns")},
			[]telego.InlineKeyboardButton{tu.InlineKeyboardButton(addVPN).WithCallbackData("benchmark_add_vpn")},
		)
	}

	inlineKeyboard = append(inlineKeyboard,
		[]telego.InlineKeyboardButton{tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("benchmark_vpn")},
	)

	_, err := bot.SendMessage(ctx, tu.Message(
		tu.ID(query.Message.GetChat().ID),
		"🌐 VPN Test Меню",
	).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))
	return err
}

func greetUser(config *conf.Config) [][]telego.InlineKeyboardButton {
	var inlineKeyboard [][]telego.InlineKeyboardButton

	if config.XwayConf.IsEnabled {
		inlineKeyboard = append(inlineKeyboard, []telego.InlineKeyboardButton{
			tu.InlineKeyboardButton("X-Wave").WithCallbackData("xray_vpn"),
		})
	}
	if config.BenchmarkSettings.IsEnabled {
		inlineKeyboard = append(inlineKeyboard, []telego.InlineKeyboardButton{
			tu.InlineKeyboardButton("Benchmark").WithCallbackData("benchmark_vpn"),
		})
	}
	if config.SwayConf.IsEnabled {
		inlineKeyboard = append(inlineKeyboard, []telego.InlineKeyboardButton{
			tu.InlineKeyboardButton("S-Wave").WithCallbackData("singbox_vpn"),
		})
	}

	return inlineKeyboard
}

// 🧩 Заглушки под VPN-логику
func getCurrentVPN(client *grpcClient.GRpcClient) string {
	return "🌍 Текущий VPN: " + client.GetCurrentVPN()
}

func listAllVPNs(client *grpcClient.GRpcClient) string {
	str, _ := client.ListVPNStatuses()
	return str
}

func addNewVPN(client *grpcClient.GRpcClient) string {
	str, _ := client.ListVPNStatuses()
	return str
}
