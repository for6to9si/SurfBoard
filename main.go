package main

import (
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

	// Присваиваем в глобальную переменную
	xrayclient.Init(config.XwayConf.Grpc)

	ctx := context.Background()
	//botToken := os.Getenv("TOKEN")

	bot, err := telego.NewBot(config.TgBot.Token, telego.WithDefaultDebugLogger())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	updates, _ := bot.UpdatesViaLongPolling(ctx, nil)
	bh, _ := th.NewBotHandler(bot, updates)
	defer func() { _ = bh.Stop() }()

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		loc := locale.Getlocalizer(message.From.LanguageCode)

		welcome, _ := loc.Localize(&i18n.LocalizeConfig{
			MessageID: "welcome",
			TemplateData: map[string]string{
				"Name": message.From.FirstName,
			},
		})
		currentVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "current_vpn"})
		allVPNs, _ := loc.LocalizeMessage(&i18n.Message{ID: "all_vpns"})
		addVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "add_vpn"})

		_, _ = bot.SendMessage(ctx, tu.Message(
			tu.ID(message.Chat.ID), welcome,
		).WithReplyMarkup(tu.InlineKeyboard(
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(currentVPN).WithCallbackData("current_vpn")),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(allVPNs).WithCallbackData("all_vpns")),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(addVPN).WithCallbackData("add_vpn")),
		)))
		return nil
	}, th.CommandEqual("start"))

	bh.HandleCallbackQuery(func(ctx *th.Context, query telego.CallbackQuery) error {
		loc := locale.Getlocalizer(query.From.LanguageCode)

		var response string
		switch query.Data {
		case "current_vpn":
			response = getCurrentVPN()
		case "all_vpns":
			response = listAllVPNs()
		case "add_vpn":
			response = addNewVPN()
		default:
			response, _ = loc.LocalizeMessage(&i18n.Message{ID: "go_response"})
		}

		done, _ := loc.LocalizeMessage(&i18n.Message{ID: "done"})

		_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), response))
		_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID).WithText(done))

		return nil
	}, th.AnyCallbackQueryWithMessage())

	_ = bh.Start()
}

// 🧩 Заглушки под VPN-логику
func getCurrentVPN() string {
	return "🌍 Текущий VPN: " + xrayclient.GetCurrentVPN()
}

func listAllVPNs() string {
	return xrayclient.ListVPNStatuses()
}

func addNewVPN() string {
	return "[Добавление нового VPN]"
}
