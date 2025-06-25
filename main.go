package main

import (
	"SurfBoard/locale"
	"context"
	"fmt"
	"os"

	"github.com/mymmrac/telego"

	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

func main() {
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
		lang := locale.NormalizeLang(message.From.LanguageCode)

		_, _ = bot.SendMessage(ctx, tu.Messagef(
			tu.ID(message.Chat.ID),
			locale.T(lang, "welcome"), message.From.FirstName,
		).WithReplyMarkup(tu.InlineKeyboard(
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.T(lang, "current_vpn")).WithCallbackData("current_vpn")),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.T(lang, "all_vpns")).WithCallbackData("all_vpns")),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.T(lang, "add_vpn")).WithCallbackData("add_vpn")),
		)))
		return nil
	}, th.CommandEqual("start"))

	bh.HandleCallbackQuery(func(ctx *th.Context, query telego.CallbackQuery) error {
		lang := locale.NormalizeLang(query.From.LanguageCode)

		var response string
		switch query.Data {
		case "current_vpn":
			response = getCurrentVPN() // Замените на вашу логику
		case "all_vpns":
			response = listAllVPNs() // Замените на вашу логику
		case "add_vpn":
			response = addNewVPN() // Замените на вашу логику
		default:
			response = locale.T(lang, "go_response")
		}

		_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), response))
		_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID).WithText(locale.T(lang, "done")))

		return nil
	}, th.AnyCallbackQueryWithMessage())

	_ = bh.Start()
}

// 🔻 Заглушки (замените на вашу VPN-логику)
func getCurrentVPN() string {
	return "[Информация о текущем VPN]"
}

func listAllVPNs() string {
	return "[Список всех VPN]"
}

func addNewVPN() string {
	return "[Добавление нового VPN]"
}
