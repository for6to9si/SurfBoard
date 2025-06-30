package main

import (
	"SurfBoard/locale"
	"context"
	"fmt"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	pb "github.com/xtls/xray-core/app/observatory/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

func main() {
	locale.InitI18n() // 📌 Инициализация i18n

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
	// Подключение к Xray gRPC
	conn, err := grpc.Dial("127.0.0.1:10085", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Ошибка подключения к Xray: %v", err)
		return "⚠️ Не удалось подключиться к Xray"
	}
	defer conn.Close()

	client := pb.NewObservatoryServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetOutboundStatus(ctx, &pb.GetOutboundStatusRequest{})
	if err != nil {
		log.Printf("Ошибка запроса: %v", err)
		return "⚠️ Не удалось получить статус VPN"
	}

	statuses := resp.GetStatus().GetStatus()

	// Сортировка: живые вверху, по delay
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Alive != statuses[j].Alive {
			return statuses[i].Alive
		}
		return statuses[i].Delay < statuses[j].Delay
	})

	// Формирование текста
	var sb strings.Builder
	sb.WriteString("📡 Список VPN и их статус:\n\n")

	for _, s := range statuses {
		icon := "✅"
		if !s.Alive {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s %s — %d мс\n", icon, s.OutboundTag, s.Delay))
	}

	return sb.String()
}

func addNewVPN() string {
	return "[Добавление нового VPN]"
}
