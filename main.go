package main

import (
	"SurfBoard/benchmarkMode"
	"SurfBoard/conf"
	"SurfBoard/locale"
	"SurfBoard/xrayclient"
	"context"
	"flag"
	"fmt"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"log"
	"os"
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
	client1, err := xrayclient.NewXrayClient(config.XwayConf.Grpc)

	if err != nil {
		log.Fatalf("Ошибка создания первого XrayClient: %v", err)
	}

	defer func() {
		if err := client1.Close(); err != nil {
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
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Second VPN options:",
			).WithReplyMarkup(tu.InlineKeyboard(
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(currentVPN).WithCallbackData("current_vpn")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(allVPNs).WithCallbackData("all_vpns")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(addVPN).WithCallbackData("add_vpn")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")),
			)))

		case "benchmark_vpn":

			inlineKeyboard := [][]telego.InlineKeyboardButton{
				{tu.InlineKeyboardButton(
					"⏹️ Стор").WithCallbackData("benchmark_stop_xray"),
				},
				{tu.InlineKeyboardButton(allVPNs).WithCallbackData("all_vpns")},
				{tu.InlineKeyboardButton(addVPN).WithCallbackData("add_vpn")},
				{tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")},
			}

			if !benchmarkMode.IsXrayRunning() {
				inlineKeyboard[0] = []telego.InlineKeyboardButton{tu.InlineKeyboardButton("▶️ Cтарт").WithCallbackData("benchmark_start_xray")}
			}

			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Benchmark mode selected",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))

		case "singbox_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				underDevelopment,
			).WithReplyMarkup(tu.InlineKeyboard(
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")),
			)))

		case "back_to_main":
			// Возвращаемся к начальному меню
			inlineKeyboard := greetUser(config)
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Вы вернулись в главное меню.",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))

		case "benchmark_start_xray":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), startXray()))
		case "benchmark_stop_xray":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), stopXray()))
		case "current_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), getCurrentVPN(client1)))
		case "all_vpns":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), listAllVPNs(client1)))
		case "add_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), addNewVPN()))
		default:
			unknownCommand, _ := loc.LocalizeMessage(&i18n.Message{ID: "unknown_command"})
			inlineKeyboard := greetUser(config)
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				unknownCommand,
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))
		}

		_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID).WithText(done))
		return nil
	}, th.AnyCallbackQueryWithMessage())

	_ = bh.Start()
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

func startXray() string {
	return "🌍 Текущий VPN: " + benchmarkMode.StartXray()
}

func stopXray() string {
	return "🌍 Текущий VPN: " + benchmarkMode.StopXray()
}

// 🧩 Заглушки под VPN-логику
func getCurrentVPN(client *xrayclient.XrayClient) string {
	return "🌍 Текущий VPN: " + client.GetCurrentVPN()
}

func listAllVPNs(client *xrayclient.XrayClient) string {
	return client.ListVPNStatuses()
}

func addNewVPN() string {
	return "[Добавление нового VPN]"
}
