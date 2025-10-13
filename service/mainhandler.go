package service

import (
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"SurfBoard/installer"
	"SurfBoard/locale"
	"context"
	"fmt"
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
	xrayClient, benchmarkClient *grpcClient.GRpcClient,
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
					text = fmt.Sprintf("📦 %s — не установлена ❌", program.App)

				case program.CompareVersions > 0:
					text = fmt.Sprintf("🚀 %s v%s → %s 🔼", program.App, program.Version, program.Release)

				case program.CompareVersions == 0:
					text = fmt.Sprintf("✅ %s v%s", program.App, program.Version)

				case program.CompareVersions < 0:
					text = fmt.Sprintf("🧪 %s v%s (тест)", program.App, program.Version)
				}

				btn = tu.InlineKeyboardButton(text).
					WithCallbackData("program_" + sanitizeCallback(program.App))

				rows = append(rows, tu.InlineKeyboardRow(btn))
			}

			// добавляем кнопку "Назад"
			backBtn := tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")
			rows = append(rows, tu.InlineKeyboardRow(backBtn))

			// отправляем — tu.InlineKeyboard принимает variadic rows
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), "📦 Applications:").WithReplyMarkup(tu.InlineKeyboard(rows...)))

		case "benchmark_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), "Запуск benchmark режима..."))
			go func() {
				err := runBenchmark(ctx, bot, query.Message.GetChat().ID, benchmarkClient)
				if err != nil {
					_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), fmt.Sprintf("Ошибка: %v", err)))
				}
			}()

		case "xray_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), "Переключение в Xray режим..."))

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

// --- Пример запуска benchmark ---
func runBenchmark(ctx context.Context, bot *telego.Bot, chatID int64, benchmarkClient *grpcClient.GRpcClient) error {
	// Здесь логика benchmarkMode.Run или аналогичная
	_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(chatID), "Benchmark запущен..."))
	// benchmarkMode.Run(...)
	return nil
}

// --- Очистка текста для callback ---
func sanitizeCallback(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	re := regexp.MustCompile(`[^a-z0-9\-_]`)
	return re.ReplaceAllString(s, "")
}
