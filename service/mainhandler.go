package service

import (
	"SurfBoard/benchmarkMode"
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"SurfBoard/installer"
	"SurfBoard/locale"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// --- /start ---
func registerStartHandler(
	bh *th.BotHandler,
	bot *telego.Bot,
	config *conf.Config,
	isUserAuthorized func(int64) bool,
) {
	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		loc := locale.Getlocalizer(message.From.LanguageCode)

		if !isUserAuthorized(message.From.ID) {
			msg, _ := loc.Localize(&i18n.LocalizeConfig{
				MessageID: "access_denied",
				TemplateData: map[string]interface{}{
					"UserID": message.From.ID,
				},
			})
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(message.Chat.ID), msg))
			return nil
		}

		welcome, _ := loc.Localize(&i18n.LocalizeConfig{
			MessageID: "welcome",
			TemplateData: map[string]string{
				"Name": message.From.FirstName,
			},
		})

		inlineKeyboard := mainMenu(config)
		_, _ = bot.SendMessage(ctx, tu.Message(
			tu.ID(message.Chat.ID), welcome,
		).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))

		return nil
	}, th.CommandEqual("start"))
}

// --- Callback кнопки ---
func registerCallbackHandler(
	bh *th.BotHandler,
	bot *telego.Bot,
	config *conf.Config,
	xraygRpcclient, benchmarkclient *grpcClient.GRpcClient,
	isUserAuthorized func(int64) bool,
) {
	bh.HandleCallbackQuery(func(ctx *th.Context, query telego.CallbackQuery) error {
		loc := locale.Getlocalizer(query.From.LanguageCode)

		if !isUserAuthorized(query.From.ID) {
			msg, _ := loc.Localize(&i18n.LocalizeConfig{MessageID: "access_denied",
				TemplateData: map[string]interface{}{
					"UserID": query.From.ID,
				},
			})
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), msg))
			_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))
			return nil
		}

		currentVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "current_vpn"})
		allVPNs, _ := loc.LocalizeMessage(&i18n.Message{ID: "all_vpns"})
		addVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "add_vpn"})
		//		done, _ := loc.LocalizeMessage(&i18n.Message{ID: "done"})
		underDevelopment, _ := loc.LocalizeMessage(&i18n.Message{ID: "under_development"})

		switch query.Data {

		case "manage_apps":
			// Отображаем скрытые кнопки

			user.State = StateSetupApps

			programs := installer.GetLocalVersion(config.Installer.Programs)

			rows := make([][]telego.InlineKeyboardButton, 0, len(programs)+1)

			for _, program := range programs {
				var btn telego.InlineKeyboardButton
				var text string

				switch {
				case !program.Installed:
					text = fmt.Sprintf("❌ %s  —  не установлена", program.App)

				case program.CompareVersions > 0:
					text = fmt.Sprintf("⬆️ %s  •  v%s → %s", program.App, program.Version, program.Release)

				case program.CompareVersions == 0:
					text = fmt.Sprintf("🟢 %s  •  v%s", program.App, program.Version)

				case program.CompareVersions < 0:
					text = fmt.Sprintf("⚙️ %s  •  v%s (dev)", program.App, program.Version)
				}

				btn = tu.InlineKeyboardButton(text).
					WithCallbackData("program_" + sanitizeCallback(program.App))

				rows = append(rows, tu.InlineKeyboardRow(btn))
			}

			// добавляем кнопку "Назад"
			backBtn := tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")
			rows = append(rows, tu.InlineKeyboardRow(backBtn))

			// отправляем — tu.InlineKeyboard принимает variadic rows
			//_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), "📦 Applications:").WithReplyMarkup(tu.InlineKeyboard(rows...)))
			//_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), "📦 Applications:").WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: rows}))
			_, err := ctx.Bot().EditMessageText(ctx, &telego.EditMessageTextParams{
				ChatID:      tu.ID(query.Message.GetChat().ID),
				MessageID:   query.Message.GetMessageID(),
				Text:        "📦 Applications:",
				ParseMode:   telego.ModeMarkdown,
				ReplyMarkup: tu.InlineKeyboard(rows...),
			})

			if err != nil {
				// если не удалось отредактировать — fallback на SendMessage
				_, _ = ctx.Bot().SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), "📦 Applications:").WithReplyMarkup(tu.InlineKeyboard(rows...)))
			}

			// всегда отвечаем на callback, чтобы убрать "часики"
			_ = ctx.Bot().AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))

		case "xray_vpn":
			// Отображаем скрытые кнопки
			user.State = StateXray
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"VPN options:",
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

		case "back_to_main":
			// Возвращаемся к начальному меню
			user.State = StateDefault
			inlineKeyboard := mainMenu(config)
			_, _ = bot.SendMessage(ctx, tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Вы вернулись в главное меню.",
			).WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: inlineKeyboard}))

		default:
			_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))
		}

		return nil
	}, th.AnyCallbackQueryWithMessage())
}

// --- Меню ---
func mainMenu(config *conf.Config) [][]telego.InlineKeyboardButton {
	var kb [][]telego.InlineKeyboardButton

	if config.Installer.IsEnabled {
		kb = append(kb, []telego.InlineKeyboardButton{
			tu.InlineKeyboardButton("Install/Update").WithCallbackData("manage_apps"),
		})
	}

	if config.XwayConf.IsEnabled {
		kb = append(kb, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("X-Wave").WithCallbackData("xray_vpn"),
		))
	}
	if config.BenchmarkSettings.IsEnabled {
		kb = append(kb, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("Benchmark").WithCallbackData("benchmark_vpn"),
		))
	}

	if config.SwayConf.IsEnabled {
		kb = append(kb, []telego.InlineKeyboardButton{
			tu.InlineKeyboardButton("S-Wave").WithCallbackData("singbox_vpn"),
		})
	}

	return kb
}

// --- Очистка текста для callback ---
func sanitizeCallback(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	re := regexp.MustCompile(`[^a-z0-9\-_]`)
	return re.ReplaceAllString(s, "")
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
