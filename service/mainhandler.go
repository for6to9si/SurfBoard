package service

import (
	"SurfBoard/benchmarkMode"
	"SurfBoard/conf"
	"SurfBoard/grpcClient"
	"SurfBoard/installer"
	"SurfBoard/locale"
	"fmt"
	"os"
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
		filesVPN, _ := loc.LocalizeMessage(&i18n.Message{ID: "change_files_vpn"})
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
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(filesVPN).WithCallbackData("xray_getfile")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("Быстрый перезапуск XRAY").WithCallbackData("xray_fast_restart")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("Сгенерировать routing-settings.json").WithCallbackData("xray_build_routing")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("Добавить домен").WithCallbackData("xray_add_domains")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("Выполнить S98xray backup").WithCallbackData("xray_run_x98xray_backup")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(addVPN).WithCallbackData("xray_add_vpn")),
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("back_to_main")),
			)))

		case "xray_add_domains":
			user.State = StateXrayAddDomainToFile
			msg := tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Введите домены в одном из форматов:\n\n"+
					"Через запятую:\n"+
					"ti.com, analog.com, qualcomm.com\n\n"+
					"Или столбиком:\n"+
					"altera.com\n"+
					"intel.com\n"+
					"nvidia.com\n\n"+
					"Также можно использовать готовые доменные группы (geosite) из проекта v2fly.\n"+
					"Список доступных наборов:\n"+
					"https://github.com/v2fly/domain-list-community/tree/master/data\n"+
					"Некоторые примеры готовых доменных групп:\n"+
					"ext:geosite_v2fly.dat:intel\n"+
					"ext:geosite_v2fly.dat:qualcomm\n"+
					"ext:geosite_v2fly.dat:category-dev\n"+
					"ext:geosite_v2fly.dat:google-gemini\n\n"+
					"Просто скопируйте нужные группы и используйте их вместо отдельных доменов.")
			msg.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
			bot.SendMessage(ctx, msg)

		case "xray_run_x98xray_backup":

			// Сразу убираем часики
			_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))

			var logBuilder strings.Builder
			// Разбиваем строку на имя команды и аргументы
			parts := strings.Fields(config.XwayConf.Paths.XrayBackup)
			if len(parts) == 0 {
				logBuilder.WriteString(fmt.Sprintf("Ошибка создания Backup (команда %s)", parts))
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), logBuilder.String()))
				break
			}
			name := parts[0]
			args := parts[1:]

			if err := RunAndLog(&logBuilder, false, name, args...); err != nil {
				// При ошибке — показываем пользователю накопленный лог и выходим
				logBuilder.WriteString(fmt.Sprintf("Ошибка создания Backup (команда %s)", parts))
			}
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), logBuilder.String()))

		case "xray_build_routing":

			// Сразу убираем часики
			_ = bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(query.ID))

			tags, err := benchmarkMode.GetVpns(config.XwayConf.Env.XrayLocationConfdir)
			if err != nil {
				str_tmp := fmt.Sprintf("Ошибка: %v", err)
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), str_tmp))
				break
			}
			for _, line := range tags {
				// Пропускаем пустые строки, если они есть
				if strings.TrimSpace(line) == "" {
					continue
				}
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), line))
			}

			fulltempdir := filepath.Join(config.XwayConf.Env.XrayLocationTemplatedir, FileTmpRoutingBalancers)

			// Проверяем, существует ли файл
			if _, err := os.Stat(fulltempdir); os.IsNotExist(err) {
				// Файл не найден — отправляем сообщение в Telegram
				strTmp := fmt.Sprintf("❗ Файл %s не найден в шаблонах!", FileTmpRoutingBalancers)
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), strTmp))
				break
			}
			fullpath := filepath.Join(config.XwayConf.Env.XrayLocationConfdir, "routing-settings.generated.json")

			results := benchmarkMode.ModifyBalancerJson(fulltempdir, fullpath, tags)

			for _, line := range results {
				// Пропускаем пустые строки, если они есть
				if strings.TrimSpace(line) == "" {
					continue
				}
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), line))
			}

			str_tmp := fmt.Sprintf("Файл %s был сформирован, перегрузите XRAY", fullpath)

			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), str_tmp))

		case "xray_fast_restart":
			var logBuilder strings.Builder
			// Разбиваем строку на имя команды и аргументы
			parts := strings.Fields(config.XwayConf.Paths.XrayRestart)
			if len(parts) == 0 {
				logBuilder.WriteString(fmt.Sprintf("\n⚠️ Ошибка Restart: \n"))
			}
			name := parts[0]
			args := parts[1:]

			if err := RunAndLog(&logBuilder, false, name, args...); err != nil {
				// При ошибке — показываем пользователю накопленный лог и выходим
				logBuilder.WriteString(fmt.Sprintf("Ошибка при перезапуске (команда %s)", parts))
			}
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), logBuilder.String()))

		case "xray_current_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), getCurrentVPN(xraygRpcclient)))
		case "xray_all_vpns":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), listAllVPNs(xraygRpcclient)))
		case "xray_add_vpn":
			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), addNewVPN(xraygRpcclient)))
		case "xray_getfile":
			var text string
			switch user.State {
			case StateBenchmark:
				text = ""

			case StateXray:
				text = "🧩 " + FileXwaveConf
			case StateSingBox:
				text = "🧩 swave-conf.json"

			default:
				panic("unknown state")
			}

			// Создаем массив рядов клавиатуры
			rows := [][]telego.InlineKeyboardButton{
				tu.InlineKeyboardRow(
					tu.InlineKeyboardButton("⚙️ " + FileSystemDefault).WithCallbackData(FileSystemDefault),
				),
				tu.InlineKeyboardRow(
					tu.InlineKeyboardButton("📄 " + FileTmpRoutingBalancers).WithCallbackData(FileTmpRoutingBalancers),
				),
			}

			// Добавляем кнопку только если это не StateBenchmark
			if user.State != StateBenchmark {
				rows = append(rows, tu.InlineKeyboardRow(
					tu.InlineKeyboardButton(text).WithCallbackData(FileXwaveConf),
				))
			}
			rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton("⬅️ Назад").WithCallbackData("xray_vpn")))

			keyboard := tu.InlineKeyboard(rows...)

			_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), "Выбери файл для скачивания:").WithReplyMarkup(keyboard))

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

			vpns, err := benchmarkMode.GetVpns(config.BenchmarkSettings.Env.XrayLocationConfdir)

			if err != nil {
				str_tmp := fmt.Sprintf("Ошибка: %v", err)
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), str_tmp))
				break
			}

			for _, line := range vpns {
				// Пропускаем пустые строки, если они есть
				if strings.TrimSpace(line) == "" {
					continue
				}
				_, _ = bot.SendMessage(ctx, tu.Message(tu.ID(query.Message.GetChat().ID), line))
			}

			// Формируем полный путь к файлу
			fulltempdir := filepath.Join(config.BenchmarkSettings.Env.XrayLocationTemplatedir, FileTmpRoutingBalancers)
			fullpath := filepath.Join(config.BenchmarkSettings.Env.XrayLocationConfdir, "routing-settings.generated.json")

			results := benchmarkMode.ModifyBalancerJson(fulltempdir, fullpath, vpns)

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

		case "benchmark_add_domains":
			user.State = StateBenchmarkAddDomainToFile
			msg := tu.Message(
				tu.ID(query.Message.GetChat().ID),
				"Введите домены в одном из форматов:\n\n"+
					"Через запятую:\n"+
					"ti.com, analog.com, qualcomm.com\n\n"+
					"Или столбиком:\n"+
					"altera.com\n"+
					"intel.com\n"+
					"nvidia.com\n\n"+
					"Также можно использовать готовые доменные группы (geosite) из проекта v2fly.\n"+
					"Список доступных наборов:\n"+
					"https://github.com/v2fly/domain-list-community/tree/master/data\n"+
					"Некоторые примеры готовых доменных групп:\n"+
					"ext:geosite_v2fly.dat:intel\n"+
					"ext:geosite_v2fly.dat:qualcomm\n"+
					"ext:geosite_v2fly.dat:category-dev\n"+
					"ext:geosite_v2fly.dat:google-gemini\n\n"+
					"Просто скопируйте нужные группы и используйте их вместо отдельных доменов.")
			msg.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
			bot.SendMessage(ctx, msg)

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

		//if query.Data == "/getfile" {
		//	// Вызываем обработчик, как будто пользователь ввёл команду вручную
		//	handleVPNState(ctx, telego.Message{Text: "/getfile"}, bot, client, confDir)
		//	return
		//}
		// === Callback от кнопок ===
		handleCallback(ctx, bot, &query, config)

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
		{tu.InlineKeyboardButton("Добавить домен").WithCallbackData("benchmark_add_domains")},
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
