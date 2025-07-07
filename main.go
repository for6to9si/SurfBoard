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

	// Локализация описания флага
	configFlagDesc, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
		MessageID: "config_flag_description",
	})

	// Регистрируем флаг с локализованным описанием
	configPath := flag.String("c", "", configFlagDesc)
	flag.StringVar(configPath, "config", "", configFlagDesc)
	flag.Parse()

	// Проверка, указан ли путь
	if *configPath == "" {
		msg, _ := locale.Loc.Localize(&i18n.LocalizeConfig{
			MessageID: "config_path_required",
		})
		fmt.Println(msg)
		os.Exit(1)
	}

	config, err := conf.LoadConfig(*configPath)
	if err != nil {
		fmt.Println("Ошибка при загрузке конфигурации:", err)
		os.Exit(1)
	}

	fmt.Println("gRPC Target IP:", config.GRPC.Target.IP)
	fmt.Println("gRPC Target Port:", config.GRPC.Target.Port)

	ctx := context.Background()
	botToken := os.Getenv("TOKEN")

	bot, err := telego.NewBot(botToken, telego.WithDefaultDebugLogger())
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
	return "[Информация о текущем VPN]"
}

func listAllVPNs() string {
	return xrayclient.ListVPNStatuses()
}

func addNewVPN() string {
	return "[Добавление нового VPN]"
}
